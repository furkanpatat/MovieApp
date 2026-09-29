package server_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/furkanpatat/movieapp/pkg/jwtauth"
	"github.com/furkanpatat/movieapp/services/gateway/internal/ratelimit"
	"github.com/furkanpatat/movieapp/services/gateway/internal/server"
)

const chatBody = `{"messages":[{"role":"user","content":"hi"}]}`

// An upstream that takes 300ms to answer: longer than the normal upstream
// timeout below, shorter than the chat one.
func slowGateway(t *testing.T, chatLimit int) (*httptest.Server, string) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(300 * time.Millisecond):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(slow.Close)
	u, _ := url.Parse(slow.URL)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	auth := jwtauth.NewManager(secret, issuer, time.Hour)
	gw := httptest.NewServer(server.New(server.Deps{
		Catalog: u, Interaction: u, AuthService: u, WatchParty: u, Auth: auth,
		Limiter: ratelimit.New(rdb, 1000, time.Minute), RateLimit: ratelimit.MiddlewareConfig{Log: quiet},
		UpstreamTimeout: 100 * time.Millisecond,
		ChatTimeout:     2 * time.Second,
		ChatLimiter:     ratelimit.New(rdb, chatLimit, time.Minute),
		ChatRateLimit:   ratelimit.MiddlewareConfig{KeyPrefix: "chat:ip:", Log: quiet},
		Log:             quiet,
	}))
	t.Cleanup(gw.Close)
	tok, _, _ := auth.Issue("alice")
	return gw, tok
}

func post(t *testing.T, gw *httptest.Server, path, tok string) int {
	t.Helper()
	req, _ := http.NewRequest("POST", gw.URL+path, strings.NewReader(chatBody))
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestChatGetsALongerUpstreamTimeout(t *testing.T) {
	gw, tok := slowGateway(t, 100)
	if code := post(t, gw, "/api/v1/chat", tok); code != http.StatusOK {
		t.Fatalf("slow chat -> %d, want 200 (chat timeout is longer)", code)
	}
	if code := post(t, gw, "/api/v1/movies/1/rate", tok); code != http.StatusGatewayTimeout {
		t.Fatalf("slow rate -> %d, want 504 (normal timeout still applies)", code)
	}
}

func TestChatHasItsOwnRateLimit(t *testing.T) {
	gw, tok := slowGateway(t, 2)
	for i := range 2 {
		if code := post(t, gw, "/api/v1/chat", tok); code != http.StatusOK {
			t.Fatalf("chat %d -> %d", i+1, code)
		}
	}
	if code := post(t, gw, "/api/v1/chat", tok); code != http.StatusTooManyRequests {
		t.Fatalf("3rd chat -> %d, want 429", code)
	}
}
