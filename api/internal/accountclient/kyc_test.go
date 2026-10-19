package accountclient

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/resilient"
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

// While ctech-account is down five failures in a row open the circuit and the next call
// is refused at once. It stays an ERROR: "no KYC record" is an empty answer with no
// error, and an outage must never read as that.
func TestADownAccountOpensTheCircuitAndIsNeverReadAsNoKYCRecord(t *testing.T) {
	srv := server(t, http.StatusServiceUnavailable, `{}`)
	defer srv.Close()
	c := New(srv.URL, "poker", "secret", cache.NewMemoryBackend(4))
	for i := 0; i < 5; i++ {
		if got, err := c.CPFHMAC(t.Context(), "user-1"); err == nil || got != "" {
			t.Fatalf("call %d against a down account: hash=%q err=%v", i, got, err)
		}
	}
	got, err := c.CPFHMAC(t.Context(), "user-1")
	if !errors.Is(err, resilient.ErrCircuitOpen) || got != "" {
		t.Fatalf("hash=%q err=%v, want the open-circuit error and no hash", got, err)
	}
}
