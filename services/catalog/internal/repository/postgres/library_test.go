package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
	"github.com/furkanpatat/movieapp/services/catalog/internal/repository/postgres"
)

// Integration tests against a database with deployments/postgres/init.sql
// applied. They use fresh random users and movie ids (far above TMDB's) and
// delete them afterwards. Skipped unless TEST_POSTGRES_DSN is set, e.g.
//
//	TEST_POSTGRES_DSN=postgres://user:pass@localhost:5433/movieapp?sslmode=disable
type fixture struct {
	pool *pgxpool.Pool
	repo *postgres.Repository
}

func setup(t *testing.T) fixture {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return fixture{pool: pool, repo: postgres.New(pool)}
}

func (f fixture) user(t *testing.T) string {
	t.Helper()
	name := fmt.Sprintf("itest_%d", rand.Int64N(1<<40))
	var id string
	err := f.pool.QueryRow(context.Background(),
		`INSERT INTO auth.users (username, email, password_hash) VALUES ($1, $1 || '@example.test', 'x') RETURNING id`,
		name).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = f.pool.Exec(context.Background(), `DELETE FROM auth.users WHERE id = $1`, id) })
	return id
}

// movie stores a movie; cleanups run LIFO, so it is deleted after the users
// (and with them the library rows) created later in the test.
func (f fixture) movie(t *testing.T, title string) int {
	t.Helper()
	id := 900_000_000 + rand.IntN(99_999_999)
	m := domain.Movie{ID: id, Title: title, Overview: "o", PosterPath: "/p.jpg", ReleaseDate: "2024-01-02", VoteAverage: 7.5, VoteCount: 12}
	if err := f.repo.UpsertMovie(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = f.pool.Exec(context.Background(), `DELETE FROM movies WHERE id = $1`, id) })
	return id
}

func TestWatchlist(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	m1, m2 := f.movie(t, "First"), f.movie(t, "Second")
	alice, bob := f.user(t), f.user(t)

	first, err := f.repo.AddToWatchlist(ctx, alice, m1)
	if err != nil {
		t.Fatal(err)
	}
	if first.Movie.Title != "First" || first.Movie.PosterPath != "/p.jpg" || first.Movie.VoteCount != 12 || first.AddedAt.IsZero() {
		t.Fatalf("item = %+v", first)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := f.repo.AddToWatchlist(ctx, alice, m2); err != nil {
		t.Fatal(err)
	}
	// Re-adding is a no-op that keeps the original time.
	again, err := f.repo.AddToWatchlist(ctx, alice, m1)
	if err != nil || !again.AddedAt.Equal(first.AddedAt) {
		t.Fatalf("re-add: %v, added_at %v -> %v", err, first.AddedAt, again.AddedAt)
	}

	list, err := f.repo.GetUserWatchlist(ctx, alice)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Movie.ID != m2 || list[1].Movie.ID != m1 {
		t.Fatalf("want [m2 m1] (newest first), got %+v", list)
	}
	if other, _ := f.repo.GetUserWatchlist(ctx, bob); len(other) != 0 {
		t.Fatalf("bob sees alice's list: %+v", other)
	}

	for range 2 {
		if err := f.repo.RemoveFromWatchlist(ctx, alice, m1); err != nil {
			t.Fatal(err)
		}
	}
	if list, _ := f.repo.GetUserWatchlist(ctx, alice); len(list) != 1 || list[0].Movie.ID != m2 {
		t.Fatalf("after remove: %+v", list)
	}
}

func TestUpsertUserRating(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	m := f.movie(t, "Rated")
	alice := f.user(t)

	r1, err := f.repo.UpsertUserRating(ctx, alice, m, 6)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	r2, err := f.repo.UpsertUserRating(ctx, alice, m, 9)
	if err != nil {
		t.Fatal(err)
	}
	if r2.Rating != 9 || !r2.CreatedAt.Equal(r1.CreatedAt) || !r2.UpdatedAt.After(r1.UpdatedAt) || r2.Movie.Title != "Rated" {
		t.Fatalf("re-rate: %+v then %+v", r1, r2)
	}

	all, err := f.repo.GetUserRatings(ctx, alice)
	if err != nil || len(all) != 1 || all[0].Rating != 9 {
		t.Fatalf("ratings = %+v, %v (want one row, rating 9)", all, err)
	}

	if _, err := f.repo.UpsertUserRating(ctx, alice, m, 11); err == nil {
		t.Fatal("rating 11 accepted; the CHECK constraint should reject it")
	}
}

func TestLibraryForeignKeys(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	alice := f.user(t)
	stored := f.movie(t, "Stored")
	unstored := 900_000_000 + rand.IntN(99_999_999)
	nobody := "00000000-0000-4000-8000-000000000000"

	if _, err := f.repo.AddToWatchlist(ctx, alice, unstored); !errors.Is(err, domain.ErrMovieNotStored) {
		t.Errorf("watchlist, unstored movie: %v", err)
	}
	if _, err := f.repo.UpsertUserRating(ctx, alice, unstored, 5); !errors.Is(err, domain.ErrMovieNotStored) {
		t.Errorf("rating, unstored movie: %v", err)
	}
	if _, err := f.repo.AddToWatchlist(ctx, nobody, stored); !errors.Is(err, domain.ErrUnknownUser) {
		t.Errorf("watchlist, unknown user: %v", err)
	}
	if _, err := f.repo.UpsertUserRating(ctx, nobody, stored, 5); !errors.Is(err, domain.ErrUnknownUser) {
		t.Errorf("rating, unknown user: %v", err)
	}

	// Deleting the account deletes its library.
	if _, err := f.repo.AddToWatchlist(ctx, alice, stored); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM auth.users WHERE id = $1`, alice); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM library.watchlists WHERE user_id = $1`, alice).Scan(&n)
	if n != 0 {
		t.Fatalf("%d watchlist rows survived account deletion", n)
	}
}
