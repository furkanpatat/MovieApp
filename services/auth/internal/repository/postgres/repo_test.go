package postgres_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

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
