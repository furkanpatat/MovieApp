// Package config defines the API Gateway configuration.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	sharedcfg "github.com/furkanpatat/movieapp/pkg/config"
)

type Config struct {
	sharedcfg.Base
	Redis sharedcfg.Redis `envPrefix:"REDIS_"`

	CatalogURL     string `env:"CATALOG_URL" envDefault:"http://localhost:8080"`
	InteractionURL string `env:"INTERACTION_URL" envDefault:"http://localhost:8081"`

	AuthURL       string `env:"AUTH_URL" envDefault:"http://localhost:8082"`
	WatchPartyURL string `env:"WATCHPARTY_URL" envDefault:"http://localhost:8083"`

	// The gateway only VERIFIES tokens; the Auth service issues them. Both must
	// be configured with the same JWT_SECRET and JWT_ISSUER.
	JWTSecret string `env:"JWT_SECRET,notEmpty"`
	JWTIssuer string `env:"JWT_ISSUER" envDefault:"movieapp-auth"`

	RateLimit RateLimit `envPrefix:"RATE_LIMIT_"`
	// AuthRateLimit is the stricter per-IP budget for /api/v1/auth/* (brute force, signup spam).
	AuthRateLimit AuthRateLimit `envPrefix:"AUTH_RATE_LIMIT_"`

	UpstreamTimeout time.Duration `env:"UPSTREAM_TIMEOUT" envDefault:"10s"`

	// CORSAllowedOrigins: comma-separated browser origins allowed to call this
	// API (e.g. "https://app.example.com"). "*" allows any. Requests with no
	// Origin header (server-to-server, curl) are never subject to CORS at
	// all — this only affects what a browser will let its own JS read.
	CORSAllowedOrigins string `env:"CORS_ALLOWED_ORIGINS" envDefault:"http://localhost:3000"`
}

type RateLimit struct {
	Requests int           `env:"REQUESTS" envDefault:"100"`
	Window   time.Duration `env:"WINDOW" envDefault:"1m"`
	// FailOpen: keep serving when Redis is unavailable (true), or reject with 503 (false).
	FailOpen bool `env:"FAIL_OPEN" envDefault:"true"`
	// TrustForwardedFor: derive the client IP from X-Forwarded-For (set only
	// behind a proxy you control; otherwise clients can spoof their IP).
	TrustForwardedFor bool `env:"TRUST_FORWARDED_FOR" envDefault:"false"`
}

type AuthRateLimit struct {
	Requests int           `env:"REQUESTS" envDefault:"10"`
	Window   time.Duration `env:"WINDOW" envDefault:"1m"`
}

const MinSecretLen = 32

// Validate rejects unsafe or unusable settings at startup.
func (c Config) Validate() error {
	var errs []error
	if len(c.JWTSecret) < MinSecretLen {
		errs = append(errs, fmt.Errorf("JWT_SECRET must be at least %d characters", MinSecretLen))
	}
	if c.RateLimit.Requests < 1 || c.RateLimit.Window <= 0 {
		errs = append(errs, errors.New("RATE_LIMIT_REQUESTS and RATE_LIMIT_WINDOW must be positive"))
	}
	if c.AuthRateLimit.Requests < 1 || c.AuthRateLimit.Window <= 0 {
		errs = append(errs, errors.New("AUTH_RATE_LIMIT_REQUESTS and AUTH_RATE_LIMIT_WINDOW must be positive"))
	}
	for name, raw := range map[string]string{"CATALOG_URL": c.CatalogURL, "INTERACTION_URL": c.InteractionURL, "AUTH_URL": c.AuthURL, "WATCHPARTY_URL": c.WatchPartyURL} {
		if u, err := url.Parse(raw); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs = append(errs, fmt.Errorf("%s must be an absolute http(s) URL", name))
		}
	}
	return errors.Join(errs...)
}

// CORSOrigins returns the parsed allow-list.
func (c Config) CORSOrigins() []string {
	var out []string
	for _, o := range strings.Split(c.CORSAllowedOrigins, ",") {
		if o = strings.TrimSpace(o); o != "" {
			out = append(out, o)
		}
	}
	return out
}
