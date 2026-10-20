package v1

import (
	"context"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/ratelimit"
	"gopkg.aoctech.app/api-commons/ratelimit/fiberlimit"

	"gopkg.aoctech.app/api-commons/problem"
)

// newLimiterCounter picks the counting backend: Valkey in prod (mandatory, T2:
// that is what makes a budget fleet-wide, #43), per-process memory in dev.
func newLimiterCounter(backend cache.Backend) ratelimit.Counter {
	if rb, ok := backend.(*cache.RedisBackend); ok {
		return ratelimit.NewValkeyCounter(rb.Client())
	}
	return ratelimit.NewMemoryCounter()
}

// newLimiter builds a fail-open fixed-window limiter: max events per window per
// identity. A counter blip must never block legitimate play — rate limiting is
// abuse mitigation here, not a correctness gate.
func newLimiter(counter ratelimit.Counter, prefix string, max int64, window time.Duration) *ratelimit.Limiter {
	return &ratelimit.Limiter{Counter: counter, Prefix: prefix, Max: max, Window: window, FailMode: ratelimit.FailOpen}
}

// allowFailOpen applies the fail-open policy outside the HTTP middleware chain:
// the table WebSocket loop (no c.Next(); ids are per player, never per
// connection, so a reconnect cannot multiply the allowance, #43) and handlers
// with their own 429 body. A nil limiter (dev wiring, tests) allows everything.
func allowFailOpen(ctx context.Context, l *ratelimit.Limiter, id string) bool {
	if l == nil {
		return true
	}
	out, err := l.Take(ctx, id)
	if err != nil {
		slog.Warn("rate limiter backend error; allowing action", "prefix", l.Prefix, "err", err)
	}
	return out != ratelimit.Deny
}

// rateLimit is Fiber middleware that returns 429 once the identity from keyFn
// exceeds the limit. #319: retry_after_seconds is the window's remaining time.
func rateLimit(l *ratelimit.Limiter, keyFn func(c fiber.Ctx) string) fiber.Handler {
	return fiberlimit.NewPolicy(fiberlimit.PolicyConfig{
		Limiter: l,
		Key:     keyFn,
		Deny: func(c fiber.Ctx, retryAfterSeconds int) error {
			return problem.TooManyRequestsAfter(limiterTooManyRequestsDetail, retryAfterSeconds).Send(c)
		},
	})
}

// ipKey builds a per-IP rate-limit id for a named route.
func ipKey(route string) func(c fiber.Ctx) string {
	return func(c fiber.Ctx) string {
		return route + ":" + c.IP()
	}
}

func playerKey(route string) func(c fiber.Ctx) string {
	return func(c fiber.Ctx) string {
		playerID, _ := c.Locals(localsUserID).(string)
		return route + ":player:" + playerID
	}
}
