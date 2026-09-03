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
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/pokerstats"
	"gopkg.aoctech.app/poker/api/internal/reports"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
)

type mockHistoryReader struct{}

// fixtureEndedAtMs is an epoch-milliseconds timestamp (13 digits, well above
// 1e12) used to assert every hand endpoint passes sessionlog.HandItem's
// EndedAt straight through, in the same unit, with no per-endpoint
// conversion (#74).
const fixtureEndedAtMs int64 = 1_700_000_000_000

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
	return []sessionlog.HandItem{{PK: playerID, HandID: "h-1", NetChange: 50, EndedAt: fixtureEndedAtMs}}, nil, nil
}
func (m *mockHistoryReader) ListHandsByTable(_ context.Context, playerID, mode, tableID string, _ int, _ map[string]types.AttributeValue) ([]sessionlog.HandItem, map[string]types.AttributeValue, error) {
	return []sessionlog.HandItem{{PK: playerID, HandID: "h-1", NetChange: 50, TableID: tableID, EndedAt: fixtureEndedAtMs}}, nil, nil
}
func (m *mockHistoryReader) GetHand(_ context.Context, playerID, mode, handID string) (*sessionlog.HandItem, error) {
	return &sessionlog.HandItem{PK: playerID, HandID: handID, NetChange: 50, EndedAt: fixtureEndedAtMs}, nil
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

// fakeMultiPlayerStore backs distinct profiles per player id (unlike
// fakePlayerStore's single shared profile) so opponent-avatar-resolution
// tests can exercise several opponents in one hand, some cleared/missing and
// some not, the way real ClearAvatar callers see it. Embedding
// fakePlayerStore supplies the write-path methods the profileStore interface
// still requires but these tests don't exercise.
type fakeMultiPlayerStore struct {
	fakePlayerStore
	profiles map[string]player.PlayerProfile
}

func (s *fakeMultiPlayerStore) Get(_ context.Context, id string) (*player.PlayerProfile, error) {
	if profile, ok := s.profiles[id]; ok {
		profile.UserID = id
		return &profile, nil
	}
	return nil, nil
}

func (s *fakeMultiPlayerStore) GetMany(_ context.Context, ids []string) (map[string]player.PlayerProfile, error) {
	out := make(map[string]player.PlayerProfile, len(ids))
	for _, id := range ids {
		if profile, ok := s.profiles[id]; ok {
			profile.UserID = id
			out[id] = profile
		}
	}
	return out, nil
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

// TestHandEndedAtIsEpochMillisecondsEverywhere locks down the unit fix for
// #74: every hand-bearing endpoint — the hand list, hand-by-id, and the
// public showcase's best_hand — must emit sessionlog.HandItem.EndedAt
// unchanged, in epoch milliseconds. A regression that divides/multiplies by
// 1000 on only one of these endpoints (the historical bug) fails this test.
func TestHandEndedAtIsEpochMillisecondsEverywhere(t *testing.T) {
	app := fiber.New()
	auth := func(c fiber.Ctx) error { c.Locals(localsUserID, "user-123"); return c.Next() }
	store := &fakePlayerStore{profile: player.PlayerProfile{ShowcasePublic: true}}
	RegisterPlayers(app.Group("/v1.0"), auth, player.NewService(store), &mockHistoryReader{},
		mockAchievementReader{}, nil, nil, nil, nil)

	decodeEndedAt := func(t *testing.T, path string, dig func(map[string]json.RawMessage) json.RawMessage) int64 {
		t.Helper()
		resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, path, nil))
		if err != nil || resp.StatusCode != fiber.StatusOK {
			t.Fatalf("GET %s: status = %d, err = %v", path, resp.StatusCode, err)
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("GET %s: decode: %v", path, err)
		}
		raw := dig(body)
		var endedAt int64
		if err := json.Unmarshal(raw, &endedAt); err != nil {
			t.Fatalf("GET %s: ended_at not a bare int64 (%s): %v", path, raw, err)
		}
		return endedAt
	}

	t.Run("GET /players/me/hand/:id", func(t *testing.T) {
		got := decodeEndedAt(t, "/v1.0/players/me/hand/h-1", func(b map[string]json.RawMessage) json.RawMessage {
			return b["ended_at"]
		})
		if got != fixtureEndedAtMs {
			t.Fatalf("ended_at = %d, want %d (ms)", got, fixtureEndedAtMs)
		}
	})

	t.Run("GET /players/:playerId/showcase best_hand", func(t *testing.T) {
		resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/v1.0/players/user-123/showcase", nil))
		if err != nil || resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, err = %v", resp.StatusCode, err)
		}
		var body struct {
			BestHand struct {
				EndedAt int64 `json:"ended_at"`
			} `json:"best_hand"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.BestHand.EndedAt != fixtureEndedAtMs {
			t.Fatalf("best_hand.ended_at = %d, want %d (ms)", body.BestHand.EndedAt, fixtureEndedAtMs)
		}
	})
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

// opponentHandReader is a sessionLogReader stub whose ListHands/GetHand
// return a fixed hand carrying the opponents fixture below, so
// TestHandHistoryDropsStaleOpponentAvatar and TestHandByIDDropsStaleOpponentAvatar
// can drive the real handHistory/handByID handlers end to end.
type opponentHandReader struct{ mockHistoryReader }

func opponentFixtureHand(playerID string) sessionlog.HandItem {
	return sessionlog.HandItem{
		PK: playerID, HandID: "h-1", NetChange: 50,
		Opponents: []sessionlog.OpponentSummary{
			{PlayerID: "still-has-avatar", Name: "Ainda Tem", AvatarURL: "https://cdn.example.com/av/still-has-avatar/1.jpg"},
			{PlayerID: "cleared-avatar", Name: "Limpou o Avatar", AvatarURL: "https://cdn.example.com/av/cleared-avatar/1.jpg"},
			{PlayerID: "deleted-player", Name: "Sumiu", AvatarURL: "https://cdn.example.com/av/deleted-player/1.jpg"},
		},
	}
}

func (m *opponentHandReader) ListHands(_ context.Context, playerID, _ string, _ int, _ map[string]types.AttributeValue) ([]sessionlog.HandItem, map[string]types.AttributeValue, error) {
	return []sessionlog.HandItem{opponentFixtureHand(playerID)}, nil, nil
}

func (m *opponentHandReader) GetHand(_ context.Context, playerID, _, handID string) (*sessionlog.HandItem, error) {
	hand := opponentFixtureHand(playerID)
	hand.HandID = handID
	return &hand, nil
}

// newOpponentAvatarPlayers seeds a live profile for "still-has-avatar" and
// leaves "cleared-avatar" present but avatar-less (the post-ClearAvatar
// shape: no avatar_key) — "deleted-player" is absent from the store
// entirely, covering a profile that no longer resolves at all.
func newOpponentAvatarPlayers() *player.Service {
	return player.NewService(&fakeMultiPlayerStore{profiles: map[string]player.PlayerProfile{
		// Renamed since the hand was recorded ("Ainda Tem" in the fixture) —
		// covers issue #64's stale OpponentSummary.Name alongside the avatar.
		"still-has-avatar": {Name: "Nome Novo", AvatarKey: "av/still-has-avatar/2.jpg", AvatarVersion: 2},
		"cleared-avatar":   {AvatarVersion: 1},
	}})
}

// TestHandHistoryResolvesRenamedOpponentName is issue #64's read-time
// resolution for hand history: a rename after the hand was recorded shows up
// on the next read without any backfill, while an opponent whose profile no
// longer resolves keeps the name the hand stored rather than going blank.
func TestHandHistoryResolvesRenamedOpponentName(t *testing.T) {
	h := &playerHandlers{
		players:  newOpponentAvatarPlayers(),
		sessions: &opponentHandReader{},
		cfg:      &config.Config{AvatarBaseURL: "https://cdn.example.com"},
	}
	app := fiber.New()
	auth := func(c fiber.Ctx) error { c.Locals(localsUserID, "viewer"); return c.Next() }
	app.Get("/players/me/hands", auth, h.handHistory)
	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/players/me/hands", nil))
	if err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, err = %v", resp.StatusCode, err)
	}
	var page struct {
		Data []sessionlog.HandItem `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode page: %v", err)
	}
	byID := map[string]sessionlog.OpponentSummary{}
	for _, opp := range page.Data[0].Opponents {
		byID[opp.PlayerID] = opp
	}
	if got := byID["still-has-avatar"].Name; got != "Nome Novo" {
		t.Fatalf("renamed opponent name = %q, want the live profile name", got)
	}
	if got := byID["deleted-player"].Name; got != "Sumiu" {
		t.Fatalf("unresolvable opponent name = %q, want the stored fallback", got)
	}
}

func assertNoStaleAvatars(t *testing.T, hand sessionlog.HandItem) {
	t.Helper()
	if len(hand.Opponents) != 3 {
		t.Fatalf("expected 3 opponents, got %d", len(hand.Opponents))
	}
	byID := make(map[string]sessionlog.OpponentSummary, len(hand.Opponents))
	for _, opp := range hand.Opponents {
		byID[opp.PlayerID] = opp
	}
	if got := byID["still-has-avatar"].AvatarURL; got != "https://cdn.example.com/still-has-avatar/2.jpg" {
		t.Fatalf("still-has-avatar: got avatar_url %q, want the live resolved URL", got)
	}
	if got := byID["cleared-avatar"].AvatarURL; got != "" {
		t.Fatalf("cleared-avatar: got stale avatar_url %q, want empty (404 risk after ClearAvatar)", got)
	}
	if got := byID["deleted-player"].AvatarURL; got != "" {
		t.Fatalf("deleted-player: got stale avatar_url %q, want empty (profile no longer exists)", got)
	}
}

func TestHandHistoryDropsStaleOpponentAvatar(t *testing.T) {
	h := &playerHandlers{
		players:  newOpponentAvatarPlayers(),
		sessions: &opponentHandReader{},
		cfg:      &config.Config{AvatarBaseURL: "https://cdn.example.com"},
	}
	app := fiber.New()
	auth := func(c fiber.Ctx) error { c.Locals(localsUserID, "viewer"); return c.Next() }
	app.Get("/players/me/hands", auth, h.handHistory)
	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/players/me/hands", nil))
	if err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, err = %v", resp.StatusCode, err)
	}
	var page struct {
		Data []sessionlog.HandItem `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode page: %v", err)
	}
	if len(page.Data) != 1 {
		t.Fatalf("expected 1 hand, got %d", len(page.Data))
	}
	assertNoStaleAvatars(t, page.Data[0])
}

func TestHandByIDDropsStaleOpponentAvatar(t *testing.T) {
	h := &playerHandlers{
		players:  newOpponentAvatarPlayers(),
		sessions: &opponentHandReader{},
		cfg:      &config.Config{AvatarBaseURL: "https://cdn.example.com"},
	}
	app := fiber.New()
	auth := func(c fiber.Ctx) error { c.Locals(localsUserID, "viewer"); return c.Next() }
	app.Get("/players/me/hands/:id", auth, h.handByID)
	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/players/me/hands/h-1", nil))
	if err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, err = %v", resp.StatusCode, err)
	}
	var hand sessionlog.HandItem
	if err := json.NewDecoder(resp.Body).Decode(&hand); err != nil {
		t.Fatalf("decode hand: %v", err)
	}
	assertNoStaleAvatars(t, hand)
}
