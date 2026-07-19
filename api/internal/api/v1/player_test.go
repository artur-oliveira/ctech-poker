package v1

import (
	"context"
	"encoding/json"
	"net/http/httptest"
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
