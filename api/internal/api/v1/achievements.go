package v1

import (
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/achievements"
)

// RegisterAchievementCatalog mounts the unauthenticated achievement catalog.
// Per-player progress lives on /players/me/achievements — see RegisterPlayers.
func RegisterAchievementCatalog(router fiber.Router) {
	router.Get("/achievements", func(c fiber.Ctx) error {
		return c.JSON(achievements.Catalog)
	})
}
