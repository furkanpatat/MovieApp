// Package config defines the Interaction service configuration.
package config

import (
	"time"

	sharedcfg "github.com/furkanpatat/movieapp/pkg/config"
)

type Config struct {
	sharedcfg.Base
	Postgres    sharedcfg.Postgres `envPrefix:"POSTGRES_"`
	Redis       sharedcfg.Redis    `envPrefix:"REDIS_"`
	RabbitMQ    sharedcfg.RabbitMQ `envPrefix:"RABBITMQ_"`
	Interaction Interaction        `envPrefix:"INTERACTION_"`
	Outbox      Outbox             `envPrefix:"OUTBOX_"`
}

type Interaction struct {
	RecentComments   int  `env:"RECENT_COMMENTS" envDefault:"20"`   // comments kept in the read model
	ConsumerPrefetch int  `env:"CONSUMER_PREFETCH" envDefault:"20"` // unacked messages per consumer
	MaxRetries       int  `env:"MAX_RETRIES" envDefault:"3"`        // before dead-lettering
	APIEnabled       bool `env:"API_ENABLED" envDefault:"true"`
	WorkerEnabled    bool `env:"WORKER_ENABLED" envDefault:"true"`
}

// Outbox configures the relay that publishes stored events to RabbitMQ.
type Outbox struct {
	RelayEnabled   bool          `env:"RELAY_ENABLED" envDefault:"true"`
	PollInterval   time.Duration `env:"POLL_INTERVAL" envDefault:"1s"`
	BatchSize      int           `env:"BATCH_SIZE" envDefault:"100"`
	PublishTimeout time.Duration `env:"PUBLISH_TIMEOUT" envDefault:"5s"`
	Retention      time.Duration `env:"RETENTION" envDefault:"24h"` // published rows are deleted after this
}
