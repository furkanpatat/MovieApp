package server

import (
	"bytes"
	"io"
	"net/http"
)

// deleteAccount deletes the signed-in user's account, across services:
//
//  1. Auth checks the password (a wrong one stops here: nothing is deleted);
//  2. Interaction removes their ratings and comments (idempotent);
//  3. Auth deletes the user, and with them (ON DELETE CASCADE) their
//     library and sessions, and clears the session cookie.
//
// The user goes last, so a failure part-way leaves an account to retry
// with; every step is safe to repeat.
func (g *gateway) deleteAccount() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := r.Context().Value(userKey{}).(string)
		if !ok || userID == "" {
			unauthorized(w, "missing identity")
			return
		}
		body, err := io.ReadAll(r.Body) // {"password": "..."}, size-limited by bodyLimit
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		verify, err := g.upstream(r, http.MethodPost, g.AuthService.String()+"/api/v1/auth/password/verify", userID, body)
		if err != nil {
			g.Log.Error("account deletion: password check", "error", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream unavailable"})
			return
		}
		if verify.StatusCode == http.StatusUnauthorized {
			// A wrong password, not a lost session: 403, so clients don't
			// take it for an expired session (a 401) and sign the user out.
			verify.Body.Close()
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "wrong password"})
			return
		}
		if verify.StatusCode/100 != 2 {
			copyResponse(w, verify) // 400: a malformed body
			return
		}
		verify.Body.Close()

		purge, err := g.upstream(r, http.MethodPost, g.Interaction.String()+"/api/v1/account/purge", userID, nil)
		if err != nil || purge.StatusCode/100 != 2 {
			g.Log.Error("account deletion: purge interactions", "error", err, "status", statusOf(purge))
			closeBody(purge)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not delete your data, please try again"})
			return
		}
		purge.Body.Close()

		del, err := g.upstream(r, http.MethodDelete, g.AuthService.String()+"/api/v1/auth/account", userID, nil)
		if err != nil || del.StatusCode/100 != 2 {
			g.Log.Error("account deletion: delete user", "error", err, "status", statusOf(del))
			closeBody(del)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not delete your account, please try again"})
			return
		}
		defer del.Body.Close()
		// Auth clears the session cookie (with the attributes it set it with).
		for _, c := range del.Header.Values("Set-Cookie") {
			w.Header().Add("Set-Cookie", c)
		}
		g.Log.Info("account deleted", "user_id", userID)
		w.WriteHeader(http.StatusNoContent)
	}
}

// upstream calls a service as the gateway, for userID.
func (g *gateway) upstream(r *http.Request, method, url, userID string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(r.Context(), method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set(UserIDHeader, userID)
	if rid, ok := r.Context().Value(requestIDKey{}).(string); ok {
		req.Header.Set("X-Request-Id", rid)
	}
	return g.transport.RoundTrip(req)
}

func copyResponse(w http.ResponseWriter, res *http.Response) {
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(res.StatusCode)
	_, _ = io.Copy(w, res.Body)
}

func statusOf(res *http.Response) int {
	if res == nil {
		return 0
	}
	return res.StatusCode
}

func closeBody(res *http.Response) {
	if res != nil {
		res.Body.Close()
	}
}
