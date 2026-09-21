// Package relay moves events from the Postgres outbox to RabbitMQ.
package relay

import (
	"context"
	"log/slog"
	"time"

	"github.com/furkanpatat/movieapp/services/interaction/internal/domain"
)

// Config tunes the relay. Zero values get sensible defaults.
type Config struct {
	PollInterval   time.Duration // idle wait between polls (default 1s)
	MaxBackoff     time.Duration // cap for the wait after failures (default 30s)
	BatchSize      int           // rows claimed per poll (default 100)
	PublishTimeout time.Duration // per message, includes waiting for the broker confirm (default 5s)
	Retention      time.Duration // how long published rows are kept (default 24h; <=0 keeps forever)
	CleanupEvery   time.Duration // default 10m
}

func (c *Config) defaults() {
	if c.PollInterval <= 0 {
		c.PollInterval = time.Second
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = 30 * time.Second
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 100
	}
	if c.PublishTimeout <= 0 {
		c.PublishTimeout = 5 * time.Second
	}
	if c.CleanupEvery <= 0 {
		c.CleanupEvery = 10 * time.Minute
	}
}

type Relay struct {
	store domain.OutboxStore
	pub   domain.OutboxPublisher
	cfg   Config
	log   *slog.Logger
}

func New(store domain.OutboxStore, pub domain.OutboxPublisher, cfg Config, log *slog.Logger) *Relay {
	cfg.defaults()
	if log == nil {
		log = slog.Default()
	}
	return &Relay{store: store, pub: pub, cfg: cfg, log: log.With("component", "outbox-relay")}
}

// Run polls until ctx is cancelled. When a poll fills a whole batch it polls
// again immediately (backlog drain); on errors it backs off exponentially.
func (r *Relay) Run(ctx context.Context) {
	r.log.Info("started", "poll_interval", r.cfg.PollInterval, "batch", r.cfg.BatchSize)

	failures := 0
	var lastCleanup time.Time
	for {
		n, err := r.store.PublishPending(ctx, r.cfg.BatchSize, r.publishOne)
		if ctx.Err() != nil {
			r.log.Info("stopped")
			return
		}

		var wait time.Duration
		switch {
		case err != nil:
			if failures == 0 {
				r.log.Warn("publishing failing, events stay in the outbox and will be retried", "error", err, "published_before_failure", n)
			}
			failures++
			wait = r.backoff(failures)
		default:
			if failures > 0 {
				r.log.Info("publishing recovered", "after_failed_polls", failures)
				failures = 0
			}
			if n > 0 {
				r.log.Debug("published", "count", n)
			}
			if n >= r.cfg.BatchSize {
				wait = 0
			} else {
				wait = r.cfg.PollInterval
			}
		}

		if r.cfg.Retention > 0 && time.Since(lastCleanup) >= r.cfg.CleanupEvery {
			lastCleanup = time.Now()
			r.cleanup(ctx)
		}

		if wait > 0 {
			select {
			case <-ctx.Done():
				r.log.Info("stopped")
				return
			case <-time.After(wait):
			}
		}
	}
}

func (r *Relay) publishOne(ctx context.Context, m domain.OutboxMessage) error {
	ctx, cancel := context.WithTimeout(ctx, r.cfg.PublishTimeout)
	defer cancel()
	return r.pub.Publish(ctx, m)
}

func (r *Relay) backoff(failures int) time.Duration {
	d := r.cfg.PollInterval
	for i := 1; i < failures && d < r.cfg.MaxBackoff; i++ {
		d *= 2
	}
	return min(d, r.cfg.MaxBackoff)
}

func (r *Relay) cleanup(ctx context.Context) {
	total := int64(0)
	for {
		n, err := r.store.DeleteExpired(ctx, r.cfg.Retention, 1000)
		if err != nil {
			r.log.Warn("outbox cleanup failed", "error", err)
			return
		}
		total += n
		if n < 1000 || ctx.Err() != nil {
			break
		}
	}
	if total > 0 {
		r.log.Info("outbox cleanup", "deleted", total)
	}
}
