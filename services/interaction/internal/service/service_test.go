package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/furkanpatat/movieapp/services/interaction/internal/domain"
	"github.com/furkanpatat/movieapp/services/interaction/internal/repository/readmodel"
	"github.com/furkanpatat/movieapp/services/interaction/internal/service"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// memRepo is an in-memory domain.Repository with the same semantics as the
// Postgres one (the real one is covered by the gated integration test).
type memRepo struct {
	mu       sync.Mutex
	ratings  map[string]rated // "movie/user"
	stats    map[domain.Title]domain.RatingStats
	comments map[string]domain.CommentAdded
	reads    atomic.Int32 // GetStats calls: proxy for "Postgres was consulted"
	failSave error
}
type rated struct {
	score int
	at    time.Time
}

func newMemRepo() *memRepo {
	return &memRepo{ratings: map[string]rated{}, stats: map[domain.Title]domain.RatingStats{}, comments: map[string]domain.CommentAdded{}}
}

func (r *memRepo) SaveRating(_ context.Context, e domain.RatingSubmitted) (domain.RatingStats, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failSave != nil {
		return domain.RatingStats{}, r.failSave
	}
	s := r.stats[e.Title()]
	s.Title = e.Title()
	k := fmt.Sprint(e.Title(), "/", e.UserID)
	old, exists := r.ratings[k]
	switch {
	case !exists:
		s.TotalScore += int64(e.Score)
		s.VoteCount++
	case e.OccurredAt.Before(old.at), old.score == e.Score:
		return s, nil
	default:
		s.TotalScore += int64(e.Score - old.score)
	}
	s.Version++
	r.ratings[k] = rated{e.Score, e.OccurredAt}
	r.stats[e.Title()] = s
	return s, nil
}

func (r *memRepo) SaveComment(_ context.Context, e domain.CommentAdded) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failSave != nil {
		return r.failSave
	}
	r.comments[e.EventID] = e
	return nil
}

func (r *memRepo) GetStats(_ context.Context, id domain.Title) (domain.RatingStats, error) {
	r.reads.Add(1)
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.stats[id]
	s.Title = id
	return s, nil
}

func (r *memRepo) RecentComments(_ context.Context, id domain.Title, limit int) ([]domain.Comment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []domain.Comment{}
	for _, e := range r.comments {
		if e.Title() == id {
			out = append(out, domain.Comment{ID: e.EventID, UserID: e.UserID, Text: e.Text, CreatedAt: domain.NormalizeTime(e.OccurredAt)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

type fakeOutbox struct {
	msgs []domain.OutboxMessage
	err  error
}

func (f *fakeOutbox) Enqueue(_ context.Context, m domain.OutboxMessage) error {
	if f.err != nil {
		return f.err
	}
	f.msgs = append(f.msgs, m)
	return nil
}

type env struct {
	repo *memRepo
	mr   *miniredis.Miniredis
	proj *service.Projector
	qry  *service.Query
}

func newEnv(t *testing.T) env {
	repo := newMemRepo()
	mr := miniredis.RunT(t)
	rm := readmodel.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), 20)
	return env{repo, mr, service.NewProjector(repo, rm, 20, quiet), service.NewQuery(repo, rm, 20, quiet)}
}

func rating(user string, score int, at time.Time) domain.RatingSubmitted {
	return domain.RatingSubmitted{EventID: fmt.Sprint(user, score, at.UnixNano()), MovieID: 1, UserID: user, Score: score, OccurredAt: at}
}

func TestCommandStoresEventInOutbox(t *testing.T) {
	out := &fakeOutbox{}
	cmd := service.NewCommand(out)
	id, err := cmd.SubmitRating(context.Background(), domain.Movie(5), "alice", 9)
	if err != nil || id == "" || len(out.msgs) != 1 {
		t.Fatalf("id=%q err=%v msgs=%+v", id, err, out.msgs)
	}
	m := out.msgs[0]
	var e domain.RatingSubmitted
	if err := json.Unmarshal(m.Payload, &e); err != nil {
		t.Fatal(err)
	}
	if m.ID != id || m.Type != domain.EventTypeRatingSubmitted ||
		e.EventID != id || e.MovieID != 5 || e.UserID != "alice" || e.Score != 9 || e.OccurredAt.IsZero() {
		t.Fatalf("bad message %+v / event %+v", m, e)
	}

	id, err = cmd.SubmitComment(context.Background(), domain.Movie(5), "alice", "great")
	if err != nil || len(out.msgs) != 2 || out.msgs[1].Type != domain.EventTypeCommentAdded || out.msgs[1].ID != id {
		t.Fatalf("%v %+v", err, out.msgs)
	}
}

func TestCommandRejectsInvalidWithoutStoring(t *testing.T) {
	out := &fakeOutbox{}
	cmd := service.NewCommand(out)
	if _, err := cmd.SubmitRating(context.Background(), domain.Movie(5), "alice", 11); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatal(err)
	}
	if _, err := cmd.SubmitComment(context.Background(), domain.Movie(5), "alice", " "); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatal(err)
	}
	if len(out.msgs) != 0 {
		t.Fatal("invalid input reached the outbox")
	}
}

func TestCommandSurfacesStorageFailure(t *testing.T) {
	cmd := service.NewCommand(&fakeOutbox{err: errors.New("postgres down")})
	if _, err := cmd.SubmitRating(context.Background(), domain.Movie(1), "u", 5); err == nil || errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("got %v", err)
	}
}

func TestFullFlowProjectThenQuery(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	t0 := time.Now().UTC()

	_ = e.proj.HandleRating(ctx, rating("a", 8, t0))
	_ = e.proj.HandleRating(ctx, rating("b", 6, t0))
	_ = e.proj.HandleComment(ctx, domain.CommentAdded{EventID: "c1", MovieID: 1, UserID: "a", Text: "first", OccurredAt: t0})
	_ = e.proj.HandleComment(ctx, domain.CommentAdded{EventID: "c2", MovieID: 1, UserID: "b", Text: "second", OccurredAt: t0.Add(time.Second)})

	reads := e.repo.reads.Load()
	got, err := e.qry.GetInteractions(ctx, domain.Movie(1), 0)
	if err != nil || got.TotalVotes != 2 || got.AverageRating != 7 {
		t.Fatalf("%+v %v", got, err)
	}
	if len(got.RecentComments) != 2 || got.RecentComments[0].Text != "second" {
		t.Fatalf("%+v", got.RecentComments)
	}
	if e.repo.reads.Load() != reads {
		t.Fatal("query hit Postgres although the read model was populated")
	}
}

func TestReRateReplacesScore(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	t0 := time.Now().UTC()
	_ = e.proj.HandleRating(ctx, rating("a", 4, t0))
	_ = e.proj.HandleRating(ctx, rating("a", 10, t0.Add(time.Second)))
	got, _ := e.qry.GetInteractions(ctx, domain.Movie(1), 0)
	if got.TotalVotes != 1 || got.AverageRating != 10 {
		t.Fatalf("%+v", got)
	}
}

func TestRedeliveryIsIdempotent(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	r := rating("a", 7, time.Now().UTC())
	c := domain.CommentAdded{EventID: "c1", MovieID: 1, UserID: "a", Text: "hi", OccurredAt: time.Now().UTC()}
	for i := 0; i < 3; i++ {
		if err := e.proj.HandleRating(ctx, r); err != nil {
			t.Fatal(err)
		}
		if err := e.proj.HandleComment(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := e.qry.GetInteractions(ctx, domain.Movie(1), 0)
	if got.TotalVotes != 1 || got.AverageRating != 7 || len(got.RecentComments) != 1 {
		t.Fatalf("duplicates leaked into the read model: %+v", got)
	}
}

func TestOutOfOrderOlderRatingIgnored(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	t0 := time.Now().UTC()
	_ = e.proj.HandleRating(ctx, rating("a", 9, t0.Add(time.Minute)))
	_ = e.proj.HandleRating(ctx, rating("a", 2, t0)) // older event arrives late
	got, _ := e.qry.GetInteractions(ctx, domain.Movie(1), 0)
	if got.AverageRating != 9 {
		t.Fatalf("stale event won: %+v", got)
	}
}

func TestCacheMissRebuildsFromPostgresThenServesFromRedis(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	t0 := time.Now().UTC()
	_ = e.proj.HandleRating(ctx, rating("a", 8, t0))
	_ = e.proj.HandleComment(ctx, domain.CommentAdded{EventID: "c1", MovieID: 1, UserID: "a", Text: "hi", OccurredAt: t0})

	e.mr.FlushAll() // Redis lost everything

	before := e.repo.reads.Load()
	got, err := e.qry.GetInteractions(ctx, domain.Movie(1), 0)
	if err != nil || got.TotalVotes != 1 || len(got.RecentComments) != 1 {
		t.Fatalf("rebuild: %+v %v", got, err)
	}
	if e.repo.reads.Load() != before+1 {
		t.Fatal("miss should consult Postgres once")
	}
	if _, _ = e.qry.GetInteractions(ctx, domain.Movie(1), 0); e.repo.reads.Load() != before+1 {
		t.Fatal("second read should come from the rebuilt read model")
	}
}

func TestUnknownMovieIsCachedAsEmpty(t *testing.T) {
	e := newEnv(t)
	got, err := e.qry.GetInteractions(context.Background(), domain.Movie(404), 0)
	if err != nil || got.TotalVotes != 0 || got.RecentComments == nil || len(got.RecentComments) != 0 {
		t.Fatalf("%+v %v", got, err)
	}
	before := e.repo.reads.Load()
	_, _ = e.qry.GetInteractions(context.Background(), domain.Movie(404), 0)
	if e.repo.reads.Load() != before {
		t.Fatal("empty result should be negatively cached")
	}
}

// The subtle one: Redis is flushed, then a *rating* arrives. The projector must
// rebuild the whole model (including old comments) instead of creating a
// rating-only model that would hide them forever.
func TestEventAfterRedisFlushRebuildsEverything(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	t0 := time.Now().UTC()
	_ = e.proj.HandleComment(ctx, domain.CommentAdded{EventID: "old", MovieID: 1, UserID: "a", Text: "old comment", OccurredAt: t0})
	e.mr.FlushAll()

	if err := e.proj.HandleRating(ctx, rating("b", 5, t0.Add(time.Second))); err != nil {
		t.Fatal(err)
	}
	got, _ := e.qry.GetInteractions(ctx, domain.Movie(1), 0)
	if got.TotalVotes != 1 || len(got.RecentComments) != 1 || got.RecentComments[0].Text != "old comment" {
		t.Fatalf("old comment lost after flush: %+v", got)
	}
}

func TestProjectorPropagatesStorageErrorsForRetry(t *testing.T) {
	e := newEnv(t)
	e.repo.failSave = errors.New("pg down")
	if err := e.proj.HandleRating(context.Background(), rating("a", 5, time.Now())); err == nil {
		t.Fatal("storage failure must surface so the message is retried / dead-lettered")
	}
	if err := e.proj.HandleComment(context.Background(), domain.CommentAdded{EventID: "c", MovieID: 1, UserID: "a", Text: "x", OccurredAt: time.Now()}); err == nil {
		t.Fatal("storage failure must surface")
	}
}

func TestQueryFallsBackToPostgresWhenRedisDown(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_ = e.proj.HandleRating(ctx, rating("a", 8, time.Now().UTC()))
	e.mr.Close()
	got, err := e.qry.GetInteractions(ctx, domain.Movie(1), 0)
	if err != nil || got.TotalVotes != 1 {
		t.Fatalf("Redis outage must not fail reads: %+v %v", got, err)
	}
}

func TestQueryInvalidMovie(t *testing.T) {
	e := newEnv(t)
	if _, err := e.qry.GetInteractions(context.Background(), domain.Movie(0), 0); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatal(err)
	}
}

func (m *memRepo) PurgeUser(ctx context.Context, userID string) ([]domain.RatingStats, error) {
	return nil, nil
}
