package v1

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
)

type fakeHistoryStore struct{}

func (f *fakeHistoryStore) LoadActionsSince(_ context.Context, _, _ string, _ int) ([]HistoryAction, error) {
	return []HistoryAction{{PlayerID: "p1", Action: "call", Seq: 1, Amount: 20}}, nil
}

func TestHandHistoryReturnsActionLog(t *testing.T) {
	app := fiber.New()
	allow := func(c fiber.Ctx) error { return c.Next() }
	RegisterHandHistory(app.Group("/v1.0"), allow, &fakeHistoryStore{})

	req := httptest.NewRequest(fiber.MethodGet, "/v1.0/tables/t1/hands/h1/history", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestHandHistoryRequiresAuth(t *testing.T) {
	app := fiber.New()
	deny := func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusUnauthorized) }
	RegisterHandHistory(app.Group("/v1.0"), deny, &fakeHistoryStore{})

	req := httptest.NewRequest(fiber.MethodGet, "/v1.0/tables/t1/hands/h1/history", nil)
	resp, err := app.Test(req)
	if err != nil || resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("expected 401 from auth middleware, got %d, err %v", resp.StatusCode, err)
	}
}
