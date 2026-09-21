// Package worker runs the Catalog service's background jobs.
package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Refresher performs one cache warm-up run and reports what it did.
type Refresher interface {
	Refresh(ctx context.Context) (summary string, err error)
}

// Func adapts a function to Refresher.
type Func func(ctx context.Context) (string, error)

func (f Func) Refresh(ctx context.Context) (string, error) { return f(ctx) }

// Run refreshes immediately (so a fresh deploy starts warm), then every
// interval, until ctx is cancelled. Runs never overlap, and a failed or
// panicking run only skips that cycle. It blocks.
func Run(ctx context.Context, r Refresher, interval time.Duration, log *slog.Logger) {
	if log == nil {
		log = slog.Default()
	}
	log = log.With("component", "refresh-worker")

	cycle := func() {
		start := time.Now()
		summary, err := safeRefresh(ctx, r)
		switch {
		case ctx.Err() != nil:
			return
		case err != nil:
			log.Warn("refresh cycle failed", "error", err, "took", time.Since(start).Round(time.Millisecond), "next_in", interval)
		default:
			log.Info("refresh cycle done", "result", summary, "took", time.Since(start).Round(time.Millisecond), "next_in", interval)
		}
	}

	log.Info("started", "interval", interval)
	cycle()

	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info("stopped")
			return
		case <-t.C:
			cycle()
		}
	}
}

func safeRefresh(ctx context.Context, r Refresher) (summary string, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("panic: %v", p)
		}
	}()
	return r.Refresh(ctx)
}
