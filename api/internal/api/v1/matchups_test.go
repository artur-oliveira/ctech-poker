package v1

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/matchup"
)

type fakeMatchupStore struct {
	stats matchup.PairStats
	err   error
}

func (f *fakeMatchupStore) Get(_ context.Context, _, _, _ string) (matchup.PairStats, error) {
	return f.stats, f.err
}

func newMatchupsTestApp(store *fakeMatchupStore) *fiber.App {
	app := fiber.New()
	auth := func(c fiber.Ctx) error { c.Locals(localsUserID, "player-1"); return c.Next() }
	RegisterMatchups(app.Group("/v1.0"), auth, store)
	return app
}

func TestMatchups_ZeroedStatsWhenPairHasNoHistory(t *testing.T) {
	app := newMatchupsTestApp(&fakeMatchupStore{stats: matchup.PairStats{IDLow: "player-1", IDHigh: "player-2"}})
	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/v1.0/players/me/matchups/player-2", nil))
	if err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200 for a pair with no shared history, got %d err %v", resp.StatusCode, err)
	}
}

func TestMatchups_RemapsStatsRelativeToViewer(t *testing.T) {
	// viewer is "player-1", which sorts as IDHigh against "player-0" —
	// WinsHigh/NetChangeHigh must surface as the viewer's own numbers.
	app := newMatchupsTestApp(&fakeMatchupStore{stats: matchup.PairStats{
		IDLow: "player-0", IDHigh: "player-1",
		Stats: matchup.Stats{
			HandsTogether: 5, WinsLow: 1, WinsHigh: 4, Ties: 0,
			HeadsUpHandsTogether: 3, NetChangeLow: -300, NetChangeHigh: 250,
		},
	}})
	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/v1.0/players/me/matchups/player-0", nil))
	if err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d err %v", resp.StatusCode, err)
	}
	var body struct {
		ViewerWins      int64 `json:"viewer_wins"`
		OpponentWins    int64 `json:"opponent_wins"`
		NetChangeViewer int64 `json:"net_change_viewer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.ViewerWins != 4 || body.OpponentWins != 1 || body.NetChangeViewer != 250 {
		t.Fatalf("remap = %+v", body)
	}
}

func TestMatchups_RejectsSelfAsOpponent(t *testing.T) {
	app := newMatchupsTestApp(&fakeMatchupStore{})
	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/v1.0/players/me/matchups/player-1", nil))
	if err != nil || resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400 for opponent==viewer, got %d err %v", resp.StatusCode, err)
	}
}
