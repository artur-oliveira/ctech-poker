package v1

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/pokerstats"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

type pokerStatsReader interface {
	Get(ctx context.Context, playerID string) (pokerstats.Stats, error)
}

func RegisterPokerStats(router fiber.Router, auth fiber.Handler, store pokerStatsReader) {
	router.Get("/players/me/poker-stats", auth, func(c fiber.Ctx) error {
		stats, err := store.Get(c.Context(), c.Locals(localsUserID).(string))
		if err != nil {
			return problem.InternalServer("failed to load poker stats", c, err).Send(c)
		}
		return c.JSON(stats)
	})
}
