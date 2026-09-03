package v1

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/leaderboard"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

type leaderboardHandlers struct {
	svc *leaderboard.Service
	// players resolves each row's display name at read time. A leaderboard
	// row's player_name is denormalized on the write that happens every hand
	// (leaderboard.Store.IncrementStats), from the name the table actor
	// cached at join — so a player who renames and stops playing keeps the
	// old name on the board forever (#64). nil in tests that don't care;
	// then the stored copy is served as-is.
	players *player.Service
}

func RegisterLeaderboard(router fiber.Router, auth fiber.Handler, svc *leaderboard.Service, players *player.Service) {
	h := &leaderboardHandlers{svc: svc, players: players}
	router.Get("/leaderboard", auth, h.top)
	router.Get("/leaderboard/me", auth, h.me)
}

// resolveNames overwrites every entry's denormalized PlayerName with the
// canonical profile name, in place — the read-time half of #64's fix, the
// same one resolveOpponentProfiles applies to hand history. One BatchGetItem
// per rendered page. An id with no profile (or a profile with no name) keeps
// whatever the row stored, so this can never blank a board.
func (h *leaderboardHandlers) resolveNames(ctx context.Context, entries []leaderboard.Entry) error {
	if h.players == nil || len(entries) == 0 {
		return nil
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.PlayerID != "" {
			ids = append(ids, entry.PlayerID)
		}
	}
	profiles, err := h.players.GetMany(ctx, ids)
	if err != nil {
		return err
	}
	for i := range entries {
		if profile, ok := profiles[entries[i].PlayerID]; ok && profile.Name != "" {
			entries[i].PlayerName = profile.Name
		}
	}
	return nil
}

func (h *leaderboardHandlers) top(c fiber.Ctx) error {
	limit := limitParam(c)
	cursor := c.Query("cursor")
	entries, lastKey, err := h.svc.Top(c.Context(), currencyModeParam(c), c.Query("metric", "hands_won"), limit, decodeCursor(cursor))
	if err != nil {
		return problem.BadRequest(err.Error()).Send(c)
	}
	if err := h.resolveNames(c.Context(), entries); err != nil {
		return problem.InternalServer("failed to resolve player names", c, err).Send(c)
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
	entries := []leaderboard.Entry{info.Entry}
	if err := h.resolveNames(c.Context(), entries); err != nil {
		return problem.InternalServer("failed to resolve player names", c, err).Send(c)
	}
	return c.JSON(meResponse{Ranked: true, Rank: &info.Rank, Total: &info.Total, Entry: &entries[0]})
}
