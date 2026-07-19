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

	// Cache / table-lease (advisory cache-affinity hint only — never
	// correctness-gating, ARCHITECTURE.md §2). Optional in dev — falls back
	// to an in-memory backend that is NOT shared across replicas.
	RedisURL string `env:"VALKEY_URL"`

	// ctech-account auth (see internal/api/v1/tablews.go) — poker's first
	// user-facing auth surface; mirrors ctech-wallet's config fields exactly.
	CtechURL           string   `env:"CTECH_URL"`
	CtechJWKSURL       string   `env:"CTECH_JWKS_URL"`
	ServiceAudience    string   `env:"SERVICE_AUDIENCE" envDefault:"poker"`
	CorsAllowedOrigins []string `env:"CORS_ALLOWED_ORIGINS" envSeparator:","`

	// DynamoDB (tablestore) — mirrors ctech-wallet's config fields exactly.
	AWSRegion        string `env:"AWS_REGION" envDefault:"us-east-1"`
	DynamoDBEndpoint string `env:"DYNAMODB_ENDPOINT"` // local override (DynamoDB Local), empty in prod

	// ctech-wallet M2M client (sandbox credit/debit — see internal/walletclient).
	// See this plan's Global Constraints: ctech-account must seed this client
	// with scopes internal:wallet:credit and internal:wallet:debit.
	WalletURL         string `env:"WALLET_URL"`
	PokerClientID     string `env:"POKER_CLIENT_ID"`
	PokerClientSecret string `env:"POKER_CLIENT_SECRET"`
}

// Load reads config from environment variables.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if cfg.RedisURL == "" && cfg.Env == "prod" {
		// Fail closed: an empty VALKEY_URL in prod means ws.Registry silently
		// degrades to an in-memory, single-instance fan-out — two players at
		// the same table on different ASG instances would stop seeing each
		// other's broadcasts with no signal. (tablelease itself has no such
		// requirement under ARCHITECTURE.md §2's revised model — it is an
		// advisory cache-affinity hint, never correctness-gating.)
		return nil, fmt.Errorf("config: VALKEY_URL must be set in production so table leases are fleet-shared")
	}
	return cfg, nil
}
