package events_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/furkanpatat/movieapp/pkg/messaging"
	"github.com/furkanpatat/movieapp/services/interaction/internal/domain"
	"github.com/furkanpatat/movieapp/services/interaction/internal/events"
)

type fakeProj struct {
	ratings, comments atomic.Int32
	err               error
}

func (f *fakeProj) HandleRating(context.Context, domain.RatingSubmitted) error {
	f.ratings.Add(1)
	return f.err
}
func (f *fakeProj) HandleComment(context.Context, domain.CommentAdded) error {
	f.comments.Add(1)
	return f.err
}

// isPermanent checks whether messaging would skip retries for this error.
func isPermanent(err error) bool {
	if err == nil {
		return false
	}
	// Permanent wraps in an unexported type; probe via behaviour: errors.As on the
	// same wrapper produced by messaging.Permanent.
	probe := messaging.Permanent(errors.New("x"))
	return fmt.Sprintf("%T", err) == fmt.Sprintf("%T", probe)
}

func TestHandlersClassifyErrors(t *testing.T) {
	p := &fakeProj{}
	h := events.RatingHandler(p)
	ctx := context.Background()

	good := `{"event_id":"e","movie_id":1,"user_id":"u","score":5,"occurred_at":"2026-01-01T00:00:00Z"}`
	if err := h(ctx, amqp.Delivery{Body: []byte(good)}); err != nil || p.ratings.Load() != 1 {
		t.Fatalf("good: %v", err)
	}

	for name, body := range map[string]string{
		"malformed json": `{not json`,
		"bad score":      `{"event_id":"e","movie_id":1,"user_id":"u","score":99}`,
	} {
		before := p.ratings.Load()
		if err := h(ctx, amqp.Delivery{Body: []byte(body)}); !isPermanent(err) {
			t.Errorf("%s: want permanent (straight to DLQ), got %v", name, err)
		}
		if p.ratings.Load() != before {
			t.Errorf("%s: projector should not be called", name)
		}
	}

	p.err = errors.New("postgres down") // transient: retry, not permanent
	if err := h(ctx, amqp.Delivery{Body: []byte(good)}); err == nil || isPermanent(err) {
		t.Fatalf("transient error must be retryable, got %v", err)
	}
	p.err = fmt.Errorf("%w: nope", domain.ErrInvalidInput)
	if err := h(ctx, amqp.Delivery{Body: []byte(good)}); !isPermanent(err) {
		t.Fatalf("projector validation error must be permanent, got %v", err)
	}
}

// Live broker test: RABBITMQ_URL=amqp://user:pass@localhost:5673/
// Uses a private topology prefix so it never touches (or steals messages from)
// the queues of a running stack.
// Verifies routing, DLQ for poison messages, and retry-then-DLQ.
func TestBrokerRoutingAndDeadLettering(t *testing.T) {
	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		t.Skip("RABBITMQ_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	conn, err := messaging.Dial(ctx, messaging.Config{URL: url, Logger: log})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	top := events.Topology{Prefix: fmt.Sprintf("itest.%d.", time.Now().UnixNano())}
	if err := events.DeclareTopology(ctx, conn, top); err != nil {
		t.Fatal(err)
	}
	defer func() { // remove the private queues/exchanges
		ch, err := conn.Channel(context.Background())
		if err != nil {
			return
		}
		defer ch.Close()
		for _, q := range []string{top.Ratings(), top.Comments(), top.Ratings() + ".dlq", top.Comments() + ".dlq"} {
			_, _ = ch.QueueDelete(q, false, false, false)
		}
		for _, x := range []string{top.Prefix + events.Exchange, top.Ratings() + ".dlx", top.Comments() + ".dlx"} {
			_ = ch.ExchangeDelete(x, false, false)
		}
	}()

	proj := &fakeProj{}
	cctx, stop := context.WithCancel(ctx)
	defer stop()
	go events.Consume(cctx, conn, proj, events.Settings{Topology: top, Prefetch: 5, MaxRetries: 1}, log)
	time.Sleep(500 * time.Millisecond)

	pub := events.NewPublisher(messaging.NewPublisher(conn, 3), top)
	if err := pub.Publish(ctx, msg(t, domain.EventTypeRatingSubmitted, domain.RatingSubmitted{EventID: "e1", MovieID: 1, UserID: "u", Score: 8, OccurredAt: time.Now()})); err != nil {
		t.Fatal(err)
	}
	if err := pub.Publish(ctx, msg(t, domain.EventTypeCommentAdded, domain.CommentAdded{EventID: "e2", MovieID: 1, UserID: "u", Text: "hi", OccurredAt: time.Now()})); err != nil {
		t.Fatal(err)
	}
	// Poison message: valid AMQP, invalid payload -> DLQ without retries.
	raw := messaging.NewPublisher(conn, 3)
	if err := raw.Publish(ctx, top.Prefix+events.Exchange, events.KeyRatingSubmitted, "application/json", []byte("{garbage"), nil); err != nil {
		t.Fatal(err)
	}

	waitFor(t, func() bool { return proj.ratings.Load() == 1 && proj.comments.Load() == 1 }, "events routed to the right handlers")

	ch, _ := conn.Channel(ctx)
	defer ch.Close()
	waitFor(t, func() bool {
		d, ok, _ := ch.Get(top.Ratings()+".dlq", true)
		return ok && string(d.Body) == "{garbage"
	}, "poison message in rating DLQ")

	// Transient failures: retried MaxRetries times, then dead-lettered.
	proj.err = errors.New("pg down")
	before := proj.comments.Load()
	if err := pub.Publish(ctx, msg(t, domain.EventTypeCommentAdded, domain.CommentAdded{EventID: "e3", MovieID: 1, UserID: "u", Text: "retry me", OccurredAt: time.Now()})); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		d, ok, _ := ch.Get(top.Comments()+".dlq", true)
		return ok && len(d.Body) > 0
	}, "exhausted retries end in the comment DLQ")
	if got := proj.comments.Load() - before; got != 2 { // 1 attempt + 1 retry
		t.Fatalf("attempts = %d, want 2", got)
	}
}

func msg(t *testing.T, typ string, e any) domain.OutboxMessage {
	t.Helper()
	m, err := domain.NewOutboxMessage("id", typ, e)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestPublisherRejectsUnknownType(t *testing.T) {
	p := events.NewPublisher(nil, events.Topology{})
	if err := p.Publish(context.Background(), domain.OutboxMessage{Type: "Nope"}); err == nil {
		t.Fatal("unknown event type must not be published")
	}
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for: %s", what)
}
