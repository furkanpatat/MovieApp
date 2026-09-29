// Package transport exposes the Auth service over HTTP.
package transport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/furkanpatat/movieapp/pkg/jwtauth"
	"github.com/furkanpatat/movieapp/services/auth/internal/domain"
	"github.com/furkanpatat/movieapp/services/auth/internal/service"
)

const maxBody = 8 << 10

// CookieOptions configures the browser session cookie set at login.
type CookieOptions struct {
	Secure bool // HTTPS-only; off only for local plain-http development
}

type Handler struct {
	svc    *service.Auth
	ready  func(context.Context) error
	log    *slog.Logger
	cookie CookieOptions
}

func NewHandler(svc *service.Auth, ready func(context.Context) error, log *slog.Logger, cookie CookieOptions) http.Handler {
	h := &Handler{svc: svc, ready: ready, log: log, cookie: cookie}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /readyz", h.readyz)
	mux.HandleFunc("POST /api/v1/auth/register", h.register)
	mux.HandleFunc("POST /api/v1/auth/login", h.login)
	mux.HandleFunc("POST /api/v1/auth/refresh", h.refresh)
	mux.HandleFunc("POST /api/v1/auth/logout", h.logout)
	// Account deletion steps, called by the gateway for the signed-in user
	// (X-User-Id, which only the gateway sets): check the password, delete.
	mux.HandleFunc("POST /api/v1/auth/password/verify", h.verifyPassword)
	mux.HandleFunc("DELETE /api/v1/auth/account", h.deleteAccount)
	return mux
}

type registerRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Login    string `json:"login"` // username or email
	Password string `json:"password"`
	// Refresh asks for a refresh token too (API clients such as the mobile
	// app; the browser keeps its session in the cookie).
	Refresh bool `json:"refresh"`
}

type passwordRequest struct {
	Password string `json:"password"`
}

// userIDHeader carries the authenticated user, set by the gateway only.
const userIDHeader = "X-User-Id"

func (h *Handler) verifyPassword(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get(userIDHeader)
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	var req passwordRequest
	if !decode(w, r, &req) {
		return
	}
	if err := h.svc.VerifyPassword(r.Context(), userID, req.Password); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get(userIDHeader)
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	if err := h.svc.DeleteAccount(r.Context(), userID); err != nil {
		h.fail(w, err)
		return
	}
	h.setSessionCookie(w, "", time.Time{})
	w.WriteHeader(http.StatusNoContent)
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if !decode(w, r, &req) {
		return
	}
	u, err := h.svc.Register(r.Context(), req.Username, req.Email, req.Password)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": u.ID, "username": u.Username, "email": u.Email, "created_at": u.CreatedAt,
	})
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := h.svc.Login(r.Context(), req.Login, req.Password)
	if err != nil {
		h.fail(w, err)
		return
	}
	if req.Refresh {
		if res.RefreshToken, res.RefreshExpires, err = h.svc.IssueRefresh(r.Context(), res.User.ID); err != nil {
			h.fail(w, err)
			return
		}
	}
	// Browsers use the HttpOnly cookie (JS can't read it, so XSS can't steal
	// it); the token stays in the body for API clients, tests and tooling.
	h.setSessionCookie(w, res.Token, res.Expires)
	writeSession(w, res)
}

// refresh exchanges a refresh token for a new access token and the next
// refresh token (rotation). The cookie is untouched: this is the API
// clients' flow.
func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := h.svc.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeSession(w, res)
}

func writeSession(w http.ResponseWriter, res service.LoginResult) {
	w.Header().Set("Cache-Control", "no-store")
	body := map[string]any{
		"access_token": res.Token,
		"token_type":   "Bearer",
		"expires_in":   int(time.Until(res.Expires).Seconds()),
		"user":         map[string]string{"id": res.User.ID, "username": res.User.Username, "email": res.User.Email},
	}
	if res.RefreshToken != "" {
		body["refresh_token"] = res.RefreshToken
		body["refresh_expires_in"] = int(time.Until(res.RefreshExpires).Seconds())
	}
	writeJSON(w, http.StatusOK, body)
}

// logout clears the session cookie and, given one, revokes the refresh
// token's family. JWTs are stateless, so a copy of an access token taken
// elsewhere stays valid until it expires; the short TTL bounds that.
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	// The body is optional (browsers send none).
	_ = json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(&req)
	if err := h.svc.RevokeRefresh(r.Context(), req.RefreshToken); err != nil {
		h.log.Error("revoke refresh token", "error", err)
	}
	h.setSessionCookie(w, "", time.Time{})
	w.WriteHeader(http.StatusNoContent)
}

// setSessionCookie sets the session cookie, or deletes it when token is "".
func (h *Handler) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	c := &http.Cookie{
		Name:     jwtauth.CookieName,
		Value:    token,
		Path:     jwtauth.CookiePath,
		HttpOnly: true,
		Secure:   h.cookie.Secure,
		// Strict: the web app and the API are same-site, so normal use is
		// unaffected, and no cross-site request ever carries the session.
		SameSite: http.SameSiteStrictMode,
	}
	if token == "" {
		c.MaxAge = -1
	} else {
		c.Expires = expires
		c.MaxAge = int(time.Until(expires).Seconds())
	}
	http.SetCookie(w, c)
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, domain.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, domain.ErrInvalidCredentials):
		w.Header().Set("WWW-Authenticate", `Bearer realm="movieapp"`)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
	case errors.Is(err, domain.ErrInvalidToken):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid refresh token"})
	default:
		h.log.Error("request failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
}

func (h *Handler) readyz(w http.ResponseWriter, r *http.Request) {
	if h.ready != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := h.ready(ctx); err != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

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
