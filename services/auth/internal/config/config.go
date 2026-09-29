// Package config defines the Auth service configuration.
package config

import (
	"errors"
	"fmt"
	"time"

	sharedcfg "github.com/furkanpatat/movieapp/pkg/config"
)

type Config struct {
	sharedcfg.Base
	Postgres sharedcfg.Postgres `envPrefix:"POSTGRES_"`

	// Must match the gateway's JWT_SECRET and JWT_ISSUER: this service signs
	// the tokens the gateway verifies.
	JWTSecret string        `env:"JWT_SECRET,notEmpty"`
	JWTIssuer string        `env:"JWT_ISSUER" envDefault:"movieapp-auth"`
	JWTTTL    time.Duration `env:"JWT_TTL" envDefault:"1h"`
	// RefreshTTL is how long a refresh token (API clients' long session)
	// lasts; each refresh rotates it and starts the clock again.
	RefreshTTL time.Duration `env:"REFRESH_TTL" envDefault:"720h"`

	// BcryptCost is the work factor; each +1 doubles the time to hash.
	BcryptCost int `env:"BCRYPT_COST" envDefault:"12"`

	// CookieSecure marks the session cookie Secure (HTTPS only). Only local
	// plain-http development should turn it off; production refuses to.
	CookieSecure bool `env:"AUTH_COOKIE_SECURE" envDefault:"true"`
}

const (
	MinSecretLen  = 32
	MinProdCost   = 10
	maxBcryptCost = 16
)

func (c Config) Validate() error {
	var errs []error
	if len(c.JWTSecret) < MinSecretLen {
		errs = append(errs, fmt.Errorf("JWT_SECRET must be at least %d characters", MinSecretLen))
	}
	if c.JWTIssuer == "" {
		errs = append(errs, errors.New("JWT_ISSUER must not be empty"))
	}
	if c.RefreshTTL <= 0 {
		errs = append(errs, errors.New("REFRESH_TTL must be positive"))
	}
	if c.JWTTTL <= 0 {
		errs = append(errs, errors.New("JWT_TTL must be positive"))
	}
	if c.BcryptCost < 4 || c.BcryptCost > maxBcryptCost {
		errs = append(errs, fmt.Errorf("BCRYPT_COST must be between 4 and %d", maxBcryptCost))
	}
	if c.Env == "production" && c.BcryptCost < MinProdCost {
		errs = append(errs, fmt.Errorf("BCRYPT_COST must be at least %d when APP_ENV=production", MinProdCost))
	}
	if c.Env == "production" && !c.CookieSecure {
		errs = append(errs, errors.New("AUTH_COOKIE_SECURE must be true when APP_ENV=production"))
	}
	return errors.Join(errs...)
}
