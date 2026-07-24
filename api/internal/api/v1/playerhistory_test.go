package v1

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
)

type mockHistoryReader struct{}

func (m *mockHistoryReader) ListSessions(_ context.Context, playerID string, _ int) ([]sessionlog.SessionItem, error) {
	return []sessionlog.SessionItem{{PK: playerID, TableID: "tbl-1", NetPnL: 100}}, nil
}

func (m *mockHistoryReader) ListHands(_ context.Context, playerID string, _ int) ([]sessionlog.HandItem, error) {
	return []sessionlog.HandItem{{PK: playerID, HandID: "h-1", NetChange: 50}}, nil
}

func mockAuthMiddleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Locals(localsUserID, "user-123")
		return c.Next()
	}
}

func TestPlayerHistoryEndpoints(t *testing.T) {
	app := fiber.New()
	RegisterPlayerHistory(app.Group("/v1.0"), mockAuthMiddleware(), &mockHistoryReader{})

	t.Run("GET /players/me/sessions", func(t *testing.T) {
		req := httptest.NewRequest(fiber.MethodGet, "/v1.0/players/me/sessions", nil)
		resp, err := app.Test(req)
		if err != nil || resp.StatusCode != fiber.StatusOK {
			t.Fatalf("expected 200, got %d, err %v", resp.StatusCode, err)
		}
	})

	t.Run("GET /players/me/hands", func(t *testing.T) {
		req := httptest.NewRequest(fiber.MethodGet, "/v1.0/players/me/hands", nil)
		resp, err := app.Test(req)
		if err != nil || resp.StatusCode != fiber.StatusOK {
			t.Fatalf("expected 200, got %d, err %v", resp.StatusCode, err)
		}
	})
}
