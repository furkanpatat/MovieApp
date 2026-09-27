// Package acceptance holds cross-layer tests of the reliability guarantees.
package acceptance_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/furkanpatat/movieapp/services/interaction/internal/domain"
	"github.com/furkanpatat/movieapp/services/interaction/internal/relay"
	"github.com/furkanpatat/movieapp/services/interaction/internal/repository/postgres"
	"github.com/furkanpatat/movieapp/services/interaction/internal/repository/readmodel"
	"github.com/furkanpatat/movieapp/services/interaction/internal/service"
	"github.com/furkanpatat/movieapp/services/interaction/internal/testsupport"
	"github.com/furkanpatat/movieapp/services/interaction/internal/transport"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// broker stands in for RabbitMQ + the consumers: while "up" it accepts events
// and hands them to the projector exactly like a real delivery would.
type broker struct {
	down atomic.Bool
	proj *service.Projector
	got  atomic.Int32
}

func (b *broker) Publish(ctx context.Context, m domain.OutboxMessage) error {
	if b.down.Load() {
		return errors.New("connection refused")
	}
	b.got.Add(1)
	switch m.Type {
	case domain.EventTypeRatingSubmitted:
		var e domain.RatingSubmitted
		if err := json.Unmarshal(m.Payload, &e); err != nil {
			return err
		}
		return b.proj.HandleRating(ctx, e)
	case domain.EventTypeCommentAdded:
		var e domain.CommentAdded
		if err := json.Unmarshal(m.Payload, &e); err != nil {
			return err
		}
		return b.proj.HandleComment(ctx, e)
	}
	return fmt.Errorf("unknown type %s", m.Type)
}

// post sends a write as the given user, the way the gateway would: identity
// arrives in X-User-Id, never in the body.
func post(t *testing.T, h http.Handler, user, path, body string) int {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("X-User-Id", user)
	h.ServeHTTP(rec, req)
	return rec.Code
}

// The guarantee: RabbitMQ down => writes still 202, nothing is lost, and
// everything flows once the broker returns, ending in a correct read model.
func TestBrokerOutageDoesNotLoseWrites(t *testing.T) {
	pool, schema := testsupport.NewSchema(t)
	repo := postgres.NewInSchema(pool, schema)
	mr := miniredis.RunT(t)
	rm := readmodel.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), 20)

	br := &broker{proj: service.NewProjector(repo, rm, 20, quiet)}
	br.down.Store(true) // <-- RabbitMQ is DOWN from the start

	h := transport.NewHandler(service.NewCommand(repo), service.NewQuery(repo, rm, 20, quiet), nil, quiet)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		relay.New(repo, br, relay.Config{PollInterval: 20 * time.Millisecond, MaxBackoff: 50 * time.Millisecond}, quiet).Run(ctx)
		close(done)
	}()
	defer func() { cancel(); <-done }()

	// 1. Writes succeed while the broker is down.
	for path, body := range map[string]string{
		"/api/v1/movies/42/rate":    `{"score":9}`,
		"/api/v1/movies/42/comment": `{"text":"written during the outage"}`,
	} {
		if code := post(t, h, "alice", path, body); code != http.StatusAccepted {
			t.Fatalf("POST %s during outage -> %d, want 202", path, code)
		}
	}
	if code := post(t, h, "bob", "/api/v1/movies/42/rate", `{"score":3}`); code != http.StatusAccepted {
		t.Fatalf("third write -> %d", code)
	}

	// 2. The events sit in the outbox, still pending, and nothing was projected.
	time.Sleep(250 * time.Millisecond) // many failed relay polls
	ctxb := context.Background()
	if n, _ := repo.PendingCount(ctxb); n != 3 {
		t.Fatalf("outbox pending = %d, want 3", n)
	}
	if br.got.Load() != 0 {
		t.Fatal("broker was down but received events")
	}
	var lastErr *string
	var attempts int
	_ = pool.QueryRow(ctxb, fmt.Sprintf(`SELECT max(attempts), max(last_error) FROM %s.outbox_events`, schema)).Scan(&attempts, &lastErr)
	if attempts < 1 || lastErr == nil || !strings.Contains(*lastErr, "connection refused") {
		t.Fatalf("failed attempts should be recorded: attempts=%d err=%v", attempts, lastErr)
	}

	// 3. Broker returns: the relay drains the outbox without any client action.
	br.down.Store(false)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if n, _ := repo.PendingCount(ctxb); n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("outbox never drained after the broker came back")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// 4. End state: source of truth and read model reflect every write.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/movies/42/interactions", nil))
	body := rec.Body.String()
	for _, want := range []string{`"total_votes":2`, `"average_rating":6`, `written during the outage`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %s in %s", want, body)
		}
	}
	stats, _ := repo.GetStats(ctxb, domain.Movie(42))
	if stats.VoteCount != 2 || stats.TotalScore != 12 {
		t.Fatalf("postgres stats %+v", stats)
	}
}

// If the process dies between "broker confirmed" and "marked published", the
// row is published again on restart. The consumer side must absorb that.
func TestRedeliveryAfterRelayCrashIsHarmless(t *testing.T) {
	pool, schema := testsupport.NewSchema(t)
	repo := postgres.NewInSchema(pool, schema)
	mr := miniredis.RunT(t)
	rm := readmodel.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), 20)
	br := &broker{proj: service.NewProjector(repo, rm, 20, quiet)}
	h := transport.NewHandler(service.NewCommand(repo), service.NewQuery(repo, rm, 20, quiet), nil, quiet)

	post(t, h, "a", "/api/v1/movies/7/rate", `{"score":8}`)
	post(t, h, "a", "/api/v1/movies/7/comment", `{"text":"once"}`)

	ctx := context.Background()
	// The relay publishes both events, then dies before committing "published":
	// the broker has them, Postgres still says pending.
	_, err := repo.PublishPending(ctx, 10, func(c context.Context, m domain.OutboxMessage) error {
		if err := br.Publish(c, m); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if br.got.Load() != 2 {
		t.Fatalf("setup: delivered %d", br.got.Load())
	}
	// Simulate the lost commit by putting the rows back to pending.
	if _, err := pool.Exec(ctx, fmt.Sprintf(`UPDATE %s.outbox_events SET status = 'pending', published_at = NULL`, schema)); err != nil {
		t.Fatal(err)
	}
	// A restarted relay publishes the same events again.
	if n, err := repo.PublishPending(ctx, 10, br.Publish); err != nil || n != 2 {
		t.Fatalf("redelivery: n=%d err=%v", n, err)
	}
	if br.got.Load() != 4 {
		t.Fatalf("each event should have been delivered twice, got %d deliveries", br.got.Load())
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/movies/7/interactions", nil))
	if !strings.Contains(rec.Body.String(), `"total_votes":1`) || strings.Count(rec.Body.String(), `"text":"once"`) != 1 {
		t.Fatalf("duplicate delivery corrupted the read model: %s", rec.Body)
	}
}
