package walletclient

import (
	"encoding/json"
	"errors"
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

func fakeAuthServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-token", "expires_in": 3600})
	})
	return httptest.NewServer(mux)
}

func TestCreditSendsExpectedRequestBody(t *testing.T) {
	var gotPath string
	var gotBody MovementRequest
	srv := fakeWalletServer(t, func(path string, body MovementRequest) {
		gotPath, gotBody = path, body
	})
	authSrv := fakeAuthServer(t)
	defer srv.Close()
	defer authSrv.Close()

	c := New(&config.Config{WalletURL: srv.URL, CtechURL: authSrv.URL, PokerClientID: "poker", PokerClientSecret: "secret"})
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
	authSrv := fakeAuthServer(t)
	defer srv.Close()
	defer authSrv.Close()

	c := New(&config.Config{WalletURL: srv.URL, CtechURL: authSrv.URL, PokerClientID: "poker", PokerClientSecret: "secret"})
	if err := c.Debit(t.Context(), "user-1", 500, "room-1#user-1#buyin-1", "buyin"); err != nil {
		t.Fatalf("debit: %v", err)
	}
	if gotPath != "/v1.0/internal/wallet/sandbox/debit" {
		t.Fatalf("expected debit endpoint, got %s", gotPath)
	}
}

func TestCreditPassesThroughWalletProblemJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-token", "expires_in": 3600})
	})
	mux.HandleFunc("/v1.0/internal/wallet/sandbox/credit", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "/problems/wallet-not-found", "title": "Wallet Not Found",
			"status": http.StatusNotFound, "detail": "no sandbox wallet for user",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(&config.Config{WalletURL: srv.URL, CtechURL: srv.URL, PokerClientID: "poker", PokerClientSecret: "secret"})
	err := c.Credit(t.Context(), "user-1", 500, "key-1", "daily_reward")

	var werr *Error
	if !errors.As(err, &werr) {
		t.Fatalf("expected *Error, got %v (%T)", err, err)
	}
	if werr.Status != http.StatusNotFound || werr.Type != "/problems/wallet-not-found" || werr.Detail != "no sandbox wallet for user" {
		t.Fatalf("unexpected wallet error: %+v", werr)
	}
}

func TestBalancesReturnsGameAndSandbox(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-token", "expires_in": 3600})
	})
	mux.HandleFunc("/v1.0/internal/wallet/balance/user-1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"game_balance": 500, "sandbox_balance": 1000})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(&config.Config{WalletURL: srv.URL, CtechURL: srv.URL, PokerClientID: "poker", PokerClientSecret: "secret"})
	b, err := c.Balances(t.Context(), "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if b.GameBalance != 500 || b.SandboxBalance != 1000 {
		t.Fatalf("unexpected balances: %+v", b)
	}
}

func TestCreditFallsBackToGenericErrorOnNonProblemBody(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-token", "expires_in": 3600})
	})
	mux.HandleFunc("/v1.0/internal/wallet/sandbox/credit", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(&config.Config{WalletURL: srv.URL, CtechURL: srv.URL, PokerClientID: "poker", PokerClientSecret: "secret"})
	err := c.Credit(t.Context(), "user-1", 500, "key-1", "daily_reward")

	var werr *Error
	if errors.As(err, &werr) {
		t.Fatalf("expected generic error, got typed *Error: %+v", werr)
	}
	if err == nil {
		t.Fatal("expected an error")
	}
}
