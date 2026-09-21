package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	sharedcfg "github.com/furkanpatat/movieapp/pkg/config"
	"github.com/furkanpatat/movieapp/pkg/logger"
	"github.com/furkanpatat/movieapp/services/catalog/internal/config"
	"github.com/furkanpatat/movieapp/services/catalog/internal/repository/rediscache"
	"github.com/furkanpatat/movieapp/services/catalog/internal/repository/tmdb"
	"github.com/furkanpatat/movieapp/services/catalog/internal/service"
	"github.com/furkanpatat/movieapp/services/catalog/internal/transport"
	"github.com/furkanpatat/movieapp/services/catalog/internal/worker"
	"github.com/sony/gobreaker/v2"
)

func main() {
	if err := run(); err != nil {
		slog.Error("catalog exited", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := sharedcfg.Load[config.Config]()
	if err != nil {
		return err
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = "catalog"
	}
	log := logger.New(logger.Config{Service: cfg.ServiceName, Level: cfg.LogLevel, Format: cfg.LogFormat})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rdb := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr(), Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	defer rdb.Close()
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	err = rdb.Ping(pctx).Err()
	cancel()
	if err != nil {
		return err
	}

	tm := tmdb.New(tmdb.Config{
		BaseURL: cfg.TMDB.BaseURL, APIKey: cfg.TMDB.APIKey, Timeout: cfg.TMDB.Timeout,
		BreakerFailures: cfg.TMDB.BreakerFailures, BreakerOpenFor: cfg.TMDB.BreakerOpenFor,
		Logger: log,
	})
	svc := service.NewCatalog(tm, rediscache.New(rdb, cfg.Cache.StaleTTL), cfg.Cache.TTL, log)

	var wg sync.WaitGroup
	if cfg.Refresh.Enabled {
		if cfg.Refresh.Interval >= cfg.Cache.TTL {
			log.Warn("REFRESH_INTERVAL is not below CACHE_TTL; entries may expire between refreshes",
				"interval", cfg.Refresh.Interval, "ttl", cfg.Cache.TTL)
		}
		opts := service.RefreshOptions{Pages: cfg.Refresh.Pages, WarmDetails: cfg.Refresh.WarmDetails, Pause: cfg.Refresh.Pause}
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker.Run(ctx, worker.Func(func(ctx context.Context) (string, error) {
				r, err := svc.RefreshPopular(ctx, opts)
				return fmt.Sprintf("pages=%d details=%d skipped=%d", r.Pages, r.Details, r.Skipped), err
			}), cfg.Refresh.Interval, log)
		}()
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           transport.NewHandler(svc, func() bool { return tm.BreakerState() != gobreaker.StateOpen }, log),
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.HTTPAddr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(sctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		wg.Wait()
		return nil
	}
}
