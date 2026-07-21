package config

import "testing"

func TestLoadFailsClosedWithoutValkeyURLInProd(t *testing.T) {
	t.Setenv("ENVIRONMENT", "prod")
	t.Setenv("VALKEY_URL", "")

	if _, err := Load(); err == nil {
		t.Fatal("expected Load to fail closed with VALKEY_URL unset in prod")
	}
}

func TestLoadDefaultsToDevWithoutValkeyURL(t *testing.T) {
	t.Setenv("VALKEY_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected Load to succeed in dev without VALKEY_URL, got %v", err)
	}
	if cfg.Port != 8003 {
		t.Fatalf("expected default port 8003, got %d", cfg.Port)
	}
}

func TestLoadParsesTrustedProxyAndCorsLists(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", "10.0.0.0/16,127.0.0.1")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://poker.aoctech.app,https://poker-stage.aoctech.app")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.TrustedProxies) != 2 || len(cfg.CorsAllowedOrigins) != 2 {
		t.Fatalf("proxies=%v origins=%v", cfg.TrustedProxies, cfg.CorsAllowedOrigins)
	}
}

func TestLoadRejectsNonPositiveServerTimeout(t *testing.T) {
	t.Setenv("READ_TIMEOUT", "0")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid timeout to fail")
	}
}

func TestLoadFailsClosedWhenRealMoneyEnabledWithoutLegalSignoff(t *testing.T) {
	t.Setenv("REAL_MONEY_ENABLED", "true")
	t.Setenv("LEGAL_SIGNOFF_REF", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected Load to fail closed: REAL_MONEY_ENABLED=true with no LEGAL_SIGNOFF_REF")
	}
}

func TestLoadSucceedsWhenRealMoneyEnabledWithLegalSignoff(t *testing.T) {
	t.Setenv("REAL_MONEY_ENABLED", "true")
	t.Setenv("LEGAL_SIGNOFF_REF", "LEGAL-2026-001")
	if _, err := Load(); err != nil {
		t.Fatalf("expected Load to succeed with a recorded legal sign-off, got %v", err)
	}
}

func TestLoadSucceedsWithRealMoneyDisabledAndNoSignoff(t *testing.T) {
	t.Setenv("REAL_MONEY_ENABLED", "false")
	t.Setenv("LEGAL_SIGNOFF_REF", "")
	if _, err := Load(); err != nil {
		t.Fatalf("expected Load to succeed with real money disabled, got %v", err)
	}
}

