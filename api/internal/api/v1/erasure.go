package v1

import (
	"context"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/erasure"
	"gopkg.aoctech.app/api-commons/jwtverify"
	"gopkg.aoctech.app/api-commons/problem"
)

// ScopeErasureEligibility is carried by the service token ctech-account
// mints to ask poker for a user's deletion blockers (saga protocol §4.1).
const ScopeErasureEligibility = "internal:poker:erasure-eligibility"

// BlockersFunc lists what stops a user's erasure right now.
type BlockersFunc func(ctx context.Context, sub string) ([]erasure.Blocker, error)

// RegisterErasure mounts GET /internal/erasure/eligibility/:sub. Only
// ctech-account's own service token may call it: azp = accountClientID, no
// session, and the eligibility scope. An unknown sub is eligible; any
// failure is a 503, never "eligible".
func RegisterErasure(router fiber.Router, verifier *jwtverify.Verifier, accountClientID string, blockers BlockersFunc) {
	router.Get("/internal/erasure/eligibility/:sub", func(c fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			return unauthorizedProblem("missing bearer token").Send(c)
		}
		claims, err := verifier.VerifyClaims(c.Context(), strings.TrimPrefix(authHeader, "Bearer "))
		if err != nil || claims == nil {
			return unauthorizedProblem("invalid credentials").Send(c)
		}
		if accountClientID == "" || claims.SID != "" || claims.AZP != accountClientID || !claims.HasScope(ScopeErasureEligibility) {
			return problem.Forbidden("erasure eligibility is reserved to ctech-account").Send(c)
		}
		found, err := blockers(c.Context(), c.Params("sub"))
		if err != nil {
			slog.ErrorContext(c.Context(), "erasure eligibility failed", "error", err)
			return unavailable("eligibility is temporarily unavailable").Send(c)
		}
		return c.JSON(erasure.NewEligibility(found...))
	})
}
