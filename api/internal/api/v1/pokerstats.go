package v1

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/pokerstats"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

type pokerStatsReader interface {
	Get(ctx context.Context, playerID, mode string) (pokerstats.Stats, error)
}

func RegisterPokerStats(router fiber.Router, auth fiber.Handler, store pokerStatsReader) {
	router.Get("/players/me/poker-stats", auth, func(c fiber.Ctx) error {
		stats, err := store.Get(c.Context(), c.Locals(localsUserID).(string), currencyModeParam(c))
		if err != nil {
			return problem.InternalServer("failed to load poker stats", c, err).Send(c)
		}
		return c.JSON(fiber.Map{
			"hands": stats.Hands, "vpip_hands": stats.VPIPHands, "pfr_hands": stats.PFRHands,
			"three_bet_hands": stats.ThreeBetHands, "three_bet_chances": stats.ThreeBetChances,
			"vpip_rate": stats.VPIPRate, "pfr_rate": stats.PFRRate, "three_bet_rate": stats.ThreeBetRate,
			"playstyle": pokerstats.StyleFor(stats, pokerstats.MinHandsSelf),
		})
	})
}
