package v1

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/jwtverify"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

const (
	localsUserID     = "user_id"
	localsFirstParty = "first_party_poker"
)

func authMiddleware(verifier *jwtverify.Verifier) fiber.Handler {
	return func(c fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			return problem.Unauthorized("missing bearer token").Send(c)
		}
		// Writes fail closed when the revocation list is unreachable (saga
		// protocol §5: money-moving routes are strict); reads fail open.
		verify := verifier.VerifyClaims
		if c.Method() != fiber.MethodGet {
			verify = verifier.VerifyClaimsStrict
		}
		claims, err := verify(c.Context(), strings.TrimPrefix(authHeader, "Bearer "))
		if errors.Is(err, jwtverify.ErrRevocationUnavailable) {
			return unavailable("authorization is temporarily unavailable, retry shortly").Send(c)
		}
		if err != nil || claims == nil || claims.Sub == "" {
			return problem.Unauthorized("invalid credentials").Send(c)
		}
		// An empty sid marks an M2M client_credentials token (ecosystem
		// convention, see jwtverify.Claims). All routes behind this middleware
		// are player-facing: M2M credentials must never act as players.
		if claims.SID == "" {
			return problem.Forbidden("m2m credentials cannot act as a player").Send(c)
		}
		if denied := enforceReadOnlyScope(c, claims); denied != nil {
			return denied.Send(c)
		}
		c.Locals(localsUserID, claims.Sub)
		c.Locals(localsFirstParty, isFirstPartyPokerSession(claims))
		return c.Next()
	}
}

func firstPartyOnly(c fiber.Ctx) error {
	firstParty, _ := c.Locals(localsFirstParty).(bool)
	if !firstParty {
		return problem.Forbidden("social data requires the first-party Poker client").Send(c)
	}
	return c.Next()
}

// unavailable is a 503: the request may succeed if retried.
func unavailable(detail string) *problem.Problem {
	return problem.New(fiber.StatusServiceUnavailable, "/problems/unavailable", "Service Unavailable", detail)
}
