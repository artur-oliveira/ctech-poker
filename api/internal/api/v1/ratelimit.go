package v1

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/valkey-io/valkey-go"
	"gopkg.aoctech.app/api-commons/cache"
)

// incrAndBoundTTLScript atomically increments key and guarantees it carries a
// TTL, closing the race from #45: a plain INCR followed by a separate
// "EXPIRE only when n == 1" left the key permanently TTL-less whenever the
// EXPIRE step failed (or the process died between the two calls), so the
// counter never reset and the bucket stayed rate-limited forever. Running
// both steps as one Lua script makes them atomic from Redis's point of view,
// and re-checking TTL on every hit (not just the first) means a key that
// somehow lost its TTL self-heals on its very next hit instead of staying
// stuck. redis.call('TTL', ...) returns -1 for "exists, no expiry" and -2 for
// "doesn't exist"; a fresh INCR always creates the key, so only -1 is
// reachable here, but the check is written to catch either.
const incrAndBoundTTLScript = `
local n = redis.call('INCR', KEYS[1])
local ttl = redis.call('TTL', KEYS[1])
if ttl < 0 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return n`

// redisCounter is the one Valkey/Redis operation the rate limiter needs: an
// atomic "increment and guarantee a TTL". It is factored out of RateLimiter
// so allowRedis's bucket-arithmetic and TTL-recovery paths can be unit-tested
// against a fake, without depending on valkey-go's (unexported) wire-message
// types.
type redisCounter interface {
	incrAndBoundTTL(ctx context.Context, key string, windowSeconds int64) (int64, error)
}

// valkeyCounter adapts a real valkey.CommandClient to redisCounter.
type valkeyCounter struct{ client valkey.CommandClient }

func (v valkeyCounter) incrAndBoundTTL(ctx context.Context, key string, windowSeconds int64) (int64, error) {
	return v.client.Do(ctx, v.client.B().Eval().Script(incrAndBoundTTLScript).Numkeys(1).
		Key(key).Arg(strconv.FormatInt(windowSeconds, 10)).Build()).ToInt64()
}

// RateLimiter is a fixed-window limiter keyed by caller. In prod the backend is
// Redis (mandatory, T2) and counting is atomic via a Lua script that combines
// INCR with a TTL guarantee (see incrAndBoundTTLScript); in dev the in-memory
// backend uses a per-instance mutex map. It stops a script from spamming room
// creation or sandbox chip spins (M6/S2).
type RateLimiter struct {
	// client is nil unless the backend is Redis. Counting there is what makes
	// the budget fleet-wide rather than per-instance (#43); tests substitute a
	// fake to simulate two API instances counting against one player's budget.
	client redisCounter
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
		rl.client = valkeyCounter{client: rb.Client()}
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

func (r *RateLimiter) allowRedis(ctx context.Context, key string) (bool, error) {
	n, err := r.client.incrAndBoundTTL(ctx, key, int64(r.window.Seconds()))
	if err != nil {
		return false, err
	}
	return n <= int64(r.limit), nil
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
