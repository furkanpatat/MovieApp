// Package transport exposes the Catalog service over HTTP.
package transport

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
	"github.com/furkanpatat/movieapp/services/catalog/internal/service"
)

type Handler struct {
	svc   *service.Catalog
	ready func() bool // optional extra readiness signal
	log   *slog.Logger
}

func NewHandler(svc *service.Catalog, ready func() bool, log *slog.Logger) http.Handler {
	h := &Handler{svc: svc, ready: ready, log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /readyz", h.readyz)
	mux.HandleFunc("GET /api/v1/movies/popular", h.popular)
	mux.HandleFunc("GET /api/v1/movies/{id}", h.details)
	return mux
}

func (h *Handler) readyz(w http.ResponseWriter, _ *http.Request) {
	if h.ready != nil && !h.ready() {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) popular(w http.ResponseWriter, r *http.Request) {
	page := 1
	if s := r.URL.Query().Get("page"); s != "" {
		p, err := strconv.Atoi(s)
		if err != nil {
			h.fail(w, domain.ErrInvalidInput)
			return
		}
		page = p
	}
	res, err := h.svc.GetPopularMovies(r.Context(), page)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) details(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		h.fail(w, domain.ErrInvalidInput)
		return
	}
	m, err := h.svc.GetMovieDetails(r.Context(), id)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid input"})
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "movie not found"})
	case errors.Is(err, domain.ErrUnavailable):
		h.log.Error("upstream unavailable", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "upstream unavailable"})
	default:
		h.log.Error("request failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
