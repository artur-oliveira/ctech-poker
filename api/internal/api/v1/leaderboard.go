package v1

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/leaderboard"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

func RegisterLeaderboard(router fiber.Router, auth fiber.Handler, svc *leaderboard.Service) {
	router.Get("/leaderboard", auth, func(c fiber.Ctx) error {
		limit := 50
		if n, err := strconv.Atoi(c.Query("limit")); err == nil && n > 0 {
			limit = n
		}
		entries, err := svc.Top(c.Context(), c.Query("metric", "hands_won"), limit)
		if err != nil {
			return problem.BadRequest(err.Error()).Send(c)
		}
		return c.JSON(entries)
	})
}
