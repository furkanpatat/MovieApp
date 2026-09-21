// Package config defines the Catalog service configuration.
package config

import (
	"time"

	sharedcfg "github.com/furkanpatat/movieapp/pkg/config"
)

// Config is loaded from environment variables.
type Config struct {
	sharedcfg.Base
	TMDB    TMDB            `envPrefix:"TMDB_"`
	Redis   sharedcfg.Redis `envPrefix:"REDIS_"`
	Cache   Cache           `envPrefix:"CACHE_"`
	Refresh Refresh         `envPrefix:"REFRESH_"`
}

// TMDB holds the external API client and circuit breaker settings.
type TMDB struct {
	APIKey          string        `env:"API_KEY,notEmpty"`
	BaseURL         string        `env:"BASE_URL" envDefault:"https://api.themoviedb.org/3"`
	Timeout         time.Duration `env:"TIMEOUT" envDefault:"5s"`
	BreakerFailures uint32        `env:"BREAKER_FAILURES" envDefault:"5"`   // consecutive failures that open the circuit
	BreakerOpenFor  time.Duration `env:"BREAKER_OPEN_FOR" envDefault:"30s"` // how long it stays open before probing
}

// Cache holds cache-aside TTLs.
type Cache struct {
	TTL      time.Duration `env:"TTL" envDefault:"1h"`        // fresh window
	StaleTTL time.Duration `env:"STALE_TTL" envDefault:"24h"` // fallback copy served when TMDB is down
}

// Refresh configures the background cache warm-up worker.
type Refresh struct {
	Enabled     bool          `env:"ENABLED" envDefault:"true"`
	Interval    time.Duration `env:"INTERVAL" envDefault:"30m"` // keep below Cache.TTL so entries never go cold
	Pages       int           `env:"PAGES" envDefault:"3"`      // 20 movies per page
	WarmDetails bool          `env:"WARM_DETAILS" envDefault:"true"`
	Pause       time.Duration `env:"PAUSE" envDefault:"100ms"` // between TMDB calls
}
