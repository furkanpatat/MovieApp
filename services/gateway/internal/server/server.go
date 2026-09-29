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
	// Chat (the LLM assistant) is slow and costs money per call: it gets a
	// longer upstream timeout and its own, stricter rate limit.
	ChatTimeout   time.Duration
	ChatLimiter   *ratelimit.Limiter
	ChatRateLimit ratelimit.MiddlewareConfig
	// CORSAllowedOrigins: browser origins allowed to call this API. "*" allows any.
	CORSAllowedOrigins []string
	// AdminUserIDs may use the /api/v1/admin/* endpoints (the admin panel).
	AdminUserIDs []string
	Ready        func(context.Context) error // dependency check for /readyz
	Log          *slog.Logger
}

type gateway struct {
	Deps
	transport     http.RoundTripper
	chatTransport http.RoundTripper
	// trustedOrigins is the explicit CORS allow-list ("*" excluded): the only
	// origins whose requests may be authenticated by the session cookie.
	trustedOrigins map[string]struct{}
}

// New builds the gateway handler.
//
//	/healthz, /readyz                       not rate limited, no auth
//	POST /api/v1/auth/register|login|logout -> Auth service   (public, strict rate limit)
//	GET  /api/v1/movies/...                 -> Catalog        (public)
//	GET  /api/v1/people/{id}               -> Catalog        (public; actor pages)
//	GET  /api/v1/search/movies?q=          -> Catalog        (public)
//	GET  /api/v1/discover/movies?genre=    -> Catalog        (public; Discover feed)
//	GET  /api/v1/tv/popular, /tv/{id}      -> Catalog        (public; TV series)
//	GET  /api/v1/tv/{id}/interactions      -> Interaction    (public)
//	POST /api/v1/tv/{id}/rate|comment      -> Interaction    (JWT required)
//	POST /api/v1/comments/{id}/report      -> Interaction    (JWT required)
//	GET|PUT|DELETE /api/v1/blocks[/{userId}] -> Interaction  (JWT required)
//	GET  /api/v1/movies/{id}/interactions  -> Interaction    (public)
//	POST /api/v1/movies/{id}/rate|comment  -> Interaction    (JWT required)
//	GET|POST /api/v1/watchlist, DELETE /api/v1/watchlist/{movie_id},
//	GET|PUT  /api/v1/ratings               -> Catalog        (JWT required; the user's library)
//	GET|POST /api/v1/watched, DELETE /api/v1/watched/{movie_id}
//	                                       -> Catalog        (JWT required; what the user watched)
//	GET  /api/v1/users/{username}/watched  -> Catalog        (public; profile pages)
//	POST /api/v1/chat                      -> Catalog        (JWT required; AI recommendation assistant)
//	GET  /api/v1/watch-party/...           -> Watch-Party    (JWT required; WebSocket upgrade,
//	                                          token in Authorization header, ?token= or cookie)
//	     ...also under /api/v1/interaction/movies/{id}/...
//
// "JWT required" accepts either an Authorization: Bearer header (API clients)
// or the HttpOnly session cookie the Auth service sets at login (browsers).
func New(d Deps) http.Handler {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.UpstreamTimeout <= 0 {
		d.UpstreamTimeout = 10 * time.Second
	}
	if d.ChatTimeout <= 0 {
		d.ChatTimeout = 60 * time.Second
	}
	newTransport := func(timeout time.Duration) *http.Transport {
		return &http.Transport{
			DialContext:           (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			ResponseHeaderTimeout: timeout,
			MaxIdleConnsPerHost:   100,
			IdleConnTimeout:       90 * time.Second,
		}
	}
	g := &gateway{Deps: d, transport: newTransport(d.UpstreamTimeout), chatTransport: newTransport(d.ChatTimeout),
		trustedOrigins: map[string]struct{}{}}
	for _, o := range d.CORSAllowedOrigins {
		if o != "*" {
			g.trustedOrigins[o] = struct{}{}
		}
	}

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
	api.Handle("GET /api/v1/people/", catalog)
	api.Handle("GET /api/v1/search/", catalog)
	api.Handle("GET /api/v1/discover/", catalog)
	api.Handle("GET /api/v1/tv/", catalog)
	api.Handle("GET /api/v1/movies/{id}/interactions", interaction)
	api.Handle("POST /api/v1/movies/{id}/rate", requireAuth(interaction))
	api.Handle("POST /api/v1/movies/{id}/comment", requireAuth(interaction))
	// The same for TV series (more specific than the catalog's /tv/ prefix).
	api.Handle("GET /api/v1/tv/{id}/interactions", interaction)
	api.Handle("POST /api/v1/tv/{id}/rate", requireAuth(interaction))
	api.Handle("POST /api/v1/tv/{id}/comment", requireAuth(interaction))
	// Reporting a comment and blocking a user (moderation).
	api.Handle("POST /api/v1/comments/{id}/report", requireAuth(interaction))
	api.Handle("GET /api/v1/blocks", requireAuth(interaction))
	api.Handle("PUT /api/v1/blocks/{userId}", requireAuth(interaction))
	api.Handle("DELETE /api/v1/blocks/{userId}", requireAuth(interaction))
	// The admin panel: reported comments. Admins only (ADMIN_USER_IDS).
	api.Handle("GET /api/v1/admin/me", requireAuth(g.adminMe()))
	api.Handle("GET /api/v1/admin/reports", requireAuth(g.requireAdmin(interaction)))
	api.Handle("POST /api/v1/admin/comments/{id}/delete", requireAuth(g.requireAdmin(interaction)))
	api.Handle("POST /api/v1/admin/comments/{id}/dismiss", requireAuth(g.requireAdmin(interaction)))
	// The user's library (watchlist + personal ratings). The user is always
	// the authenticated one: requireAuth injects X-User-Id, clients can't.
	api.Handle("GET /api/v1/watchlist", requireAuth(catalog))
	api.Handle("POST /api/v1/watchlist", requireAuth(catalog))
	api.Handle("DELETE /api/v1/watchlist/{movie_id}", requireAuth(catalog))
	api.Handle("GET /api/v1/ratings", requireAuth(catalog))
	api.Handle("PUT /api/v1/ratings", requireAuth(catalog))
	api.Handle("GET /api/v1/watched", requireAuth(catalog))
	api.Handle("POST /api/v1/watched", requireAuth(catalog))
	api.Handle("DELETE /api/v1/watched/{movie_id}", requireAuth(catalog))
	// Account orchestration
	api.Handle("POST /api/v1/account/delete", requireAuth(g.deleteAccount()))
	// Public profiles: anyone may see what a user watched.
	api.Handle("GET /api/v1/users/{username}/watched", catalog)
	// AI assistant: signed-in only (an LLM call costs money per request),
	// with a longer timeout and a stricter per-client budget.
	chat := requireAuth(g.proxyVia(d.Catalog, g.chatTransport))
	if d.ChatLimiter != nil {
		chat = ratelimit.Middleware(d.ChatLimiter, d.ChatRateLimit)(chat)
	}
	api.Handle("POST /api/v1/chat", chat)
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

	return requestID(g.accessLog(recoverPanics(d.Log)(cors(d.CORSAllowedOrigins)(bodyLimit(root)))))
}

// --- proxying ---

func (g *gateway) proxy(target *url.URL) http.Handler { return g.proxyVia(target, g.transport) }

func (g *gateway) proxyVia(target *url.URL, transport http.RoundTripper) http.Handler {
	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			// Rewrite starts from a copy of the client's headers, so scrub what
			// upstreams must never take from a client:
			r.Out.Header.Del(UserIDHeader)    // no identity spoofing
			r.Out.Header.Del("Authorization") // upstreams don't need (or see) the JWT
			r.Out.Header.Del("Cookie")        // ...nor the session cookie that carries it
			r.SetXForwarded()                 // replaces any client-supplied X-Forwarded-*
			if id, ok := r.In.Context().Value(userKey{}).(string); ok {
				r.Out.Header.Set(UserIDHeader, id)
			}
			if rid, ok := r.In.Context().Value(requestIDKey{}).(string); ok {
				r.Out.Header.Set("X-Request-Id", rid)
			}
		},
		Transport: transport,
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
		fromCookie := false
		if !ok {
			token, ok = sessionCookie(r)
			fromCookie = ok
		}
		if !ok {
			unauthorized(w, "missing or malformed bearer token")
			return
		}
		// Browsers attach cookies on their own, so a cookie-authenticated
		// write must also prove it came from our web app (CSRF). SameSite=
		// Strict already stops cross-site requests; this also covers other
		// same-site origins (sibling subdomains, other localhost ports).
		if fromCookie && !safeMethod(r.Method) && !g.trustedOrigin(r) {
			forbidden(w, "cross-origin request rejected")
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
// clients cannot send custom headers, but they do send cookies, so browsers
// authenticate with the session cookie. Non-browser clients may instead pass
// the JWT in the `token` query parameter, accepted only on an upgrade request
// without an Authorization header. Either way the credential is verified here
// and never forwarded: the upstream only ever sees the injected X-User-Id.
func (g *gateway) requireAuthWS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok && isWebSocketUpgrade(r) {
			if t := strings.TrimSpace(r.URL.Query().Get("token")); t != "" {
				token, ok = t, true
			}
		}
		fromCookie := false
		if !ok {
			token, ok = sessionCookie(r)
			fromCookie = ok
		}
		if !ok {
			unauthorized(w, "missing or malformed bearer token")
			return
		}
		// WebSockets ignore CORS, so a cookie-authenticated handshake from
		// another origin would otherwise hijack the user's session (CSWSH).
		if fromCookie && !g.trustedOrigin(r) {
			forbidden(w, "cross-origin request rejected")
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

// sessionCookie returns the JWT from the browser session cookie, if present.
func sessionCookie(r *http.Request) (string, bool) {
	c, err := r.Cookie(jwtauth.CookieName)
	if err != nil || strings.TrimSpace(c.Value) == "" {
		return "", false
	}
	return c.Value, true
}

// trustedOrigin reports whether the request's Origin is in the explicit CORS
// allow-list. A missing Origin fails: browsers always send one on the requests
// this guards (non-GET fetches and WebSocket handshakes).
func (g *gateway) trustedOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	_, ok := g.trustedOrigins[origin]
	return origin != "" && ok
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
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

func forbidden(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusForbidden, map[string]string{"error": msg})
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

// cors answers preflight requests and tags every response so a browser will
// actually let its own JS read it. It runs outside rate limiting and auth:
// a preflight OPTIONS carries neither an Authorization header nor a body, so
// it must never be rejected as unauthorized or metered against the request
// budget the real request behind it will also consume.
//
// Browsers authenticate with the HttpOnly session cookie, which fetch only
// sends (and lets JS read the response of) when the response also carries
// Access-Control-Allow-Credentials. That is granted to explicitly listed
// origins only, never through "*": echoing any origin with credentials would
// let every site on the web make authenticated calls as the user.
func cors(allowedOrigins []string) func(http.Handler) http.Handler {
	any := false
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o == "*" {
			any = true
		}
		allowed[o] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				_, ok := allowed[origin]
				if any || ok {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Vary", "Origin")
				}
				if ok {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
			}

			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				w.Header().Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
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

func (g *gateway) isAdmin(userID string) bool {
	for _, id := range g.AdminUserIDs {
		if id == userID {
			return true
		}
	}
	return false
}

// requireAdmin lets only the admins (ADMIN_USER_IDS) through; it runs after
// requireAuth, which has put the verified user in the context.
func (g *gateway) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if userID, _ := r.Context().Value(userKey{}).(string); !g.isAdmin(userID) {
			forbidden(w, "admins only")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// adminMe tells the panel who is asking and whether they are an admin: a
// user who isn't is shown their id, to be added to ADMIN_USER_IDS.
func (g *gateway) adminMe() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, _ := r.Context().Value(userKey{}).(string)
		writeJSON(w, http.StatusOK, map[string]any{"user_id": userID, "admin": g.isAdmin(userID)})
	})
}
