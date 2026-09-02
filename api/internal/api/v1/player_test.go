package v1

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/achievements"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/pokerstats"
	"gopkg.aoctech.app/poker/api/internal/reports"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
)

type mockHistoryReader struct{}

type mockAchievementReader struct {
	all []achievements.PlayerAchievementProgress
}

func (mockAchievementReader) ListAchievements(context.Context, string, string, int, map[string]types.AttributeValue) ([]achievements.PlayerAchievementProgress, map[string]types.AttributeValue, error) {
	return nil, nil, nil
}

func (m mockAchievementReader) AllAchievements(context.Context, string, string) ([]achievements.PlayerAchievementProgress, error) {
	return m.all, nil
}

type mockPokerStatsReader struct{ stats pokerstats.Stats }

func (m mockPokerStatsReader) Get(context.Context, string, string) (pokerstats.Stats, error) {
	return m.stats, nil
}

func (m *mockHistoryReader) ListSessions(_ context.Context, playerID string, _ int, _ map[string]types.AttributeValue) ([]sessionlog.SessionItem, map[string]types.AttributeValue, error) {
	return []sessionlog.SessionItem{{PK: playerID, TableID: "tbl-1", NetPnL: 100}}, nil, nil
}

func (m *mockHistoryReader) ListHands(_ context.Context, playerID, mode string, _ int, _ map[string]types.AttributeValue) ([]sessionlog.HandItem, map[string]types.AttributeValue, error) {
	return []sessionlog.HandItem{{PK: playerID, HandID: "h-1", NetChange: 50}}, nil, nil
}
func (m *mockHistoryReader) ListHandsByTable(_ context.Context, playerID, mode, tableID string, _ int, _ map[string]types.AttributeValue) ([]sessionlog.HandItem, map[string]types.AttributeValue, error) {
	return []sessionlog.HandItem{{PK: playerID, HandID: "h-1", NetChange: 50, TableID: tableID}}, nil, nil
}
func (m *mockHistoryReader) GetHand(_ context.Context, playerID, mode, handID string) (*sessionlog.HandItem, error) {
	return &sessionlog.HandItem{PK: playerID, HandID: handID, NetChange: 50}, nil
}

func TestPlayerHistoryEndpoints(t *testing.T) {
	app := fiber.New()
	auth := func(c fiber.Ctx) error { c.Locals(localsUserID, "user-123"); return c.Next() }
	RegisterPlayers(app.Group("/v1.0"), auth, player.NewService(&fakePlayerStore{}), &mockHistoryReader{}, nil, nil, nil, nil, nil)

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

type fakePlayerStore struct {
	profile       player.PlayerProfile
	lookupID      string
	avatarReports int
}

func (s *fakePlayerStore) GetOrCreate(_ context.Context, id string) (*player.PlayerProfile, error) {
	s.profile.UserID = id
	return &s.profile, nil
}
func (s *fakePlayerStore) Get(_ context.Context, id string) (*player.PlayerProfile, error) {
	s.profile.UserID = id
	return &s.profile, nil
}
func (s *fakePlayerStore) LookupByFriendCode(_ context.Context, code string) (*player.PlayerProfile, error) {
	if normalized, ok := player.NormalizeFriendCode(code); !ok || normalized != s.profile.FriendCode {
		return nil, nil
	}
	id := s.lookupID
	if id == "" {
		id = "friend-target"
	}
	profile := s.profile
	profile.UserID = id
	return &profile, nil
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
func (s *fakePlayerStore) SetTableTheme(_ context.Context, id string, theme string) error {
	s.profile.UserID = id
	s.profile.TableTheme = theme
	return nil
}
func (s *fakePlayerStore) SetShowcase(_ context.Context, id string, public, playstylePublic, tablePublic bool, featured []string) error {
	s.profile.UserID = id
	s.profile.ShowcasePublic = public
	s.profile.PlaystylePublic = playstylePublic
	s.profile.TablePublic = tablePublic
	s.profile.FeaturedAchievements = featured
	return nil
}
func (s *fakePlayerStore) SetFavoriteReactions(_ context.Context, id string, favorites []string) error {
	s.profile.UserID = id
	s.profile.FavoriteReactions = favorites
	return nil
}
func (s *fakePlayerStore) ReportAvatar(context.Context, string, string) error {
	s.avatarReports++
	return nil
}

func TestLegacyAvatarReportAlsoCreatesModerationQueueItem(t *testing.T) {
	playerStore := &fakePlayerStore{}
	players := player.NewService(playerStore)
	reportStore := &apiReportStore{}
	h := &playerHandlers{players: players, reports: reports.NewService(reportStore, nil, players)}
	app := fiber.New()
	app.Post("/players/:playerId/avatar/report", func(c fiber.Ctx) error {
		c.Locals(localsUserID, "reporter")
		return c.Next()
	}, h.avatarReport)
	resp, err := app.Test(httptest.NewRequest(fiber.MethodPost, "/players/target/avatar/report", nil))
	if err != nil || resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("status=%d err=%v", resp.StatusCode, err)
	}
	if playerStore.avatarReports != 1 || reportStore.report.Category != reports.CategoryInappropriateProfile || reportStore.report.Surface != reports.SurfaceProfile {
		t.Fatalf("legacy=%d queue=%+v", playerStore.avatarReports, reportStore.report.Summary())
	}
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

func TestUpdateMeSetsFavoriteReactionsAndReturnsThem(t *testing.T) {
	store := &fakePlayerStore{}
	h := &playerHandlers{players: player.NewService(store)}
	app := fiber.New()
	auth := func(c fiber.Ctx) error { c.Locals(localsUserID, "u1"); return c.Next() }
	app.Post("/players/me", auth, h.updateMe)

	req := httptest.NewRequest(fiber.MethodPost, "/players/me", strings.NewReader(`{"favorite_reactions":["knife","flowers"]}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		FavoriteReactions []string `json:"favorite_reactions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if want := []string{"knife", "flowers"}; !reflect.DeepEqual(body.FavoriteReactions, want) {
		t.Fatalf("FavoriteReactions = %v, want %v (persisted but never echoed back is the regression this guards)", body.FavoriteReactions, want)
	}
}

// Regression: table_theme was fully wired through the service/store/catalog but
// updateMe never read the field and playerResponse never echoed it, so felt
// changes from the client were silently dropped.
func TestUpdateMeSetsTableThemeAndEchoesIt(t *testing.T) {
	store := &fakePlayerStore{}
	h := &playerHandlers{players: player.NewService(store)}
	app := fiber.New()
	auth := func(c fiber.Ctx) error { c.Locals(localsUserID, "u1"); return c.Next() }
	app.Post("/players/me", auth, h.updateMe)

	req := httptest.NewRequest(fiber.MethodPost, "/players/me", strings.NewReader(`{"table_theme":"classic"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		TableTheme string `json:"table_theme"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.TableTheme != "classic" {
		t.Fatalf("TableTheme = %q, want %q (persisted but never echoed back is the regression this guards)", body.TableTheme, "classic")
	}
	if store.profile.TableTheme != "classic" {
		t.Fatalf("store TableTheme = %q, want it persisted", store.profile.TableTheme)
	}

	req = httptest.NewRequest(fiber.MethodPost, "/players/me", strings.NewReader(`{"table_theme":"not-a-felt"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an unknown felt id", resp.StatusCode)
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

func TestAchievementsSummaryReturnsFullState(t *testing.T) {
	h := &playerHandlers{
		players:  player.NewService(&fakePlayerStore{}),
		sessions: &mockHistoryReader{},
		achievements: mockAchievementReader{all: []achievements.PlayerAchievementProgress{
			{Key: achievements.KeyWins, Count: 15},
			{Key: achievements.KeyFirstHandAllInWin, Count: 1},        // secret, past first tier -> revealed
			{Key: achievements.KeyLostStraightFlushToRoyal, Count: 1}, // secret, past first tier -> revealed
			{Key: achievements.KeySamePocketPairStreak, Count: 1},     // secret, below first tier -> hidden
		}},
	}
	app := fiber.New()
	app.Get("/players/me/achievements/summary", func(c fiber.Ctx) error {
		c.Locals(localsUserID, "user-123")
		return h.achievementsSummary(c)
	})

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/players/me/achievements/summary", nil))
	if err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, err = %v", resp.StatusCode, err)
	}
	var body achievements.Summary
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}

	states := make(map[string]achievements.AchievementState, len(body.Achievements))
	for _, s := range body.Achievements {
		states[s.Key] = s
	}
	if len(body.Achievements) != len(achievements.Catalog)-1 {
		t.Fatalf("revealed %d achievements, want %d (whole catalog minus the still-locked secret)",
			len(body.Achievements), len(achievements.Catalog)-1)
	}
	if _, hidden := states[achievements.KeySamePocketPairStreak]; hidden {
		t.Fatal("still-locked secret achievement leaked into the summary")
	}

	wins := states[achievements.KeyWins]
	if wins.Stars != 2 || !wins.Unlocked || wins.Completed {
		t.Fatalf("wins state = %+v, want stars 2, unlocked, not completed", wins)
	}
	if wins.NextTarget == nil || *wins.NextTarget != 100 {
		t.Fatalf("wins next target = %v, want 100", wins.NextTarget)
	}

	secret := states[achievements.KeyFirstHandAllInWin]
	if !secret.Secret || !secret.Unlocked || !secret.Completed || secret.NextTarget != nil {
		t.Fatalf("revealed secret state = %+v, want secret unlocked & completed", secret)
	}

	// A never-touched achievement is still present, just at zero.
	if hu, ok := states[achievements.KeyWonHeadsUp]; !ok || hu.Unlocked || hu.Progress != 0 {
		t.Fatalf("untouched achievement state = %+v (present %v)", hu, ok)
	}
	if body.Totals.Stars < 3 || body.Totals.Unlocked < 2 || body.Totals.MaxStars == 0 {
		t.Fatalf("totals = %+v, want stars>=3, unlocked>=2, max stars set", body.Totals)
	}
}

func TestShowcasePlaystyleRequiresOptInAndPublicSample(t *testing.T) {
	tests := []struct {
		name      string
		optedIn   bool
		hands     int64
		wantBadge bool
	}{
		{name: "opted out", optedIn: false, hands: 5000},
		{name: "below floor", optedIn: true, hands: pokerstats.MinHandsPublic - 1},
		{name: "at floor", optedIn: true, hands: pokerstats.MinHandsPublic, wantBadge: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakePlayerStore{profile: player.PlayerProfile{
				ShowcasePublic: true, PlaystylePublic: tt.optedIn,
			}}
			h := &playerHandlers{
				players: player.NewService(store), sessions: &mockHistoryReader{},
				achievements: mockAchievementReader{},
				stats:        mockPokerStatsReader{stats: pokerstats.Stats{Hands: tt.hands, VPIPRate: .2}},
			}
			app := fiber.New()
			app.Get("/players/:playerId/showcase", h.showcase)
			resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/players/u1/showcase", nil))
			if err != nil || resp.StatusCode != fiber.StatusOK {
				t.Fatalf("status = %d, err = %v", resp.StatusCode, err)
			}
			var body map[string]json.RawMessage
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			_, hasBadge := body["playstyle"]
			if hasBadge != tt.wantBadge {
				t.Fatalf("playstyle present = %v, want %v; body = %v", hasBadge, tt.wantBadge, body)
			}
		})
	}
}
