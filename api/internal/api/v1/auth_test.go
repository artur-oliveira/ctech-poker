package v1

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
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
		m2m := signToken(t, key, jwt.MapClaims{"sub": "client_poker", "scope": "internal:wallet:credit", "token_use": "access", "exp": exp})
		if resp := do(t, "Bearer "+m2m); resp.StatusCode != fiber.StatusForbidden {
			t.Fatalf("expected 403, got %d", resp.StatusCode)
		}
	})

	t.Run("user token (sub + sid) → 200, playerID from sub", func(t *testing.T) {
		user := signToken(t, key, jwt.MapClaims{"sub": "user_1", "sid": "sess_1", "token_use": "access", "exp": exp})
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

// The public ranking page must stay readable without a token: GET
// /leaderboard is mounted WITHOUT the auth middleware. (Its sibling
// /leaderboard/me still requires one — TestLeaderboardMeRequiresAuth.)
func TestLeaderboardTopIsPublic(t *testing.T) {
	app := fiber.New()
	deny := func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusUnauthorized) }
	store := &fakeLeaderboardStore{entries: map[string]*leaderboard.Entry{}}
	RegisterLeaderboard(app.Group("/v1.0"), deny, leaderboard.NewServiceWithStore(store), nil, nil)

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/v1.0/leaderboard?mode=sandbox", nil))
	if err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("public leaderboard: expected 200, got %d, err %v", resp.StatusCode, err)
	}
}

// downCache is a revocation backend that is unreachable.
type downCache struct{ cache.Backend }

func (downCache) Get(context.Context, string) ([]byte, bool, error) {
	return nil, false, errors.New("valkey down")
}

// A revoked user is cut off at once; a revocation list outage fails open on
// reads and closed on writes (saga protocol §5).
func TestAuthMiddlewareRevocation(t *testing.T) {
	key, srv := newJWKSServer(t)
	revoked := cache.NewMemoryBackend(10)
	if err := jwtverify.Revoke(context.Background(), revoked, "user_1", time.Now(), jwtverify.RevocationTTL); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	exp := time.Now().Add(15 * time.Minute).Unix()
	iat := time.Now().Add(-time.Minute).Unix()
	userTok := signToken(t, key, jwt.MapClaims{"sub": "user_1", "sid": "s1", "azp": "poker", "token_use": "access", "exp": exp, "iat": iat})
	otherTok := signToken(t, key, jwt.MapClaims{"sub": "user_2", "sid": "s2", "azp": "poker", "token_use": "access", "exp": exp, "iat": iat})

	newApp := func(revocation cache.Backend) *fiber.App {
		verifier := jwtverify.NewVerifier(srv.URL, "", "", cache.NewMemoryBackend(10)).WithRevocation(revocation)
		app := fiber.New()
		ok := func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }
		auth := authMiddleware(verifier)
		app.Get("/protected", auth, ok)
		app.Post("/protected", auth, ok)
		return app
	}
	status := func(t *testing.T, app *fiber.App, method, token string) int {
		t.Helper()
		req := httptest.NewRequest(method, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		return resp.StatusCode
	}

	app := newApp(revoked)
	if got := status(t, app, fiber.MethodGet, userTok); got != fiber.StatusUnauthorized {
		t.Fatalf("revoked user: want 401, got %d", got)
	}
	if got := status(t, app, fiber.MethodGet, otherTok); got != fiber.StatusOK {
		t.Fatalf("other user: want 200, got %d", got)
	}
	down := newApp(downCache{})
	if got := status(t, down, fiber.MethodGet, userTok); got != fiber.StatusOK {
		t.Fatalf("read with revocation list down: want 200 (fail open), got %d", got)
	}
	if got := status(t, down, fiber.MethodPost, userTok); got != fiber.StatusServiceUnavailable {
		t.Fatalf("write with revocation list down: want 503 (fail closed), got %d", got)
	}
}
