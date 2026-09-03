package v1

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/valkey-io/valkey-go"
	"gopkg.aoctech.app/api-commons/cache"
)

// RateLimiter is a fixed-window limiter keyed by caller. In prod the backend is
// Redis (mandatory, T2) and counting is atomic via INCR+EXPIRE; in dev the
// in-memory backend uses a per-instance mutex map. It stops a script from
// spamming room creation or sandbox chip spins (M6/S2).
type RateLimiter struct {
	// incr is nil unless the backend is Redis. It returns key's hit count in
	// the current window, which is what makes the counter fleet-wide rather
	// than per-instance (#43). Tests substitute a shared fake to simulate two
	// API instances counting against one player's budget.
	incr   func(ctx context.Context, key string) (int64, error)
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
		rl.incr = redisIncr(rb.Client(), window)
	}
	return rl
}

func redisIncr(client valkey.Client, window time.Duration) func(context.Context, string) (int64, error) {
	return func(ctx context.Context, key string) (int64, error) {
		n, err := client.Do(ctx, client.B().Incr().Key(key).Build()).ToInt64()
		if err != nil {
			return 0, err
		}
		if n == 1 {
			// First hit in this window: bound the key's lifetime to one window so
			// the counter eventually resets without manual cleanup.
			client.Do(ctx, client.B().Expire().Key(key).Seconds(int64(window.Seconds())).Build())
		}
		return n, nil
	}
}

// Allow reports whether key is still within its window. Safe for concurrent use.
func (r *RateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	if r.incr != nil {
		n, err := r.incr(ctx, key)
		if err != nil {
			return false, err
		}
		return n <= int64(r.limit), nil
	}
	return r.allowMem(key), nil
}

// AllowFailOpen applies rateLimit's fail-open policy outside the HTTP
// middleware chain — the table WebSocket loop, which has no c.Next() to fall
// through to. A nil limiter (dev wiring, tests) allows everything.
func (r *RateLimiter) AllowFailOpen(ctx context.Context, key string) bool {
	if r == nil {
		return true
	}
	allow, err := r.Allow(ctx, key)
	if err != nil {
		slog.Warn("rate limiter backend error; allowing action", "err", err)
		return true
	}
	return allow
}

// wsActionKey / wsReactionKey are the fleet-wide WebSocket limiter keys. They
// are per player, never per connection: a client spread across instances or
// reconnecting must not multiply its allowance (#43).
func wsActionKey(playerID string) string   { return "ws:act:" + playerID }
func wsReactionKey(playerID string) string { return "ws:react:" + playerID }

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
		if rl == nil {
			return c.Next()
		}
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

func playerKey(route string) func(c fiber.Ctx) string {
	return func(c fiber.Ctx) string {
		playerID, _ := c.Locals(localsUserID).(string)
		return "rl:" + route + ":player:" + playerID
	}
}
