package transport_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/furkanpatat/movieapp/pkg/jwtauth"
	"github.com/furkanpatat/movieapp/services/auth/internal/domain"
	"github.com/furkanpatat/movieapp/services/auth/internal/password"
	"github.com/furkanpatat/movieapp/services/auth/internal/service"
	"github.com/furkanpatat/movieapp/services/auth/internal/testsupport"
	"github.com/furkanpatat/movieapp/services/auth/internal/transport"
)

const secret = "shared-secret-shared-secret-shared-secret-01"

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

type memRepo struct{ users []domain.User }

func (r *memRepo) Create(_ context.Context, u domain.User) (domain.User, error) {
	for _, x := range r.users {
		if strings.EqualFold(x.Username, u.Username) || strings.EqualFold(x.Email, u.Email) {
			return domain.User{}, domain.ErrConflict
		}
	}
	u.ID, u.CreatedAt = "11111111-2222-3333-4444-555555555555", time.Now()
	r.users = append(r.users, u)
	return u, nil
}
func (r *memRepo) FindByLogin(_ context.Context, l string) (domain.User, error) {
	for _, x := range r.users {
		if strings.EqualFold(x.Username, l) || strings.EqualFold(x.Email, l) {
			return x, nil
		}
	}
	return domain.User{}, domain.ErrNotFound
}

func (r *memRepo) Delete(_ context.Context, id string) error {
	for i, x := range r.users {
		if x.ID == id {
			r.users = append(r.users[:i], r.users[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *memRepo) FindByID(_ context.Context, id string) (domain.User, error) {
	for _, x := range r.users {
		if x.ID == id {
			return x, nil
		}
	}
	return domain.User{}, domain.ErrNotFound
}

func server(t *testing.T) http.Handler {
	b, _ := password.NewBcrypt(bcrypt.MinCost)
	svc, err := service.New(&memRepo{}, b, jwtauth.NewManager(secret, "movieapp-auth", time.Hour), quiet,
		service.WithRefresh(testsupport.NewMemRefresh(), time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return transport.NewHandler(svc, nil, quiet, transport.CookieOptions{Secure: true})
}

func post(h http.Handler, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", path, strings.NewReader(body)))
	return rec
}

const registerBody = `{"username":"alice","email":"alice@example.com","password":"s3cret-password"}`

func TestRegisterThenLoginReturnsAWorkingToken(t *testing.T) {
	h := server(t)
	rec := post(h, "/api/v1/auth/register", registerBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register -> %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "password") || strings.Contains(rec.Body.String(), "$2") {
		t.Fatalf("register response leaks credentials: %s", rec.Body)
	}
	var reg struct{ ID string }
	_ = json.Unmarshal(rec.Body.Bytes(), &reg)

	rec = post(h, "/api/v1/auth/login", `{"login":"alice","password":"s3cret-password"}`)
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("login -> %d %s (cache-control %q)", rec.Code, rec.Body, rec.Header().Get("Cache-Control"))
	}
	var out struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
		User        struct{ ID, Username, Email string }
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.TokenType != "Bearer" || out.ExpiresIn < 3500 {
		t.Fatalf("%+v", out)
	}
	if out.User.Username != "alice" || out.User.Email != "alice@example.com" {
		t.Fatalf("login should return the profile the web app displays: %+v", out.User)
	}
	sub, err := jwtauth.NewManager(secret, "movieapp-auth", time.Hour).Verify(out.AccessToken)
	if err != nil || sub != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("token not accepted by the gateway's verifier: %q %v", sub, err)
	}
}

func TestInvalidCredentialsReturn401(t *testing.T) {
	h := server(t)
	post(h, "/api/v1/auth/register", registerBody)

	var bodies []string
	for _, b := range []string{
		`{"login":"alice","password":"wrong-password"}`,              // wrong password
		`{"login":"nobody","password":"s3cret-password"}`,            // unknown user
		`{"login":"alice@example.com","password":"S3CRET-PASSWORD"}`, // right user, wrong case
	} {
		rec := post(h, "/api/v1/auth/login", b)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s -> %d, want 401", b, rec.Code)
		}
		if rec.Header().Get("WWW-Authenticate") == "" {
			t.Errorf("%s: missing WWW-Authenticate", b)
		}
		if strings.Contains(rec.Body.String(), "access_token") {
			t.Errorf("%s: a token was issued", b)
		}
		bodies = append(bodies, rec.Body.String())
	}
	if bodies[0] != bodies[1] {
		t.Fatalf("responses differ, allowing user enumeration:\n%s\n%s", bodies[0], bodies[1])
	}
}

func TestBadRequests(t *testing.T) {
	h := server(t)
	for name, c := range map[string]struct{ path, body string }{
		"register short password": {"/api/v1/auth/register", `{"username":"alice","email":"a@example.com","password":"short"}`},
		"register bad email":      {"/api/v1/auth/register", `{"username":"alice","email":"nope","password":"password1"}`},
		"register bad username":   {"/api/v1/auth/register", `{"username":"a b","email":"a@example.com","password":"password1"}`},
		"register unknown field":  {"/api/v1/auth/register", `{"username":"alice","email":"a@example.com","password":"password1","admin":true}`},
		"register not json":       {"/api/v1/auth/register", `nope`},
		"register trailing":       {"/api/v1/auth/register", registerBody + " x"},
		"login missing password":  {"/api/v1/auth/login", `{"login":"alice"}`},
		"login missing login":     {"/api/v1/auth/login", `{"password":"x"}`},
		"login unknown field":     {"/api/v1/auth/login", `{"login":"a","password":"b","remember":true}`},
		"login oversized":         {"/api/v1/auth/login", `{"login":"` + strings.Repeat("a", 20000) + `","password":"x"}`},
	} {
		if rec := post(h, c.path, c.body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s -> %d, want 400", name, rec.Code)
		}
	}
}

func TestDuplicateRegistrationIs409(t *testing.T) {
	h := server(t)
	post(h, "/api/v1/auth/register", registerBody)
	if rec := post(h, "/api/v1/auth/register", `{"username":"ALICE","email":"new@example.com","password":"password1"}`); rec.Code != http.StatusConflict {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestLoginSetsHardenedSessionCookie(t *testing.T) {
	h := server(t)
	post(h, "/api/v1/auth/register", registerBody)
	rec := post(h, "/api/v1/auth/login", `{"login":"alice","password":"s3cret-password"}`)
	if rec.Code != 200 {
		t.Fatalf("login -> %d %s", rec.Code, rec.Body)
	}

	var session *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == jwtauth.CookieName {
			session = c
		}
	}
	if session == nil {
		t.Fatalf("no %s cookie in %q", jwtauth.CookieName, rec.Header().Values("Set-Cookie"))
	}
	if !session.HttpOnly || !session.Secure || session.SameSite != http.SameSiteStrictMode || session.Path != jwtauth.CookiePath {
		t.Fatalf("cookie not hardened: %+v", session)
	}
	if session.MaxAge < 3500 {
		t.Fatalf("cookie should live as long as the token, MaxAge=%d", session.MaxAge)
	}
	if _, err := jwtauth.NewManager(secret, "movieapp-auth", time.Hour).Verify(session.Value); err != nil {
		t.Fatalf("cookie value is not a valid token: %v", err)
	}
}

func TestFailedLoginSetsNoCookie(t *testing.T) {
	h := server(t)
	post(h, "/api/v1/auth/register", registerBody)
	rec := post(h, "/api/v1/auth/login", `{"login":"alice","password":"wrong-password"}`)
	if rec.Code != http.StatusUnauthorized || len(rec.Result().Cookies()) != 0 {
		t.Fatalf("-> %d, cookies %q", rec.Code, rec.Header().Values("Set-Cookie"))
	}
}

func TestLogoutClearsSessionCookie(t *testing.T) {
	rec := post(server(t), "/api/v1/auth/logout", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout -> %d", rec.Code)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != jwtauth.CookieName || cookies[0].MaxAge >= 0 || cookies[0].Path != jwtauth.CookiePath {
		t.Fatalf("expected a deleting %s cookie, got %q", jwtauth.CookieName, rec.Header().Values("Set-Cookie"))
	}
}

func session(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestLoginGivesARefreshTokenOnlyWhenAsked(t *testing.T) {
	h := server(t)
	post(h, "/api/v1/auth/register", registerBody)
	plain := session(t, post(h, "/api/v1/auth/login", `{"login":"alice","password":"s3cret-password"}`))
	if _, ok := plain["refresh_token"]; ok {
		t.Fatal("a browser login must not get a refresh token")
	}
	withRefresh := session(t, post(h, "/api/v1/auth/login", `{"login":"alice","password":"s3cret-password","refresh":true}`))
	if tok, _ := withRefresh["refresh_token"].(string); len(tok) < 40 {
		t.Fatalf("refresh_token: %v", withRefresh["refresh_token"])
	}
	if exp, _ := withRefresh["refresh_expires_in"].(float64); exp <= 0 {
		t.Fatalf("refresh_expires_in: %v", withRefresh["refresh_expires_in"])
	}
}

func TestRefreshEndpointRotatesAndRejectsSpentTokens(t *testing.T) {
	h := server(t)
	post(h, "/api/v1/auth/register", registerBody)
	login := session(t, post(h, "/api/v1/auth/login", `{"login":"alice","password":"s3cret-password","refresh":true}`))
	first := login["refresh_token"].(string)

	rec := post(h, "/api/v1/auth/refresh", `{"refresh_token":"`+first+`"}`)
	next := session(t, rec)
	if next["access_token"] == "" || next["refresh_token"] == first || next["user"].(map[string]any)["username"] != "alice" {
		t.Fatalf("refresh body: %v", next)
	}
	if rec.Header().Get("Set-Cookie") != "" {
		t.Fatal("refresh must not touch the browser cookie")
	}
	if rec := post(h, "/api/v1/auth/refresh", `{"refresh_token":"`+first+`"}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("spent token: status %d", rec.Code)
	}
	if rec := post(h, "/api/v1/auth/refresh", `{"refresh_token":"garbage"}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("garbage: status %d", rec.Code)
	}
}

func TestLogoutRevokesTheRefreshToken(t *testing.T) {
	h := server(t)
	post(h, "/api/v1/auth/register", registerBody)
	tok := session(t, post(h, "/api/v1/auth/login", `{"login":"alice","password":"s3cret-password","refresh":true}`))["refresh_token"].(string)
	if rec := post(h, "/api/v1/auth/logout", `{"refresh_token":"`+tok+`"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", rec.Code)
	}
	if rec := post(h, "/api/v1/auth/refresh", `{"refresh_token":"`+tok+`"}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("after logout: status %d", rec.Code)
	}
}

func do(h http.Handler, method, path, body, userID string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if userID != "" {
		req.Header.Set("X-User-Id", userID)
	}
	h.ServeHTTP(rec, req)
	return rec
}

func TestAccountEndpointsNeedTheGatewaysUserID(t *testing.T) {
	h := server(t)
	post(h, "/api/v1/auth/register", registerBody)
	if rec := do(h, "POST", "/api/v1/auth/password/verify", `{"password":"s3cret-password"}`, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("verify without X-User-Id: %d", rec.Code)
	}
	if rec := do(h, "DELETE", "/api/v1/auth/account", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("delete without X-User-Id: %d", rec.Code)
	}
}

func TestDeleteAccount(t *testing.T) {
	h := server(t)
	post(h, "/api/v1/auth/register", registerBody)
	userID := session(t, post(h, "/api/v1/auth/login", `{"login":"alice","password":"s3cret-password"}`))["user"].(map[string]any)["id"].(string)
	if rec := do(h, "POST", "/api/v1/auth/password/verify", `{"password":"wrong-password"}`, userID); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d", rec.Code)
	}
	if rec := do(h, "POST", "/api/v1/auth/password/verify", `{"password":"s3cret-password"}`, userID); rec.Code != http.StatusNoContent {
		t.Fatalf("right password: %d %s", rec.Code, rec.Body)
	}
	rec := do(h, "DELETE", "/api/v1/auth/account", "", userID)
	if rec.Code != http.StatusNoContent || !strings.Contains(rec.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("delete: %d, cookie %q", rec.Code, rec.Header().Get("Set-Cookie"))
	}
	if rec := post(h, "/api/v1/auth/login", `{"login":"alice","password":"s3cret-password"}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("login after deletion: %d", rec.Code)
	}
	if rec := do(h, "DELETE", "/api/v1/auth/account", "", userID); rec.Code != http.StatusNoContent {
		t.Fatalf("a retried delete must succeed: %d", rec.Code)
	}
}
