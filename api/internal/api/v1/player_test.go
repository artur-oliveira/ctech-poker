package v1

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
)

type mockHistoryReader struct{}

func (m *mockHistoryReader) ListSessions(_ context.Context, playerID string, _ int, _ map[string]types.AttributeValue) ([]sessionlog.SessionItem, map[string]types.AttributeValue, error) {
	return []sessionlog.SessionItem{{PK: playerID, TableID: "tbl-1", NetPnL: 100}}, nil, nil
}

func (m *mockHistoryReader) ListHands(_ context.Context, playerID string, _ int, _ map[string]types.AttributeValue) ([]sessionlog.HandItem, map[string]types.AttributeValue, error) {
	return []sessionlog.HandItem{{PK: playerID, HandID: "h-1", NetChange: 50}}, nil, nil
}
func (m *mockHistoryReader) ListHandsByTable(_ context.Context, playerID, tableID string, _ int, _ map[string]types.AttributeValue) ([]sessionlog.HandItem, map[string]types.AttributeValue, error) {
	return []sessionlog.HandItem{{PK: playerID, HandID: "h-1", NetChange: 50, TableID: tableID}}, nil, nil
}
func (m *mockHistoryReader) GetHand(_ context.Context, playerID, handID string) (*sessionlog.HandItem, error) {
	return &sessionlog.HandItem{PK: playerID, HandID: handID, NetChange: 50}, nil
}

func TestPlayerHistoryEndpoints(t *testing.T) {
	app := fiber.New()
	auth := func(c fiber.Ctx) error { c.Locals(localsUserID, "user-123"); return c.Next() }
	RegisterPlayers(app.Group("/v1.0"), auth, player.NewService(&fakePlayerStore{}), &mockHistoryReader{}, nil, nil, nil, nil)

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

type fakePlayerStore struct{ profile player.PlayerProfile }

func (s *fakePlayerStore) GetOrCreate(_ context.Context, id string) (*player.PlayerProfile, error) {
	s.profile.UserID = id
	return &s.profile, nil
}
func (s *fakePlayerStore) Get(_ context.Context, id string) (*player.PlayerProfile, error) {
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
func (s *fakePlayerStore) SetDeckVariant(_ context.Context, id string, variant string) error {
	s.profile.UserID = id
	s.profile.DeckVariant = variant
	return nil
}
func (s *fakePlayerStore) SetShowcase(_ context.Context, id string, public bool, featured []string) error {
	s.profile.UserID = id
	s.profile.ShowcasePublic = public
	s.profile.FeaturedAchievements = featured
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

	req := httptest.NewRequest(fiber.MethodPost, "/players/me", strings.NewReader(`{"wallet_mode":"sandbox"}`))
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
	if body.WalletMode != "sandbox" {
		t.Fatalf("WalletMode = %q, want %q", body.WalletMode, "sandbox")
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
