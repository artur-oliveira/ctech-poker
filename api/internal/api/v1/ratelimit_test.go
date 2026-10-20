package v1

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/ratelimit"

	"gopkg.aoctech.app/api-commons/problem"
)

// failingCounter simulates Valkey being down.
type failingCounter struct{ ratelimit.Counter }

func (failingCounter) Incr(context.Context, string, time.Duration) (int64, error) {
	return 0, errors.New("valkey down")
}

func TestSeatLimiterBlocksBurstAboveLimit(t *testing.T) {
	l := newLimiter(ratelimit.NewMemoryCounter(), "ws-action", 3, time.Second)
	for i := range 3 {
		if !allowFailOpen(context.Background(), l, "p1") {
			t.Fatalf("expected request %d within limit to be allowed", i)
		}
	}
	if allowFailOpen(context.Background(), l, "p1") {
		t.Fatal("expected 4th request in the same window to be blocked")
	}
}

func TestSeatLimiterTracksPlayersIndependently(t *testing.T) {
	l := newLimiter(ratelimit.NewMemoryCounter(), "ws-action", 1, time.Second)
	if !allowFailOpen(context.Background(), l, "p1") {
		t.Fatal("expected p1's first request allowed")
	}
	if !allowFailOpen(context.Background(), l, "p2") {
		t.Fatal("expected p2's first request allowed independently of p1's count")
	}
}

// The #43 regression: two instances, one player, one budget (one shared counter
// stands in for the fleet's Valkey).
func TestWSLimiterBudgetIsSharedAcrossInstances(t *testing.T) {
	fleet := ratelimit.NewMemoryCounter()
	instanceA := newLimiter(fleet, "ws-action", 3, time.Minute)
	instanceB := newLimiter(fleet, "ws-action", 3, time.Minute)
	ctx := context.Background()

	for i := range 2 {
		if !allowFailOpen(ctx, instanceA, "p1") {
			t.Fatalf("action %d on instance A must be allowed", i+1)
		}
	}
	if !allowFailOpen(ctx, instanceB, "p1") {
		t.Fatal("the third action must still be within the shared budget")
	}
	if allowFailOpen(ctx, instanceB, "p1") {
		t.Fatal("reconnecting to another instance must not refill the player's budget")
	}
	if !allowFailOpen(ctx, instanceB, "p2") {
		t.Fatal("one player's exhausted budget must not block another player")
	}
}

func TestLimitersFailOpenOnBackendError(t *testing.T) {
	l := newLimiter(failingCounter{}, "ws-action", 1, time.Second)
	if !allowFailOpen(context.Background(), l, "p1") {
		t.Fatal("a backend outage must never block legitimate play")
	}
	app := fiber.New()
	app.Get("/x", rateLimit(l, ipKey("x")), func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
	resp, err := app.Test(httptest.NewRequest("GET", "/x", nil))
	if err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("middleware must fail open: err=%v status=%v", err, resp)
	}
}

func TestNilLimiterAllows(t *testing.T) {
	if !allowFailOpen(context.Background(), nil, "p1") {
		t.Fatal("an unwired limiter must allow (dev/test path)")
	}
	app := fiber.New()
	app.Get("/x", rateLimit(nil, ipKey("x")), func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
	if resp, err := app.Test(httptest.NewRequest("GET", "/x", nil)); err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("nil limiter middleware must pass through: err=%v status=%v", err, resp)
	}
}

// #319: a rate-limited request gets a problem+json body with next_action and
// retry_after_seconds (the window's remaining time, here at most the window).
func TestRateLimitMiddlewareReturnsRetryAfterSeconds(t *testing.T) {
	app := fiber.New()
	rl := newLimiter(ratelimit.NewMemoryCounter(), "x", 1, 30*time.Second)
	app.Get("/x", rateLimit(rl, ipKey("x")), func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	if resp, err := app.Test(httptest.NewRequest("GET", "/x", nil)); err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("first request: err=%v status=%v", err, resp)
	}
	resp, err := app.Test(httptest.NewRequest("GET", "/x", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusTooManyRequests {
		t.Fatalf("status=%d, want 429", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != problem.ContentType {
		t.Fatalf("content-type=%q, want %q", got, problem.ContentType)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["next_action"] != "retry" || body["detail"] != "too many requests, slow down" {
		t.Fatalf("body=%+v", body)
	}
	if secs, _ := body["retry_after_seconds"].(float64); secs < 1 || secs > 30 {
		t.Fatalf("retry_after_seconds=%v, want 1..30", body["retry_after_seconds"])
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Fatal("Retry-After header missing")
	}
}
