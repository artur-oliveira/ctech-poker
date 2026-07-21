package v1

import (
	"strings"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/jwtverify"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

const localsUserID = "user_id"

func authMiddleware(verifier *jwtverify.Verifier) fiber.Handler {
	return func(c fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			return problem.Unauthorized("missing bearer token").Send(c)
		}
		claims, err := verifier.VerifyClaims(c.Context(), strings.TrimPrefix(authHeader, "Bearer "))
		if err != nil || claims == nil || claims.Sub == "" {
			return problem.Unauthorized("invalid credentials").Send(c)
		}
		// An empty sid marks an M2M client_credentials token (ecosystem
		// convention, see jwtverify.Claims). All routes behind this middleware
		// are player-facing: M2M credentials must never act as players.
		if claims.SID == "" {
			return problem.Forbidden("m2m credentials cannot act as a player").Send(c)
		}
		c.Locals(localsUserID, claims.Sub)
		return c.Next()
	}
}
