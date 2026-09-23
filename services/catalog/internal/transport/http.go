// Package transport exposes the Catalog service over HTTP.
package transport

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
	"github.com/furkanpatat/movieapp/services/catalog/internal/service"
)

type Handler struct {
	svc   *service.Catalog
	lib   *service.Library // nil: library routes are not served
	ready func() bool      // optional extra readiness signal
	log   *slog.Logger
}

func NewHandler(svc *service.Catalog, lib *service.Library, ready func() bool, log *slog.Logger) http.Handler {
	h := &Handler{svc: svc, lib: lib, ready: ready, log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /readyz", h.readyz)
	mux.HandleFunc("GET /api/v1/movies/popular", h.popular)
	mux.HandleFunc("GET /api/v1/movies/{id}", h.details)
	mux.HandleFunc("GET /api/v1/people/{id}", h.person)
	mux.HandleFunc("GET /api/v1/search/movies", h.search)
	if lib != nil {
		mux.HandleFunc("GET /api/v1/watchlist", h.watchlist)
		mux.HandleFunc("POST /api/v1/watchlist", h.addToWatchlist)
		mux.HandleFunc("DELETE /api/v1/watchlist/{movie_id}", h.removeFromWatchlist)
		mux.HandleFunc("GET /api/v1/ratings", h.ratings)
		mux.HandleFunc("PUT /api/v1/ratings", h.rate)
	}
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

func (h *Handler) person(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		h.fail(w, domain.ErrInvalidInput)
		return
	}
	p, err := h.svc.GetPerson(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "person not found"})
			return
		}
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// search: GET /api/v1/search/movies?q=dune&page=1
func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	page := 1
	if s := r.URL.Query().Get("page"); s != "" {
		p, err := strconv.Atoi(s)
		if err != nil {
			h.fail(w, domain.ErrInvalidInput)
			return
		}
		page = p
	}
	res, err := h.svc.SearchMovies(r.Context(), r.URL.Query().Get("q"), page)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid input"})
	case errors.Is(err, domain.ErrUnknownUser):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unknown user"})
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

// --- user library ---------------------------------------------------------

// UserIDHeader carries the authenticated user (the JWT subject), set by the
// gateway after verifying the session cookie or bearer token. Trust boundary:
// only the gateway may reach this service in production.
const UserIDHeader = "X-User-Id"

var uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// userID returns the caller's id, or replies 401. Bodies never name the user.
func userID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := strings.TrimSpace(r.Header.Get(UserIDHeader))
	if !uuidRE.MatchString(id) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user identity"})
		return "", false
	}
	return id, true
}

type watchlistRequest struct {
	MovieID int `json:"movie_id"`
}

type ratingRequest struct {
	MovieID int `json:"movie_id"`
	Rating  int `json:"rating"`
}

func (h *Handler) watchlist(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	items, err := h.lib.GetUserWatchlist(r.Context(), uid)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) addToWatchlist(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	var req watchlistRequest
	if !decode(w, r, &req) {
		return
	}
	item, err := h.lib.AddToWatchlist(r.Context(), uid, req.MovieID)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) removeFromWatchlist(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	id, err := strconv.Atoi(r.PathValue("movie_id"))
	if err != nil {
		h.fail(w, domain.ErrInvalidInput)
		return
	}
	if err := h.lib.RemoveFromWatchlist(r.Context(), uid, id); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ratings(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	items, err := h.lib.GetUserRatings(r.Context(), uid)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) rate(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	var req ratingRequest
	if !decode(w, r, &req) {
		return
	}
	item, err := h.lib.RateMovie(r.Context(), uid, req.MovieID, req.Rating)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

const maxBody = 16 << 10

// decode reads exactly one JSON object; unknown fields (e.g. a user_id) are
// rejected so nobody can believe they are acting as someone else.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
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
