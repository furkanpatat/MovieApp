package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	sharedcfg "github.com/furkanpatat/movieapp/pkg/config"
	"github.com/furkanpatat/movieapp/pkg/jwtauth"
	"github.com/furkanpatat/movieapp/pkg/logger"
	"github.com/furkanpatat/movieapp/services/gateway/internal/config"
	"github.com/furkanpatat/movieapp/services/gateway/internal/ratelimit"
	"github.com/furkanpatat/movieapp/services/gateway/internal/server"
)

func main() {
	if err := run(); err != nil {
		slog.Error("gateway exited", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := sharedcfg.Load[config.Config]()
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = "gateway"
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

	catalog, _ := url.Parse(cfg.CatalogURL) // validated above
	interaction, _ := url.Parse(cfg.InteractionURL)
	authSvc, _ := url.Parse(cfg.AuthURL)
	watchParty, _ := url.Parse(cfg.WatchPartyURL)

	handler := server.New(server.Deps{
		Catalog: catalog, Interaction: interaction, AuthService: authSvc, WatchParty: watchParty,
		// TTL is irrelevant here: the gateway only verifies.
		Auth:    jwtauth.NewManager(cfg.JWTSecret, cfg.JWTIssuer, time.Hour),
		Limiter: ratelimit.New(rdb, cfg.RateLimit.Requests, cfg.RateLimit.Window),
		RateLimit: ratelimit.MiddlewareConfig{
			FailOpen: cfg.RateLimit.FailOpen, TrustForwardedFor: cfg.RateLimit.TrustForwardedFor, Log: log,
		},
		AuthLimiter: ratelimit.New(rdb, cfg.AuthRateLimit.Requests, cfg.AuthRateLimit.Window),
		AuthRateLimit: ratelimit.MiddlewareConfig{
			KeyPrefix: "auth:ip:", FailOpen: false, // credential endpoints fail closed
			TrustForwardedFor: cfg.RateLimit.TrustForwardedFor, Log: log,
		},
		UpstreamTimeout: cfg.UpstreamTimeout,
		ChatTimeout:     cfg.ChatTimeout,
		ChatLimiter:     ratelimit.New(rdb, cfg.ChatRateLimit.Requests, cfg.ChatRateLimit.Window),
		ChatRateLimit: ratelimit.MiddlewareConfig{
			KeyPrefix: "chat:ip:", FailOpen: cfg.RateLimit.FailOpen,
			TrustForwardedFor: cfg.RateLimit.TrustForwardedFor, Log: log,
		},
		CORSAllowedOrigins: cfg.CORSOrigins(),
		Ready:              func(c context.Context) error { return rdb.Ping(c).Err() },
		Log:                log,
	})

	srv := &http.Server{
		Addr: cfg.HTTPAddr, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.HTTPAddr, "catalog", cfg.CatalogURL, "interaction", cfg.InteractionURL,
			"rate_limit", cfg.RateLimit.Requests, "window", cfg.RateLimit.Window)
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
		return nil
	}
}
