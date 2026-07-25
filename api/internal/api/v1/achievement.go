package v1

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/achievements"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

type playerAchievementStore interface {
	ListAchievements(ctx context.Context, playerID string, limit int) ([]achievements.PlayerAchievementProgress, error)
}

func RegisterAchievements(router fiber.Router, auth fiber.Handler, store playerAchievementStore) {
	router.Get("/achievements", func(c fiber.Ctx) error {
		return c.JSON(achievements.Catalog)
	})

	router.Get("/players/me/achievements", auth, func(c fiber.Ctx) error {
		userID, _ := c.Locals(localsUserID).(string)
		if userID == "" {
			return problem.Unauthorized("unauthenticated").Send(c)
		}
		achvs, err := store.ListAchievements(c.Context(), userID, 100)
		if err != nil {
			return problem.InternalServer("failed to list achievements", c, err).Send(c)
		}
		return c.JSON(achvs)
	})
}
