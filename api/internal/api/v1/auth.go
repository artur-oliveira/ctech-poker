package v1

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/jwtverify"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

const (
	localsUserID     = "user_id"
	localsFirstParty = "first_party_poker"
)

func authMiddleware(verifier *jwtverify.Verifier, blocked BlockedFunc) fiber.Handler {
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
		if denied := erasureLock(c, blocked, claims.Sub); denied != nil {
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

// BlockedFunc reports whether the account-deletion saga has locked or erased
// sub (erasure.Store.Blocked). nil disables the check; only the narrow test
// seam in internal/app passes nil.
type BlockedFunc func(ctx context.Context, sub string) (bool, error)

// erasureLock refuses a user the deletion saga locked or erased (saga
// protocol §4.2) on every method, because some GETs write (GET /players/me
// lazily creates the profile). Leaving a table is the one exception: money
// only flows back to the player there. A store failure fails closed on
// writes and open on reads.
func erasureLock(c fiber.Ctx, blocked BlockedFunc, sub string) *problem.Problem {
	if blocked == nil {
		return nil
	}
	locked, err := blocked(c.Context(), sub)
	switch {
	case err != nil && c.Method() != fiber.MethodGet:
		slog.ErrorContext(c.Context(), "erasure lock check failed", "error", err)
		return unavailable("account state is temporarily unavailable, retry shortly")
	case err != nil:
		slog.WarnContext(c.Context(), "erasure lock check failed; serving the read", "error", err)
		return nil
	case locked && !isTableLeave(c):
		return problem.Forbidden("this account is being deleted")
	}
	return nil
}

func isTableLeave(c fiber.Ctx) bool {
	return c.Method() == fiber.MethodPost &&
		strings.HasPrefix(c.Path(), "/v1.0/rooms/") && strings.HasSuffix(c.Path(), "/leave")
}

// wsLocked is erasureLock for the WebSocket gateways. A socket outlives any
// later check, so a store failure refuses the connection.
func wsLocked(ctx context.Context, blocked BlockedFunc, sub string) bool {
	if blocked == nil {
		return false
	}
	locked, err := blocked(ctx, sub)
	if err != nil {
		slog.ErrorContext(ctx, "erasure lock check failed; refusing websocket", "error", err)
		return true
	}
	return locked
}
