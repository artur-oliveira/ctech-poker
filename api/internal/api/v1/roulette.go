package v1

import (
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/problem"
	"gopkg.aoctech.app/poker/api/internal/roulette"
)

func RegisterRoulette(router fiber.Router, auth fiber.Handler, svc *roulette.Service, spinLimiter *RateLimiter) {
	router.Post("/sandbox-credits", auth, rateLimit(spinLimiter, ipKey("roulette:spin")), func(c fiber.Ctx) error {
		amount, err := svc.Spin(c.Context(), c.Locals(localsUserID).(string))
		if err != nil {
			return problem.InternalServer("spin failed").Send(c)
		}
		return c.JSON(fiber.Map{"amount": amount})
	})

	router.Get("/sandbox-credits", auth, func(c fiber.Ctx) error {
		seconds, err := svc.RemainingTime(c.Context(), c.Locals(localsUserID).(string))
		if err != nil {
			return problem.InternalServer("cooldown check failed").Send(c)
		}
		return c.JSON(fiber.Map{"seconds": seconds})
	})
}
