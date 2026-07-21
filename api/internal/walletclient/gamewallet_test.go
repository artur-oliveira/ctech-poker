package walletclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/config"
)

func TestIsGamblingActivated(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-token", "expires_in": 3600})
	})
	mux.HandleFunc("/v1.0/internal/wallet/game/status/user-1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"activated": true})
	})
	mux.HandleFunc("/v1.0/internal/wallet/game/status/user-2", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"activated": false})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(&config.Config{WalletURL: srv.URL, CtechURL: srv.URL, PokerClientID: "poker", PokerClientSecret: "secret"})

	t.Run("activated", func(t *testing.T) {
		ok, err := c.IsGamblingActivated(t.Context(), "user-1")
		if err != nil || !ok {
			t.Fatalf("expected activated=true, got ok=%v err=%v", ok, err)
		}
	})
	t.Run("not activated", func(t *testing.T) {
		ok, err := c.IsGamblingActivated(t.Context(), "user-2")
		if err != nil || ok {
			t.Fatalf("expected activated=false, got ok=%v err=%v", ok, err)
		}
	})
}

func TestHoldAndRelease(t *testing.T) {
	var holdCalled bool
	var releaseCalled bool
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-token", "expires_in": 3600})
	})
	mux.HandleFunc("/v1.0/internal/wallet/game/hold", func(w http.ResponseWriter, r *http.Request) {
		holdCalled = true
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "hold-123"})
	})
	mux.HandleFunc("/v1.0/internal/wallet/game/hold/hold-123/release", func(w http.ResponseWriter, r *http.Request) {
		releaseCalled = true
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(&config.Config{WalletURL: srv.URL, CtechURL: srv.URL, PokerClientID: "poker", PokerClientSecret: "secret"})

	id, err := c.HoldGame(t.Context(), "user-1", 500, "k1", "buyin")
	if err != nil || id != "hold-123" {
		t.Fatalf("HoldGame failed: id=%s err=%v", id, err)
	}
	if !holdCalled {
		t.Fatal("expected HoldGame to hit endpoint")
	}

	if err := c.ReleaseHold(t.Context(), "hold-123"); err != nil {
		t.Fatalf("ReleaseHold failed: %v", err)
	}
	if !releaseCalled {
		t.Fatal("expected ReleaseHold to hit endpoint")
	}
}

func TestCashoutGame(t *testing.T) {
	var cashoutCalled bool
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-token", "expires_in": 3600})
	})
	mux.HandleFunc("/v1.0/internal/wallet/game/cashout", func(w http.ResponseWriter, r *http.Request) {
		cashoutCalled = true
		w.WriteHeader(http.StatusCreated)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(&config.Config{WalletURL: srv.URL, CtechURL: srv.URL, PokerClientID: "poker", PokerClientSecret: "secret"})

	if err := c.CashoutGame(t.Context(), "user-1", "hold-123", "k2", "cashout"); err != nil {
		t.Fatalf("CashoutGame failed: %v", err)
	}
	if !cashoutCalled {
		t.Fatal("expected CashoutGame to hit endpoint")
	}
}
