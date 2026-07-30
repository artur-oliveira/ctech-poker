package walletclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/config"
)

func TestListSandboxSKUs(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-token", "expires_in": 3600})
	})
	mux.HandleFunc("/v1.0/internal/wallet/sandbox-purchase/skus", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "pack_100", "price_cents": 100, "base_credits": 1000, "bonus_percent": 0, "total_credits": 1000},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(&config.Config{WalletURL: srv.URL, CtechURL: srv.URL, PokerClientID: "poker", PokerClientSecret: "secret"}, cache.NewMemoryBackend(10))

	skus, err := c.ListSandboxSKUs(t.Context())
	if err != nil {
		t.Fatalf("ListSandboxSKUs: %v", err)
	}
	if len(skus) != 1 || skus[0].ID != "pack_100" || skus[0].TotalCredits != 1000 {
		t.Fatalf("unexpected skus: %+v", skus)
	}
}

func TestPurchaseSandbox(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-token", "expires_in": 3600})
	})
	mux.HandleFunc("/v1.0/internal/wallet/sandbox-purchase", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"purchase_id": "sbxp#poker#user-1#k1", "sku": "pack_100", "amount": 100,
			"credits_granted": 1000, "status": "pending",
			"pix_copia_e_cola": "00020126...", "qr_code_base64": "iVBORw0...",
			"expires_at": "2026-07-30T12:00:00Z",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(&config.Config{WalletURL: srv.URL, CtechURL: srv.URL, PokerClientID: "poker", PokerClientSecret: "secret"}, cache.NewMemoryBackend(10))

	p, err := c.PurchaseSandbox(t.Context(), "user-1", "pack_100", "k1")
	if err != nil {
		t.Fatalf("PurchaseSandbox: %v", err)
	}
	if p.PurchaseID != "sbxp#poker#user-1#k1" || p.Amount != 100 || p.CreditsGranted != 1000 || p.Status != "pending" {
		t.Fatalf("unexpected purchase: %+v", p)
	}
	if gotBody["user_id"] != "user-1" || gotBody["sku"] != "pack_100" || gotBody["idempotency_key"] != "k1" {
		t.Fatalf("unexpected request body: %+v", gotBody)
	}
}

func TestGetSandboxPurchaseNormalizesAmount(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-token", "expires_in": 3600})
	})
	mux.HandleFunc("/v1.0/internal/wallet/sandbox-purchase/sbxp-1", func(w http.ResponseWriter, r *http.Request) {
		// ctech-wallet's GET/refund responses use amount_expected, not amount.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"purchase_id": "sbxp-1", "user_id": "user-1", "sku": "pack_100",
			"amount_expected": 100, "credits_granted": 1000, "status": "confirmed",
			"expires_at": "2026-07-30T12:00:00Z",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(&config.Config{WalletURL: srv.URL, CtechURL: srv.URL, PokerClientID: "poker", PokerClientSecret: "secret"}, cache.NewMemoryBackend(10))

	p, err := c.GetSandboxPurchase(t.Context(), "sbxp-1")
	if err != nil {
		t.Fatalf("GetSandboxPurchase: %v", err)
	}
	if p.Amount != 100 {
		t.Fatalf("expected normalized Amount=100, got %d", p.Amount)
	}
	if p.UserID != "user-1" || p.Status != "confirmed" {
		t.Fatalf("unexpected purchase: %+v", p)
	}
}

func TestRefundSandboxPurchase(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-token", "expires_in": 3600})
	})
	mux.HandleFunc("/v1.0/internal/wallet/sandbox-purchase/sbxp-1/refund", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"purchase_id": "sbxp-1", "user_id": "user-1", "sku": "pack_100",
			"amount_expected": 100, "credits_granted": 1000, "status": "refunded",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(&config.Config{WalletURL: srv.URL, CtechURL: srv.URL, PokerClientID: "poker", PokerClientSecret: "secret"}, cache.NewMemoryBackend(10))

	p, err := c.RefundSandboxPurchase(t.Context(), "user-1", "sbxp-1", "k2")
	if err != nil {
		t.Fatalf("RefundSandboxPurchase: %v", err)
	}
	if p.Status != "refunded" {
		t.Fatalf("unexpected status: %+v", p)
	}
	if gotBody["user_id"] != "user-1" || gotBody["idempotency_key"] != "k2" {
		t.Fatalf("unexpected request body: %+v", gotBody)
	}
}
