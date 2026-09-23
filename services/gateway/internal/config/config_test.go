package config

import (
	"strings"
	"testing"
	"time"

	sharedcfg "github.com/furkanpatat/movieapp/pkg/config"
)

func valid() Config {
	return Config{
		Base:       sharedcfg.Base{Env: "development"},
		CatalogURL: "http://catalog:8080", InteractionURL: "http://interaction:8080", AuthURL: "http://auth:8080", WatchPartyURL: "http://watchparty:8080",
		JWTSecret:     strings.Repeat("s", 32),
		RateLimit:     RateLimit{Requests: 100, Window: time.Minute},
		AuthRateLimit: AuthRateLimit{Requests: 10, Window: time.Minute},
	}
}

func TestValidate(t *testing.T) {
	if err := valid().Validate(); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*Config){
		"short secret":         func(c *Config) { c.JWTSecret = "short" },
		"zero auth limit":      func(c *Config) { c.AuthRateLimit.Requests = 0 },
		"bad auth url":         func(c *Config) { c.AuthURL = "auth" },
		"bad watchparty url":   func(c *Config) { c.WatchPartyURL = "ws://x" },
		"zero limit":           func(c *Config) { c.RateLimit.Requests = 0 },
		"zero window":          func(c *Config) { c.RateLimit.Window = 0 },
		"bad catalog url":      func(c *Config) { c.CatalogURL = "catalog:8080" },
		"non-http interaction": func(c *Config) { c.InteractionURL = "ftp://x" },
	}
	for name, mut := range cases {
		c := valid()
		mut(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestCORSOrigins(t *testing.T) {
	c := valid()
	c.CORSAllowedOrigins = " https://a.example.com ,https://b.example.com,"
	if got := c.CORSOrigins(); len(got) != 2 || got[0] != "https://a.example.com" || got[1] != "https://b.example.com" {
		t.Fatalf("got %v", got)
	}
	c.CORSAllowedOrigins = ""
	if got := c.CORSOrigins(); got != nil {
		t.Fatalf("empty config should parse to no origins, got %v", got)
	}
}
