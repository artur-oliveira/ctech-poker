package v1

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/cache"
	"github.com/valkey-io/valkey-go"
)

// RateLimiter is a fixed-window limiter keyed by caller. In prod the backend is
// Redis (mandatory, T2) and counting is atomic via INCR+EXPIRE; in dev the
// in-memory backend uses a per-instance mutex map. It stops a script from
// spamming room creation or sandbox chip spins (M6/S2).
type RateLimiter struct {
	client valkey.Client // nil unless the backend is Redis
	limit  int
	window time.Duration

	mu  sync.Mutex
	mem map[string]*rateWindow
}

type rateWindow struct {
	count   int
	resetAt time.Time
}

// NewRateLimiter builds a limiter over backend. limit is the max requests per
// window; window is the fixed window length.
func NewRateLimiter(backend cache.Backend, limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{limit: limit, window: window, mem: make(map[string]*rateWindow)}
	if rb, ok := backend.(*cache.RedisBackend); ok {
		rl.client = rb.Client()
	}
	return rl
}

// Allow reports whether key is still within its window. Safe for concurrent use.
func (r *RateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	if r.client != nil {
		return r.allowRedis(ctx, key)
	}
	return r.allowMem(key), nil
}

func (r *RateLimiter) allowRedis(ctx context.Context, key string) (bool, error) {
	n, err := r.client.Do(ctx, r.client.B().Incr().Key(key).Build()).ToInt64()
	if err != nil {
		return false, err
	}
	if n == 1 {
		// First hit in this window: bound the key's lifetime to one window so
		// the counter eventually resets without manual cleanup.
		r.client.Do(ctx, r.client.B().Expire().Key(key).Seconds(int64(r.window.Seconds())).Build())
	}
	return n <= int64(r.limit), nil
}

func (r *RateLimiter) allowMem(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	w, ok := r.mem[key]
	if !ok || now.After(w.resetAt) {
		r.mem[key] = &rateWindow{count: 1, resetAt: now.Add(r.window)}
		return true
	}
	w.count++
	return w.count <= r.limit
}

// rateLimit is Fiber middleware that returns 429 once key exceeds the limit.
// A backend error fails open (allows) so a Redis blip never blocks legitimate
// play — rate limiting is abuse mitigation, not a correctness gate.
func rateLimit(rl *RateLimiter, keyFn func(c fiber.Ctx) string) fiber.Handler {
	return func(c fiber.Ctx) error {
		allow, err := rl.Allow(c.Context(), keyFn(c))
		if err != nil {
			slog.Warn("rate limiter backend error; allowing request", "err", err)
			return c.Next()
		}
		if !allow {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":   "rate_limit_exceeded",
				"message": "too many requests, slow down",
			})
		}
		return c.Next()
	}
}

// ipKey builds a per-IP rate-limit key for a named route.
func ipKey(route string) func(c fiber.Ctx) string {
	return func(c fiber.Ctx) string {
		return "rl:" + route + ":" + c.IP()
	}
}
