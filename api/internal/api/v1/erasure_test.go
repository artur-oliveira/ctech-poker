package v1

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/erasure"
	"gopkg.aoctech.app/api-commons/jwtverify"
)

func TestErasureEligibility(t *testing.T) {
	key, srv := newJWKSServer(t)
	verifier := jwtverify.NewVerifier(srv.URL, "", "", cache.NewMemoryBackend(10))
	exp := time.Now().Add(5 * time.Minute).Unix()
	var fail error
	blockers := func(_ context.Context, sub string) ([]erasure.Blocker, error) {
		if fail != nil {
			return nil, fail
		}
		if sub == "seated" {
			return []erasure.Blocker{{Code: "poker.seated_at_table"}}, nil
		}
		return nil, nil
	}
	app := fiber.New()
	RegisterErasure(app.Group("/v1.0"), verifier, "accounts", blockers)
	tok := func(claims jwt.MapClaims) string {
		claims["token_use"], claims["exp"] = "access", exp
		return signToken(t, key, claims)
	}
	account := tok(jwt.MapClaims{"sub": "ctech-account", "azp": "accounts", "scope": ScopeErasureEligibility})

	get := func(t *testing.T, sub, token string) (int, erasure.Eligibility) {
		t.Helper()
		req := httptest.NewRequest(fiber.MethodGet, "/v1.0/internal/erasure/eligibility/"+sub, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		var el erasure.Eligibility
		if resp.StatusCode == fiber.StatusOK {
			if err := json.NewDecoder(resp.Body).Decode(&el); err != nil {
				t.Fatalf("decode: %v", err)
			}
		}
		return resp.StatusCode, el
	}

	t.Run("blockers are listed", func(t *testing.T) {
		code, el := get(t, "seated", account)
		if code != fiber.StatusOK || el.Eligible || len(el.Blockers) != 1 || el.Blockers[0].Code != "poker.seated_at_table" {
			t.Fatalf("got %d %+v", code, el)
		}
	})
	t.Run("unknown user is eligible", func(t *testing.T) {
		code, el := get(t, "nobody", account)
		if code != fiber.StatusOK || !el.Eligible || el.Blockers == nil {
			t.Fatalf("got %d %+v", code, el)
		}
	})
	for name, token := range map[string]string{
		"user session refused":  tok(jwt.MapClaims{"sub": "u", "sid": "s", "azp": "accounts", "scope": ScopeErasureEligibility}),
		"other client refused":  tok(jwt.MapClaims{"sub": "poker", "azp": "poker", "scope": ScopeErasureEligibility}),
		"missing scope refused": tok(jwt.MapClaims{"sub": "ctech-account", "azp": "accounts"}),
	} {
		t.Run(name, func(t *testing.T) {
			if code, _ := get(t, "u", token); code != fiber.StatusForbidden {
				t.Fatalf("want 403, got %d", code)
			}
		})
	}
	t.Run("no token", func(t *testing.T) {
		if code, _ := get(t, "u", ""); code != fiber.StatusUnauthorized {
			t.Fatalf("want 401, got %d", code)
		}
	})
	t.Run("failure is not eligible", func(t *testing.T) {
		fail = errors.New("dynamo down")
		defer func() { fail = nil }()
		if code, _ := get(t, "u", account); code != fiber.StatusServiceUnavailable {
			t.Fatalf("want 503, got %d", code)
		}
	})
}
