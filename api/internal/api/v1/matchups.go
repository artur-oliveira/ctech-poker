package v1

import (
	"context"
	"net/url"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/matchup"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

type matchupReader interface {
	Get(ctx context.Context, mode, playerA, playerB string) (matchup.PairStats, error)
}

// RegisterMatchups mounts the read-only head-to-head comparator. The path
// param names the opponent only — viewer identity always comes from the JWT
// (localsUserID), never the client — same IDOR-safe pattern as
// RegisterPlayerNotes's save handler.
func RegisterMatchups(router fiber.Router, auth fiber.Handler, store matchupReader) {
	router.Get("/players/me/matchups/:opponentId", auth, func(c fiber.Ctx) error {
		viewerID := c.Locals(localsUserID).(string)
		opponentID, err := url.PathUnescape(c.Params("opponentId"))
		if err != nil || opponentID == "" || opponentID == viewerID {
			return problem.BadRequest("opponent id is invalid").Send(c)
		}
		pair, err := store.Get(c.Context(), currencyModeParam(c), viewerID, opponentID)
		if err != nil {
			return problem.InternalServer("failed to load matchup stats", c, err).Send(c)
		}
		// pair.Stats is always idLow/idHigh-relative; remap to
		// viewer/opponent here, once, rather than in storage.
		viewerWins, opponentWins, netChangeViewer := pair.Stats.WinsLow, pair.Stats.WinsHigh, pair.Stats.NetChangeLow
		if viewerID != pair.IDLow {
			viewerWins, opponentWins, netChangeViewer = pair.Stats.WinsHigh, pair.Stats.WinsLow, pair.Stats.NetChangeHigh
		}
		return c.JSON(fiber.Map{
			"hands_together":          pair.Stats.HandsTogether,
			"viewer_wins":             viewerWins,
			"opponent_wins":           opponentWins,
			"ties":                    pair.Stats.Ties,
			"heads_up_hands_together": pair.Stats.HeadsUpHandsTogether,
			"net_change_viewer":       netChangeViewer,
		})
	})
}
