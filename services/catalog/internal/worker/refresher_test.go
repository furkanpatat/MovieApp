package worker_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanpatat/movieapp/services/catalog/internal/worker"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func runFor(t *testing.T, d time.Duration, interval time.Duration, f worker.Func) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	done := make(chan struct{})
	go func() { worker.Run(ctx, f, interval, quiet); close(done) }()
	select {
	case <-done:
	case <-time.After(d + 2*time.Second):
		t.Fatal("worker did not stop after context cancellation")
	}
}

func TestRunsImmediatelyThenPeriodically(t *testing.T) {
	var n atomic.Int32
	runFor(t, 260*time.Millisecond, 50*time.Millisecond, func(context.Context) (string, error) {
		n.Add(1)
		return "ok", nil
	})
	if got := n.Load(); got < 4 { // 1 immediate + ~5 ticks
		t.Fatalf("runs = %d, want >= 4", got)
	}
}

func TestImmediateRunWithLongInterval(t *testing.T) {
	var n atomic.Int32
	runFor(t, 100*time.Millisecond, time.Hour, func(context.Context) (string, error) { n.Add(1); return "", nil })
	if n.Load() != 1 {
		t.Fatalf("runs = %d, want exactly the startup run", n.Load())
	}
}

func TestErrorsAndPanicsOnlySkipTheCycle(t *testing.T) {
	var n atomic.Int32
	runFor(t, 260*time.Millisecond, 40*time.Millisecond, func(context.Context) (string, error) {
		switch n.Add(1) % 3 {
		case 0:
			panic("boom")
		case 1:
			return "", errors.New("tmdb down")
		}
		return "ok", nil
	})
	if n.Load() < 4 {
		t.Fatalf("worker died after a failure: runs = %d", n.Load())
	}
}

func TestRunsNeverOverlap(t *testing.T) {
	var active, maxActive atomic.Int32
	runFor(t, 300*time.Millisecond, 10*time.Millisecond, func(context.Context) (string, error) {
		cur := active.Add(1)
		if cur > maxActive.Load() {
			maxActive.Store(cur)
		}
		time.Sleep(60 * time.Millisecond) // slower than the interval
		active.Add(-1)
		return "", nil
	})
	if maxActive.Load() != 1 {
		t.Fatalf("overlapping runs: %d", maxActive.Load())
	}
}
