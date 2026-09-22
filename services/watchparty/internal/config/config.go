// Package config defines the Watch-Party service configuration.
package config

import (
	"errors"
	"strings"
	"time"

	sharedcfg "github.com/furkanpatat/movieapp/pkg/config"
)

type Config struct {
	sharedcfg.Base

	// AllowedOrigins: comma-separated browser origins allowed to open sockets,
	// e.g. "https://app.example.com". "*" allows any. Requests without an
	// Origin header (non-browser clients) are always allowed. Authentication is
	// by JWT at the gateway, not by cookies, so a cross-site page cannot ride
	// on the user's session.
	AllowedOrigins string `env:"WS_ALLOWED_ORIGINS" envDefault:"*"`

	MaxMessageBytes int64         `env:"WS_MAX_MESSAGE_BYTES" envDefault:"4096"`
	MaxTextRunes    int           `env:"WS_MAX_TEXT_CHARS" envDefault:"500"`
	SendBuffer      int           `env:"WS_SEND_BUFFER" envDefault:"64"`
	WriteWait       time.Duration `env:"WS_WRITE_WAIT" envDefault:"10s"`
	PongWait        time.Duration `env:"WS_PONG_WAIT" envDefault:"60s"`
	PingInterval    time.Duration `env:"WS_PING_INTERVAL" envDefault:"30s"`
	MsgRate         float64       `env:"WS_MSG_RATE" envDefault:"20"`
	MsgBurst        int           `env:"WS_MSG_BURST" envDefault:"40"`
	MaxRoomClients  int           `env:"WS_MAX_ROOM_CLIENTS" envDefault:"100"`
}

func (c Config) Validate() error {
	var errs []error
	if c.PingInterval >= c.PongWait {
		errs = append(errs, errors.New("WS_PING_INTERVAL must be shorter than WS_PONG_WAIT"))
	}
	if c.MaxMessageBytes < 64 || c.MsgRate <= 0 || c.MsgBurst < 1 || c.SendBuffer < 8 {
		errs = append(errs, errors.New("WS_MAX_MESSAGE_BYTES>=64, WS_MSG_RATE>0, WS_MSG_BURST>=1 and WS_SEND_BUFFER>=8 are required"))
	}
	return errors.Join(errs...)
}

// Origins returns the parsed allow-list.
func (c Config) Origins() []string {
	var out []string
	for _, o := range strings.Split(c.AllowedOrigins, ",") {
		if o = strings.TrimSpace(o); o != "" {
			out = append(out, o)
		}
	}
	return out
}
