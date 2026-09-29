package server_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/furkanpatat/movieapp/pkg/jwtauth"
	"github.com/furkanpatat/movieapp/services/gateway/internal/ratelimit"
	"github.com/furkanpatat/movieapp/services/gateway/internal/server"
)

// accountBackends fakes the auth and interaction services for account
// deletion, recording each call as "METHOD /path user".
type accountBackends struct {
	mu        sync.Mutex
	calls     []string
	password  string // the right one
	purgeFail bool
}

func (b *accountBackends) record(r *http.Request) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, r.Method+" "+r.URL.Path+" "+r.Header.Get("X-User-Id"))
}

func (b *accountBackends) log() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.calls...)
}

func newAccountGateway(t *testing.T, b *accountBackends) (*httptest.Server, *jwtauth.Manager) {
	t.Helper()
	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.record(r)
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v1/auth/password/verify":
			var body strings.Builder
			buf := make([]byte, 512)
			n, _ := r.Body.Read(buf)
			body.Write(buf[:n])
			if !strings.Contains(body.String(), `"`+b.password+`"`) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"invalid credentials"}`))
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == "DELETE" && r.URL.Path == "/api/v1/auth/account":
			http.SetCookie(w, &http.Cookie{Name: jwtauth.CookieName, Path: jwtauth.CookiePath, MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(auth.Close)
	interaction := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.record(r)
		if b.purgeFail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(interaction.Close)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	au, _ := url.Parse(auth.URL)
	iu, _ := url.Parse(interaction.URL)
	mgr := jwtauth.NewManager(secret, issuer, time.Hour)
	gw := httptest.NewServer(server.New(server.Deps{
		Catalog: iu, Interaction: iu, AuthService: au, WatchParty: iu, Auth: mgr,
		Limiter:         ratelimit.New(rdb, 1000, time.Minute),
		RateLimit:       ratelimit.MiddlewareConfig{Log: quiet},
		AuthLimiter:     ratelimit.New(rdb, 1000, time.Minute),
		AuthRateLimit:   ratelimit.MiddlewareConfig{KeyPrefix: "auth:ip:", Log: quiet},
		UpstreamTimeout: time.Second,
		Log:             quiet,
	}))
	t.Cleanup(gw.Close)
	return gw, mgr
}

func deleteAccount(t *testing.T, gw *httptest.Server, token, body string, extra map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", gw.URL+"/api/v1/account/delete", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func TestDeleteAccountChecksThePasswordThenPurgesThenDeletes(t *testing.T) {
	b := &accountBackends{password: "s3cret-password"}
	gw, mgr := newAccountGateway(t, b)
	tok, _, _ := mgr.Issue("alice")

	// A client-supplied identity is ignored: the token's user is used.
	res := deleteAccount(t, gw, tok, `{"password":"s3cret-password"}`, map[string]string{"X-User-Id": "mallory"})
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status %d", res.StatusCode)
	}
	want := []string{
		"POST /api/v1/auth/password/verify alice",
		"POST /api/v1/account/purge alice",
		"DELETE /api/v1/auth/account alice",
	}
	if got := b.log(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if c := res.Header.Get("Set-Cookie"); !strings.Contains(c, jwtauth.CookieName+"=") || !strings.Contains(c, "Max-Age=0") {
		t.Fatalf("session cookie not cleared: %q", c)
	}
}

func TestDeleteAccountWithAWrongPasswordDeletesNothing(t *testing.T) {
	b := &accountBackends{password: "s3cret-password"}
	gw, mgr := newAccountGateway(t, b)
	tok, _, _ := mgr.Issue("alice")
	if res := deleteAccount(t, gw, tok, `{"password":"guess"}`, nil); res.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d", res.StatusCode)
	}
	if got := b.log(); len(got) != 1 {
		t.Fatalf("only the password check may run, got %v", got)
	}
}

func TestDeleteAccountStopsIfThePurgeFails(t *testing.T) {
	b := &accountBackends{password: "s3cret-password", purgeFail: true}
	gw, mgr := newAccountGateway(t, b)
	tok, _, _ := mgr.Issue("alice")
	if res := deleteAccount(t, gw, tok, `{"password":"s3cret-password"}`, nil); res.StatusCode != http.StatusBadGateway {
		t.Fatalf("status %d", res.StatusCode)
	}
	for _, c := range b.log() {
		if strings.HasPrefix(c, "DELETE") {
			t.Fatalf("the user was deleted though their data wasn't: %v", b.log())
		}
	}
}

func TestDeleteAccountNeedsASession(t *testing.T) {
	b := &accountBackends{password: "s3cret-password"}
	gw, _ := newAccountGateway(t, b)
	if res := deleteAccount(t, gw, "", `{"password":"s3cret-password"}`, map[string]string{"X-User-Id": "alice"}); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d", res.StatusCode)
	}
	if got := b.log(); len(got) != 0 {
		t.Fatalf("nothing may be called without a session, got %v", got)
	}
}
