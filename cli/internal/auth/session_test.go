package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/cli/internal/config"
)

// stubAccount fakes just enough of ctech-account's authorize+token endpoints
// to drive a full PKCE round trip against a real loopback receiver.
func stubAccount(t *testing.T) *httptest.Server {
	t.Helper()
	var mux http.ServeMux
	mux.HandleFunc("/v1.0/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		redirect := q.Get("redirect_uri")
		if q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
			http.Error(w, "missing PKCE challenge", 400)
			return
		}
		dest, _ := url.Parse(redirect)
		v := dest.Query()
		v.Set("code", "the-code")
		v.Set("state", q.Get("state"))
		dest.RawQuery = v.Encode()
		http.Redirect(w, r, dest.String(), http.StatusFound)
	})
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at-" + r.Form.Get("grant_type"), "refresh_token": "rt-1",
			"expires_in": 3600, "token_type": "Bearer",
		})
	})
	return httptest.NewServer(&mux)
}

func TestLoginPKCEEndToEndAgainstStubAccount(t *testing.T) {
	stub := stubAccount(t)
	defer stub.Close()

	cfg := config.Settings{AccountBaseURL: stub.URL, ClientID: "poker-cli", ConfigDir: t.TempDir()}
	s := NewSession(cfg, stub.Client())

	err := s.LoginPKCE(context.Background(), func(u string) error {
		go stub.Client().Get(u) //nolint:errcheck // fire-and-forget, mimics a real browser launch
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	c, ok, err := LoadCredentials(config.CredentialsPath(cfg))
	if err != nil || !ok {
		t.Fatalf("credentials not saved: ok=%v err=%v", ok, err)
	}
	if c.ObtainedVia != "pkce" || c.AccessToken != "at-authorization_code" {
		t.Fatalf("unexpected saved credentials: %+v", c)
	}
}

func TestLoginAPIKeySavesCredentials(t *testing.T) {
	stub := stubAccount(t)
	defer stub.Close()

	cfg := config.Settings{AccountBaseURL: stub.URL, ClientID: "poker-cli", ConfigDir: t.TempDir()}
	s := NewSession(cfg, stub.Client())
	if err := s.LoginAPIKey(context.Background(), "k-1"); err != nil {
		t.Fatal(err)
	}
	c, ok, _ := LoadCredentials(config.CredentialsPath(cfg))
	if !ok || c.ObtainedVia != "api_key" || c.APIKey != "k-1" {
		t.Fatalf("got %+v ok=%v", c, ok)
	}
}

func TestTokenReturnsLoggedOutWhenNoCredentials(t *testing.T) {
	cfg := config.Settings{AccountBaseURL: "http://unused.invalid", ConfigDir: t.TempDir()}
	s := NewSession(cfg, http.DefaultClient)
	if _, err := s.Token(context.Background()); err != ErrLoggedOut {
		t.Fatalf("want ErrLoggedOut, got %v", err)
	}
}

func TestTokenRefreshesWhenNearExpiryAndPersists(t *testing.T) {
	stub := stubAccount(t)
	defer stub.Close()

	cfg := config.Settings{AccountBaseURL: stub.URL, ClientID: "poker-cli", ConfigDir: t.TempDir()}
	path := config.CredentialsPath(cfg)
	if err := SaveCredentials(path, Credentials{
		AccessToken: "stale", ObtainedVia: "pkce", RefreshToken: "rt-old",
		ExpiresAt: time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	s := NewSession(cfg, stub.Client())
	tok, err := s.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok != "at-refresh_token" {
		t.Fatalf("token = %q, want the refreshed value", tok)
	}
	got, _, _ := LoadCredentials(path)
	if got.AccessToken != "at-refresh_token" {
		t.Fatalf("refreshed token was not persisted: %+v", got)
	}
}

func TestTokenDoesNotRefreshWhenStillValid(t *testing.T) {
	cfg := config.Settings{AccountBaseURL: "http://unused.invalid", ConfigDir: t.TempDir()}
	path := config.CredentialsPath(cfg)
	SaveCredentials(path, Credentials{AccessToken: "fresh", ExpiresAt: time.Now().Add(time.Hour)})

	s := NewSession(cfg, http.DefaultClient)
	tok, err := s.Token(context.Background())
	if err != nil || tok != "fresh" {
		t.Fatalf("tok=%q err=%v, want the stored token with no network call", tok, err)
	}
}

func TestLogoutClearsCredentials(t *testing.T) {
	cfg := config.Settings{ConfigDir: t.TempDir()}
	path := config.CredentialsPath(cfg)
	SaveCredentials(path, Credentials{AccessToken: "x"})

	s := NewSession(cfg, http.DefaultClient)
	if err := s.Logout(); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := LoadCredentials(path); ok {
		t.Fatal("credentials should be gone after logout")
	}
}
