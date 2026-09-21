package postgres_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanpatat/movieapp/services/interaction/internal/domain"
	"github.com/furkanpatat/movieapp/services/interaction/internal/repository/postgres"
)

// Needs a database with deployments/postgres/init.sql applied (make db-init):
//
//	TEST_POSTGRES_DSN=postgres://user:pass@localhost:5433/movieapp?sslmode=disable
func setup(t *testing.T) (*postgres.Repo, int) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	movie := 900_000_000 + int(time.Now().UnixNano()%100_000_000) // isolated id per test
	t.Cleanup(func() {
		for _, q := range []string{
			`DELETE FROM interaction.ratings WHERE movie_id=$1`,
			`DELETE FROM interaction.movie_rating_stats WHERE movie_id=$1`,
			`DELETE FROM interaction.comments WHERE movie_id=$1`,
		} {
			_, _ = pool.Exec(ctx, q, movie)
		}
		pool.Close()
	})
	return postgres.New(pool), movie
}

func rate(movie int, user string, score int, at time.Time) domain.RatingSubmitted {
	return domain.RatingSubmitted{EventID: uuid.NewString(), MovieID: movie, UserID: user, Score: score, OccurredAt: at}
}

func TestRatingAggregateRerateAndIdempotency(t *testing.T) {
	repo, movie := setup(t)
	ctx := context.Background()
	t0 := time.Now().UTC().Truncate(time.Microsecond)

	s, err := repo.SaveRating(ctx, rate(movie, "a", 8, t0))
	if err != nil || s.VoteCount != 1 || s.TotalScore != 8 || s.Version != 1 {
		t.Fatalf("%+v %v", s, err)
	}
	s, _ = repo.SaveRating(ctx, rate(movie, "b", 4, t0))
	if s.VoteCount != 2 || s.TotalScore != 12 || s.Average() != 6 {
		t.Fatalf("%+v", s)
	}

	// Re-rate: replaces, does not add a vote.
	s, _ = repo.SaveRating(ctx, rate(movie, "a", 10, t0.Add(time.Second)))
	if s.VoteCount != 2 || s.TotalScore != 14 {
		t.Fatalf("re-rate: %+v", s)
	}
	v := s.Version

	// Same score again (redelivery): nothing changes, version doesn't move.
	s, _ = repo.SaveRating(ctx, rate(movie, "a", 10, t0.Add(2*time.Second)))
	if s.TotalScore != 14 || s.Version != v {
		t.Fatalf("redelivery changed state: %+v", s)
	}

	// Older event arriving late loses.
	s, _ = repo.SaveRating(ctx, rate(movie, "a", 1, t0))
	if s.TotalScore != 14 || s.Version != v {
		t.Fatalf("stale event applied: %+v", s)
	}

	got, _ := repo.GetStats(ctx, movie)
	if got != s {
		t.Fatalf("GetStats %+v != %+v", got, s)
	}
}

func TestConcurrentRatingsKeepAggregateExact(t *testing.T) {
	repo, movie := setup(t)
	ctx := context.Background()
	const n = 40
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.SaveRating(ctx, rate(movie, fmt.Sprint("user", i), 5, time.Now()))
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	s, _ := repo.GetStats(ctx, movie)
	if s.VoteCount != n || s.TotalScore != 5*n || s.Version != n {
		t.Fatalf("lost updates: %+v", s)
	}
}

func TestCommentsIdempotentAndOrdered(t *testing.T) {
	repo, movie := setup(t)
	ctx := context.Background()
	t0 := time.Now().UTC().Truncate(time.Microsecond)
	c1 := domain.CommentAdded{EventID: uuid.NewString(), MovieID: movie, UserID: "a", Text: "one", OccurredAt: t0}
	c2 := domain.CommentAdded{EventID: uuid.NewString(), MovieID: movie, UserID: "b", Text: "two", OccurredAt: t0.Add(time.Second)}
	for _, c := range []domain.CommentAdded{c1, c2, c1, c1} { // c1 redelivered
		if err := repo.SaveComment(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.RecentComments(ctx, movie, 10)
	if err != nil || len(got) != 2 || got[0].Text != "two" || got[1].ID != c1.EventID {
		t.Fatalf("%+v %v", got, err)
	}
	if !got[1].CreatedAt.Equal(t0) {
		t.Fatalf("timestamp round trip: %v vs %v", got[1].CreatedAt, t0)
	}
	if lim, _ := repo.RecentComments(ctx, movie, 1); len(lim) != 1 {
		t.Fatal("limit ignored")
	}
	if empty, _ := repo.RecentComments(ctx, movie+1, 5); empty == nil || len(empty) != 0 {
		t.Fatal("unknown movie should give an empty, non-nil slice")
	}
}

func TestStatsForUnknownMovie(t *testing.T) {
	repo, movie := setup(t)
	s, err := repo.GetStats(context.Background(), movie)
	if err != nil || s.VoteCount != 0 || s.MovieID != movie {
		t.Fatalf("%+v %v", s, err)
	}
}
