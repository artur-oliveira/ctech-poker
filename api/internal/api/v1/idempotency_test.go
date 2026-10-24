package v1

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/idempotency"
)

// A retry with the same key must replay the first answer, not run the money
// route twice; without a key the route is untouched; two players never share a key.
func TestIdempotencyKeyReplaysAndIsScopedPerPlayer(t *testing.T) {
	var runs atomic.Int32
	idem := idempotency.New(idempotency.Config{
		Store: &idempotency.MemoryStore{},
		Scope: func(c fiber.Ctx) (string, bool) { id, ok := c.Locals(localsUserID).(string); return id, ok },
	})
	app := fiber.New()
	g := app.Group("/", func(c fiber.Ctx) error { c.Locals(localsUserID, c.Get("X-User")); return c.Next() }, optional(idem))
	g.Post("/buy", func(c fiber.Ctx) error { runs.Add(1); return c.Status(http.StatusCreated).SendString("ok") })

	do := func(user, key string) *http.Response {
		req := httptest.NewRequest(http.MethodPost, "/buy", nil)
		req.Header.Set("X-User", user)
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	do("a", "k1")
	if r := do("a", "k1"); r.Header.Get("Idempotent-Replay") != "true" || runs.Load() != 1 {
		t.Fatalf("same key must replay: header=%q runs=%d", r.Header.Get("Idempotent-Replay"), runs.Load())
	}
	do("b", "k1")
	do("a", "")
	if runs.Load() != 3 {
		t.Fatalf("other player and keyless calls must run: runs=%d", runs.Load())
	}
}

func TestOptionalIsPassThroughWithoutStore(t *testing.T) {
	app := fiber.New()
	app.Post("/x", optional(nil), func(c fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })
	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/x", nil))
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("resp=%v err=%v", resp, err)
	}
}
