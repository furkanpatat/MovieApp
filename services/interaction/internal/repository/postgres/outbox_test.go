package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/furkanpatat/movieapp/services/interaction/internal/domain"
	"github.com/furkanpatat/movieapp/services/interaction/internal/repository/postgres"
	"github.com/furkanpatat/movieapp/services/interaction/internal/testsupport"
)

func msg(i int) domain.OutboxMessage {
	return domain.OutboxMessage{ID: uuid.NewString(), Type: domain.EventTypeCommentAdded, Payload: []byte(fmt.Sprintf(`{"n":%d}`, i))}
}

func collect(into *[]domain.OutboxMessage) func(context.Context, domain.OutboxMessage) error {
	return func(_ context.Context, m domain.OutboxMessage) error { *into = append(*into, m); return nil }
}

func TestOutboxEnqueueAndPublishInOrder(t *testing.T) {
	pool, schema := testsupport.NewSchema(t)
	repo := postgres.NewInSchema(pool, schema)
	ctx := context.Background()

	var sent []domain.OutboxMessage
	for i := 0; i < 5; i++ {
		m := msg(i)
		sent = append(sent, m)
		if err := repo.Enqueue(ctx, m); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond) // distinct created_at
	}
	if err := repo.Enqueue(ctx, sent[0]); err != nil { // duplicate id is a no-op
		t.Fatal(err)
	}
	if n, _ := repo.PendingCount(ctx); n != 5 {
		t.Fatalf("pending = %d", n)
	}

	var got []domain.OutboxMessage
	n, err := repo.PublishPending(ctx, 3, collect(&got)) // batch limit respected
	if err != nil || n != 3 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	n, err = repo.PublishPending(ctx, 10, collect(&got))
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	for i := range sent {
		if got[i].ID != sent[i].ID || got[i].Type != domain.EventTypeCommentAdded {
			t.Fatalf("order/identity broken at %d: %+v", i, got[i])
		}
	}
	if string(got[2].Payload) != `{"n": 2}` && string(got[2].Payload) != `{"n":2}` { // jsonb may normalise spacing
		t.Fatalf("payload: %s", got[2].Payload)
	}
	if n, _ := repo.PendingCount(ctx); n != 0 {
		t.Fatalf("pending = %d after publish", n)
	}
	if n, _ := repo.PublishPending(ctx, 10, collect(&got)); n != 0 {
		t.Fatal("published rows must not be picked up again")
	}
}

func TestOutboxStopsAtFirstFailureAndKeepsOrder(t *testing.T) {
	pool, schema := testsupport.NewSchema(t)
	repo := postgres.NewInSchema(pool, schema)
	ctx := context.Background()

	var all []domain.OutboxMessage
	for i := 0; i < 5; i++ {
		m := msg(i)
		all = append(all, m)
		_ = repo.Enqueue(ctx, m)
		time.Sleep(2 * time.Millisecond)
	}

	calls := 0
	n, err := repo.PublishPending(ctx, 10, func(_ context.Context, m domain.OutboxMessage) error {
		calls++
		if calls == 3 {
			return errors.New("broker down")
		}
		return nil
	})
	if err == nil || n != 2 || calls != 3 {
		t.Fatalf("n=%d calls=%d err=%v (want 2 published, stop at the 3rd)", n, calls, err)
	}
	if pending, _ := repo.PendingCount(ctx); pending != 3 {
		t.Fatalf("pending = %d, want 3", pending)
	}

	var attempts int
	var lastErr *string
	_ = pool.QueryRow(ctx, fmt.Sprintf(`SELECT attempts, last_error FROM %s.outbox_events WHERE id = $1`, schema), all[2].ID).Scan(&attempts, &lastErr)
	if attempts != 1 || lastErr == nil || *lastErr != "broker down" {
		t.Fatalf("failure not recorded: attempts=%d last_error=%v", attempts, lastErr)
	}

	// Next poll resumes exactly where it stopped.
	var got []domain.OutboxMessage
	if n, err := repo.PublishPending(ctx, 10, collect(&got)); err != nil || n != 3 || got[0].ID != all[2].ID {
		t.Fatalf("resume: n=%d err=%v first=%v", n, err, got)
	}
}

func TestOutboxConcurrentRelaysNeverOverlap(t *testing.T) {
	pool, schema := testsupport.NewSchema(t)
	repo := postgres.NewInSchema(pool, schema)
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		_ = repo.Enqueue(ctx, msg(i))
	}

	var mu sync.Mutex
	seen := map[string]int{}
	holding := make(chan struct{})
	release := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { // relay A claims 10 rows and stalls mid-publish, holding their locks
		defer wg.Done()
		first := true
		_, _ = repo.PublishPending(ctx, 10, func(_ context.Context, m domain.OutboxMessage) error {
			mu.Lock()
			seen[m.ID]++
			mu.Unlock()
			if first {
				first = false
				close(holding)
				<-release
			}
			return nil
		})
	}()
	<-holding
	// Relay B runs while A holds its locks: it must get the OTHER rows, not block or duplicate.
	nB, err := repo.PublishPending(ctx, 20, func(_ context.Context, m domain.OutboxMessage) error {
		mu.Lock()
		seen[m.ID]++
		mu.Unlock()
		return nil
	})
	close(release)
	wg.Wait()
	if err != nil || nB != 10 {
		t.Fatalf("relay B published %d (err %v), want the 10 unlocked rows", nB, err)
	}
	for id, c := range seen {
		if c != 1 {
			t.Fatalf("%s published %d times", id, c)
		}
	}
	if len(seen) != 20 {
		t.Fatalf("published %d distinct, want 20", len(seen))
	}
}

func TestOutboxRetention(t *testing.T) {
	pool, schema := testsupport.NewSchema(t)
	repo := postgres.NewInSchema(pool, schema)
	ctx := context.Background()
	old, fresh, pending := msg(1), msg(2), msg(3)
	for _, m := range []domain.OutboxMessage{old, fresh} {
		_ = repo.Enqueue(ctx, m)
	}
	var sink []domain.OutboxMessage
	_, _ = repo.PublishPending(ctx, 10, collect(&sink))
	_ = repo.Enqueue(ctx, pending)
	_, _ = pool.Exec(ctx, fmt.Sprintf(`UPDATE %s.outbox_events SET published_at = now() - interval '2 days' WHERE id = $1`, schema), old.ID)

	n, err := repo.DeleteExpired(ctx, 24*time.Hour, 100)
	if err != nil || n != 1 {
		t.Fatalf("deleted %d err %v, want only the old published row", n, err)
	}
	var total int
	_ = pool.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %s.outbox_events`, schema)).Scan(&total)
	if total != 2 { // fresh published + pending survive
		t.Fatalf("rows left = %d, want 2", total)
	}
}
