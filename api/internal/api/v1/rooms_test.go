package v1

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
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

func TestCreatePublicRoomRejectsTurnTimeoutSeconds(t *testing.T) {
	app := fiber.New()
	h := &roomHandlers{}
	app.Post("/rooms", func(c fiber.Ctx) error { c.Locals(localsUserID, "u1"); return c.Next() }, h.createRoom)
	body := []byte(`{"visibility":"public","small_blind":10,"big_blind":20,"max_seats":9,"buy_in_min":400,"buy_in_max":2000,"turn_timeout_seconds":20}`)
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

func TestCreatePrivateRoomRejectsTurnTimeoutBelowMinimum(t *testing.T) {
	app := fiber.New()
	h := &roomHandlers{}
	app.Post("/rooms", func(c fiber.Ctx) error { c.Locals(localsUserID, "u1"); return c.Next() }, h.createRoom)
	body := []byte(`{"visibility":"private","small_blind":10,"big_blind":20,"max_seats":6,"buy_in_min":400,"buy_in_max":2000,"turn_timeout_seconds":3}`)
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

func TestCreatePrivateRoomAcceptsValidTurnTimeout(t *testing.T) {
	app := fiber.New()
	h := &roomHandlers{}
	app.Post("/rooms", func(c fiber.Ctx) error { c.Locals(localsUserID, "u1"); return c.Next() }, h.createRoom)
	body := []byte(`{"visibility":"private","small_blind":10,"big_blind":20,"max_seats":6,"buy_in_min":400,"buy_in_max":2000,"turn_timeout_seconds":20}`)
	req := httptest.NewRequest(fiber.MethodPost, "/rooms", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("got %d", resp.StatusCode)
	}
	var room roomstore.Room
	if err := json.NewDecoder(resp.Body).Decode(&room); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if room.TurnTimeoutSeconds != 20 {
		t.Fatalf("expected TurnTimeoutSeconds 20, got %d", room.TurnTimeoutSeconds)
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

func TestSanitizeRoomHidesShareCodeFromNonCreators(t *testing.T) {
	room := &roomstore.Room{ID: "r1", Visibility: "private", ShareCode: "ABCD1234", CreatedBy: "owner"}
	if got := sanitizeRoom(room, "someone-else"); got.ShareCode != "" {
		t.Fatalf("share code leaked to non-creator: %q", got.ShareCode)
	}
	if got := sanitizeRoom(room, "owner"); got.ShareCode != "ABCD1234" {
		t.Fatal("creator should still see the share code")
	}
	if room.ShareCode != "ABCD1234" {
		t.Fatal("sanitize must not mutate the stored room")
	}
}

func TestPrivateRoomAccessRequiresShareCode(t *testing.T) {
	room := &roomstore.Room{ID: "r1", Visibility: "private", ShareCode: "ABCD1234", CreatedBy: "owner"}
	if privateRoomAccessAllowed(room, "guest", "") {
		t.Fatal("guest without share code must be rejected")
	}
	if privateRoomAccessAllowed(room, "guest", "WRONG000") {
		t.Fatal("wrong share code must be rejected")
	}
	if !privateRoomAccessAllowed(room, "guest", "ABCD1234") {
		t.Fatal("correct share code must be accepted")
	}
	if !privateRoomAccessAllowed(room, "owner", "") {
		t.Fatal("creator must always be allowed")
	}
	if !privateRoomAccessAllowed(&roomstore.Room{Visibility: "public"}, "guest", "") {
		t.Fatal("public rooms are always allowed")
	}
}
