package walletclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/config"
)

func TestHoldAndRelease(t *testing.T) {
	var holdCalled bool
	var releaseCalled bool
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-token", "expires_in": 3600})
	})
	mux.HandleFunc("/v1.0/internal/wallet/holds", func(w http.ResponseWriter, r *http.Request) {
		holdCalled = true
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "hold-123"})
	})
	mux.HandleFunc("/v1.0/internal/wallet/holds/hold-123/release", func(w http.ResponseWriter, r *http.Request) {
		releaseCalled = true
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(&config.Config{WalletURL: srv.URL, CtechURL: srv.URL, PokerClientID: "poker", PokerClientSecret: "secret"}, cache.NewMemoryBackend(10))

	id, err := c.HoldGame(t.Context(), "user-1", 500, "table-1", "k1", "buyin")
	if err != nil || id != "hold-123" {
		t.Fatalf("HoldGame failed: id=%s err=%v", id, err)
	}
	if !holdCalled {
		t.Fatal("expected HoldGame to hit endpoint")
	}

	if err := c.ReleaseHold(t.Context(), "user-1", "hold-123", "idem-1"); err != nil {
		t.Fatalf("ReleaseHold failed: %v", err)
	}
	if !releaseCalled {
		t.Fatal("expected ReleaseHold to hit endpoint")
	}
}

func TestCashoutGame(t *testing.T) {
	var cashoutCalled bool
	var cashoutBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-token", "expires_in": 3600})
	})
	mux.HandleFunc("/v1.0/internal/wallet/holds/cashout", func(w http.ResponseWriter, r *http.Request) {
		cashoutCalled = true
		_ = json.NewDecoder(r.Body).Decode(&cashoutBody)
		w.WriteHeader(http.StatusCreated)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(&config.Config{WalletURL: srv.URL, CtechURL: srv.URL, PokerClientID: "poker", PokerClientSecret: "secret"}, cache.NewMemoryBackend(10))

	if err := c.CashoutGame(t.Context(), "user-1", 500, "table-1", []string{"hold-123"}, "k2", "cashout", "Mesa #table-1, saída"); err != nil {
		t.Fatalf("CashoutGame failed: %v", err)
	}
	if !cashoutCalled {
		t.Fatal("expected CashoutGame to hit endpoint")
	}
	if cashoutBody["description"] != "Mesa #table-1, saída" {
		t.Fatalf("description on the wire = %v", cashoutBody["description"])
	}
}
