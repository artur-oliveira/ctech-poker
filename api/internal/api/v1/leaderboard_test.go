package v1

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/leaderboard"
	"gopkg.aoctech.app/poker/api/internal/player"
)

// fakeLeaderboardStore is a minimal in-memory stand-in for leaderboard's
// unexported statsStore interface — mirrors leaderboard/service_test.go's
// memStats fake, kept local since that type isn't exported across packages.
type fakeLeaderboardStore struct {
	entries map[string]*leaderboard.Entry // key: mode + "#" + playerID
}

func (f *fakeLeaderboardStore) IncrementStats(_ context.Context, id, _, mode string, played, won int) error {
	key := mode + "#" + id
	e := f.entries[key]
	if e == nil {
		e = &leaderboard.Entry{PlayerID: id}
		f.entries[key] = e
	}
	e.HandsPlayed += played
	e.HandsWon += won
	return nil
}

func (f *fakeLeaderboardStore) IncrementAchievementPoints(_ context.Context, id, mode string, points int) error {
	key := mode + "#" + id
	e := f.entries[key]
	if e == nil {
		e = &leaderboard.Entry{PlayerID: id}
		f.entries[key] = e
	}
	e.AchievementPoints += points
	return nil
}

func (f *fakeLeaderboardStore) Top(_ context.Context, mode, _ string, _ int, _ map[string]types.AttributeValue) ([]leaderboard.Entry, map[string]types.AttributeValue, error) {
	out := []leaderboard.Entry{}
	for key, e := range f.entries {
		if len(key) > len(mode) && key[:len(mode)+1] == mode+"#" {
			out = append(out, *e)
		}
	}
	return out, nil, nil
}

func (f *fakeLeaderboardStore) PlayerEntry(_ context.Context, id, mode string) (*leaderboard.Entry, error) {
	e, ok := f.entries[mode+"#"+id]
	if !ok {
		return nil, nil
	}
	return e, nil
}

func (f *fakeLeaderboardStore) RankOf(_ context.Context, mode, metric string, entry leaderboard.Entry) (int64, int64, error) {
	score := func(e *leaderboard.Entry) float64 {
		switch metric {
		case "hands_played":
			return float64(e.HandsPlayed)
		case "win_rate":
			return e.WinRate
		default:
			return float64(e.HandsWon)
		}
	}
	mine := score(&entry)
	var better, tied, total int64
	for key, e := range f.entries {
		if len(key) <= len(mode) || key[:len(mode)+1] != mode+"#" {
			continue
		}
		total++
		s := score(e)
		if s > mine {
			better++
		} else if s == mine && e.PlayerID < entry.PlayerID {
			tied++
		}
	}
	return better + tied + 1, total, nil
}

func withUser(id string) fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Locals(localsUserID, id)
		return c.Next()
	}
}

func TestLeaderboardMeUnranked(t *testing.T) {
	app := fiber.New()
	store := &fakeLeaderboardStore{entries: map[string]*leaderboard.Entry{}}
	svc := leaderboard.NewServiceWithStore(store)
	RegisterLeaderboard(app.Group("/v1.0"), withUser("nobody"), svc, nil)

	req := httptest.NewRequest(fiber.MethodGet, "/v1.0/leaderboard/me?mode=sandbox&metric=hands_won", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body meResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Ranked {
		t.Fatalf("expected unranked for a player with no stats row, got %+v", body)
	}
}

func TestLeaderboardMeRanked(t *testing.T) {
	app := fiber.New()
	store := &fakeLeaderboardStore{entries: map[string]*leaderboard.Entry{
		"sandbox#p1": {PlayerID: "p1", HandsPlayed: 5, HandsWon: 3},
		"sandbox#p2": {PlayerID: "p2", HandsPlayed: 5, HandsWon: 1},
	}}
	svc := leaderboard.NewServiceWithStore(store)
	RegisterLeaderboard(app.Group("/v1.0"), withUser("p2"), svc, nil)

	req := httptest.NewRequest(fiber.MethodGet, "/v1.0/leaderboard/me?mode=sandbox&metric=hands_won", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body meResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.Ranked || body.Rank == nil || *body.Rank != 2 || body.Total == nil || *body.Total != 2 {
		t.Fatalf("expected p2 rank 2 of 2, got %+v", body)
	}
	if body.Entry == nil || body.Entry.PlayerID != "p2" {
		t.Fatalf("expected entry for p2, got %+v", body.Entry)
	}
}

func TestLeaderboardMeRequiresAuth(t *testing.T) {
	app := fiber.New()
	deny := func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusUnauthorized) }
	store := &fakeLeaderboardStore{entries: map[string]*leaderboard.Entry{}}
	svc := leaderboard.NewServiceWithStore(store)
	RegisterLeaderboard(app.Group("/v1.0"), deny, svc, nil)

	req := httptest.NewRequest(fiber.MethodGet, "/v1.0/leaderboard/me", nil)
	resp, err := app.Test(req)
	if err != nil || resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("expected 401 from auth middleware, got %d, err %v", resp.StatusCode, err)
	}
}

// TestLeaderboardResolvesRenamedPlayerName is issue #64 on the leaderboard: a
// row's denormalized player_name is only refreshed by the write a completed
// hand makes, so a player who renames and stops playing keeps the old name on
// the board forever. Both /leaderboard and /leaderboard/me now resolve names
// from the canonical profile at read time, and a row whose profile does not
// resolve keeps whatever it stored rather than going blank.
func TestLeaderboardResolvesRenamedPlayerName(t *testing.T) {
	store := &fakeLeaderboardStore{entries: map[string]*leaderboard.Entry{
		"sandbox#p1": {PlayerID: "p1", PlayerName: "Nome Antigo", HandsPlayed: 5, HandsWon: 3},
		"sandbox#p2": {PlayerID: "p2", PlayerName: "Sem Perfil", HandsPlayed: 5, HandsWon: 1},
	}}
	players := player.NewService(&fakeMultiPlayerStore{profiles: map[string]player.PlayerProfile{
		"p1": {Name: "Nome Novo"},
	}})
	svc := leaderboard.NewServiceWithStore(store)
	app := fiber.New()
	RegisterLeaderboard(app.Group("/v1.0"), withUser("p1"), svc, players)

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/v1.0/leaderboard?mode=sandbox&metric=hands_won", nil))
	if err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, err = %v", resp.StatusCode, err)
	}
	var page struct {
		Data []leaderboard.Entry `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	byID := map[string]leaderboard.Entry{}
	for _, entry := range page.Data {
		byID[entry.PlayerID] = entry
	}
	if got := byID["p1"].PlayerName; got != "Nome Novo" {
		t.Fatalf("p1 player_name = %q, want the live profile name", got)
	}
	if got := byID["p2"].PlayerName; got != "Sem Perfil" {
		t.Fatalf("p2 player_name = %q, want the stored fallback when no profile resolves", got)
	}

	resp, err = app.Test(httptest.NewRequest(fiber.MethodGet, "/v1.0/leaderboard/me?mode=sandbox&metric=hands_won", nil))
	if err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("me status = %d, err = %v", resp.StatusCode, err)
	}
	var me meResponse
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil {
		t.Fatal(err)
	}
	if me.Entry == nil || me.Entry.PlayerName != "Nome Novo" {
		t.Fatalf("/leaderboard/me served a stale name: %+v", me.Entry)
	}
}
