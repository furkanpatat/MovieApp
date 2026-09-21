// Package logger builds the structured slog logger shared by all services.
package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Config controls logger construction.
type Config struct {
	Service string // attached to every record as "service"
	Level   string // debug | info | warn | error (default info)
	Format  string // json | text (default json)
	Output  io.Writer
}

// New returns a *slog.Logger. It does not touch the global default; call
// slog.SetDefault yourself if you want that.
func New(cfg Config) *slog.Logger {
	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}
	opts := &slog.HandlerOptions{Level: ParseLevel(cfg.Level)}

	var h slog.Handler
	if strings.EqualFold(cfg.Format, "text") {
		h = slog.NewTextHandler(out, opts)
	} else {
		h = slog.NewJSONHandler(out, opts)
	}

	l := slog.New(h)
	if cfg.Service != "" {
		l = l.With("service", cfg.Service)
	}
	return l
}

// ParseLevel maps a level name to slog.Level, defaulting to info.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
