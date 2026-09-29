package postgres_test

import (
	"context"
	"errors"
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

	got, _ := repo.GetStats(ctx, domain.Movie(movie))
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
	s, _ := repo.GetStats(ctx, domain.Movie(movie))
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
	got, err := repo.RecentComments(ctx, domain.Movie(movie), 10)
	if err != nil || len(got) != 2 || got[0].Text != "two" || got[1].ID != c1.EventID {
		t.Fatalf("%+v %v", got, err)
	}
	if !got[1].CreatedAt.Equal(t0) {
		t.Fatalf("timestamp round trip: %v vs %v", got[1].CreatedAt, t0)
	}
	if lim, _ := repo.RecentComments(ctx, domain.Movie(movie), 1); len(lim) != 1 {
		t.Fatal("limit ignored")
	}
	if empty, _ := repo.RecentComments(ctx, domain.Movie(movie+1), 5); empty == nil || len(empty) != 0 {
		t.Fatal("unknown movie should give an empty, non-nil slice")
	}
}

func TestStatsForUnknownMovie(t *testing.T) {
	repo, movie := setup(t)
	s, err := repo.GetStats(context.Background(), domain.Movie(movie))
	if err != nil || s.VoteCount != 0 || s.Title != domain.Movie(movie) {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestSeriesAndMoviesWithTheSameIDStayApart(t *testing.T) {
	repo, id := setup(t)
	ctx := context.Background()
	series := domain.Title{Media: domain.MediaTV, ID: id}
	t0 := time.Now().UTC().Truncate(time.Microsecond)

	tvRating := rate(id, "u1", 9, t0)
	tvRating.MediaType = domain.MediaTV
	if _, err := repo.SaveRating(ctx, tvRating); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveRating(ctx, rate(id, "u1", 2, t0)); err != nil { // same user, same id, the movie
		t.Fatal(err)
	}
	tvStats, _ := repo.GetStats(ctx, series)
	movieStats, _ := repo.GetStats(ctx, domain.Movie(id))
	if tvStats.VoteCount != 1 || tvStats.TotalScore != 9 || movieStats.VoteCount != 1 || movieStats.TotalScore != 2 || tvStats.Title != series {
		t.Fatalf("series %+v, movie %+v", tvStats, movieStats)
	}

	c := domain.CommentAdded{EventID: uuid.NewString(), MediaType: domain.MediaTV, MovieID: id, UserID: "u1", Text: "series", OccurredAt: t0}
	if err := repo.SaveComment(ctx, c); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.RecentComments(ctx, series, 5); len(got) != 1 || got[0].Text != "series" {
		t.Fatalf("series comments %+v", got)
	}
	if got, _ := repo.RecentComments(ctx, domain.Movie(id), 5); len(got) != 0 {
		t.Fatalf("the movie got the series' comment: %+v", got)
	}
}

// Account deletion: the user's ratings come off the aggregate (others stay),
// their comments go, and the stale titles are reported.
func TestPurgeUser(t *testing.T) {
	repo, movie := setup(t)
	ctx := context.Background()
	user := "purge-" + uuid.NewString()
	t0 := time.Now().UTC().Truncate(time.Millisecond)
	for _, e := range []domain.RatingSubmitted{rate(movie, user, 9, t0), rate(movie, "other", 5, t0)} {
		if _, err := repo.SaveRating(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []domain.CommentAdded{
		{EventID: uuid.NewString(), MovieID: movie, UserID: user, Text: "mine", OccurredAt: t0},
		{EventID: uuid.NewString(), MovieID: movie, UserID: "other", Text: "theirs", OccurredAt: t0},
	} {
		if err := repo.SaveComment(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	before, _ := repo.GetStats(ctx, domain.Title{Media: domain.MediaMovie, ID: movie})

	touched, err := repo.PurgeUser(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	if len(touched) != 1 || touched[0].Title.ID != movie {
		t.Fatalf("touched: %+v", touched)
	}
	after, _ := repo.GetStats(ctx, domain.Title{Media: domain.MediaMovie, ID: movie})
	if after.TotalScore != 5 || after.VoteCount != 1 || after.Version <= before.Version {
		t.Fatalf("stats after purge: %+v (before %+v)", after, before)
	}
	comments, _ := repo.RecentComments(ctx, domain.Title{Media: domain.MediaMovie, ID: movie}, 10)
	if len(comments) != 1 || comments[0].UserID != "other" {
		t.Fatalf("comments after purge: %+v", comments)
	}
	// Idempotent: a retried purge finds nothing.
	if again, err := repo.PurgeUser(ctx, user); err != nil || len(again) != 0 {
		t.Fatalf("second purge: %+v %v", again, err)
	}
}

func TestModeration(t *testing.T) {
	repo, movie := setup(t)
	ctx := context.Background()
	t0 := time.Now().UTC().Truncate(time.Millisecond)
	author, reporter := "author-"+uuid.NewString(), "reporter-"+uuid.NewString()
	commentID := uuid.NewString()
	pool, err := pgxpool.New(ctx, os.Getenv("TEST_POSTGRES_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM interaction.user_blocks WHERE blocker_id = ANY($1) OR blocked_id = ANY($1)`, []string{author, reporter, "someone"})
		pool.Close()
	})
	if err := repo.SaveComment(ctx, domain.CommentAdded{EventID: commentID, MovieID: movie, UserID: author, Text: "rude", OccurredAt: t0}); err != nil {
		t.Fatal(err)
	}

	// A report is idempotent; a comment that isn't there is not found.
	for range 2 {
		if err := repo.ReportComment(ctx, commentID, reporter); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.ReportComment(ctx, uuid.NewString(), reporter); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("report of a missing comment: %v", err)
	}
	var n int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM interaction.comment_reports WHERE comment_id = $1`, commentID).Scan(&n)
	if n != 1 {
		t.Fatalf("%d reports stored, want 1", n)
	}

	// Blocks: idempotent, listed, undone; nobody blocks themselves.
	for range 2 {
		if err := repo.BlockUser(ctx, reporter, author); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.BlockUser(ctx, reporter, reporter); err == nil {
		t.Fatal("blocking yourself must fail")
	}
	if got, _ := repo.BlockedUsers(ctx, reporter); len(got) != 1 || got[0] != author {
		t.Fatalf("blocked: %v", got)
	}
	if got, err := repo.BlockedUsers(ctx, author); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("the author blocked nobody: %v %v", got, err)
	}
	if err := repo.UnblockUser(ctx, reporter, author); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.BlockedUsers(ctx, reporter); len(got) != 0 {
		t.Fatalf("after unblock: %v", got)
	}

	// Account deletion takes a user's reports and blocks, both ways.
	_ = repo.BlockUser(ctx, reporter, author)
	_ = repo.BlockUser(ctx, author, "someone")
	if _, err := repo.PurgeUser(ctx, author); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.BlockedUsers(ctx, reporter); len(got) != 0 {
		t.Fatalf("a purged user stays blocked: %v", got)
	}
	if got, _ := repo.BlockedUsers(ctx, author); len(got) != 0 {
		t.Fatalf("a purged user's blocks stay: %v", got)
	}
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM interaction.comment_reports WHERE comment_id = $1`, commentID).Scan(&n)
	if n != 0 {
		t.Fatalf("reports of a deleted comment stay: %d", n)
	}
}

func TestAdminReports(t *testing.T) {
	repo, movie := setup(t)
	ctx := context.Background()
	t0 := time.Now().UTC().Truncate(time.Millisecond)
	a, b := uuid.NewString(), uuid.NewString()
	for _, id := range []string{a, b} {
		if err := repo.SaveComment(ctx, domain.CommentAdded{EventID: id, MovieID: movie, UserID: "author", Text: "c-" + id, OccurredAt: t0}); err != nil {
			t.Fatal(err)
		}
	}
	// a is reported twice, b once: a comes first.
	for _, r := range [][2]string{{a, "r1"}, {a, "r2"}, {b, "r1"}} {
		if err := repo.ReportComment(ctx, r[0], r[1]); err != nil {
			t.Fatal(err)
		}
	}
	all, err := repo.ReportedComments(ctx, 1000)
	if err != nil {
		t.Fatal(err)
	}
	var mine []domain.ReportedComment
	for _, c := range all {
		if c.MovieID == movie {
			mine = append(mine, c)
		}
	}
	if len(mine) != 2 || mine[0].ID != a || mine[0].Reports != 2 || mine[1].ID != b || mine[1].Reports != 1 || mine[0].MediaType != "movie" {
		t.Fatalf("reported: %+v", mine)
	}

	if err := repo.DismissReports(ctx, b); err != nil {
		t.Fatal(err)
	}
	title, err := repo.DeleteComment(ctx, a)
	if err != nil || title.ID != movie || title.Media != domain.MediaMovie {
		t.Fatalf("delete: %+v %v", title, err)
	}
	if _, err := repo.DeleteComment(ctx, a); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
	left, _ := repo.RecentComments(ctx, domain.Title{Media: domain.MediaMovie, ID: movie}, 10)
	if len(left) != 1 || left[0].ID != b {
		t.Fatalf("comments left: %+v", left)
	}
	for _, c := range mustReported(t, repo) {
		if c.MovieID == movie {
			t.Fatalf("still reported: %+v", c)
		}
	}
}

func mustReported(t *testing.T, repo *postgres.Repo) []domain.ReportedComment {
	t.Helper()
	out, err := repo.ReportedComments(context.Background(), 1000)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
