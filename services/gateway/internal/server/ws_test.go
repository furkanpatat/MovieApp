package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

// wsUpstream is a fake Watch-Party service: it upgrades, tells the client what
// the handshake looked like, then echoes every message back.
type wsUpstream struct {
	*httptest.Server
	mu   sync.Mutex
	hits int
	last struct {
		Path, Query, UserID, Authorization, Origin string
	}
}

func newWSUpstream(t *testing.T) *wsUpstream {
	u := &wsUpstream{}
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.mu.Lock()
		u.hits++
		u.last.Path, u.last.Query = r.URL.Path, r.URL.RawQuery
		u.last.UserID, u.last.Authorization, u.last.Origin = r.Header.Get("X-User-Id"), r.Header.Get("Authorization"), r.Header.Get("Origin")
		snap := u.last
		u.mu.Unlock()

		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		hello, _ := json.Marshal(map[string]string{"type": "hello", "user_id": snap.UserID, "query": snap.Query, "authorization": snap.Authorization, "path": snap.Path})
		_ = conn.WriteMessage(websocket.TextMessage, hello)
		for {
			mt, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			_ = conn.WriteMessage(mt, append([]byte("echo:"), data...))
		}
	}))
	t.Cleanup(u.Close)
	return u
}

func (u *wsUpstream) snapshot() (int, string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.hits, u.last.UserID
}

func (e *env) wsURL(path string) string {
	return "ws" + strings.TrimPrefix(e.gw.URL, "http") + path
}

func (e *env) dialWS(path string, hdr http.Header) (*websocket.Conn, *http.Response, error) {
	return websocket.DefaultDialer.Dial(e.wsURL(path), hdr)
}

func readHello(t *testing.T, c *websocket.Conn) map[string]string {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, data, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("no hello from upstream: %v", err)
	}
	var m map[string]string
	_ = json.Unmarshal(data, &m)
	return m
}

const wsPath = "/api/v1/watch-party/rooms/movie-night/ws"

// ------------------------------------------------------------------ auth

func TestWebSocketUpgradeWithoutValidTokenIsRejected(t *testing.T) {
	e := newEnv(t, opts{})
	now := time.Now()
	valid := jwt.RegisteredClaims{Issuer: issuer, Subject: "mallory", IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour))}
	expired := valid
	expired.IssuedAt, expired.ExpiresAt = jwt.NewNumericDate(now.Add(-2*time.Hour)), jwt.NewNumericDate(now.Add(-time.Hour))
	noneTok, _ := jwt.NewWithClaims(jwt.SigningMethodNone, valid).SignedString(jwt.UnsafeAllowNoneSignatureType)

	for name, path := range map[string]string{
		"no token":      wsPath,
		"empty token":   wsPath + "?token=",
		"garbage token": wsPath + "?token=abc.def.ghi",
		"wrong secret":  wsPath + "?token=" + signed(t, jwt.SigningMethodHS256, []byte("another-secret-another-secret-another-secret"), valid),
		"expired":       wsPath + "?token=" + signed(t, jwt.SigningMethodHS256, []byte(secret), expired),
		"alg none":      wsPath + "?token=" + noneTok,
		"tampered":      wsPath + "?token=" + e.token(t, "alice")[:len(e.token(t, "alice"))-3] + "AAA",
	} {
		conn, resp, err := e.dialWS(path, http.Header{"X-User-Id": {"admin"}}) // and a spoofed identity header for good measure
		if conn != nil {
			conn.Close()
		}
		if err == nil || resp == nil || resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s: want a 401 handshake failure, got resp=%v err=%v", name, resp, err)
		}
	}
	if hits, _ := e.wp.snapshot(); hits != 0 {
		t.Fatalf("rejected handshakes reached the watch-party service %d times", hits)
	}
}

func TestQueryTokenIsValidatedAndIdentityInjected(t *testing.T) {
	e := newEnv(t, opts{})
	tok := e.token(t, "alice")

	conn, resp, err := e.dialWS(wsPath+"?token="+tok+"&lang=tr", http.Header{
		"X-User-Id": {"admin"}, // the client tries to choose its own identity
		"Origin":    {"https://app.example.com"},
	})
	if err != nil {
		t.Fatalf("handshake failed: %v (resp %v)", err, resp)
	}
	defer conn.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status %d", resp.StatusCode)
	}

	hello := readHello(t, conn)
	if hello["user_id"] != "alice" {
		t.Fatalf("upstream saw X-User-Id=%q, want the token's user 'alice' (client value must be overridden)", hello["user_id"])
	}
	if strings.Contains(hello["query"], "token") || strings.Contains(hello["query"], tok) {
		t.Fatalf("the JWT leaked to the upstream in the query string: %q", hello["query"])
	}
	if hello["query"] != "lang=tr" {
		t.Fatalf("other query parameters must be preserved, got %q", hello["query"])
	}
	if hello["authorization"] != "" {
		t.Fatal("Authorization must not be forwarded")
	}
	if hello["path"] != wsPath {
		t.Fatalf("path %q", hello["path"])
	}
	if _, origin := e.wp.snapshotOrigin(); origin != "https://app.example.com" {
		t.Fatalf("Origin must reach the upstream for its own origin check, got %q", origin)
	}
}

func (u *wsUpstream) snapshotOrigin() (string, string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.last.UserID, u.last.Origin
}

func TestAuthorizationHeaderStillWorksForNonBrowserClients(t *testing.T) {
	e := newEnv(t, opts{})
	conn, _, err := e.dialWS(wsPath, http.Header{"Authorization": {"Bearer " + e.token(t, "bob")}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if h := readHello(t, conn); h["user_id"] != "bob" || h["authorization"] != "" {
		t.Fatalf("%v", h)
	}
}

func TestWebSocketMessagesFlowBothWaysThroughTheProxy(t *testing.T) {
	e := newEnv(t, opts{})
	conn, _, err := e.dialWS(wsPath+"?token="+e.token(t, "alice"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	readHello(t, conn)

	for i := 0; i < 50; i++ {
		msg := `{"type":"chat_message","text":"hello ` + strings.Repeat("x", i) + `"}`
		if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
			t.Fatal(err)
		}
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		_, data, err := conn.ReadMessage()
		if err != nil || string(data) != "echo:"+msg {
			t.Fatalf("round trip %d: %q %v", i, data, err)
		}
	}
	// a clean close from the client is relayed and does not wedge the gateway
	_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye"), time.Now().Add(time.Second))
	conn.Close()
	if r := e.do(t, "GET", "/api/v1/movies/1", "", nil); r.StatusCode != 200 {
		t.Fatalf("gateway unhealthy after a WebSocket closed: %d", r.StatusCode)
	}
}

func TestManyConcurrentWebSocketsThroughTheGateway(t *testing.T) {
	e := newEnv(t, opts{})
	const n = 40
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			user := "user" + strings.Repeat("x", i%5)
			conn, _, err := e.dialWS(wsPath+"?token="+e.token(t, user), nil)
			if err != nil {
				errs <- err
				return
			}
			defer conn.Close()
			readHello(t, conn)
			for j := 0; j < 20; j++ {
				if err := conn.WriteMessage(websocket.TextMessage, []byte("ping")); err != nil {
					errs <- err
					return
				}
				_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
				if _, _, err := conn.ReadMessage(); err != nil {
					errs <- err
					return
				}
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

// ------------------------------------------------------------------ the query token is a WebSocket-only escape hatch

func TestQueryTokenIsNotAcceptedAnywhereElse(t *testing.T) {
	e := newEnv(t, opts{})
	tok := e.token(t, "alice")

	// (a) on other authenticated routes
	for _, path := range []string{"/api/v1/movies/7/rate?token=" + tok, "/api/v1/movies/7/comment?token=" + tok, "/api/v1/interaction/movies/7/rate?token=" + tok} {
		if r := e.do(t, "POST", path, `{"score":5,"text":"x"}`, nil); r.StatusCode != 401 {
			t.Errorf("%s -> %d, want 401 (query tokens are for WebSocket handshakes only)", path, r.StatusCode)
		}
	}
	if hits, _ := e.interaction.snapshot(); hits != 0 {
		t.Fatal("a query token authorised a write")
	}

	// (a2) even a request that *claims* to be a WebSocket upgrade must not turn
	// a query token into credentials on a non-WebSocket route
	for _, path := range []string{"/api/v1/movies/7/rate?token=" + tok, "/api/v1/movies/7/comment?token=" + tok, "/api/v1/interaction/movies/7/rate?token=" + tok} {
		r := e.do(t, "POST", path, `{"score":5,"text":"x"}`, map[string]string{"Upgrade": "websocket", "Connection": "Upgrade"})
		if r.StatusCode != 401 {
			t.Errorf("%s with upgrade headers -> %d, want 401", path, r.StatusCode)
		}
	}

	// (b) on a plain (non-upgrade) request to the WebSocket route itself
	if r := e.do(t, "GET", wsPath+"?token="+tok, "", nil); r.StatusCode != 401 {
		t.Fatalf("plain GET with ?token= -> %d, want 401", r.StatusCode)
	}
	// ...but a proper Authorization header on a plain request is proxied (and the upstream will refuse the non-upgrade)
	if r := e.do(t, "GET", wsPath, "", bearer(tok)); r.StatusCode == 401 {
		t.Fatalf("header auth should pass the gateway: %d", r.StatusCode)
	}
}

func TestWebSocketHandshakesShareTheRateLimit(t *testing.T) {
	e := newEnv(t, opts{limit: 3})
	tok := e.token(t, "alice")
	var conns []*websocket.Conn
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()
	for i := 0; i < 3; i++ {
		c, _, err := e.dialWS(wsPath+"?token="+tok, nil)
		if err != nil {
			t.Fatalf("handshake %d: %v", i, err)
		}
		conns = append(conns, c)
	}
	c, resp, err := e.dialWS(wsPath+"?token="+tok, nil)
	if c != nil {
		c.Close()
	}
	if err == nil || resp == nil || resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("4th handshake: want 429, got resp=%v err=%v", resp, err)
	}
}

func TestWatchPartyUpstreamDownIs502(t *testing.T) {
	e := newEnv(t, opts{})
	e.wp.Close()
	c, resp, err := e.dialWS(wsPath+"?token="+e.token(t, "alice"), nil)
	if c != nil {
		c.Close()
	}
	if err == nil || resp == nil || resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("want 502, got resp=%v err=%v", resp, err)
	}
}
