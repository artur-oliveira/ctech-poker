package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
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

// TestTokenConcurrentCallsRefreshOnce reproduces the "authentication failed
// shortly after a real login" bug: without refreshMu, N callers racing near
// expiry all send the same (single-use) refresh_token, and every loser after
// the first gets invalid_grant even though the session is fine.
func TestTokenConcurrentCallsRefreshOnce(t *testing.T) {
	var mu sync.Mutex
	used := map[string]bool{}
	var refreshCalls int32

	var mux http.ServeMux
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		rt := r.Form.Get("refresh_token")
		mu.Lock()
		if used[rt] {
			mu.Unlock()
			json.NewEncoder(w).Encode(map[string]any{"error": "invalid_grant", "error_description": "refresh token already used"})
			return
		}
		used[rt] = true
		mu.Unlock()
		atomic.AddInt32(&refreshCalls, 1)
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at-new", "refresh_token": "rt-new", "expires_in": 3600, "token_type": "Bearer",
		})
	})
	stub := httptest.NewServer(&mux)
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

	const n = 8
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Token(context.Background())
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Token call failed: %v", err)
		}
	}
	if refreshCalls != 1 {
		t.Fatalf("refresh endpoint called %d times, want exactly 1", refreshCalls)
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
