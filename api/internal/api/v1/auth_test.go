package v1

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/jwtverify"
	"gopkg.aoctech.app/poker/api/internal/leaderboard"
)

const testKID = "test-key-1"

// newJWKSServer returns an RSA key and an httptest server serving its public
// JWKS. Mirrors ctech-wallet's middleware auth test: JWKS mechanics are
// covered by jwtverify's own tests; this file only checks poker's authz policy.
func newJWKSServer(t *testing.T) (*rsa.PrivateKey, *httptest.Server) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())
	body, _ := json.Marshal(map[string]any{"keys": []map[string]any{{"kid": testKID, "kty": "RSA", "n": n, "e": e}}})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return key, srv
}

func signToken(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = testKID
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}

func TestAuthMiddleware(t *testing.T) {
	key, srv := newJWKSServer(t)
	verifier := jwtverify.NewVerifier(srv.URL, "", "", cache.NewMemoryBackend(10))

	app := fiber.New()
	app.Get("/protected", authMiddleware(verifier), func(c fiber.Ctx) error {
		return c.SendString(c.Locals(localsUserID).(string))
	})

	exp := time.Now().Add(15 * time.Minute).Unix()
	do := func(t *testing.T, authorization string) *http.Response {
		t.Helper()
		req := httptest.NewRequest(fiber.MethodGet, "/protected", nil)
		if authorization != "" {
			req.Header.Set("Authorization", authorization)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		return resp
	}

	t.Run("missing bearer token → 401", func(t *testing.T) {
		if resp := do(t, ""); resp.StatusCode != fiber.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", resp.StatusCode)
		}
	})

	t.Run("garbage token → 401", func(t *testing.T) {
		if resp := do(t, "Bearer not-a-jwt"); resp.StatusCode != fiber.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", resp.StatusCode)
		}
	})

	t.Run("M2M token (empty sid) → 403", func(t *testing.T) {
		m2m := signToken(t, key, jwt.MapClaims{"sub": "client_poker", "scope": "internal:wallet:credit", "exp": exp})
		if resp := do(t, "Bearer "+m2m); resp.StatusCode != fiber.StatusForbidden {
			t.Fatalf("expected 403, got %d", resp.StatusCode)
		}
	})

	t.Run("user token (sub + sid) → 200, playerID from sub", func(t *testing.T) {
		user := signToken(t, key, jwt.MapClaims{"sub": "user_1", "sid": "sess_1", "exp": exp})
		resp := do(t, "Bearer "+user)
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		var buf [16]byte
		n, _ := resp.Body.Read(buf[:])
		if got := string(buf[:n]); got != "user_1" {
			t.Fatalf("expected playerID user_1, got %q", got)
		}
	})
}

// GET /leaderboard must sit behind the auth middleware (B9).
func TestLeaderboardRequiresAuth(t *testing.T) {
	app := fiber.New()
	deny := func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusUnauthorized) }
	RegisterLeaderboard(app.Group("/v1.0"), deny, &leaderboard.Service{})

	req := httptest.NewRequest(fiber.MethodGet, "/v1.0/leaderboard", nil)
	resp, err := app.Test(req)
	if err != nil || resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("expected 401 from auth middleware, got %d, err %v", resp.StatusCode, err)
	}
}
