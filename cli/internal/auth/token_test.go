package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestExchangeAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.URL.Path != "/v1.0/token" || r.Form.Get("grant_type") != "api_key" || r.Form.Get("api_key") != "k-123" {
			t.Errorf("unexpected request: %s %v", r.URL.Path, r.Form)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at-1", "token_type": "Bearer", "expires_in": 3600,
		})
	}))
	defer srv.Close()

	c, err := NewTokenClient(srv.URL, "poker-cli", srv.Client()).ExchangeAPIKey(context.Background(), "k-123")
	if err != nil {
		t.Fatal(err)
	}
	if c.AccessToken != "at-1" || c.ObtainedVia != "api_key" || c.APIKey != "k-123" {
		t.Fatalf("got %+v", c)
	}
	if time.Until(c.ExpiresAt) < 50*time.Minute {
		t.Errorf("expiry not computed from expires_in: %v", c.ExpiresAt)
	}
}

func TestRefreshRotatesRefreshTokenForPKCE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "rt-old" {
			t.Errorf("bad refresh request: %v", r.Form)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at-2", "refresh_token": "rt-new", "expires_in": 3600, "token_type": "Bearer",
		})
	}))
	defer srv.Close()

	got, err := NewTokenClient(srv.URL, "poker-cli", srv.Client()).
		Refresh(context.Background(), Credentials{ObtainedVia: "pkce", RefreshToken: "rt-old"})
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "at-2" || got.RefreshToken != "rt-new" || got.ObtainedVia != "pkce" {
		t.Fatalf("got %+v", got)
	}
}

func TestRefreshReExchangesAPIKey(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		r.ParseForm()
		if r.Form.Get("grant_type") != "api_key" || r.Form.Get("api_key") != "k-999" {
			t.Errorf("refresh of an api_key credential must re-exchange the key: %v", r.Form)
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": "at-3", "token_type": "Bearer", "expires_in": 3600})
	}))
	defer srv.Close()

	got, err := NewTokenClient(srv.URL, "poker-cli", srv.Client()).
		Refresh(context.Background(), Credentials{ObtainedVia: "api_key", APIKey: "k-999"})
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "at-3" || calls != 1 {
		t.Fatalf("got %+v calls=%d", got, calls)
	}
}

func TestTokenEndpointErrorIsWrapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant", "error_description": "expired"})
	}))
	defer srv.Close()

	_, err := NewTokenClient(srv.URL, "poker-cli", srv.Client()).
		Refresh(context.Background(), Credentials{ObtainedVia: "pkce", RefreshToken: "x"})
	if !errors.Is(err, ErrAuthFailed) || !strings.Contains(err.Error(), "invalid_grant") {
		t.Fatalf("want wrapped ErrAuthFailed carrying invalid_grant, got %v", err)
	}
}

func TestExchangeCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code") != "c-1" ||
			r.Form.Get("code_verifier") != "v-1" || r.Form.Get("redirect_uri") != "http://127.0.0.1:1/callback" ||
			r.Form.Get("client_id") != "poker-cli" {
			t.Errorf("bad exchange request: %v", r.Form)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at-4", "refresh_token": "rt-4", "expires_in": 3600, "token_type": "Bearer",
		})
	}))
	defer srv.Close()

	got, err := NewTokenClient(srv.URL, "poker-cli", srv.Client()).
		ExchangeCode(context.Background(), "c-1", "v-1", "http://127.0.0.1:1/callback")
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "at-4" || got.ObtainedVia != "pkce" {
		t.Fatalf("got %+v", got)
	}
}
