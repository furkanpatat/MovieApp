package service_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/furkanpatat/movieapp/pkg/jwtauth"
	"github.com/furkanpatat/movieapp/services/auth/internal/domain"
	"github.com/furkanpatat/movieapp/services/auth/internal/password"
	"github.com/furkanpatat/movieapp/services/auth/internal/service"
)

const (
	secret = "shared-secret-shared-secret-shared-secret-01"
	issuer = "movieapp-auth"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// memRepo mirrors the Postgres semantics: case-insensitive unique username/email.
type memRepo struct {
	mu    sync.Mutex
	users []domain.User
	n     int
}

func (r *memRepo) Create(_ context.Context, u domain.User) (domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, x := range r.users {
		if strings.EqualFold(x.Username, u.Username) || strings.EqualFold(x.Email, u.Email) {
			return domain.User{}, domain.ErrConflict
		}
	}
	r.n++
	u.ID = fmt.Sprintf("00000000-0000-0000-0000-%012d", r.n)
	u.CreatedAt = time.Now()
	r.users = append(r.users, u)
	return u, nil
}

func (r *memRepo) FindByLogin(_ context.Context, login string) (domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, x := range r.users {
		if strings.EqualFold(x.Username, login) || strings.EqualFold(x.Email, login) {
			return x, nil
		}
	}
	return domain.User{}, domain.ErrNotFound
}

// countingHasher wraps bcrypt and counts Compare calls.
type countingHasher struct {
	*password.Bcrypt
	compares atomic.Int32
}

func (c *countingHasher) Compare(hash, pw string) error {
	c.compares.Add(1)
	return c.Bcrypt.Compare(hash, pw)
}

func newSvc(t *testing.T) (*service.Auth, *memRepo, *countingHasher) {
	t.Helper()
	b, _ := password.NewBcrypt(bcrypt.MinCost)
	h := &countingHasher{Bcrypt: b}
	repo := &memRepo{}
	svc, err := service.New(repo, h, jwtauth.NewManager(secret, issuer, time.Hour), quiet)
	if err != nil {
		t.Fatal(err)
	}
	h.compares.Store(0) // ignore the constructor's dummy hash
	return svc, repo, h
}

func TestRegisterStoresOnlyAHashOfThePassword(t *testing.T) {
	svc, repo, _ := newSvc(t)
	u, err := svc.Register(context.Background(), "alice", "Alice@Example.com", "s3cret-password")
	if err != nil || u.ID == "" {
		t.Fatalf("%+v %v", u, err)
	}
	stored := repo.users[0]
	if stored.PasswordHash == "" || strings.Contains(stored.PasswordHash, "s3cret") {
		t.Fatalf("password stored unsafely: %q", stored.PasswordHash)
	}
	if bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte("s3cret-password")) != nil {
		t.Fatal("stored hash does not verify against the password")
	}
	if stored.Email != "alice@example.com" {
		t.Fatalf("email should be normalised, got %q", stored.Email)
	}
}

func TestRegisterRejectsInvalidAndDuplicate(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ctx := context.Background()
	if _, err := svc.Register(ctx, "al", "a@example.com", "password1"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("got %v", err)
	}
	if len(repo.users) != 0 {
		t.Fatal("invalid registration was stored")
	}
	if _, err := svc.Register(ctx, "alice", "alice@example.com", "password1"); err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string][2]string{
		"same username":        {"alice", "other@example.com"},
		"username, other case": {"ALICE", "other@example.com"},
		"same email":           {"other", "alice@example.com"},
		"email, other case":    {"other", "ALICE@EXAMPLE.COM"},
	} {
		if _, err := svc.Register(ctx, args[0], args[1], "password1"); !errors.Is(err, domain.ErrConflict) {
			t.Errorf("%s: got %v, want ErrConflict", name, err)
		}
	}
}

func TestLoginSucceedsWithUsernameOrEmailAndTokenCarriesUserID(t *testing.T) {
	svc, _, _ := newSvc(t)
	ctx := context.Background()
	u, _ := svc.Register(ctx, "alice", "alice@example.com", "s3cret-password")

	for _, login := range []string{"alice", "ALICE", "alice@example.com", "  Alice@Example.com "} {
		res, err := svc.Login(ctx, login, "s3cret-password")
		if err != nil {
			t.Fatalf("login as %q: %v", login, err)
		}
		if res.User.ID != u.ID || res.Token == "" || !res.Expires.After(time.Now().Add(55*time.Minute)) {
			t.Fatalf("%q: %+v", login, res)
		}
	}
}

// The gateway verifies with pkg/jwtauth using the shared secret and issuer
// (see services/gateway/cmd/gateway/main.go). Verifying the minted token with
// that same code is the compatibility contract between the two services.
func TestIssuedTokenVerifiesWithTheGatewaysVerifier(t *testing.T) {
	svc, _, _ := newSvc(t)
	ctx := context.Background()
	u, _ := svc.Register(ctx, "alice", "alice@example.com", "s3cret-password")
	res, err := svc.Login(ctx, "alice", "s3cret-password")
	if err != nil {
		t.Fatal(err)
	}

	gateway := jwtauth.NewManager(secret, issuer, time.Hour)
	if sub, err := gateway.Verify(res.Token); err != nil || sub != u.ID {
		t.Fatalf("gateway rejected the token or read the wrong identity: sub=%q err=%v (want %q)", sub, err, u.ID)
	}
	if _, err := jwtauth.NewManager("a-different-secret-a-different-secret-xx", issuer, time.Hour).Verify(res.Token); err == nil {
		t.Fatal("a gateway with another secret must reject the token")
	}
	if _, err := jwtauth.NewManager(secret, "another-issuer", time.Hour).Verify(res.Token); err == nil {
		t.Fatal("a gateway expecting another issuer must reject the token")
	}
}

func TestInvalidCredentialsAreIndistinguishable(t *testing.T) {
	svc, _, h := newSvc(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, "alice", "alice@example.com", "s3cret-password")

	_, errWrong := svc.Login(ctx, "alice", "wrong-password")
	before := h.compares.Load()
	_, errUnknown := svc.Login(ctx, "nobody", "wrong-password")

	if !errors.Is(errWrong, domain.ErrInvalidCredentials) || !errors.Is(errUnknown, domain.ErrInvalidCredentials) {
		t.Fatalf("wrong=%v unknown=%v, want ErrInvalidCredentials for both", errWrong, errUnknown)
	}
	if errWrong.Error() != errUnknown.Error() {
		t.Fatalf("errors differ (%q vs %q): user enumeration", errWrong, errUnknown)
	}
	if h.compares.Load() != before+1 {
		t.Fatal("an unknown login must still run a bcrypt comparison (timing equalisation)")
	}
	// Long passwords can't match (bcrypt would truncate) and must fail cleanly.
	if _, err := svc.Login(ctx, "alice", strings.Repeat("x", 200)); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("got %v", err)
	}
	// Missing fields are a client error, not a credentials failure.
	for _, args := range [][2]string{{"", "pw"}, {"alice", ""}, {"  ", "pw"}} {
		if _, err := svc.Login(ctx, args[0], args[1]); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%v: got %v, want ErrInvalidInput", args, err)
		}
	}
}
