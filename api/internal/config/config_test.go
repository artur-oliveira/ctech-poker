package config

import (
	"strings"
	"testing"
)

func TestLoadFailsClosedWithoutValkeyURLInProd(t *testing.T) {
	t.Setenv("ENVIRONMENT", "prod")
	t.Setenv("VALKEY_URL", "")

	if _, err := Load(); err == nil {
		t.Fatal("expected Load to fail closed with VALKEY_URL unset in prod")
	}
}

func TestLoadRequiresWalletWebhookHMACSecretInProd(t *testing.T) {
	t.Setenv("ENVIRONMENT", "prod")
	t.Setenv("VALKEY_URL", "redis://x")
	t.Setenv("SERVICE_AUDIENCE", "https://poker.aoctech.app")
	t.Setenv("CTECH_URL", "https://accounts-api.aoctech.app")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://poker.aoctech.app")
	t.Setenv("WALLET_WEBHOOK_HMAC_SECRET", "")

	if _, err := Load(); err == nil {
		t.Fatal("expected an error when WALLET_WEBHOOK_HMAC_SECRET is empty in prod")
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

func TestLoadForLambdaFailsClosedWhenRealMoneyEnabledWithoutLegalSignoff(t *testing.T) {
	t.Setenv("REAL_MONEY_ENABLED", "true")
	t.Setenv("LEGAL_SIGNOFF_REF", "")
	if _, err := LoadForLambda(); err == nil {
		t.Fatal("expected LoadForLambda to fail closed: REAL_MONEY_ENABLED=true with no LEGAL_SIGNOFF_REF")
	}
}

func TestLoadForLambdaSucceedsWhenRealMoneyEnabledWithLegalSignoff(t *testing.T) {
	t.Setenv("REAL_MONEY_ENABLED", "true")
	t.Setenv("LEGAL_SIGNOFF_REF", "LEGAL-2026-001")
	if _, err := LoadForLambda(); err != nil {
		t.Fatalf("expected LoadForLambda to succeed with a recorded legal sign-off, got %v", err)
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

func TestLoadDefaultsSocialGraphToDisabled(t *testing.T) {
	t.Setenv("SOCIAL_GRAPH_ENABLED", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SocialGraphEnabled {
		t.Fatal("expected social graph rollout to default to disabled")
	}
}

func TestLoadParsesSocialGraphEnabled(t *testing.T) {
	t.Setenv("SOCIAL_GRAPH_ENABLED", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SocialGraphEnabled {
		t.Fatal("expected SOCIAL_GRAPH_ENABLED=true to enable the rollout gate")
	}
}

// Prod must never run without the deletion consumer: account would wait for
// poker's ack forever (saga protocol §7).
func TestLoadRequiresErasureWiringInProd(t *testing.T) {
	t.Setenv("ENVIRONMENT", "prod")
	t.Setenv("VALKEY_URL", "redis://localhost:6379")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://poker.aoctech.app")
	t.Setenv("WALLET_WEBHOOK_HMAC_SECRET", "s")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "ERASURE_QUEUE_URL") {
		t.Fatalf("want ERASURE_QUEUE_URL error, got %v", err)
	}
	t.Setenv("ERASURE_QUEUE_URL", "https://sqs.us-east-1.amazonaws.com/1/prod-poker-user-erasure")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "ACTION_LOG_ARCHIVE_BUCKET") {
		t.Fatalf("want ACTION_LOG_ARCHIVE_BUCKET error, got %v", err)
	}
	t.Setenv("ACTION_LOG_ARCHIVE_BUCKET", "poker-action-log-archive-prod")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "ERASURE_ACK_CLIENT_ID") {
		t.Fatalf("want ERASURE_ACK_CLIENT_ID error, got %v", err)
	}
	t.Setenv("ERASURE_ACK_CLIENT_ID", "poker-erasure")
	t.Setenv("ERASURE_ACK_CLIENT_SECRET", "secret")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("complete prod config rejected: %v", err)
	}
	if cfg.ErasureAccountClientID != "accounts" {
		t.Fatalf("ERASURE_ACCOUNT_CLIENT_ID default: %q", cfg.ErasureAccountClientID)
	}
}

func TestPurchasesViaDefaultsToTheWalletAndNeedsBillingSettingsForBilling(t *testing.T) {
	t.Setenv("VALKEY_URL", "")
	cfg, err := Load()
	if err != nil || cfg.PurchasesVia != "wallet" || cfg.BillingSalesOn() {
		t.Fatalf("default = %q err=%v", cfg.PurchasesVia, err)
	}

	t.Setenv("PURCHASES_VIA", "billing")
	if _, err := Load(); err == nil {
		t.Fatal("PURCHASES_VIA=billing without the Billing settings must not start")
	}
	t.Setenv("BILLING_URL", "https://billing-api.aoctech.app")
	t.Setenv("BILLING_CLIENT_ID", "poker-billing")
	t.Setenv("BILLING_CLIENT_SECRET", "s")
	t.Setenv("BILLING_EVENT_SECRET", "e")
	cfg, err = Load()
	if err != nil || !cfg.BillingSalesOn() {
		t.Fatalf("complete settings: on=%v err=%v", cfg != nil && cfg.BillingSalesOn(), err)
	}

	t.Setenv("PURCHASES_VIA", "stripe")
	if _, err := Load(); err == nil {
		t.Fatal("an unknown PURCHASES_VIA must not start")
	}
}
