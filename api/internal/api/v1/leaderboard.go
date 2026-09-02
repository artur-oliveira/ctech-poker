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
	router.Get("/leaderboard/me", auth, h.me)
}

func (h *leaderboardHandlers) top(c fiber.Ctx) error {
	limit := limitParam(c)
	cursor := c.Query("cursor")
	entries, lastKey, err := h.svc.Top(c.Context(), currencyModeParam(c), c.Query("metric", "hands_won"), limit, decodeCursor(cursor))
	if err != nil {
		return problem.BadRequest(err.Error()).Send(c)
	}
	return sendPage(c, entries, lastKey, cursor)
}

// meResponse is the /leaderboard/me envelope. Ranked is false (with every
// other field omitted) when the caller has no stats row for mode yet — the
// client's "unranked yet" state, not an error.
type meResponse struct {
	Ranked bool               `json:"ranked"`
	Rank   *int64             `json:"rank,omitempty"`
	Total  *int64             `json:"total,omitempty"`
	Entry  *leaderboard.Entry `json:"entry,omitempty"`
}

func (h *leaderboardHandlers) me(c fiber.Ctx) error {
	playerID := c.Locals(localsUserID).(string)
	info, err := h.svc.MyRank(c.Context(), currencyModeParam(c), c.Query("metric", "hands_won"), playerID)
	if err != nil {
		return problem.BadRequest(err.Error()).Send(c)
	}
	if info == nil {
		return c.JSON(meResponse{Ranked: false})
	}
	return c.JSON(meResponse{Ranked: true, Rank: &info.Rank, Total: &info.Total, Entry: &info.Entry})
}
