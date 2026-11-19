package v1

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/problem"
	"gopkg.aoctech.app/api-commons/pushsvc"
	"gopkg.aoctech.app/api-commons/ratelimit"
	"gopkg.aoctech.app/poker/api/internal/pokerpush"
)

// PushJobSender hands a push job to the sender Lambda (pushsvc.Invoker).
type PushJobSender interface {
	Send(ctx context.Context, job pushsvc.Job) error
}

// PushDeps is what the push routes need; app wires it from config and the AWS clients.
type PushDeps struct {
	Service *pushsvc.Service
	Prefs   *pokerpush.Service
	Sender  PushJobSender
}

// RegisterPush mounts /v1.0/push/*, first-party only (a device belongs to a person and no public scope grants it, like
// /social): the device routes of pushsvc (config, subscriptions) plus the player's reminder
// preferences and a test push to the caller's own devices. Subscribing seeds "come play" on (the service's OnSubscribe
// hook is set where it is built). docs/specs/2026-11-18-scheduled-web-push.md.
func RegisterPush(router fiber.Router, auth fiber.Handler, push *pushsvc.Service, prefs *pokerpush.Service, sender PushJobSender, testLimiter *ratelimit.Limiter) {
	userID := func(c fiber.Ctx) string { return c.Locals(localsUserID).(string) }
	pushsvc.Mount(router, push, auth, userID, firstPartyOnly)

	g := router.Group("/push", auth, firstPartyOnly)
	g.Get("/preferences", func(c fiber.Ctx) error {
		p, err := prefs.Get(c.Context(), userID(c))
		if err != nil {
			return internalProblem("preferences read failed", err).Send(c)
		}
		return c.JSON(p)
	})
	g.Put("/preferences", func(c fiber.Ctx) error {
		var body pokerpush.Preferences
		if p := bindBody(c, &body); p != nil {
			return p.Send(c)
		}
		p, err := prefs.Set(c.Context(), userID(c), body)
		if err != nil {
			return problem.FromError(err).Send(c)
		}
		return c.JSON(p)
	})
	g.Post("/test", rateLimit(testLimiter, playerKey("push-test")), func(c fiber.Ctx) error {
		if !push.Enabled() {
			return unavailable("notificações desativadas nesta instalação").Send(c)
		}
		id := make([]byte, 12)
		_, _ = rand.Read(id)
		err := sender.Send(c.Context(), pushsvc.Job{
			EventID: hex.EncodeToString(id), Type: "test", Title: "Teste de notificação",
			Body: "Se você viu isto, as notificações estão funcionando.", Navigate: "/", Tag: "test", Topic: "test",
			TTLSeconds: 60, Urgency: "high", Recipients: []string{userID(c)},
		})
		if err != nil {
			return internalProblem("test push failed", err).Send(c)
		}
		return c.SendStatus(fiber.StatusAccepted)
	})
}
