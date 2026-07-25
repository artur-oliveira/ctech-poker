package v1

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/dailyreward"
	"gopkg.aoctech.app/poker/api/internal/problem"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

type dailyRewardHandlers struct{ svc *dailyreward.Service }

func RegisterDailyReward(router fiber.Router, auth fiber.Handler, svc *dailyreward.Service, spinLimiter *RateLimiter) {
	h := &dailyRewardHandlers{svc: svc}
	g := router.Group("/sandbox-credits", auth)
	g.Post("/", rateLimit(spinLimiter, ipKey("dailyreward:spin")), h.spin)
	g.Get("/", h.cooldown)
}

func (h *dailyRewardHandlers) spin(c fiber.Ctx) error {
	userID := c.Locals(localsUserID).(string)
	seconds, err := h.svc.RemainingTime(c.Context(), userID)
	if err != nil {
		return problem.InternalServer("spin failed", c, err).Send(c)
	}
	if seconds == 0 {
		amount, rem, err := h.svc.Spin(c.Context(), userID)
		if err != nil {
			return walletOrInternalProblem(err, "spin failed", c).Send(c)
		}
		return c.JSON(fiber.Map{"amount": amount, "remaining_time_seconds": rem})
	}
	return c.JSON(fiber.Map{"amount": 0, "remaining_time_seconds": seconds})
}

func (h *dailyRewardHandlers) cooldown(c fiber.Ctx) error {
	seconds, err := h.svc.RemainingTime(c.Context(), c.Locals(localsUserID).(string))
	if err != nil {
		return problem.InternalServer("cooldown check failed", c, err).Send(c)
	}
	return c.JSON(fiber.Map{"remaining_time_seconds": seconds})
}

// walletOrInternalProblem passes ctech-wallet's own problem+json straight
// through (e.g. once ctech-wallet auto-creates a sandbox wallet on credit,
// any error left is a real business error like "no wallet in real-money
// mode" that the frontend needs to see and act on) — anything else (store
// failures, tier-pick failures) stays a generic internal error.
func walletOrInternalProblem(err error, detail string, c fiber.Ctx) *problem.Problem {
	var werr *walletclient.Error
	if errors.As(err, &werr) {
		return problem.New(werr.Status, werr.Type, werr.Title, werr.Detail)
	}
	return problem.InternalServer(detail, c, err)
}
