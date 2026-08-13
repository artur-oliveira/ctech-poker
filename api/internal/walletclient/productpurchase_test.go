package walletclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/config"
)

func TestPurchaseProductRoundTrip(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-token", "expires_in": 3600})
	})
	mux.HandleFunc("/v1.0/internal/wallet/product-purchase", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"purchase_id": "prdp-1", "sku": "poker_reaction_cold", "amount": 100, "status": "pending",
			"pix_copia_e_cola": "copia", "qr_code_base64": "qr",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(&config.Config{WalletURL: srv.URL, CtechURL: srv.URL, PokerClientID: "poker", PokerClientSecret: "secret"}, cache.NewMemoryBackend(10))

	p, err := c.PurchaseProduct(t.Context(), "user-1", "poker_reaction_cold", "idem-1")
	if err != nil {
		t.Fatalf("PurchaseProduct: %v", err)
	}
	if p.PurchaseID != "prdp-1" || p.Amount != 100 {
		t.Fatalf("unexpected purchase: %+v", p)
	}
	if gotBody["user_id"] != "user-1" || gotBody["sku"] != "poker_reaction_cold" || gotBody["idempotency_key"] != "idem-1" {
		t.Fatalf("unexpected request body: %+v", gotBody)
	}
}

func TestListProductSKUs(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-token", "expires_in": 3600})
	})
	mux.HandleFunc("/v1.0/internal/wallet/product-purchase/skus", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "poker_reaction_cold", "price_cents": 100}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(&config.Config{WalletURL: srv.URL, CtechURL: srv.URL, PokerClientID: "poker", PokerClientSecret: "secret"}, cache.NewMemoryBackend(10))

	skus, err := c.ListProductSKUs(t.Context())
	if err != nil {
		t.Fatalf("ListProductSKUs: %v", err)
	}
	if len(skus) != 1 || skus[0].ID != "poker_reaction_cold" || skus[0].PriceCents != 100 {
		t.Fatalf("unexpected skus: %+v", skus)
	}
}

func TestGetProductPurchaseNormalizesAmount(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-token", "expires_in": 3600})
	})
	mux.HandleFunc("/v1.0/internal/wallet/product-purchase/prdp-1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"purchase_id": "prdp-1", "user_id": "user-1", "sku": "poker_reaction_cold",
			"amount_expected": 100, "status": "confirmed",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(&config.Config{WalletURL: srv.URL, CtechURL: srv.URL, PokerClientID: "poker", PokerClientSecret: "secret"}, cache.NewMemoryBackend(10))

	p, err := c.GetProductPurchase(t.Context(), "prdp-1")
	if err != nil {
		t.Fatalf("GetProductPurchase: %v", err)
	}
	if p.Amount != 100 || p.UserID != "user-1" || p.Status != "confirmed" {
		t.Fatalf("unexpected purchase: %+v", p)
	}
}

func TestRefundProductPurchase(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-token", "expires_in": 3600})
	})
	mux.HandleFunc("/v1.0/internal/wallet/product-purchase/prdp-1/refund", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"purchase_id": "prdp-1", "user_id": "user-1", "sku": "poker_reaction_cold",
			"amount_expected": 100, "status": "refunded",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(&config.Config{WalletURL: srv.URL, CtechURL: srv.URL, PokerClientID: "poker", PokerClientSecret: "secret"}, cache.NewMemoryBackend(10))

	p, err := c.RefundProductPurchase(t.Context(), "user-1", "prdp-1", "idem-refund-1")
	if err != nil {
		t.Fatalf("RefundProductPurchase: %v", err)
	}
	if p.Status != "refunded" {
		t.Fatalf("unexpected status: %+v", p)
	}
	if gotBody["user_id"] != "user-1" || gotBody["idempotency_key"] != "idem-refund-1" {
		t.Fatalf("unexpected request body: %+v", gotBody)
	}
}
