package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
)

// Config holds the 12-Factor environment configuration for the poker API.
type Config struct {
	AppVersion string `env:"APP_VERSION" envDefault:"0.0.1"`

	Port int    `env:"PORT" envDefault:"8003"`
	Env  string `env:"ENVIRONMENT" envDefault:"dev"`

	ReadTimeout    int64    `env:"READ_TIMEOUT" envDefault:"10"`
	IdleTimeout    int64    `env:"IDLE_TIMEOUT" envDefault:"60"`
	WriteTimeout   int64    `env:"WRITE_TIMEOUT" envDefault:"10"`
	TrustedProxies []string `env:"TRUSTED_PROXIES" envSeparator:","`

	// Cache / table-lease (advisory cache-affinity hint only — never
	// correctness-gating, ARCHITECTURE.md §2). Optional in dev — falls back
	// to an in-memory backend that is NOT shared across replicas.
	RedisURL string `env:"VALKEY_URL"`

	// ctech-account auth (see internal/api/v1/tablews.go) — poker's first
	// user-facing auth surface; mirrors ctech-wallet's config fields exactly.
	CtechURL           string   `env:"CTECH_URL" envDefault:"https://accounts-api.aoctech.app"`
	CtechIssuerURL     string   `env:"CTECH_ISSUER_URL" envDefault:"https://accounts.aoctech.app"`
	CtechJWKSURL       string   `env:"CTECH_JWKS_URL"`
	ServiceAudience    string   `env:"SERVICE_AUDIENCE" envDefault:"https://poker.aoctech.app"`
	CorsAllowedOrigins []string `env:"CORS_ALLOWED_ORIGINS" envSeparator:","`

	// DynamoDB (tablestore) — mirrors ctech-wallet's config fields exactly.
	AWSRegion        string `env:"AWS_REGION" envDefault:"us-east-1"`
	DynamoDBEndpoint string `env:"DYNAMODB_ENDPOINT"` // local override (DynamoDB Local), empty in prod
	AvatarBucket     string `env:"AVATAR_BUCKET"`
	AvatarBaseURL    string `env:"AVATAR_BASE_URL"`

	// ctech-wallet M2M client (sandbox credit/debit — see internal/walletclient).
	// See this plan's Global Constraints: ctech-account must seed this client
	// with scopes internal:wallet:credit and internal:wallet:debit.
	WalletURL         string `env:"WALLET_URL" envDefault:"https://wallet-api.aoctech.app"`
	PokerClientID     string `env:"POKER_CLIENT_ID"`
	PokerClientSecret string `env:"POKER_CLIENT_SECRET"`

	// Adaptive bot checks are disabled when no secret is configured. The
	// public site key belongs to the static frontend build; only this secret
	// is accepted by Cloudflare Siteverify.
	TurnstileSecret           string `env:"TURNSTILE_SECRET"`
	TurnstileExpectedHostname string `env:"TURNSTILE_EXPECTED_HOSTNAME"`

	// Real-money mode gate (Phase 5) — see this plan's Global Constraints.
	// Both fields fail closed together: RealMoneyEnabled=true with no
	// LegalSignoffRef means "an engineer flipped a flag with no recorded
	// business sign-off", which Load refuses to start with, not just warn
	// about — the legal risk here is explicitly bigger than any engineering
	// risk in this codebase (OVERVIEW.md §11).
	RealMoneyEnabled bool   `env:"REAL_MONEY_ENABLED" envDefault:"false"`
	LegalSignoffRef  string `env:"LEGAL_SIGNOFF_REF"`

	// SocialGraphEnabled gates friendship, presence and table invitations for
	// gradual rollout. Player-safety controls remain available independently.
	SocialGraphEnabled bool `env:"SOCIAL_GRAPH_ENABLED" envDefault:"false"`

	// WalletWebhookHMACSecret verifies inbound POST /v1.0/webhooks/wallet calls
	// (X-Wallet-Signature: sha256=<hex>) — must match the secret registered for
	// poker's client_id in ctech-wallet's SSM M2M-clients param (see this
	// plan's Global Constraints — a cross-repo/config blocker, not a code gap).
	WalletWebhookHMACSecret string `env:"WALLET_WEBHOOK_HMAC_SECRET"`
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
	if cfg.ServiceAudience == "" && cfg.Env == "prod" {
		return nil, fmt.Errorf("config: SERVICE_AUDIENCE must be set in production")
	}
	if cfg.CtechURL == "" && cfg.Env == "prod" {
		return nil, fmt.Errorf("config: CTECH_URL must be set in production so the issuer is verified")
	}
	if len(cfg.CorsAllowedOrigins) == 0 && cfg.Env == "prod" {
		// An empty list is the dev wildcard: CORS allows any origin without
		// credentials, and wsAllowedOrigin accepts every WebSocket upgrade.
		// That is a deliberate development convenience and must never be what
		// production silently falls back to when the env file is incomplete.
		return nil, fmt.Errorf("config: CORS_ALLOWED_ORIGINS must be set in production — an empty list allows every origin")
	}
	if cfg.RealMoneyEnabled && cfg.LegalSignoffRef == "" {
		return nil, fmt.Errorf("config: REAL_MONEY_ENABLED=true requires a non-empty LEGAL_SIGNOFF_REF (OVERVIEW.md §11 — this is a business decision, not an engineering toggle)")
	}
	if cfg.WalletWebhookHMACSecret == "" && cfg.Env == "prod" {
		return nil, fmt.Errorf("config: WALLET_WEBHOOK_HMAC_SECRET must be set in production so wallet webhooks can be verified")
	}
	if cfg.ReadTimeout <= 0 || cfg.WriteTimeout <= 0 || cfg.IdleTimeout <= 0 {
		return nil, fmt.Errorf("config: server timeouts must be positive")
	}
	if cfg.CtechJWKSURL == "" && cfg.CtechURL != "" {
		cfg.CtechJWKSURL = cfg.CtechURL + "/.well-known/jwks.json"
	}
	return cfg, nil
}

func LoadForLambda() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if cfg.CtechURL == "" && cfg.Env == "prod" {
		return nil, fmt.Errorf("config: CTECH_URL must be set in production so the issuer is verified")
	}
	if cfg.CtechJWKSURL == "" && cfg.CtechURL != "" {
		cfg.CtechJWKSURL = cfg.CtechURL + "/.well-known/jwks.json"
	}
	return cfg, nil
}
