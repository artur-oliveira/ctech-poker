package v1

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/highlights"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

type highlightsReader interface {
	GetToday(ctx context.Context, tableID string) (*highlights.Highlight, error)
}

type tableSessionChecker interface {
	HasSessionAtTable(ctx context.Context, playerID, tableID string) (bool, error)
}

// RegisterHighlights mounts GET /rooms/:id/highlights/today, scoped to
// players who actually had a session at that table (open or closed, any
// time) — a stranger who never sat there gets 404, same shape as
// handshares.go's public handler for an unknown token.
func RegisterHighlights(router fiber.Router, auth fiber.Handler, sessions tableSessionChecker, store highlightsReader) {
	router.Get("/rooms/:id/highlights/today", auth, func(c fiber.Ctx) error {
		tableID := c.Params("id")
		userID := c.Locals(localsUserID).(string)
		hasSession, err := sessions.HasSessionAtTable(c.Context(), userID, tableID)
		if err != nil {
			return problem.InternalServer("failed to check table session", c, err).Send(c)
		}
		if !hasSession {
			return problem.NotFound("highlight not found").Send(c)
		}
		highlight, err := store.GetToday(c.Context(), tableID)
		if err != nil {
			return problem.InternalServer("failed to load highlight", c, err).Send(c)
		}
		if highlight == nil {
			return problem.NotFound("highlight not found").Send(c)
		}
		return c.JSON(highlight)
	})
}
