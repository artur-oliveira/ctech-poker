package walletclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/config"
)

// fakeM2MToken builds an unsigned-but-well-formed JWT carrying the given
// space-separated scope claim, the same shape ctech-account issues — good
// enough for jwtGrantedScope, which only ever reads claims, never verifies
// the signature (see ValidateRequiredScopes's doc comment on why that's safe
// here).
func fakeM2MToken(t *testing.T, scope string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"scope": scope})
	signed, err := tok.SignedString([]byte("test-signing-key"))
	if err != nil {
		t.Fatalf("sign fake token: %v", err)
	}
	return signed
}

func newScopeTestServer(t *testing.T, tokenForScope map[string]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse token form: %v", err)
		}
		requested := r.PostForm.Get("scope")
		token, ok := tokenForScope[requested]
		if !ok {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"invalid_scope"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": token, "expires_in": 3600})
	})
	return httptest.NewServer(mux)
}

func newTestClient(srv *httptest.Server) *Client {
	return New(&config.Config{Env: "test", WalletURL: srv.URL, CtechURL: srv.URL,
		PokerClientID: "poker", PokerClientSecret: "secret"}, cache.NewMemoryBackend(10))
}

func TestValidateRequiredScopesPassesWhenBothGranted(t *testing.T) {
	srv := newScopeTestServer(t, map[string]string{
		scopeDebitReal:  fakeM2MToken(t, "internal:wallet:debit-real internal:wallet:credit"),
		scopeGameStatus: fakeM2MToken(t, "internal:wallet:game-status"),
	})
	defer srv.Close()

	if err := newTestClient(srv).ValidateRequiredScopes(t.Context()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidateRequiredScopesFailsWhenTokenEndpointRejectsScope(t *testing.T) {
	// Only game-status is a scope the token endpoint recognizes at all —
	// mirrors ctech-wallet's scope catalog missing internal:wallet:game-status
	// entirely (issue #39 point 1).
	srv := newScopeTestServer(t, map[string]string{
		scopeGameStatus: fakeM2MToken(t, "internal:wallet:game-status"),
	})
	defer srv.Close()

	err := newTestClient(srv).ValidateRequiredScopes(t.Context())
	if err == nil {
		t.Fatal("expected an error when the token endpoint rejects a required scope")
	}
	if !strings.Contains(err.Error(), scopeDebitReal) {
		t.Fatalf("error should name the missing scope %q, got: %v", scopeDebitReal, err)
	}
}

func TestValidateRequiredScopesFailsWhenGrantedTokenOmitsScope(t *testing.T) {
	// Token endpoint accepts the request but narrows the grant down —
	// mirrors poker's M2M client never having been granted
	// internal:wallet:debit-real (issue #39 point 2).
	srv := newScopeTestServer(t, map[string]string{
		scopeDebitReal:  fakeM2MToken(t, "internal:wallet:credit internal:wallet:debit"),
		scopeGameStatus: fakeM2MToken(t, "internal:wallet:game-status"),
	})
	defer srv.Close()

	err := newTestClient(srv).ValidateRequiredScopes(t.Context())
	if err == nil {
		t.Fatal("expected an error when the granted token omits a required scope")
	}
	if !strings.Contains(err.Error(), scopeDebitReal) {
		t.Fatalf("error should name the missing scope %q, got: %v", scopeDebitReal, err)
	}
}

func TestValidateRequiredScopesAllowsOpaqueTokens(t *testing.T) {
	// A non-JWT opaque token can't be inspected for its granted scope — that
	// must not be treated as proof the scope is missing.
	srv := newScopeTestServer(t, map[string]string{
		scopeDebitReal:  "opaque-token-1",
		scopeGameStatus: "opaque-token-2",
	})
	defer srv.Close()

	if err := newTestClient(srv).ValidateRequiredScopes(t.Context()); err != nil {
		t.Fatalf("expected no error for opaque tokens, got %v", err)
	}
}
