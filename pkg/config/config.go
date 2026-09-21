// Package config loads service configuration from environment variables.
//
// Each service defines its own struct, embedding the shared sections it needs:
//
//	type Config struct {
//		config.Base
//		RabbitMQ config.RabbitMQ `envPrefix:"RABBITMQ_"`
//		Redis    config.Redis    `envPrefix:"REDIS_"`
//		TMDBKey  string          `env:"TMDB_API_KEY,required"`
//	}
//	cfg, err := config.Load[Config]()
package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
)

// Load parses environment variables into a T. Missing `required` fields are
// reported together in a single error.
func Load[T any]() (T, error) {
	var cfg T
	if err := env.Parse(&cfg); err != nil {
		return cfg, fmt.Errorf("load config: %w", err)
	}
	return cfg, nil
}

// MustLoad is Load that panics; intended for main() only.
func MustLoad[T any]() T {
	cfg, err := Load[T]()
	if err != nil {
		panic(err)
	}
	return cfg
}

// Base holds settings every service needs.
type Base struct {
	ServiceName string `env:"SERVICE_NAME"`
	Env         string `env:"APP_ENV" envDefault:"development"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`
	LogFormat   string `env:"LOG_FORMAT" envDefault:"json"`
	HTTPAddr    string `env:"HTTP_ADDR" envDefault:":8080"`
}

// RabbitMQ connection settings (use with envPrefix:"RABBITMQ_").
type RabbitMQ struct {
	Host     string `env:"HOST" envDefault:"localhost"`
	Port     int    `env:"PORT" envDefault:"5672"`
	User     string `env:"USER,required"`
	Password string `env:"PASSWORD,required"`
	VHost    string `env:"VHOST" envDefault:"/"`
}

// Postgres connection settings (use with envPrefix:"POSTGRES_").
type Postgres struct {
	Host     string `env:"HOST" envDefault:"localhost"`
	Port     int    `env:"PORT" envDefault:"5432"`
	User     string `env:"USER,required"`
	Password string `env:"PASSWORD,required"`
	DB       string `env:"DB,required"`
	SSLMode  string `env:"SSLMODE" envDefault:"disable"`
}

// DSN returns a libpq-style connection URL.
func (p Postgres) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		urlEscape(p.User), urlEscape(p.Password), p.Host, p.Port, p.DB, p.SSLMode)
}

// Redis connection settings (use with envPrefix:"REDIS_").
type Redis struct {
	Host     string `env:"HOST" envDefault:"localhost"`
	Port     int    `env:"PORT" envDefault:"6379"`
	Password string `env:"PASSWORD"`
	DB       int    `env:"DB" envDefault:"0"`
}

// Addr returns host:port.
func (r Redis) Addr() string { return fmt.Sprintf("%s:%d", r.Host, r.Port) }
