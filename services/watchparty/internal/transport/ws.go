// Package transport exposes the Watch-Party service over HTTP/WebSocket.
package transport

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/furkanpatat/movieapp/pkg/jwtauth"
	"github.com/furkanpatat/movieapp/services/watchparty/internal/hub"
)

// UserIDHeader carries the user authenticated by the gateway.
// Trust boundary: only the gateway may reach this service in production.
const UserIDHeader = "X-User-Id"

var roomRE = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type Handler struct {
	hub      *hub.Hub
	upgrader websocket.Upgrader
	log      *slog.Logger
}

// NewHandler routes:
//
//	GET /api/v1/watch-party/rooms/{room}/ws   WebSocket upgrade (needs X-User-Id)
//	GET /internal/stats                        rooms/clients snapshot (not routed by the gateway)
//	GET /healthz, /readyz
func NewHandler(h *hub.Hub, allowedOrigins []string, log *slog.Logger) http.Handler {
	hd := &Handler{
		hub: h, log: log,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     originChecker(allowedOrigins),
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /internal/stats", hd.stats)
	mux.HandleFunc("GET /api/v1/watch-party/rooms/{room}/ws", hd.serveWS)
	return mux
}

func (h *Handler) serveWS(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.Header.Get(UserIDHeader))
	if !jwtauth.ValidUserID(userID) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing or invalid user identity"})
		return
	}
	room := r.PathValue("room")
	if !roomRE.MatchString(room) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "room must be 1-64 characters: letters, digits, '_' or '-'"})
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil) // on failure gorilla has already replied (400/403)
	if err != nil {
		h.log.Debug("upgrade failed", "error", err)
		return
	}
	h.hub.Serve(conn, room, userID) // blocks until the connection ends
}

func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	s, err := h.hub.Stats(ctx)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// originChecker allows requests without an Origin (non-browser clients) and
// browser origins on the allow-list ("*" allows any).
func originChecker(allowed []string) func(*http.Request) bool {
	any := false
	set := make(map[string]struct{}, len(allowed))
	for _, o := range allowed {
		if o == "*" {
			any = true
		}
		set[strings.ToLower(strings.TrimRight(o, "/"))] = struct{}{}
	}
	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" || any {
			return true
		}
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" {
			return false
		}
		_, ok := set[strings.ToLower(u.Scheme+"://"+u.Host)]
		return ok
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
