package v1

import (
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/dailyreward"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

type dailyRewardHandlers struct{ svc *dailyreward.Service }

func RegisterDailyReward(router fiber.Router, auth fiber.Handler, svc *dailyreward.Service, spinLimiter *RateLimiter) {
	h := &dailyRewardHandlers{svc: svc}
	g := router.Group("/sandbox-credits", auth)
	g.Post("/", rateLimit(spinLimiter, ipKey("dailyreward:spin")), h.spin)
	g.Get("/", h.cooldown)
}

// spin claims today's slot on the streak trail. The response keeps `amount`
// and `remaining_time_seconds` exactly where they were and adds the refreshed
// streak calendar, so a client that only reads the cooldown is unaffected.
func (h *dailyRewardHandlers) spin(c fiber.Ctx) error {
	userID := c.Locals(localsUserID).(string)
	status, err := h.svc.Status(c.Context(), userID)
	if err != nil {
		return problem.InternalServer("spin failed", c, err).Send(c)
	}
	amount := int64(0)
	if !status.ClaimedToday {
		won, _, spinErr := h.svc.Spin(c.Context(), userID)
		if spinErr != nil {
			return walletOrInternalProblem(spinErr, "spin failed", c).Send(c)
		}
		amount = won
		if status, err = h.svc.Status(c.Context(), userID); err != nil {
			return problem.InternalServer("spin failed", c, err).Send(c)
		}
	}
	return c.JSON(spinResponse{Amount: amount, Status: status})
}

// spinResponse inlines the calendar next to the claimed amount so the client
// never needs a follow-up GET to repaint the trail after a claim.
type spinResponse struct {
	Amount int64 `json:"amount"`
	dailyreward.Status
}

func (h *dailyRewardHandlers) cooldown(c fiber.Ctx) error {
	status, err := h.svc.Status(c.Context(), c.Locals(localsUserID).(string))
	if err != nil {
		return problem.InternalServer("cooldown check failed", c, err).Send(c)
	}
	return c.JSON(status)
}

// walletOrInternalProblem passes ctech-wallet's own problem+json straight
// through (e.g. once ctech-wallet auto-creates a sandbox wallet on credit,
// any error left is a real business error like "no wallet in real-money
// mode" that the frontend needs to see and act on) — anything else (store
// failures, tier-pick failures) stays a generic internal error.
func walletOrInternalProblem(err error, detail string, c fiber.Ctx) *problem.Problem {
	if p, ok := problem.FromWalletError(err); ok {
		return p
	}
	return problem.InternalServer(detail, c, err)
}
