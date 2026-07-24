package v1

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/problem"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
)

type sessionLogReader interface {
	ListSessions(ctx context.Context, playerID string, limit int) ([]sessionlog.SessionItem, error)
	ListHands(ctx context.Context, playerID string, limit int) ([]sessionlog.HandItem, error)
}

func RegisterPlayerHistory(router fiber.Router, auth fiber.Handler, reader sessionLogReader) {
	router.Get("/players/me/sessions", auth, func(c fiber.Ctx) error {
		userID, _ := c.Locals(localsUserID).(string)
		if userID == "" {
			return problem.Unauthorized("unauthenticated").Send(c)
		}
		sessions, err := reader.ListSessions(c.Context(), userID, 50)
		if err != nil {
			return problem.InternalServer("failed to list sessions", c, err).Send(c)
		}
		return c.JSON(fiber.Map{"sessions": sessions})
	})

	router.Get("/players/me/hands", auth, func(c fiber.Ctx) error {
		userID, _ := c.Locals(localsUserID).(string)
		if userID == "" {
			return problem.Unauthorized("unauthenticated").Send(c)
		}
		hands, err := reader.ListHands(c.Context(), userID, 50)
		if err != nil {
			return problem.InternalServer("failed to list hands", c, err).Send(c)
		}
		return c.JSON(fiber.Map{"hands": hands})
	})
}
