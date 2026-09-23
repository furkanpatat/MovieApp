package server_test

import (
	"net/http"
	"testing"

	"github.com/furkanpatat/movieapp/pkg/jwtauth"
)

// Browsers authenticate with the HttpOnly session cookie instead of a Bearer
// header. These cover that path and the CSRF / CSWSH guards it needs.

const appOrigin = "https://app.example.com" // newEnv's default CORS allow-list

func cookie(tok string) string { return jwtauth.CookieName + "=" + tok }

func TestSessionCookieAuthenticatesSameOriginWrites(t *testing.T) {
	e := newEnv(t, opts{})
	resp := e.do(t, "POST", "/api/v1/movies/7/rate", `{"score":9}`, map[string]string{
		"Cookie": cookie(e.token(t, "alice")),
		"Origin": appOrigin,
	})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status %d", resp.StatusCode)
	}
	_, got := e.interaction.snapshot()
	if got.Header.Get("X-User-Id") != "alice" {
		t.Fatalf("upstream saw X-User-Id=%q", got.Header.Get("X-User-Id"))
	}
	if got.Header.Get("Cookie") != "" {
		t.Fatal("the session cookie (and the JWT in it) must not be forwarded upstream")
	}
}

func TestSessionCookieWriteFromUntrustedOriginIsRejected(t *testing.T) {
	e := newEnv(t, opts{})
	tok := e.token(t, "alice")
	for name, origin := range map[string]string{
		"foreign origin": "https://evil.example.net",
		"no origin":      "",
	} {
		hdr := map[string]string{"Cookie": cookie(tok)}
		if origin != "" {
			hdr["Origin"] = origin
		}
		if resp := e.do(t, "POST", "/api/v1/movies/7/rate", `{"score":1}`, hdr); resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", name, resp.StatusCode)
		}
	}
	if hits, _ := e.interaction.snapshot(); hits != 0 {
		t.Fatal("a rejected cross-origin write must never reach the upstream")
	}
}

func TestWildcardCORSNeverTrustsCookieWrites(t *testing.T) {
	e := newEnv(t, opts{corsOrigins: []string{"*"}})
	resp := e.do(t, "POST", "/api/v1/movies/7/rate", `{"score":1}`, map[string]string{
		"Cookie": cookie(e.token(t, "alice")),
		"Origin": "https://anything.example",
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403: \"*\" must not make every origin a trusted cookie origin", resp.StatusCode)
	}
}

func TestBearerHeaderIsUnaffectedByOriginChecks(t *testing.T) {
	// API clients send no Origin; a Bearer header can't be attached by a
	// third-party page, so it needs no CSRF check.
	e := newEnv(t, opts{})
	if resp := e.do(t, "POST", "/api/v1/movies/7/rate", `{"score":9}`, bearer(e.token(t, "bob"))); resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestInvalidSessionCookieIs401(t *testing.T) {
	e := newEnv(t, opts{})
	resp := e.do(t, "POST", "/api/v1/movies/7/rate", `{"score":9}`, map[string]string{
		"Cookie": cookie("not-a-jwt"),
		"Origin": appOrigin,
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestCORSAllowsCredentialsOnlyForListedOrigins(t *testing.T) {
	listed := newEnv(t, opts{})
	if got := doOrigin(t, listed, "GET", "/api/v1/movies/popular", appOrigin).Header.Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("listed origin: Allow-Credentials=%q, want true (else the browser drops the cookie)", got)
	}
	wildcard := newEnv(t, opts{corsOrigins: []string{"*"}})
	if got := doOrigin(t, wildcard, "GET", "/api/v1/movies/popular", "https://anything.example").Header.Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("wildcard origin: Allow-Credentials=%q, want none", got)
	}
}

func TestWebSocketAuthenticatesWithSessionCookie(t *testing.T) {
	e := newEnv(t, opts{})
	conn, _, err := e.dialWS(wsPath, http.Header{
		"Cookie": {cookie(e.token(t, "carol"))},
		"Origin": {appOrigin},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if h := readHello(t, conn); h["user_id"] != "carol" {
		t.Fatalf("%v", h)
	}
}

func TestCrossSiteWebSocketHijackingIsRejected(t *testing.T) {
	e := newEnv(t, opts{})
	_, resp, err := e.dialWS(wsPath, http.Header{
		"Cookie": {cookie(e.token(t, "carol"))},
		"Origin": {"https://evil.example.net"},
	})
	if err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403, got err=%v resp=%v", err, resp)
	}
	if hits, _ := e.wp.snapshot(); hits != 0 {
		t.Fatal("a hijack attempt must never reach the upstream")
	}
}
