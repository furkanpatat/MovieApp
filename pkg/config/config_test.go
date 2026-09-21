package config

import (
	"os"
	"testing"
)

func TestLoadDefaultsAndRequired(t *testing.T) {
	type cfg struct {
		Base
		RabbitMQ RabbitMQ `envPrefix:"RABBITMQ_"`
	}
	for _, k := range []string{"RABBITMQ_HOST", "RABBITMQ_PORT", "RABBITMQ_USER", "RABBITMQ_PASSWORD", "RABBITMQ_VHOST", "LOG_LEVEL"} {
		t.Setenv(k, "") // registers restore of the caller's value...
		os.Unsetenv(k)  // ...then makes the variable genuinely unset for this test
	}
	if _, err := Load[cfg](); err == nil {
		t.Fatal("expected error for missing required vars")
	}
	t.Setenv("RABBITMQ_USER", "u")
	t.Setenv("RABBITMQ_PASSWORD", "p@ss/word")
	c, err := Load[cfg]()
	if err != nil {
		t.Fatal(err)
	}
	if c.LogLevel != "info" || c.RabbitMQ.Port != 5672 {
		t.Fatalf("bad defaults: %+v", c)
	}
	if got, want := c.RabbitMQ.URL(), "amqp://u:p%40ss%2Fword@localhost:5672/"; got != want {
		t.Fatalf("url = %s, want %s", got, want)
	}
}
