package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	sharedcfg "github.com/furkanpatat/movieapp/pkg/config"
	"github.com/furkanpatat/movieapp/pkg/logger"
	"github.com/furkanpatat/movieapp/pkg/messaging"
	"github.com/furkanpatat/movieapp/services/interaction/internal/config"
	"github.com/furkanpatat/movieapp/services/interaction/internal/events"
	"github.com/furkanpatat/movieapp/services/interaction/internal/relay"
	"github.com/furkanpatat/movieapp/services/interaction/internal/repository/postgres"
	"github.com/furkanpatat/movieapp/services/interaction/internal/repository/readmodel"
	"github.com/furkanpatat/movieapp/services/interaction/internal/service"
	"github.com/furkanpatat/movieapp/services/interaction/internal/transport"
)

func main() {
	if err := run(); err != nil {
		slog.Error("interaction exited", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := sharedcfg.Load[config.Config]()
	if err != nil {
		return err
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = "interaction"
	}
	log := logger.New(logger.Config{Service: cfg.ServiceName, Level: cfg.LogLevel, Format: cfg.LogFormat})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// --- infrastructure ---
	pool, err := pgxpool.New(ctx, cfg.Postgres.DSN())
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := ping(ctx, func(c context.Context) error { return pool.Ping(c) }); err != nil {
		return err
	}

	rdb := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr(), Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	defer rdb.Close()
	if err := ping(ctx, func(c context.Context) error { return rdb.Ping(c).Err() }); err != nil {
		return err
	}

	// --- wiring ---
	repo := postgres.New(pool)
	rm := readmodel.New(rdb, cfg.Interaction.RecentComments)
	keep := cfg.Interaction.RecentComments

	// Everything that needs RabbitMQ runs in the background and connects
	// whenever the broker is available. The API depends only on Postgres and
	// Redis, so a broker outage never stops it from accepting writes: they
	// accumulate in the outbox and the relay drains them after reconnecting.
	var wg sync.WaitGroup
	if cfg.Interaction.WorkerEnabled || cfg.Outbox.RelayEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runBroker(ctx, cfg, repo, rm, log)
		}()
	}

	var srv *http.Server
	errCh := make(chan error, 1)
	if cfg.Interaction.APIEnabled {
		srv = &http.Server{
			Addr: cfg.HTTPAddr,
			Handler: transport.NewHandler(service.NewCommand(repo), service.NewQuery(repo, rm, keep, log), service.NewAccount(repo, rm),
				func() bool { return ping(ctx, pool.Ping) == nil }, // ready = can store events; broker state is irrelevant
				log, transport.WithModeration(service.NewModeration(repo))),
			ReadHeaderTimeout: 5 * time.Second,
		}
		go func() {
			log.Info("listening", "addr", cfg.HTTPAddr)
			errCh <- srv.ListenAndServe()
		}()
	}

	select {
	case err := <-errCh:
		stop()
		wg.Wait()
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down")
	if srv != nil {
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(sctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	wg.Wait() // consumers and relay stop on ctx cancellation
	return nil
}

// runBroker waits for RabbitMQ, declares the topology, then runs the
// projector consumers and the outbox relay until ctx is cancelled.
// The relay starts only after the queues exist, so it can never publish into
// a void (an unrouted message would be dropped).
func runBroker(ctx context.Context, cfg config.Config, repo *postgres.Repo, rm *readmodel.Model, log *slog.Logger) {
	conn, err := messaging.Dial(ctx, messaging.Config{URL: cfg.RabbitMQ.URL(), Logger: log}) // retries until connected or ctx done
	if err != nil {
		return
	}
	defer conn.Close()

	for {
		err := events.DeclareTopology(ctx, conn, events.Topology{})
		if err == nil {
			break
		}
		log.Warn("declaring topology failed, retrying", "error", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}

	var wg sync.WaitGroup
	if cfg.Interaction.WorkerEnabled {
		projector := service.NewProjector(repo, rm, cfg.Interaction.RecentComments, log)
		wg.Add(1)
		go func() {
			defer wg.Done()
			events.Consume(ctx, conn, projector,
				events.Settings{Prefetch: cfg.Interaction.ConsumerPrefetch, MaxRetries: cfg.Interaction.MaxRetries}, log)
		}()
	}
	if cfg.Outbox.RelayEnabled {
		// One attempt per poll: the relay's own backoff handles retries.
		pub := events.NewPublisher(messaging.NewPublisher(conn, 1), events.Topology{})
		r := relay.New(repo, pub, relay.Config{
			PollInterval:   cfg.Outbox.PollInterval,
			BatchSize:      cfg.Outbox.BatchSize,
			PublishTimeout: cfg.Outbox.PublishTimeout,
			Retention:      cfg.Outbox.Retention,
		}, log)
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Run(ctx)
		}()
	}
	wg.Wait()
}

func ping(ctx context.Context, f func(context.Context) error) error {
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return f(c)
}
