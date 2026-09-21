// Package integration holds black-box tests against a running stack.
//
//	make up && GATEWAY_URL=http://localhost:8000 go test -count=1 -v ./tests/integration
//
// They use only the public gateway API, so they prove the real wiring:
// Auth service -> JWT -> Gateway verification -> Interaction service.
package integration

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func gatewayURL(t *testing.T) string {
	u := os.Getenv("GATEWAY_URL")
	if u == "" {
		t.Skip("GATEWAY_URL not set")
	}
	return strings.TrimRight(u, "/")
}

type resp struct {
	code   int
	body   string
	header http.Header
}

func call(t *testing.T, method, url, token, body string) resp {
	t.Helper()
	req, _ := http.NewRequest(method, url, bytes.NewBufferString(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	r, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return resp{r.StatusCode, string(b), r.Header}
}

func field(t *testing.T, body, key string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("not JSON: %s", body)
	}
	v, _ := m[key].(string)
	return v
}

func jwtSubject(t *testing.T, token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("not a JWT: %q", token)
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var c struct{ Sub string }
	_ = json.Unmarshal(b, &c)
	return c.Sub
}

func TestRegisterLoginAndUseTokenThroughTheGateway(t *testing.T) {
	gw := gatewayURL(t)
	suffix := fmt.Sprint(time.Now().UnixNano())
	username, email, pw := "e2e_"+suffix[len(suffix)-10:], "e2e"+suffix+"@e2e.test", "correct-horse-battery"
	movie := 990000 + int(time.Now().UnixNano()%9000)

	// --- register
	r := call(t, "POST", gw+"/api/v1/auth/register", "", fmt.Sprintf(`{"username":%q,"email":%q,"password":%q}`, username, email, pw))
	if r.code != 201 {
		t.Fatalf("register -> %d %s", r.code, r.body)
	}
	userID := field(t, r.body, "id")
	if userID == "" || strings.Contains(r.body, pw) || strings.Contains(r.body, "$2") {
		t.Fatalf("bad register response: %s", r.body)
	}
	if r := call(t, "POST", gw+"/api/v1/auth/register", "", fmt.Sprintf(`{"username":%q,"email":"other%s@e2e.test","password":%q}`, strings.ToUpper(username), suffix, pw)); r.code != 409 {
		t.Fatalf("duplicate username (other case) -> %d, want 409", r.code)
	}

	// --- invalid credentials are 401 and indistinguishable
	wrong := call(t, "POST", gw+"/api/v1/auth/login", "", fmt.Sprintf(`{"login":%q,"password":"not-the-password"}`, username))
	unknown := call(t, "POST", gw+"/api/v1/auth/login", "", `{"login":"nobody_e2e_unknown","password":"not-the-password"}`)
	if wrong.code != 401 || unknown.code != 401 {
		t.Fatalf("wrong password -> %d, unknown user -> %d, want 401/401", wrong.code, unknown.code)
	}
	if wrong.body != unknown.body {
		t.Fatalf("responses differ (user enumeration):\n%s\n%s", wrong.body, unknown.body)
	}

	// --- valid login (by email this time) -> a token the gateway accepts
	r = call(t, "POST", gw+"/api/v1/auth/login", "", fmt.Sprintf(`{"login":%q,"password":%q}`, email, pw))
	if r.code != 200 {
		t.Fatalf("login -> %d %s", r.code, r.body)
	}
	token := field(t, r.body, "access_token")
	if sub := jwtSubject(t, token); sub != userID {
		t.Fatalf("token subject %q, want the registered user id %q", sub, userID)
	}

	// --- the gateway rejects requests without / with damaged tokens
	rate := gw + fmt.Sprintf("/api/v1/movies/%d/rate", movie)
	if c := call(t, "POST", rate, "", `{"score":8}`).code; c != 401 {
		t.Fatalf("no token -> %d, want 401", c)
	}
	if c := call(t, "POST", rate, token[:len(token)-3]+"AAA", `{"score":8}`).code; c != 401 {
		t.Fatalf("tampered token -> %d, want 401", c)
	}

	// --- ...and accepts the real one, all the way to the interaction service
	if c := call(t, "POST", rate, token, `{"score":8}`).code; c != 202 {
		t.Fatalf("valid token rejected by the gateway: %d", c)
	}
	if c := call(t, "POST", gw+fmt.Sprintf("/api/v1/movies/%d/comment", movie), token, `{"text":"hello from e2e"}`).code; c != 202 {
		t.Fatalf("comment -> %d", c)
	}

	// The event must carry the identity from the token (the Auth service's user id).
	deadline := time.Now().Add(20 * time.Second)
	for {
		g := call(t, "GET", gw+fmt.Sprintf("/api/v1/movies/%d/interactions", movie), "", "")
		if strings.Contains(g.body, `"total_votes":1`) && strings.Contains(g.body, `"user_id":"`+userID+`"`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("interaction never showed a vote and comment by %s: %s", userID, g.body)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// Runs last on purpose: it exhausts the per-IP budget of the credential
// endpoints, so re-running the suite needs a minute in between.
func TestLoginBruteForceIsThrottled(t *testing.T) {
	gw := gatewayURL(t)
	if os.Getenv("E2E_BRUTE_FORCE") == "" {
		t.Skip("set E2E_BRUTE_FORCE=1 (it exhausts the auth rate limit for the next minute)")
	}
	var got429 bool
	for i := 0; i < 40 && !got429; i++ {
		r := call(t, "POST", gw+"/api/v1/auth/login", "", `{"login":"victim","password":"guess`+fmt.Sprint(i)+`"}`)
		switch r.code {
		case 401:
		case 429:
			got429 = true
			if r.header.Get("Retry-After") == "" {
				t.Fatal("429 without Retry-After")
			}
		default:
			t.Fatalf("attempt %d -> %d", i, r.code)
		}
	}
	if !got429 {
		t.Fatal("40 password guesses were never throttled")
	}
}
