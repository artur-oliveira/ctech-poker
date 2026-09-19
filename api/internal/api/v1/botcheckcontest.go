package v1

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/botcheck"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

// botCheckContestStore is the auditable-contestation half of #322 — kept
// completely separate from the ws gateway's Verify/bot_challenge flow
// (tablews.go). Filing a contest here never re-checks or accepts a
// Turnstile token; it only records the player's claim for later review.
type botCheckContestStore interface {
	Record(ctx context.Context, playerID, tableID, reason string) (*botcheck.Contest, error)
	List(ctx context.Context, playerID string) ([]botcheck.Contest, error)
}

type botCheckContestHandlers struct{ store botCheckContestStore }

type botCheckContestRequest struct {
	TableID string `json:"table_id"`
	Reason  string `json:"reason"`
}

// RegisterBotCheckContest mounts /players/me/bot-challenge/contests.
func RegisterBotCheckContest(router fiber.Router, auth fiber.Handler, store botCheckContestStore) {
	h := &botCheckContestHandlers{store: store}
	g := router.Group("/players/me/bot-challenge/contests", auth)
	g.Get("/", h.list)
	g.Post("/", h.file)
}

func (h *botCheckContestHandlers) list(c fiber.Ctx) error {
	if h.store == nil {
		return problem.InternalServer("bot-challenge contests unavailable", c, errors.New("no store wired")).Send(c)
	}
	contests, err := h.store.List(c.Context(), c.Locals(localsUserID).(string))
	if err != nil {
		return problem.InternalServer("failed to list contests", c, err).Send(c)
	}
	return c.JSON(fiber.Map{"data": contests})
}

func (h *botCheckContestHandlers) file(c fiber.Ctx) error {
	if h.store == nil {
		return problem.InternalServer("bot-challenge contests unavailable", c, errors.New("no store wired")).Send(c)
	}
	var req botCheckContestRequest
	if err := c.Bind().Body(&req); err != nil {
		return problem.BadRequest("invalid body").Send(c)
	}
	contest, err := h.store.Record(c.Context(), c.Locals(localsUserID).(string), req.TableID, req.Reason)
	switch {
	case errors.Is(err, botcheck.ErrContestPlayerRequired):
		return problem.BadRequest("player is invalid").Send(c)
	case errors.Is(err, botcheck.ErrContestReasonTooLong):
		return problem.BadRequest("reason must be at most 500 characters").Send(c)
	case err != nil:
		return problem.InternalServer("failed to file contest", c, err).Send(c)
	}
	return c.JSON(contest)
}
