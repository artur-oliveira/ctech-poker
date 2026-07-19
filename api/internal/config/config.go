package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
)

// Config holds the 12-Factor environment configuration for the poker API.
type Config struct {
	AppVersion string `env:"APP_VERSION" envDefault:"0.0.1"`
	Port       int    `env:"PORT" envDefault:"8010"`
	Env        string `env:"ENVIRONMENT" envDefault:"dev"`

	ReadTimeout  int64 `env:"READ_TIMEOUT" envDefault:"10"`
	IdleTimeout  int64 `env:"IDLE_TIMEOUT" envDefault:"60"`
	WriteTimeout int64 `env:"WRITE_TIMEOUT" envDefault:"10"`

	// Cache / table-lease (see Task 4). Optional in dev — falls back to an
	// in-memory backend that is NOT shared across replicas.
	RedisURL string `env:"VALKEY_URL"`

	// InstancePrivateIP is this instance's own address, advertised via
	// tableowner.Registry so sibling instances can proxy WebSocket traffic
	// for tables this instance owns (see internal/tablemanager/manager.go).
	InstancePrivateIP string `env:"INSTANCE_PRIVATE_IP" envDefault:"127.0.0.1"`

	// ctech-account auth (see internal/api/v1/tablews.go) — poker's first
	// user-facing auth surface; mirrors ctech-wallet's config fields exactly.
	CtechURL           string   `env:"CTECH_URL"`
	CtechJWKSURL       string   `env:"CTECH_JWKS_URL"`
	ServiceAudience    string   `env:"SERVICE_AUDIENCE" envDefault:"poker"`
	CorsAllowedOrigins []string `env:"CORS_ALLOWED_ORIGINS" envSeparator:","`
}

// Load reads config from environment variables.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if cfg.RedisURL == "" && cfg.Env == "prod" {
		// Fail closed: an empty VALKEY_URL in prod means the table-lease
		// service silently degrades to an in-memory store that is NOT shared
		// across the ASG's other instances — table-authority (single-writer
		// per table) stops holding fleet-wide with no signal.
		return nil, fmt.Errorf("config: VALKEY_URL must be set in production so table leases are fleet-shared")
	}
	return cfg, nil
}
