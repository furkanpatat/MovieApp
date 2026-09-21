package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"

	"github.com/furkanpatat/movieapp/pkg/jwtauth"
	"github.com/furkanpatat/movieapp/services/gateway/internal/ratelimit"
	"github.com/furkanpatat/movieapp/services/gateway/internal/server"
)

const (
	secret = "test-secret-test-secret-test-secret-0123456789"
	issuer = "movieapp-gateway"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// upstream is a fake backend that records what it receives.
type upstream struct {
	*httptest.Server
	name string
	mu   sync.Mutex
	hits int
	last seen
}

type seen struct {
	Method, Path, Query, Body string
	Header                    http.Header
}

func newUpstream(t *testing.T, name string, status int) *upstream {
	u := &upstream{name: name}
	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		u.mu.Lock()
		u.hits++
		u.last = seen{r.Method, r.URL.Path, r.URL.RawQuery, string(b), r.Header.Clone()}
		u.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprintf(w, `{"upstream":%q}`, name)
	}))
	t.Cleanup(u.Close)
	return u
}

func (u *upstream) snapshot() (int, seen) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.hits, u.last
}

type env struct {
	gw          *httptest.Server
	catalog     *upstream
	interaction *upstream
	mr          *miniredis.Miniredis
	auth        *jwtauth.Manager
	authSvc     *upstream
}

type opts struct {
	limit     int
	window    time.Duration
	failOpen  bool
	trustXFF  bool
	authLimit int // per-window budget for /api/v1/auth/* (default 1000)
	ready     func(context.Context) error
}

func newEnv(t *testing.T, o opts) *env {
	t.Helper()
	if o.limit == 0 {
		o.limit = 1000
	}
	if o.window == 0 {
		o.window = time.Minute
	}
	e := &env{
		catalog:     newUpstream(t, "catalog", 200),
		interaction: newUpstream(t, "interaction", 202),
		authSvc:     newUpstream(t, "auth", 200),
		mr:          miniredis.RunT(t),
		auth:        jwtauth.NewManager(secret, issuer, time.Hour),
	}
	rdb := redis.NewClient(&redis.Options{Addr: e.mr.Addr(), MaxRetries: -1, DialTimeout: 200 * time.Millisecond, ReadTimeout: 500 * time.Millisecond})
	cu, _ := url.Parse(e.catalog.URL)
	iu, _ := url.Parse(e.interaction.URL)
	au, _ := url.Parse(e.authSvc.URL)
	if o.authLimit == 0 {
		o.authLimit = 1000
	}
	e.gw = httptest.NewServer(server.New(server.Deps{
		Catalog: cu, Interaction: iu, AuthService: au, Auth: e.auth,
		Limiter:         ratelimit.New(rdb, o.limit, o.window),
		RateLimit:       ratelimit.MiddlewareConfig{FailOpen: o.failOpen, TrustForwardedFor: o.trustXFF, Log: quiet},
		AuthLimiter:     ratelimit.New(rdb, o.authLimit, o.window),
		AuthRateLimit:   ratelimit.MiddlewareConfig{KeyPrefix: "auth:ip:", FailOpen: false, TrustForwardedFor: o.trustXFF, Log: quiet},
		UpstreamTimeout: 300 * time.Millisecond,
		Ready:           o.ready,
		Log:             quiet,
	}))
	t.Cleanup(e.gw.Close)
	return e
}

func (e *env) token(t *testing.T, user string) string {
	t.Helper()
	tok, _, err := e.auth.Issue(user)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func (e *env) do(t *testing.T, method, path, body string, hdr map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, e.gw.URL+path, strings.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func bearer(tok string) map[string]string { return map[string]string{"Authorization": "Bearer " + tok} }

func signed(t *testing.T, method jwt.SigningMethod, key any, claims jwt.RegisteredClaims) string {
	t.Helper()
	s, err := jwt.NewWithClaims(method, claims).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// ---------------------------------------------------------------- 401

func TestProtectedRoutesRejectUnauthorizedRequests(t *testing.T) {
	e := newEnv(t, opts{})
	now := time.Now()
	valid := jwt.RegisteredClaims{Issuer: issuer, Subject: "mallory", IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour))}

	expired := valid
	expired.IssuedAt, expired.ExpiresAt = jwt.NewNumericDate(now.Add(-2*time.Hour)), jwt.NewNumericDate(now.Add(-time.Hour))
	wrongIssuer := valid
	wrongIssuer.Issuer = "someone-else"
	noExp := valid
	noExp.ExpiresAt = nil
	badSub := valid
	badSub.Subject = "alice\r\nX-Admin: 1"
	emptySub := valid
	emptySub.Subject = ""

	noneTok, _ := jwt.NewWithClaims(jwt.SigningMethodNone, valid).SignedString(jwt.UnsafeAllowNoneSignatureType)
	good := signed(t, jwt.SigningMethodHS256, []byte(secret), valid)
	tampered := good[:len(good)-4] + "AAAA"

	cases := map[string]map[string]string{
		"no header":         {},
		"empty bearer":      {"Authorization": "Bearer "},
		"wrong scheme":      {"Authorization": "Basic YWxpY2U6cHc="},
		"garbage":           bearer("not-a-jwt"),
		"wrong secret":      bearer(signed(t, jwt.SigningMethodHS256, []byte("another-secret-another-secret-another-secret"), valid)),
		"expired":           bearer(signed(t, jwt.SigningMethodHS256, []byte(secret), expired)),
		"wrong issuer":      bearer(signed(t, jwt.SigningMethodHS256, []byte(secret), wrongIssuer)),
		"no expiry":         bearer(signed(t, jwt.SigningMethodHS256, []byte(secret), noExp)),
		"alg none":          bearer(noneTok),
		"other HMAC alg":    bearer(signed(t, jwt.SigningMethodHS512, []byte(secret), valid)),
		"tampered":          bearer(tampered),
		"header-injection":  bearer(signed(t, jwt.SigningMethodHS256, []byte(secret), badSub)),
		"empty subject":     bearer(signed(t, jwt.SigningMethodHS256, []byte(secret), emptySub)),
		"spoofed user only": {"X-User-Id": "alice"},
	}
	for _, path := range []string{"/api/v1/movies/7/rate", "/api/v1/movies/7/comment", "/api/v1/interaction/movies/7/rate"} {
		for name, hdr := range cases {
			resp := e.do(t, "POST", path, `{"score":9,"text":"hi"}`, hdr)
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("%s [%s] -> %d, want 401", path, name, resp.StatusCode)
			}
			if resp.Header.Get("WWW-Authenticate") == "" {
				t.Errorf("%s [%s]: missing WWW-Authenticate", path, name)
			}
		}
	}
	if hits, _ := e.interaction.snapshot(); hits != 0 {
		t.Fatalf("unauthorized requests reached the upstream %d times", hits)
	}

	// Sanity: the same crafted-token path accepts a genuinely valid token.
	if resp := e.do(t, "POST", "/api/v1/movies/7/rate", `{"score":9}`, bearer(good)); resp.StatusCode != http.StatusAccepted {
		t.Fatalf("valid token rejected: %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------- routing + injection

func TestValidRequestIsRoutedWithInjectedIdentity(t *testing.T) {
	e := newEnv(t, opts{})
	tok := e.token(t, "alice")

	resp := e.do(t, "POST", "/api/v1/movies/7/rate", `{"score":9}`, map[string]string{
		"Authorization": "Bearer " + tok,
		"Content-Type":  "application/json",
		"X-User-Id":     "admin", // the client tries to pick its own identity
	})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var body map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["upstream"] != "interaction" {
		t.Fatalf("routed to %v", body)
	}
	hits, got := e.interaction.snapshot()
	if hits != 1 || got.Method != "POST" || got.Path != "/api/v1/movies/7/rate" || got.Body != `{"score":9}` {
		t.Fatalf("hits=%d %+v", hits, got)
	}
	if id := got.Header.Get("X-User-Id"); id != "alice" {
		t.Fatalf("upstream saw X-User-Id=%q, want the token's user 'alice' (client value must be overridden)", id)
	}
	if got.Header.Get("Authorization") != "" {
		t.Fatal("the JWT must not be forwarded upstream")
	}
	if got.Header.Get("X-Forwarded-For") == "" {
		t.Fatal("X-Forwarded-For should be set")
	}
	if hits, _ := e.catalog.snapshot(); hits != 0 {
		t.Fatal("catalog must not see interaction traffic")
	}

	// comment route + query string preserved through the alias
	resp = e.do(t, "POST", "/api/v1/interaction/movies/7/comment?src=web", `{"text":"nice"}`, bearer(e.token(t, "bob")))
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("alias status %d", resp.StatusCode)
	}
	_, got = e.interaction.snapshot()
	if got.Path != "/api/v1/movies/7/comment" || got.Query != "src=web" || got.Header.Get("X-User-Id") != "bob" {
		t.Fatalf("alias not rewritten correctly: %+v", got)
	}
}

func TestPublicRoutesNeedNoTokenAndNeverCarryIdentity(t *testing.T) {
	e := newEnv(t, opts{})
	spoof := map[string]string{"X-User-Id": "admin", "Authorization": "Bearer junk"}

	type route struct{ path, upstream, wantPath string }
	for _, r := range []route{
		{"/api/v1/movies/popular?page=2", "catalog", "/api/v1/movies/popular"},
		{"/api/v1/movies/550", "catalog", "/api/v1/movies/550"},
		{"/api/v1/movies/7/interactions", "interaction", "/api/v1/movies/7/interactions"},
		{"/api/v1/interaction/movies/7/interactions", "interaction", "/api/v1/movies/7/interactions"},
	} {
		resp := e.do(t, "GET", r.path, "", spoof)
		if resp.StatusCode != 200 && resp.StatusCode != 202 {
			t.Fatalf("%s -> %d", r.path, resp.StatusCode)
		}
		var body map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&body)
		if body["upstream"] != r.upstream {
			t.Errorf("%s routed to %q, want %q", r.path, body["upstream"], r.upstream)
		}
		up := map[string]*upstream{"catalog": e.catalog, "interaction": e.interaction}[r.upstream]
		_, got := up.snapshot()
		if got.Path != r.wantPath {
			t.Errorf("%s reached upstream as %q, want %q", r.path, got.Path, r.wantPath)
		}
		if got.Header.Get("X-User-Id") != "" || got.Header.Get("Authorization") != "" {
			t.Errorf("%s: spoofed identity/credentials leaked upstream: %v", r.path, got.Header)
		}
	}
	_, got := e.catalog.snapshot()
	_, _ = got, e
	if _, c := e.catalog.snapshot(); c.Query != "" && !strings.Contains(c.Query, "page=") {
		t.Fatalf("query lost: %q", c.Query)
	}
}

func TestUnknownRoutesAndMethods(t *testing.T) {
	e := newEnv(t, opts{})
	for _, c := range []struct {
		method, path string
		want         int
	}{
		{"GET", "/", 404},
		{"GET", "/api/v2/movies/1", 404},
		{"GET", "/admin", 404},
		{"DELETE", "/api/v1/movies/7", 405},
		{"PUT", "/api/v1/movies/7/rate", 405},
		{"GET", "/api/v1/auth/login", 405}, // credentials only via POST
	} {
		if resp := e.do(t, c.method, c.path, "{}", bearer(e.token(t, "alice"))); resp.StatusCode != c.want {
			t.Errorf("%s %s -> %d, want %d", c.method, c.path, resp.StatusCode, c.want)
		}
	}
}

func TestUpstreamFailuresAreReportedCleanly(t *testing.T) {
	e := newEnv(t, opts{})
	e.catalog.Close() // catalog is down
	resp := e.do(t, "GET", "/api/v1/movies/1", "", nil)
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("down upstream -> %d, want 502", resp.StatusCode)
	}
	if b, _ := io.ReadAll(resp.Body); !strings.Contains(string(b), "upstream unavailable") || strings.Contains(string(b), "127.0.0.1") {
		t.Fatalf("error body should be generic: %s", b)
	}
	// other upstream unaffected
	if resp := e.do(t, "GET", "/api/v1/movies/1/interactions", "", nil); resp.StatusCode != 202 {
		t.Fatalf("interaction affected by catalog outage: %d", resp.StatusCode)
	}

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(3 * time.Second):
		case <-r.Context().Done():
		}
	}))
	defer slow.Close()
	e2 := newEnv(t, opts{})
	su, _ := url.Parse(slow.URL)
	rdb := redis.NewClient(&redis.Options{Addr: e2.mr.Addr()})
	gw := httptest.NewServer(server.New(server.Deps{Catalog: su, Interaction: su, AuthService: su, Auth: e2.auth, Limiter: ratelimit.New(rdb, 100, time.Minute),
		UpstreamTimeout: 100 * time.Millisecond, Log: quiet}))
	defer gw.Close()
	r, err := http.Get(gw.URL + "/api/v1/movies/1")
	if err != nil || r.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("slow upstream -> %v %v, want 504", r, err)
	}
}

func TestRequestIDIsGeneratedAndPropagated(t *testing.T) {
	e := newEnv(t, opts{})
	resp := e.do(t, "GET", "/api/v1/movies/1", "", nil)
	id := resp.Header.Get("X-Request-Id")
	_, got := e.catalog.snapshot()
	if id == "" || got.Header.Get("X-Request-Id") != id {
		t.Fatalf("response id %q, upstream id %q", id, got.Header.Get("X-Request-Id"))
	}
	resp = e.do(t, "GET", "/api/v1/movies/1", "", map[string]string{"X-Request-Id": "trace-12345678"})
	if resp.Header.Get("X-Request-Id") != "trace-12345678" {
		t.Fatal("a well-formed client request id should be kept")
	}
	resp = e.do(t, "GET", "/api/v1/movies/1", "", map[string]string{"X-Request-Id": "bad id with spaces"})
	if r := resp.Header.Get("X-Request-Id"); r == "bad id with spaces" || r == "" {
		t.Fatalf("unsafe request id must be replaced, got %q", r)
	}
}

// ---------------------------------------------------------------- 429

func TestRateLimitIsEnforcedPerClient(t *testing.T) {
	e := newEnv(t, opts{limit: 5, window: time.Minute, trustXFF: true})
	get := func(ip string) *http.Response {
		return e.do(t, "GET", "/api/v1/movies/1", "", map[string]string{"X-Forwarded-For": ip})
	}

	for i := 1; i <= 5; i++ {
		resp := get("203.0.113.10")
		if resp.StatusCode != 200 {
			t.Fatalf("request %d -> %d", i, resp.StatusCode)
		}
		if resp.Header.Get("X-RateLimit-Limit") != "5" || resp.Header.Get("X-RateLimit-Remaining") != fmt.Sprint(5-i) {
			t.Fatalf("request %d headers: limit=%s remaining=%s", i, resp.Header.Get("X-RateLimit-Limit"), resp.Header.Get("X-RateLimit-Remaining"))
		}
	}
	before, _ := e.catalog.snapshot()
	resp := get("203.0.113.10")
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("6th request -> %d, want 429", resp.StatusCode)
	}
	if ra := resp.Header.Get("Retry-After"); ra == "" || ra == "0" {
		t.Fatalf("Retry-After = %q", ra)
	}
	if after, _ := e.catalog.snapshot(); after != before {
		t.Fatal("a rate-limited request must not reach the upstream")
	}

	// A different client is unaffected.
	if resp := get("203.0.113.99"); resp.StatusCode != 200 {
		t.Fatalf("other IP -> %d", resp.StatusCode)
	}
}

func TestRateLimitAppliesToAuthenticatedAndUnauthenticatedTraffic(t *testing.T) {
	e := newEnv(t, opts{limit: 3})
	for i := 0; i < 3; i++ { // 401s still consume the budget: floods of bad tokens are throttled too
		e.do(t, "POST", "/api/v1/movies/1/rate", `{}`, nil)
	}
	if resp := e.do(t, "POST", "/api/v1/movies/1/rate", `{"score":5}`, bearer(e.token(t, "alice"))); resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("even a valid token is throttled once the IP is over budget: %d", resp.StatusCode)
	}
}

func TestSpoofedForwardedForCannotEvadeLimit(t *testing.T) {
	e := newEnv(t, opts{limit: 3, trustXFF: false}) // not behind a trusted proxy
	codes := []int{}
	for i := 0; i < 5; i++ {
		codes = append(codes, e.do(t, "GET", "/api/v1/movies/1", "", map[string]string{"X-Forwarded-For": fmt.Sprintf("198.51.100.%d", i)}).StatusCode)
	}
	if codes[3] != 429 || codes[4] != 429 {
		t.Fatalf("rotating X-Forwarded-For evaded the limit: %v", codes)
	}
}

func TestTrustedForwardedForUsesRightmostEntry(t *testing.T) {
	e := newEnv(t, opts{limit: 2, trustXFF: true})
	// The client controls the left part; our proxy appended 192.0.2.1 on the right.
	for i := 0; i < 3; i++ {
		e.do(t, "GET", "/api/v1/movies/1", "", map[string]string{"X-Forwarded-For": fmt.Sprintf("10.0.0.%d, 192.0.2.1", i)})
	}
	if resp := e.do(t, "GET", "/api/v1/movies/1", "", map[string]string{"X-Forwarded-For": "10.9.9.9, 192.0.2.1"}); resp.StatusCode != 429 {
		t.Fatalf("client-controlled left entries must not create new buckets: %d", resp.StatusCode)
	}
}

func TestRateLimitWindowSlides(t *testing.T) {
	e := newEnv(t, opts{limit: 3, window: 400 * time.Millisecond})
	for i := 0; i < 3; i++ {
		e.do(t, "GET", "/api/v1/movies/1", "", nil)
	}
	if resp := e.do(t, "GET", "/api/v1/movies/1", "", nil); resp.StatusCode != 429 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	time.Sleep(450 * time.Millisecond)
	if resp := e.do(t, "GET", "/api/v1/movies/1", "", nil); resp.StatusCode != 200 {
		t.Fatalf("limit should recover after the window: %d", resp.StatusCode)
	}
}

func TestRateLimitIsAtomicUnderConcurrency(t *testing.T) {
	e := newEnv(t, opts{limit: 20})
	var wg sync.WaitGroup
	var mu sync.Mutex
	counts := map[int]int{}
	for i := 0; i < 80; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest("GET", e.gw.URL+"/api/v1/movies/1", nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return
			}
			resp.Body.Close()
			mu.Lock()
			counts[resp.StatusCode]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	if counts[200] != 20 || counts[429] != 60 {
		t.Fatalf("limit 20 of 80 concurrent requests: %v (want exactly 20 allowed)", counts)
	}
}

func TestHealthEndpointsAreNotRateLimited(t *testing.T) {
	e := newEnv(t, opts{limit: 1})
	e.do(t, "GET", "/api/v1/movies/1", "", nil)
	e.do(t, "GET", "/api/v1/movies/1", "", nil) // budget exhausted
	for _, p := range []string{"/healthz", "/readyz"} {
		if resp := e.do(t, "GET", p, "", nil); resp.StatusCode != 200 {
			t.Errorf("%s -> %d", p, resp.StatusCode)
		}
	}
}

func TestRedisOutageFailOpenAndFailClosed(t *testing.T) {
	open := newEnv(t, opts{limit: 1, failOpen: true})
	open.mr.Close()
	for i := 0; i < 3; i++ {
		if resp := open.do(t, "GET", "/api/v1/movies/1", "", nil); resp.StatusCode != 200 {
			t.Fatalf("fail-open: %d", resp.StatusCode)
		}
	}
	closed := newEnv(t, opts{limit: 1, failOpen: false})
	closed.mr.Close()
	if resp := closed.do(t, "GET", "/api/v1/movies/1", "", nil); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("fail-closed: %d, want 503", resp.StatusCode)
	}
}

// ---------------------------------------------------------------- dev login + readiness

func TestAuthRoutesAreProxiedToTheAuthService(t *testing.T) {
	e := newEnv(t, opts{})
	for _, path := range []string{"/api/v1/auth/register", "/api/v1/auth/login"} {
		resp := e.do(t, "POST", path, `{"username":"alice","password":"correct horse"}`, map[string]string{
			"X-User-Id":     "admin",       // must never reach the auth service
			"Authorization": "Bearer junk", // nor a stale credential
			"Content-Type":  "application/json",
		})
		if resp.StatusCode != 200 {
			t.Fatalf("%s -> %d", path, resp.StatusCode)
		}
		var body map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&body)
		if body["upstream"] != "auth" {
			t.Fatalf("%s routed to %v", path, body)
		}
		_, got := e.authSvc.snapshot()
		if got.Method != "POST" || got.Path != path || got.Body != `{"username":"alice","password":"correct horse"}` {
			t.Fatalf("%s reached the auth service as %+v", path, got)
		}
		if got.Header.Get("X-User-Id") != "" || got.Header.Get("Authorization") != "" {
			t.Fatalf("client identity leaked to the auth service: %v", got.Header)
		}
	}
	// Register/login need no token: they are how you get one.
	if hits, _ := e.interaction.snapshot(); hits != 0 {
		t.Fatal("auth traffic must not reach other services")
	}
}

func TestDevTokenEndpointIsGone(t *testing.T) {
	e := newEnv(t, opts{})
	// /api/v1/auth/* now belongs to the auth service; the gateway itself mints nothing.
	e.do(t, "POST", "/api/v1/auth/token", `{"user_id":"mallory"}`, nil)
	if _, got := e.authSvc.snapshot(); got.Path != "/api/v1/auth/token" {
		t.Fatalf("unknown auth paths should be forwarded to the auth service (which 404s), got %+v", got)
	}
	// And no locally forged token is accepted anywhere: identity comes from a verified JWT only.
	if resp := e.do(t, "POST", "/api/v1/movies/7/rate", `{"score":5}`, map[string]string{"X-User-Id": "mallory"}); resp.StatusCode != 401 {
		t.Fatalf("got %d", resp.StatusCode)
	}
}

func TestAuthEndpointsHaveTheirOwnStricterLimit(t *testing.T) {
	e := newEnv(t, opts{limit: 1000, authLimit: 3})
	for i := 0; i < 3; i++ {
		if resp := e.do(t, "POST", "/api/v1/auth/login", `{}`, nil); resp.StatusCode != 200 {
			t.Fatalf("login %d -> %d", i, resp.StatusCode)
		}
	}
	resp := e.do(t, "POST", "/api/v1/auth/login", `{}`, nil)
	if resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("4th login attempt -> %d, want 429 (brute-force protection)", resp.StatusCode)
	}
	if hits, _ := e.authSvc.snapshot(); hits != 3 {
		t.Fatalf("throttled attempts reached the auth service: %d", hits)
	}
	// register shares the credential budget; the rest of the API is unaffected.
	if resp := e.do(t, "POST", "/api/v1/auth/register", `{}`, nil); resp.StatusCode != 429 {
		t.Fatalf("register %d", resp.StatusCode)
	}
	if resp := e.do(t, "GET", "/api/v1/movies/1", "", nil); resp.StatusCode != 200 {
		t.Fatalf("general API affected by the auth limiter: %d", resp.StatusCode)
	}
}

func TestAuthLimiterFailsClosedEvenWhenGeneralLimiterFailsOpen(t *testing.T) {
	e := newEnv(t, opts{failOpen: true})
	e.mr.Close()
	if resp := e.do(t, "GET", "/api/v1/movies/1", "", nil); resp.StatusCode != 200 {
		t.Fatalf("general API should fail open: %d", resp.StatusCode)
	}
	if resp := e.do(t, "POST", "/api/v1/auth/login", `{}`, nil); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("credential endpoints must fail closed: %d", resp.StatusCode)
	}
}

func TestReadyz(t *testing.T) {
	var down bool
	e := newEnv(t, opts{ready: func(context.Context) error {
		if down {
			return errors.New("redis down")
		}
		return nil
	}})
	if resp := e.do(t, "GET", "/readyz", "", nil); resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	down = true
	if resp := e.do(t, "GET", "/readyz", "", nil); resp.StatusCode != 503 {
		t.Fatal(resp.StatusCode)
	}
}
