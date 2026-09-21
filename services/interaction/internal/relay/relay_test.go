package relay_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanpatat/movieapp/services/interaction/internal/domain"
	"github.com/furkanpatat/movieapp/services/interaction/internal/relay"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// fakeStore is an in-memory OutboxStore with the same contract as Postgres.
type fakeStore struct {
	mu        sync.Mutex
	pending   []domain.OutboxMessage
	published []domain.OutboxMessage
	polls     atomic.Int32
	deleted   atomic.Int32
}

func (s *fakeStore) PublishPending(ctx context.Context, limit int, publish func(context.Context, domain.OutboxMessage) error) (int, error) {
	s.polls.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for len(s.pending) > 0 && n < limit {
		if err := publish(ctx, s.pending[0]); err != nil {
			return n, err
		}
		s.published = append(s.published, s.pending[0])
		s.pending = s.pending[1:]
		n++
	}
	return n, nil
}
func (s *fakeStore) DeleteExpired(context.Context, time.Duration, int) (int64, error) {
	s.deleted.Add(1)
	return 0, nil
}
func (s *fakeStore) PendingCount(context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return int64(len(s.pending)), nil
}
func (s *fakeStore) add(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := 0; i < n; i++ {
		s.pending = append(s.pending, domain.OutboxMessage{ID: string(rune('a' + i%26)), Type: domain.EventTypeCommentAdded})
	}
}
func (s *fakeStore) counts() (pending, published int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pending), len(s.published)
}

// switchPub is a broker that can be taken down and brought back.
type switchPub struct {
	down  atomic.Bool
	calls atomic.Int32
	block bool
}

func (p *switchPub) Publish(ctx context.Context, _ domain.OutboxMessage) error {
	p.calls.Add(1)
	if p.block {
		<-ctx.Done()
		return ctx.Err()
	}
	if p.down.Load() {
		return errors.New("broker unreachable")
	}
	return nil
}

func start(t *testing.T, s domain.OutboxStore, p domain.OutboxPublisher, cfg relay.Config) func() {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { relay.New(s, p, cfg, quiet).Run(ctx); close(done) }()
	stop := func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("relay did not stop")
		}
	}
	t.Cleanup(stop)
	return stop
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out: %s", what)
}

func TestPublishesPendingAndPicksUpNewEvents(t *testing.T) {
	s, p := &fakeStore{}, &switchPub{}
	s.add(3)
	start(t, s, p, relay.Config{PollInterval: 10 * time.Millisecond})
	eventually(t, "initial 3 published", func() bool { _, n := s.counts(); return n == 3 })
	s.add(2)
	eventually(t, "later 2 published", func() bool { _, n := s.counts(); return n == 5 })
}

func TestEventsSurviveBrokerOutageAndFlowAfterRecovery(t *testing.T) {
	s, p := &fakeStore{}, &switchPub{}
	p.down.Store(true)
	s.add(4)
	start(t, s, p, relay.Config{PollInterval: 10 * time.Millisecond, MaxBackoff: 40 * time.Millisecond})

	time.Sleep(200 * time.Millisecond) // broker stays down across many polls
	if pend, pub := s.counts(); pend != 4 || pub != 0 {
		t.Fatalf("events must stay pending while the broker is down: pending=%d published=%d", pend, pub)
	}
	if p.calls.Load() < 2 {
		t.Fatal("relay should keep retrying")
	}

	p.down.Store(false) // broker is back
	eventually(t, "all events delivered after recovery", func() bool { pend, pub := s.counts(); return pend == 0 && pub == 4 })
}

func TestBackoffLimitsRetryRateDuringOutage(t *testing.T) {
	s, p := &fakeStore{}, &switchPub{}
	p.down.Store(true)
	s.add(1)
	start(t, s, p, relay.Config{PollInterval: 20 * time.Millisecond, MaxBackoff: 200 * time.Millisecond})
	time.Sleep(600 * time.Millisecond)
	// Without backoff this would be ~30 attempts; with doubling (20,40,80,160,200..) it is <= ~8.
	if c := p.calls.Load(); c > 10 {
		t.Fatalf("%d publish attempts in 600ms: no backoff", c)
	}
}

func TestBacklogDrainsWithoutWaitingForPollInterval(t *testing.T) {
	s, p := &fakeStore{}, &switchPub{}
	s.add(50)
	start(t, s, p, relay.Config{PollInterval: time.Hour, BatchSize: 10}) // would never finish if it slept between full batches
	eventually(t, "50 events drained in full batches", func() bool { _, n := s.counts(); return n == 50 })
}

func TestPublishTimeoutUnblocksStuckBroker(t *testing.T) {
	s, p := &fakeStore{}, &switchPub{block: true}
	s.add(1)
	stop := start(t, s, p, relay.Config{PollInterval: 10 * time.Millisecond, PublishTimeout: 30 * time.Millisecond})
	eventually(t, "retry after publish timeout", func() bool { return p.calls.Load() >= 2 })
	stop()
}

func TestRetentionCleanupRuns(t *testing.T) {
	s, p := &fakeStore{}, &switchPub{}
	start(t, s, p, relay.Config{PollInterval: 10 * time.Millisecond, Retention: time.Hour})
	eventually(t, "cleanup called", func() bool { return s.deleted.Load() >= 1 })

	s2 := &fakeStore{}
	start(t, s2, p, relay.Config{PollInterval: 10 * time.Millisecond}) // retention disabled
	time.Sleep(100 * time.Millisecond)
	if s2.deleted.Load() != 0 {
		t.Fatal("cleanup must be off when Retention <= 0")
	}
}
