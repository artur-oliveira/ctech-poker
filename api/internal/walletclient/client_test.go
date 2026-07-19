package walletclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/config"
)

func fakeWalletServer(t *testing.T, onMovement func(path string, body MovementRequest)) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-token", "expires_in": 3600})
	})
	mux.HandleFunc("/v1.0/internal/wallet/sandbox/credit", func(w http.ResponseWriter, r *http.Request) {
		var body MovementRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		onMovement(r.URL.Path, body)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "entry-1"})
	})
	mux.HandleFunc("/v1.0/internal/wallet/sandbox/debit", func(w http.ResponseWriter, r *http.Request) {
		var body MovementRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		onMovement(r.URL.Path, body)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "entry-2"})
	})
	return httptest.NewServer(mux)
}

func TestCreditSendsExpectedRequestBody(t *testing.T) {
	var gotPath string
	var gotBody MovementRequest
	srv := fakeWalletServer(t, func(path string, body MovementRequest) {
		gotPath, gotBody = path, body
	})
	defer srv.Close()

	c := New(&config.Config{WalletURL: srv.URL, PokerClientID: "poker", PokerClientSecret: "secret"})
	if err := c.Credit(t.Context(), "user-1", 500, "room-1#user-1#buyin-1", "buyin"); err != nil {
		t.Fatalf("credit: %v", err)
	}
	if gotPath != "/v1.0/internal/wallet/sandbox/credit" {
		t.Fatalf("expected credit endpoint, got %s", gotPath)
	}
	if gotBody.UserID != "user-1" || gotBody.Amount != 500 || gotBody.IdempotencyKey != "room-1#user-1#buyin-1" {
		t.Fatalf("unexpected request body: %+v", gotBody)
	}
}

func TestDebitSendsExpectedRequestBody(t *testing.T) {
	var gotPath string
	srv := fakeWalletServer(t, func(path string, body MovementRequest) { gotPath = path })
	defer srv.Close()

	c := New(&config.Config{WalletURL: srv.URL, PokerClientID: "poker", PokerClientSecret: "secret"})
	if err := c.Debit(t.Context(), "user-1", 500, "room-1#user-1#buyin-1", "buyin"); err != nil {
		t.Fatalf("debit: %v", err)
	}
	if gotPath != "/v1.0/internal/wallet/sandbox/debit" {
		t.Fatalf("expected debit endpoint, got %s", gotPath)
	}
}
