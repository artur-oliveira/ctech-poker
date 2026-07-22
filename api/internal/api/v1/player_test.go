package v1

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/player"
)

type fakePlayerStore struct{ profile player.PlayerProfile }

func (s *fakePlayerStore) GetOrCreate(_ context.Context, id string) (*player.PlayerProfile, error) {
	s.profile.UserID = id
	return &s.profile, nil
}
func (s *fakePlayerStore) AcceptTerms(_ context.Context, id string) error {
	s.profile.UserID = id
	s.profile.PokerTermsVersion = player.CurrentPokerTermsVersion
	s.profile.TermsAcceptedAt = "now"
	return nil
}
func (s *fakePlayerStore) SetName(_ context.Context, id string, name string) error {
	s.profile.UserID = id
	s.profile.Name = name
	return nil
}
func (s *fakePlayerStore) SetWalletMode(_ context.Context, id string, mode string) error {
	s.profile.UserID = id
	s.profile.WalletMode = mode
	return nil
}

func TestPlayerTermsLifecycle(t *testing.T) {
	store := &fakePlayerStore{}
	h := &playerHandlers{players: player.NewService(store)}
	app := fiber.New()
	auth := func(c fiber.Ctx) error { c.Locals(localsUserID, "u1"); return c.Next() }
	app.Get("/players/me", auth, h.me)
	app.Post("/players/me/terms/accept", auth, h.acceptTerms)
	assertAccepted := func(method, path string, want bool) {
		resp, err := app.Test(httptest.NewRequest(method, path, nil))
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Accepted bool `json:"poker_terms_accepted"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Accepted != want {
			t.Fatalf("%s: got %v want %v", path, body.Accepted, want)
		}
	}
	assertAccepted(fiber.MethodGet, "/players/me", false)
	assertAccepted(fiber.MethodPost, "/players/me/terms/accept", true)
	assertAccepted(fiber.MethodGet, "/players/me", true)
}

func TestUpdateMeSetsName(t *testing.T) {
	store := &fakePlayerStore{}
	h := &playerHandlers{players: player.NewService(store)}
	app := fiber.New()
	auth := func(c fiber.Ctx) error { c.Locals(localsUserID, "u1"); return c.Next() }
	app.Post("/players/me", auth, h.updateMe)

	req := httptest.NewRequest(fiber.MethodPost, "/players/me", strings.NewReader(`{"name":"Artur"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Name != "Artur" {
		t.Fatalf("Name = %q, want %q", body.Name, "Artur")
	}

	req = httptest.NewRequest(fiber.MethodPost, "/players/me", strings.NewReader(`{"name":"  "}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestUpdateMeSetsWalletModeWithoutTouchingName(t *testing.T) {
	store := &fakePlayerStore{profile: player.PlayerProfile{Name: "Artur"}}
	h := &playerHandlers{players: player.NewService(store)}
	app := fiber.New()
	auth := func(c fiber.Ctx) error { c.Locals(localsUserID, "u1"); return c.Next() }
	app.Post("/players/me", auth, h.updateMe)

	req := httptest.NewRequest(fiber.MethodPost, "/players/me", strings.NewReader(`{"wallet_mode":"real"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		Name       string `json:"name"`
		WalletMode string `json:"wallet_mode"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Name != "Artur" {
		t.Fatalf("Name = %q, want untouched %q", body.Name, "Artur")
	}
	if body.WalletMode != "real" {
		t.Fatalf("WalletMode = %q, want %q", body.WalletMode, "real")
	}

	req = httptest.NewRequest(fiber.MethodPost, "/players/me", strings.NewReader(`{"wallet_mode":"bogus"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}
