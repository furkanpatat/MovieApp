package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	sharedcfg "github.com/furkanpatat/movieapp/pkg/config"
	"github.com/furkanpatat/movieapp/pkg/jwtauth"
	"github.com/furkanpatat/movieapp/pkg/logger"
	"github.com/furkanpatat/movieapp/services/auth/internal/config"
	"github.com/furkanpatat/movieapp/services/auth/internal/password"
	"github.com/furkanpatat/movieapp/services/auth/internal/repository/postgres"
	"github.com/furkanpatat/movieapp/services/auth/internal/service"
	"github.com/furkanpatat/movieapp/services/auth/internal/transport"
)

func main() {
	if err := run(); err != nil {
		slog.Error("auth exited", "error", err)
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
		cfg.ServiceName = "auth"
	}
	log := logger.New(logger.Config{Service: cfg.ServiceName, Level: cfg.LogLevel, Format: cfg.LogFormat})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.Postgres.DSN())
	if err != nil {
		return err
	}
	defer pool.Close()
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	err = pool.Ping(pctx)
	cancel()
	if err != nil {
		return err
	}

	hasher, err := password.NewBcrypt(cfg.BcryptCost)
	if err != nil {
		return err
	}
	tokens := jwtauth.NewManager(cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTTTL)
	svc, err := service.New(postgres.New(pool), hasher, tokens, log)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           transport.NewHandler(svc, pool.Ping, log),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.HTTPAddr, "bcrypt_cost", cfg.BcryptCost, "jwt_ttl", cfg.JWTTTL)
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
