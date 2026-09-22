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

	sharedcfg "github.com/furkanpatat/movieapp/pkg/config"
	"github.com/furkanpatat/movieapp/pkg/logger"
	"github.com/furkanpatat/movieapp/services/watchparty/internal/config"
	"github.com/furkanpatat/movieapp/services/watchparty/internal/hub"
	"github.com/furkanpatat/movieapp/services/watchparty/internal/transport"
)

func main() {
	if err := run(); err != nil {
		slog.Error("watchparty exited", "error", err)
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
		cfg.ServiceName = "watchparty"
	}
	log := logger.New(logger.Config{Service: cfg.ServiceName, Level: cfg.LogLevel, Format: cfg.LogFormat})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	h := hub.New(hub.Options{
		MaxMessageBytes: cfg.MaxMessageBytes, MaxTextRunes: cfg.MaxTextRunes, SendBuffer: cfg.SendBuffer,
		WriteWait: cfg.WriteWait, PongWait: cfg.PongWait, PingInterval: cfg.PingInterval,
		MsgRate: cfg.MsgRate, MsgBurst: cfg.MsgBurst, MaxRoomClients: cfg.MaxRoomClients, Logger: log,
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           transport.NewHandler(h, cfg.Origins(), log),
		ReadHeaderTimeout: 5 * time.Second, // WebSocket connections are hijacked, so no Read/WriteTimeout
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.HTTPAddr, "origins", cfg.AllowedOrigins)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
		// Hijacked WebSocket connections are not covered by srv.Shutdown; the
		// hub closes them itself (close frame 1001) and we wait for their goroutines.
		h.Close()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = h.Wait(sctx)
		if err := srv.Shutdown(sctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}
