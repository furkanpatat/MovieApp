// Package transport exposes the Catalog service over HTTP.
package transport

import (
	"context"
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
	chat  *service.Assistant
	ready func() bool // optional extra readiness signal
	log   *slog.Logger
}

// Option adds optional routes.
type Option func(*Handler)

// WithAssistant serves POST /api/v1/chat.
func WithAssistant(a *service.Assistant) Option { return func(h *Handler) { h.chat = a } }

func NewHandler(svc *service.Catalog, lib *service.Library, ready func() bool, log *slog.Logger, opts ...Option) http.Handler {
	h := &Handler{svc: svc, lib: lib, ready: ready, log: log}
	for _, o := range opts {
		o(h)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /readyz", h.readyz)
	mux.HandleFunc("GET /api/v1/movies/popular", h.popular)
	mux.HandleFunc("GET /api/v1/movies/{id}", h.details)
	mux.HandleFunc("GET /api/v1/people/{id}", h.person)
	mux.HandleFunc("GET /api/v1/search/movies", h.search)
	mux.HandleFunc("GET /api/v1/discover/movies", h.discover)
	mux.HandleFunc("GET /api/v1/discover/tv", h.discoverTV)
	mux.HandleFunc("GET /api/v1/search/tv", h.searchTV)
	mux.HandleFunc("GET /api/v1/tv/popular", h.popularTV)
	mux.HandleFunc("GET /api/v1/tv/{id}", h.tvDetails)
	if lib != nil {
		mux.HandleFunc("GET /api/v1/watchlist", h.watchlist)
		mux.HandleFunc("POST /api/v1/watchlist", h.addToWatchlist)
		mux.HandleFunc("DELETE /api/v1/watchlist/{movie_id}", h.removeFromWatchlist)
		mux.HandleFunc("GET /api/v1/ratings", h.ratings)
		mux.HandleFunc("PUT /api/v1/ratings", h.rate)
	}
	if h.chat != nil {
		mux.HandleFunc("POST /api/v1/chat", h.chatReply)
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

// intParam reads an optional integer query parameter (def when absent).
func intParam(r *http.Request, name string, def int) (int, error) {
	s := r.URL.Query().Get(name)
	if s == "" {
		return def, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, domain.ErrInvalidInput
	}
	return n, nil
}

func (h *Handler) popular(w http.ResponseWriter, r *http.Request) {
	page, err := intParam(r, "page", 1)
	if err != nil {
		h.fail(w, err)
		return
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

// discover: GET /api/v1/discover/movies?genre=878&page=3 (genre optional)
func (h *Handler) discover(w http.ResponseWriter, r *http.Request) {
	h.discoverWith(w, r, h.svc.DiscoverMovies)
}

// discoverTV: GET /api/v1/discover/tv?genre=10765&page=3 (TMDB TV genres)
func (h *Handler) discoverTV(w http.ResponseWriter, r *http.Request) {
	h.discoverWith(w, r, h.svc.DiscoverTV)
}

func (h *Handler) discoverWith(w http.ResponseWriter, r *http.Request, discover func(context.Context, int, int) (domain.MoviePage, error)) {
	genre, err := intParam(r, "genre", 0)
	if err != nil {
		h.fail(w, err)
		return
	}
	page, err := intParam(r, "page", 1)
	if err != nil {
		h.fail(w, err)
		return
	}
	res, err := discover(r.Context(), genre, page)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// searchTV: GET /api/v1/search/tv?q=office&page=1
func (h *Handler) searchTV(w http.ResponseWriter, r *http.Request) {
	page, err := intParam(r, "page", 1)
	if err != nil {
		h.fail(w, err)
		return
	}
	res, err := h.svc.SearchTV(r.Context(), r.URL.Query().Get("q"), page)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) popularTV(w http.ResponseWriter, r *http.Request) {
	page, err := intParam(r, "page", 1)
	if err != nil {
		h.fail(w, err)
		return
	}
	res, err := h.svc.GetPopularTV(r.Context(), page)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) tvDetails(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		h.fail(w, domain.ErrInvalidInput)
		return
	}
	m, err := h.svc.GetTVDetails(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "tv series not found"})
			return
		}
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
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

// Library requests name a title by media_type ("movie", the default, or
// "tv") and movie_id (the TMDB id of either; the name predates series).
type watchlistRequest struct {
	MediaType string `json:"media_type"`
	MovieID   int    `json:"movie_id"`
}

type ratingRequest struct {
	MediaType string `json:"media_type"`
	MovieID   int    `json:"movie_id"`
	Rating    int    `json:"rating"`
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
	ref, err := domain.ParseTitleRef(req.MediaType, req.MovieID)
	if err != nil {
		h.fail(w, err)
		return
	}
	item, err := h.lib.AddToWatchlist(r.Context(), uid, ref)
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
	// DELETE /api/v1/watchlist/{movie_id}?media_type=tv (default: movie)
	ref, err := domain.ParseTitleRef(r.URL.Query().Get("media_type"), id)
	if err != nil {
		h.fail(w, err)
		return
	}
	if err := h.lib.RemoveFromWatchlist(r.Context(), uid, ref); err != nil {
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
	ref, err := domain.ParseTitleRef(req.MediaType, req.MovieID)
	if err != nil {
		h.fail(w, err)
		return
	}
	item, err := h.lib.Rate(r.Context(), uid, ref, req.Rating)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

type chatRequest struct {
	Messages []domain.ChatMessage `json:"messages"`
	// Context: where the user is in the app. Only the route is accepted; the
	// server looks up what is on that page itself.
	Context struct {
		Path string `json:"path"`
	} `json:"context"`
	// Locale is the UI language ("en" or "tr"): the reply is written in it.
	Locale string `json:"locale"`
}

// chatReply: the recommendation assistant. Signed-in users only: once an
// LLM answers, every call costs money.
func (h *Handler) chatReply(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	var req chatRequest
	if !decodeLimit(w, r, &req, maxChatBody) {
		return
	}
	res, err := h.chat.Chat(r.Context(), uid, req.Messages, req.Context.Path, req.Locale)
	if errors.Is(err, domain.ErrInvalidInput) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err != nil {
		h.log.Error("chat failed", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "the assistant is unavailable, try again"})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// maxChatBody fits a full history (MaxChatMessages x MaxChatMessageLen runes).
const maxChatBody = 256 << 10

const maxBody = 16 << 10

// decode reads exactly one JSON object; unknown fields (e.g. a user_id) are
// rejected so nobody can believe they are acting as someone else.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	return decodeLimit(w, r, v, maxBody)
}

func decodeLimit(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
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
