package accountclient

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"gopkg.aoctech.app/api-commons/cache"
)

func server(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("scope") != scopeKYC {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"kyc-token","expires_in":3600}`))
	})
	mux.HandleFunc("/v1.0/internal/kyc/user-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer kyc-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
	return httptest.NewServer(mux)
}

func TestCPFHMACReadsTheHashWithTheKYCScope(t *testing.T) {
	srv := server(t, 200, `{"level":"enhanced","cpf_hmac":"abc123"}`)
	defer srv.Close()
	got, err := New(srv.URL, "poker", "secret", cache.NewMemoryBackend(4)).CPFHMAC(t.Context(), "user-1")
	if err != nil || got != "abc123" {
		t.Fatalf("hash=%q err=%v", got, err)
	}
}

func TestNoKYCRecordIsNotAnError(t *testing.T) {
	srv := server(t, 404, `{}`)
	defer srv.Close()
	got, err := New(srv.URL, "poker", "secret", cache.NewMemoryBackend(4)).CPFHMAC(t.Context(), "user-1")
	if err != nil || got != "" {
		t.Fatalf("hash=%q err=%v", got, err)
	}
}

func TestAServerErrorFailsRatherThanReturningNoHash(t *testing.T) {
	srv := server(t, 500, `boom`)
	defer srv.Close()
	if _, err := New(srv.URL, "poker", "secret", cache.NewMemoryBackend(4)).CPFHMAC(t.Context(), "user-1"); err == nil {
		t.Fatal("a failing lookup must not look like \"no CPF\", or an exclusion would be skipped")
	}
}
