package postgres_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanpatat/movieapp/services/auth/internal/domain"
	"github.com/furkanpatat/movieapp/services/auth/internal/repository/postgres"
	"github.com/furkanpatat/movieapp/services/auth/internal/testsupport"
)

func newRepo(t *testing.T) (*postgres.Repo, string) {
	pool, schema := testsupport.NewSchema(t)
	return postgres.NewInSchema(pool, schema), schema
}

func TestCreateAndFind(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	u, err := repo.Create(ctx, domain.User{Username: "Alice", Email: "alice@example.com", PasswordHash: "$2a$04$hash"})
	if err != nil || u.ID == "" || u.CreatedAt.IsZero() {
		t.Fatalf("%+v %v", u, err)
	}
	for _, login := range []string{"Alice", "alice", "ALICE", "alice@example.com", "ALICE@EXAMPLE.COM"} {
		got, err := repo.FindByLogin(ctx, login)
		if err != nil || got.ID != u.ID || got.PasswordHash != "$2a$04$hash" || got.Username != "Alice" {
			t.Fatalf("find %q: %+v %v", login, got, err)
		}
	}
	if _, err := repo.FindByLogin(ctx, "nobody"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestUniquenessIsCaseInsensitive(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	_, _ = repo.Create(ctx, domain.User{Username: "alice", Email: "alice@example.com", PasswordHash: "h"})
	for name, u := range map[string]domain.User{
		"username":            {Username: "alice", Email: "x1@example.com"},
		"username other case": {Username: "ALICE", Email: "x2@example.com"},
		"email":               {Username: "user3", Email: "alice@example.com"},
		"email other case":    {Username: "user4", Email: "Alice@Example.COM"},
	} {
		u.PasswordHash = "h"
		if _, err := repo.Create(ctx, u); !errors.Is(err, domain.ErrConflict) {
			t.Errorf("%s: got %v, want ErrConflict", name, err)
		}
	}
}

func TestConcurrentDuplicateRegistrationsCreateExactlyOne(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	var mu sync.Mutex
	var ok, conflict int
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.Create(ctx, domain.User{Username: "race", Email: "race@example.com", PasswordHash: "h"})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errors.Is(err, domain.ErrConflict):
				conflict++
			default:
				t.Errorf("unexpected: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok != 1 || conflict != 19 {
		t.Fatalf("ok=%d conflict=%d, want exactly one winner", ok, conflict)
	}
}

func TestSchemaRejectsBadData(t *testing.T) {
	repo, _ := newRepo(t)
	if _, err := repo.Create(context.Background(), domain.User{Username: "ab", Email: "a@example.com", PasswordHash: "h"}); err == nil || errors.Is(err, domain.ErrConflict) {
		t.Fatalf("too-short username should violate the CHECK constraint, got %v", err)
	}
	if _, err := repo.Create(context.Background(), domain.User{Username: "okname", Email: strings.Repeat("a", 250) + "@x.io", PasswordHash: "h"}); err == nil {
		t.Fatal("over-long email accepted")
	}
}

// --- Refresh tokens ---------------------------------------------------------

func hash(b byte) []byte { return []byte(strings.Repeat(string(rune(b)), 32)) }

const family = "7f2c1d3e-0b4a-4c5d-8e9f-a1b2c3d4e5f6"

func newUser(t *testing.T, repo *postgres.Repo) string {
	t.Helper()
	u, err := repo.Create(context.Background(), domain.User{Username: "alice", Email: "alice@example.com", PasswordHash: "$2a$04$hash"})
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func TestRefreshRotationAndReuse(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	userID := newUser(t, repo)
	exp := time.Now().Add(time.Hour)
	if err := repo.SaveRefresh(ctx, userID, family, hash('a'), exp); err != nil {
		t.Fatal(err)
	}
	got, err := repo.RotateRefresh(ctx, hash('a'), hash('b'), exp)
	if err != nil || got != userID {
		t.Fatalf("rotate: %q %v", got, err)
	}
	// 'a' again: a replay. The family (including 'b') is revoked.
	if _, err := repo.RotateRefresh(ctx, hash('a'), hash('c'), exp); !errors.Is(err, domain.ErrTokenReused) {
		t.Fatalf("replay: got %v", err)
	}
	if _, err := repo.RotateRefresh(ctx, hash('b'), hash('d'), exp); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("after replay: got %v", err)
	}
	if _, err := repo.RotateRefresh(ctx, hash('z'), hash('y'), exp); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("unknown: got %v", err)
	}
	if u, err := repo.FindByID(ctx, userID); err != nil || u.Username != "alice" {
		t.Fatalf("find by id: %+v %v", u, err)
	}
}

func TestExpiredAndRevokedRefreshTokens(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	userID := newUser(t, repo)
	if err := repo.SaveRefresh(ctx, userID, family, hash('e'), time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RotateRefresh(ctx, hash('e'), hash('f'), time.Now().Add(time.Hour)); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("expired: got %v", err)
	}
	other := "0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d"
	if err := repo.SaveRefresh(ctx, userID, other, hash('g'), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := repo.RevokeRefresh(ctx, hash('g')); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RotateRefresh(ctx, hash('g'), hash('h'), time.Now().Add(time.Hour)); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("revoked: got %v", err)
	}
}

// Ten requests race with the same token: the row lock lets exactly one
// rotate; the rest see a spent token (and revoke the family).
func TestConcurrentRefreshesRotateExactlyOnce(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	userID := newUser(t, repo)
	exp := time.Now().Add(time.Hour)
	if err := repo.SaveRefresh(ctx, userID, family, hash('r'), exp); err != nil {
		t.Fatal(err)
	}
	var (
		wg sync.WaitGroup
		mu sync.Mutex
		ok int
	)
	for i := range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := repo.RotateRefresh(ctx, hash('r'), hash(byte('A'+i)), exp); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Fatalf("%d rotations succeeded, want exactly 1", ok)
	}
}

func TestDeleteCascadesToRefreshTokens(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	userID := newUser(t, repo)
	if err := repo.SaveRefresh(ctx, userID, family, hash('x'), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FindByID(ctx, userID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("user still there: %v", err)
	}
	if _, err := repo.RotateRefresh(ctx, hash('x'), hash('y'), time.Now().Add(time.Hour)); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("refresh token outlived its user: %v", err)
	}
	if err := repo.Delete(ctx, userID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}
