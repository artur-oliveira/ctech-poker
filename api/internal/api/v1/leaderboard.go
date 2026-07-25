package v1

import (
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/leaderboard"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

type leaderboardHandlers struct{ svc *leaderboard.Service }

func RegisterLeaderboard(router fiber.Router, auth fiber.Handler, svc *leaderboard.Service) {
	h := &leaderboardHandlers{svc: svc}
	router.Get("/leaderboard", auth, h.top)
}

func (h *leaderboardHandlers) top(c fiber.Ctx) error {
	limit := limitParam(c)
	cursor := c.Query("cursor")
	entries, lastKey, err := h.svc.Top(c.Context(), c.Query("metric", "hands_won"), limit, decodeCursor(cursor))
	if err != nil {
		return problem.BadRequest(err.Error()).Send(c)
	}
	return sendPage(c, entries, lastKey, cursor)
}
