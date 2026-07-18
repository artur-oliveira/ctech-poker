package app

import (
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestHealthEndpointReturnsOK(t *testing.T) {
	app := fiber.New()
	registerRoutes(app)

	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
