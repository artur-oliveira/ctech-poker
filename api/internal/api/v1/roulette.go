package v1

import (
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/dailyreward"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

func RegisterDailyReward(router fiber.Router, auth fiber.Handler, svc *dailyreward.Service, spinLimiter *RateLimiter) {
	router.Post("/sandbox-credits", auth, rateLimit(spinLimiter, ipKey("dailyreward:spin")), func(c fiber.Ctx) error {
		seconds, err := svc.RemainingTime(c.Context(), c.Locals(localsUserID).(string))
		if err != nil {
			return problem.InternalServer("spin failed", c, err).Send(c)
		}
		if seconds == 0 {
			amount, rem, err := svc.Spin(c.Context(), c.Locals(localsUserID).(string))
			if err != nil {
				return problem.InternalServer("spin failed", c, err).Send(c)
			}
			return c.JSON(fiber.Map{"amount": amount, "remaining_time_seconds": rem})
		}
		return c.JSON(fiber.Map{
			"amount":                 0,
			"remaining_time_seconds": seconds,
		})

	})

	router.Get("/sandbox-credits", auth, func(c fiber.Ctx) error {
		seconds, err := svc.RemainingTime(c.Context(), c.Locals(localsUserID).(string))
		if err != nil {
			return problem.InternalServer("cooldown check failed", c, err).Send(c)
		}
		return c.JSON(fiber.Map{"remaining_time_seconds": seconds})
	})
}
