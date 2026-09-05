package v1

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/pokerstats"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

type pokerStatsReader interface {
	Get(ctx context.Context, playerID, mode string) (pokerstats.Stats, error)
}

// statsGoalRequest is the POST /players/me/stats/goals body: a full
// replacement of the caller's goal set, mirroring updateMe's
// FavoriteReactions replace-not-merge shape. An absent/empty map clears
// every goal.
type statsGoalRequest struct {
	Goals map[string]float64 `json:"goals"`
}

// statsResponse builds the shared GET /players/me/poker-stats and
// GET /players/me/stats payload: raw Stats, StyleFor(MinHandsSelf) badges,
// Leaks tips (#331), and — only on the newer route — this player's own
// goal_progress against any StatsGoals they've set.
func statsResponse(stats pokerstats.Stats, goals map[string]float64, includeGoals bool) fiber.Map {
	resp := fiber.Map{
		"hands": stats.Hands, "vpip_hands": stats.VPIPHands, "pfr_hands": stats.PFRHands,
		"three_bet_hands": stats.ThreeBetHands, "three_bet_chances": stats.ThreeBetChances,
		"vpip_rate": stats.VPIPRate, "pfr_rate": stats.PFRRate, "three_bet_rate": stats.ThreeBetRate,
		"playstyle": pokerstats.StyleFor(stats, pokerstats.MinHandsSelf),
		"leaks":     pokerstats.Leaks(stats, pokerstats.MinHandsSelf),
	}
	if includeGoals {
		progress := make(fiber.Map, len(goals))
		for key, target := range goals {
			progress[key] = fiber.Map{"target": target, "current": statMetric(stats, key)}
		}
		resp["goals"] = goals
		resp["goal_progress"] = progress
	}
	return resp
}

func statMetric(stats pokerstats.Stats, key string) float64 {
	switch key {
	case "vpip_rate":
		return stats.VPIPRate
	case "pfr_rate":
		return stats.PFRRate
	case "three_bet_rate":
		return stats.ThreeBetRate
	default:
		return 0
	}
}

func RegisterPokerStats(router fiber.Router, auth fiber.Handler, store pokerStatsReader, players *player.Service) {
	router.Get("/players/me/poker-stats", auth, func(c fiber.Ctx) error {
		stats, err := store.Get(c.Context(), c.Locals(localsUserID).(string), currencyModeParam(c))
		if err != nil {
			return problem.InternalServer("failed to load poker stats", c, err).Send(c)
		}
		return c.JSON(statsResponse(stats, nil, false))
	})

	// GET /players/me/stats (#331): Stats + badges + leak tips + this
	// player's own goal progress, always at MinHandsSelf — never gated on
	// PlaystylePublic, unlike the public showcase's badge.
	router.Get("/players/me/stats", auth, func(c fiber.Ctx) error {
		userID := c.Locals(localsUserID).(string)
		stats, err := store.Get(c.Context(), userID, currencyModeParam(c))
		if err != nil {
			return problem.InternalServer("failed to load poker stats", c, err).Send(c)
		}
		profile, err := players.GetOrCreate(c.Context(), userID)
		if err != nil {
			return problem.InternalServer("failed to load player profile", c, err).Send(c)
		}
		return c.JSON(statsResponse(stats, profile.StatsGoals, true))
	})

	router.Post("/players/me/stats/goals", auth, func(c fiber.Ctx) error {
		var req statsGoalRequest
		if err := c.Bind().Body(&req); err != nil {
			return problem.BadRequest("invalid body").Send(c)
		}
		userID := c.Locals(localsUserID).(string)
		profile, err := players.SetStatsGoals(c.Context(), userID, req.Goals)
		if err != nil {
			if errors.Is(err, player.ErrInvalidStatsGoals) {
				return problem.BadRequest("goals keys must be one of vpip_rate, pfr_rate, three_bet_rate").Send(c)
			}
			return problem.InternalServer("failed to update stats goals", c, err).Send(c)
		}
		stats, err := store.Get(c.Context(), userID, currencyModeParam(c))
		if err != nil {
			return problem.InternalServer("failed to load poker stats", c, err).Send(c)
		}
		return c.JSON(statsResponse(stats, profile.StatsGoals, true))
	})
}
