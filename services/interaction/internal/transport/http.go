// Package transport exposes the Interaction service over HTTP.
package transport

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanpatat/movieapp/services/interaction/internal/domain"
	"github.com/furkanpatat/movieapp/services/interaction/internal/service"
)

const maxBody = 16 << 10

type Handler struct {
	cmd   *service.Command
	query *service.Query
	ready func() bool
	log   *slog.Logger
}

// NewHandler builds the router. cmd or query may be nil to serve only one side.
func NewHandler(cmd *service.Command, query *service.Query, ready func() bool, log *slog.Logger) http.Handler {
	h := &Handler{cmd: cmd, query: query, ready: ready, log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if h.ready != nil && !h.ready() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	if cmd != nil {
		mux.HandleFunc("POST /api/v1/movies/{id}/rate", h.rate)
		mux.HandleFunc("POST /api/v1/movies/{id}/comment", h.comment)
	}
	if query != nil {
		mux.HandleFunc("GET /api/v1/movies/{id}/interactions", h.interactions)
	}
	return mux
}

// The acting user is never taken from the body: the API Gateway authenticates
// the caller and injects X-User-Id. Bodies that still carry a user_id are
// rejected (unknown field) so nobody can believe they are acting as someone else.
type rateRequest struct {
	Score int `json:"score"`
}

type commentRequest struct {
	Text string `json:"text"`
}

// UserIDHeader carries the authenticated user, set by the gateway.
// Trust boundary: only the gateway may reach this service in production.
const UserIDHeader = "X-User-Id"

func userID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := strings.TrimSpace(r.Header.Get(UserIDHeader))
	if id == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user identity"})
		return "", false
	}
	return id, true
}

func (h *Handler) rate(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	id, ok := movieID(w, r)
	if !ok {
		return
	}
	var req rateRequest
	if !decode(w, r, &req) {
		return
	}
	eventID, err := h.cmd.SubmitRating(r.Context(), id, uid, req.Score)
	h.accepted(w, eventID, err)
}

func (h *Handler) comment(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	id, ok := movieID(w, r)
	if !ok {
		return
	}
	var req commentRequest
	if !decode(w, r, &req) {
		return
	}
	eventID, err := h.cmd.SubmitComment(r.Context(), id, uid, req.Text)
	h.accepted(w, eventID, err)
}

// accepted replies 202: the event is safely on the broker, but not yet
// visible in the read model (eventual consistency).
func (h *Handler) accepted(w http.ResponseWriter, eventID string, err error) {
	switch {
	case err == nil:
		writeJSON(w, http.StatusAccepted, map[string]string{"event_id": eventID, "status": "accepted"})
	case errors.Is(err, domain.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		h.log.Error("publish failed", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "could not accept event, retry later"})
	}
}

func (h *Handler) interactions(w http.ResponseWriter, r *http.Request) {
	id, ok := movieID(w, r)
	if !ok {
		return
	}
	limit := 0
	if s := r.URL.Query().Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be a positive integer"})
			return
		}
		limit = n
	}
	res, err := h.query.GetInteractions(r.Context(), id, limit)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, res)
	case errors.Is(err, domain.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		h.log.Error("query failed", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "temporarily unavailable"})
	}
}

func movieID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "movie id must be a positive integer"})
		return 0, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) { // trailing garbage
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
