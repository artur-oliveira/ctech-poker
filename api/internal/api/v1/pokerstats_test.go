package v1

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/pokerstats"
)

type fakeStatsStore struct{ stats pokerstats.Stats }

func (f fakeStatsStore) Get(context.Context, string, string) (pokerstats.Stats, error) {
	return f.stats, nil
}

func newPokerStatsTestApp(t *testing.T, stats pokerstats.Stats) (*fiber.App, *fakePlayerStore) {
	t.Helper()
	store := &fakePlayerStore{}
	players := player.NewService(store)
	app := fiber.New()
	auth := func(c fiber.Ctx) error { c.Locals(localsUserID, "u1"); return c.Next() }
	RegisterPokerStats(app, auth, fakeStatsStore{stats: stats}, players)
	return app, store
}

// TestPlayerStatsReportsLeaksAndGoalProgress pins #331's GET /players/me/stats
// acceptance criteria: badges + leak tips at MinHandsSelf, with no
// PlaystylePublic requirement, plus goal_progress for any configured goal.
func TestPlayerStatsReportsLeaksAndGoalProgress(t *testing.T) {
	stats := pokerstats.Stats{Hands: 100, VPIPHands: 45, PFRHands: 40, VPIPRate: 0.45, PFRRate: 0.40}
	app, store := newPokerStatsTestApp(t, stats)
	store.profile.StatsGoals = map[string]float64{"vpip_rate": 0.25}

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/players/me/stats", nil))
	if err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status=%d err=%v", resp.StatusCode, err)
	}
	var body struct {
		Leaks        []pokerstats.Leak `json:"leaks"`
		GoalProgress map[string]struct {
			Target  float64 `json:"target"`
			Current float64 `json:"current"`
		} `json:"goal_progress"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Leaks) == 0 {
		t.Fatalf("expected at least one leak tip for a loose VPIP, got none")
	}
	progress, ok := body.GoalProgress["vpip_rate"]
	if !ok || progress.Target != 0.25 || progress.Current == 0 {
		t.Fatalf("unexpected goal_progress: %+v", body.GoalProgress)
	}
}

func TestPlayerStatsGoalsRejectsUnknownKey(t *testing.T) {
	app, _ := newPokerStatsTestApp(t, pokerstats.Stats{})
	body := strings.NewReader(`{"goals":{"not_a_metric":0.5}}`)
	req := httptest.NewRequest(fiber.MethodPost, "/players/me/stats/goals", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status=%d, want 400 for an unknown goal key", resp.StatusCode)
	}
}

func TestPlayerStatsGoalsAcceptsKnownKeys(t *testing.T) {
	app, store := newPokerStatsTestApp(t, pokerstats.Stats{})
	body := strings.NewReader(`{"goals":{"vpip_rate":0.22,"pfr_rate":0.18}}`)
	req := httptest.NewRequest(fiber.MethodPost, "/players/me/stats/goals", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status=%d err=%v", resp.StatusCode, err)
	}
	if len(store.profile.StatsGoals) != 2 {
		t.Fatalf("expected 2 goals persisted, got %+v", store.profile.StatsGoals)
	}
}
