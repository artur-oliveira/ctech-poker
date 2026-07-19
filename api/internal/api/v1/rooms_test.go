package v1

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestCreatePublicRoomRejectsBlindEscalation(t *testing.T) {
	app := fiber.New()
	h := &roomHandlers{}
	app.Post("/rooms", func(c fiber.Ctx) error { c.Locals(localsUserID, "u1"); return c.Next() }, h.createRoom)
	body := []byte(`{"visibility":"public","small_blind":10,"big_blind":20,"max_seats":9,"buy_in_min":400,"buy_in_max":2000,"blind_escalation":{"interval_minutes":10,"multiplier":150,"max":100}}`)
	req := httptest.NewRequest(fiber.MethodPost, "/rooms", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("got %d", resp.StatusCode)
	}
}

func TestCreateRoomRejectsInvalidBuyInMultiples(t *testing.T) {
	app := fiber.New()
	h := &roomHandlers{}
	app.Post("/rooms", func(c fiber.Ctx) error { c.Locals(localsUserID, "u1"); return c.Next() }, h.createRoom)
	body := []byte(`{"visibility":"private","small_blind":10,"big_blind":20,"max_seats":6,"buy_in_min":401,"buy_in_max":2000}`)
	req := httptest.NewRequest(fiber.MethodPost, "/rooms", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("got %d", resp.StatusCode)
	}
}

func TestPublicSandboxStakesAreCurated(t *testing.T) {
	if !isAllowedPublicStake("sandbox", 10, 25) || !isAllowedPublicStake("sandbox", 50000, 100000) {
		t.Fatal("expected the lowest and highest sandbox stakes to be allowed")
	}
	if isAllowedPublicStake("sandbox", 10, 20) {
		t.Fatal("uncurated public stake was accepted")
	}
	if isAllowedPublicStake("real", 10000, 25000) {
		t.Fatal("sandbox-only high stake leaked into the real-money catalog")
	}
}
