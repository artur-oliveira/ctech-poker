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
	if cfg.Port != 8010 {
		t.Fatalf("expected default port 8010, got %d", cfg.Port)
	}
}
