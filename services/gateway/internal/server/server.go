// Package server assembles the gateway: middleware chain, auth and routing.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/furkanpatat/movieapp/pkg/jwtauth"
	"github.com/furkanpatat/movieapp/services/gateway/internal/ratelimit"
)

// UserIDHeader is how the authenticated identity reaches upstream services.
const UserIDHeader = "X-User-Id"

const maxBody = 1 << 20

type Deps struct {
	Catalog, Interaction *url.URL
	AuthService          *url.URL // Auth service: register / login
	WatchParty           *url.URL // Watch-Party service: WebSocket rooms
	Auth                 *jwtauth.Manager
	Limiter              *ratelimit.Limiter
	RateLimit            ratelimit.MiddlewareConfig
	// AuthRateLimit is a stricter limiter for credential endpoints (brute force).
	AuthLimiter     *ratelimit.Limiter
	AuthRateLimit   ratelimit.MiddlewareConfig
	UpstreamTimeout time.Duration
	Ready           func(context.Context) error // dependency check for /readyz
	Log             *slog.Logger
}

type gateway struct {
	Deps
	transport http.RoundTripper
}

// New builds the gateway handler.
//
//	/healthz, /readyz                       not rate limited, no auth
//	POST /api/v1/auth/register|login        -> Auth service   (public, strict rate limit)
//	GET  /api/v1/movies/...                 -> Catalog        (public)
//	GET  /api/v1/movies/{id}/interactions  -> Interaction    (public)
//	POST /api/v1/movies/{id}/rate|comment  -> Interaction    (JWT required)
//	GET  /api/v1/watch-party/...           -> Watch-Party    (JWT required; WebSocket upgrade,
//	                                          token in Authorization header or ?token=)
//	     ...also under /api/v1/interaction/movies/{id}/...
func New(d Deps) http.Handler {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.UpstreamTimeout <= 0 {
		d.UpstreamTimeout = 10 * time.Second
	}
	g := &gateway{Deps: d, transport: &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ResponseHeaderTimeout: d.UpstreamTimeout,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
	}}

	catalog := g.proxy(d.Catalog)
	interaction := g.proxy(d.Interaction)
	requireAuth := g.requireAuth

	api := http.NewServeMux()
	// Credential endpoints: public, proxied to the Auth service, and throttled
	// much harder than the rest of the API.
	watchParty := g.proxy(d.WatchParty)
	authSvc := g.proxy(d.AuthService)
	if d.AuthLimiter != nil {
		authSvc = ratelimit.Middleware(d.AuthLimiter, d.AuthRateLimit)(authSvc)
	}
	api.Handle("POST /api/v1/auth/", authSvc)
	api.Handle("GET /api/v1/movies/", catalog)
	api.Handle("GET /api/v1/movies/{id}/interactions", interaction)
	api.Handle("POST /api/v1/movies/{id}/rate", requireAuth(interaction))
	api.Handle("POST /api/v1/movies/{id}/comment", requireAuth(interaction))
	// WebSockets: browsers cannot set an Authorization header on the handshake,
	// so this route (and only this route) also accepts ?token=<jwt> on an upgrade request.
	api.Handle("GET /api/v1/watch-party/", g.requireAuthWS(watchParty))
	// /api/v1/interaction/* is an alias for the interaction endpoints above.
	api.Handle("GET /api/v1/interaction/movies/{id}/interactions", aliasOf(interaction))
	api.Handle("POST /api/v1/interaction/movies/{id}/rate", requireAuth(aliasOf(interaction)))
	api.Handle("POST /api/v1/interaction/movies/{id}/comment", requireAuth(aliasOf(interaction)))

	limited := ratelimit.Middleware(d.Limiter, d.RateLimit)(api)

	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	root.HandleFunc("GET /readyz", g.readyz)
	root.Handle("/", limited)

	return requestID(g.accessLog(recoverPanics(d.Log)(bodyLimit(root))))
}

// --- proxying ---

func (g *gateway) proxy(target *url.URL) http.Handler {
	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			// Rewrite starts from a copy of the client's headers, so scrub what
			// upstreams must never take from a client:
			r.Out.Header.Del(UserIDHeader)    // no identity spoofing
			r.Out.Header.Del("Authorization") // upstreams don't need (or see) the JWT
			r.SetXForwarded()                 // replaces any client-supplied X-Forwarded-*
			if id, ok := r.In.Context().Value(userKey{}).(string); ok {
				r.Out.Header.Set(UserIDHeader, id)
			}
			if rid, ok := r.In.Context().Value(requestIDKey{}).(string); ok {
				r.Out.Header.Set("X-Request-Id", rid)
			}
		},
		Transport: g.transport,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			status, msg := http.StatusBadGateway, "upstream unavailable"
			var ne net.Error
			if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
				status, msg = http.StatusGatewayTimeout, "upstream timed out"
			}
			if r.Context().Err() == nil { // client still there; log real upstream trouble
				g.Log.Error("upstream error", "path", r.URL.Path, "error", err)
			}
			writeJSON(w, status, map[string]string{"error": msg})
		},
	}
}

// aliasOf maps /api/v1/interaction/movies/... to /api/v1/movies/... for the upstream.
func aliasOf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r2 := r.Clone(r.Context())
		r2.URL.Path = strings.Replace(r.URL.Path, "/api/v1/interaction/", "/api/v1/", 1)
		r2.URL.RawPath = ""
		next.ServeHTTP(w, r2)
	})
}

// --- authentication ---

type userKey struct{}

func (g *gateway) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			unauthorized(w, "missing or malformed bearer token")
			return
		}
		userID, err := g.Auth.Verify(token)
		if err != nil {
			unauthorized(w, "invalid or expired token")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, userID)))
	})
}

// requireAuthWS is requireAuth for WebSocket handshakes. Browser WebSocket
// clients cannot send custom headers, so when (and only when) the request is a
// WebSocket upgrade and carries no Authorization header, the JWT may come from
// the `token` query parameter. Either way the credential is verified here and
// never forwarded: the upstream only ever sees the injected X-User-Id.
func (g *gateway) requireAuthWS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok && isWebSocketUpgrade(r) {
			if t := strings.TrimSpace(r.URL.Query().Get("token")); t != "" {
				token, ok = t, true
			}
		}
		if !ok {
			unauthorized(w, "missing or malformed bearer token")
			return
		}
		userID, err := g.Auth.Verify(token)
		if err != nil {
			unauthorized(w, "invalid or expired token")
			return
		}

		r = r.WithContext(context.WithValue(r.Context(), userKey{}, userID))
		if r.URL.Query().Has("token") { // keep the JWT out of the upstream request
			u := *r.URL // copy: never mutate the shared URL
			q := u.Query()
			q.Del("token")
			u.RawQuery = q.Encode()
			r.URL = &u
		}
		next.ServeHTTP(w, r)
	})
}

// bearerToken extracts a token from "Authorization: Bearer <token>".
func bearerToken(r *http.Request) (string, bool) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	token = strings.TrimSpace(token)
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return "", false
	}
	return token, true
}

func isWebSocketUpgrade(r *http.Request) bool {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return false
	}
	for _, v := range r.Header.Values("Connection") {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), "upgrade") {
				return true
			}
		}
	}
	return false
}

func unauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="movieapp"`)
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": msg})
}

// --- health ---

func (g *gateway) readyz(w http.ResponseWriter, r *http.Request) {
	if g.Ready != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := g.Ready(ctx); err != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

// --- cross-cutting middleware ---

type requestIDKey struct{}

var safeRequestID = regexp.MustCompile(`^[A-Za-z0-9-]{8,64}$`)

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if !safeRequestID.MatchString(id) {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
	})
}

func bodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		}
		next.ServeHTTP(w, r)
	})
}

func recoverPanics(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if p := recover(); p != nil && p != http.ErrAbortHandler {
					log.Error("panic in handler", "panic", p, "path", r.URL.Path)
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(c int)           { s.status = c; s.ResponseWriter.WriteHeader(c) }
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func (g *gateway) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		g.Log.Info("request", "method", r.Method, "path", r.URL.Path, "status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"ip", ratelimit.ClientIP(r, g.RateLimit.TrustForwardedFor),
			"request_id", w.Header().Get("X-Request-Id"))
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
