package v1

import (
	"context"
	"net/url"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/handreveal"
	"gopkg.aoctech.app/poker/api/internal/problem"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
)

// handRevealStore and handRevealService are narrow interfaces over
// *handreveal.Store / *handreveal.Service (both satisfy them automatically)
// so handler tests can use fakes instead of a real DynamoDB/wallet client —
// same pattern as sessionLogReader/historyStore elsewhere in this package.
type handRevealStore interface {
	Get(ctx context.Context, handID string) (*handreveal.HandRecord, error)
}

type handRevealService interface {
	HasPaid(ctx context.Context, handID, viewerID string) (bool, error)
	PayForReveal(ctx context.Context, buyerID, winnerID, handID string, fee int64) error
}

type handRevealHandlers struct {
	sessions sessionLogReader
	records  handRevealStore
	svc      handRevealService
}

func RegisterHandReveal(router fiber.Router, auth fiber.Handler, sessions sessionLogReader, records handRevealStore, svc handRevealService, limiter *RateLimiter) {
	h := &handRevealHandlers{sessions: sessions, records: records, svc: svc}
	g := router.Group("/players/me/hands/:handId", auth)
	g.Post("/reveal-winner", rateLimit(limiter, ipKey("handreveal:create")), h.reveal)
	g.Get("/reveal-winner", h.check)
}

func (h *handRevealHandlers) loadEligibleRecord(c fiber.Ctx, playerID, handID string) (*handreveal.HandRecord, *problem.Problem) {
	// The caller must have been dealt into this hand — the only place that
	// already ties a player to a hand id today (mirrors handshares.go's
	// create handler, which does the same lookup for the same reason).
	participant, err := h.sessions.GetHand(c.Context(), playerID, roomstore.CurrencyModeSandbox, handID)
	if err != nil {
		return nil, problem.InternalServer("failed to load hand", c, err)
	}
	if participant == nil {
		return nil, problem.NotFound("hand not found")
	}
	record, err := h.records.Get(c.Context(), handID)
	if err != nil {
		return nil, problem.InternalServer("failed to load hand reveal", c, err)
	}
	if record == nil {
		return nil, problem.NotFound("hand reveal not available")
	}
	return record, nil
}

func (h *handRevealHandlers) reveal(c fiber.Ctx) error {
	playerID := c.Locals(localsUserID).(string)
	handID, err := url.PathUnescape(c.Params("handId"))
	if err != nil || handID == "" {
		return problem.BadRequest("hand id is invalid").Send(c)
	}
	record, prob := h.loadEligibleRecord(c, playerID, handID)
	if prob != nil {
		return prob.Send(c)
	}
	if record.WinnerShown {
		return problem.BadRequest("the winner's cards were already shown").Send(c)
	}
	if playerID == record.WinnerID {
		return problem.BadRequest("the winner cannot buy their own hand").Send(c)
	}
	paid, err := h.svc.HasPaid(c.Context(), handID, playerID)
	if err != nil {
		return problem.InternalServer("failed to check payment", c, err).Send(c)
	}
	if paid {
		return problem.BadRequest("already purchased").Send(c)
	}
	if err := h.svc.PayForReveal(c.Context(), playerID, record.WinnerID, handID, record.BigBlind); err != nil {
		return walletOrInternalProblem(err, "reveal purchase failed", c).Send(c)
	}
	cards := record.PlayerHands[record.WinnerID].Cards
	return c.JSON(fiber.Map{"cards": cards[:]})
}

func (h *handRevealHandlers) check(c fiber.Ctx) error {
	playerID := c.Locals(localsUserID).(string)
	handID, err := url.PathUnescape(c.Params("handId"))
	if err != nil || handID == "" {
		return problem.BadRequest("hand id is invalid").Send(c)
	}
	record, prob := h.loadEligibleRecord(c, playerID, handID)
	if prob != nil {
		return prob.Send(c)
	}
	paid, err := h.svc.HasPaid(c.Context(), handID, playerID)
	if err != nil {
		return problem.InternalServer("failed to check payment", c, err).Send(c)
	}
	resp := fiber.Map{"fee": record.BigBlind, "already_paid": paid}
	if paid {
		cards := record.PlayerHands[record.WinnerID].Cards
		resp["cards"] = cards[:]
	}
	return c.JSON(resp)
}
