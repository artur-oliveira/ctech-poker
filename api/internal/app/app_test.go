package app

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/jwtverify"
	"gopkg.aoctech.app/api-commons/ws"
	"gopkg.aoctech.app/poker/api/internal/buyin"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/dailyreward"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/leaderboard"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
)

// TestHandItemForMarksWinnerAmongMultipleOpponents pins down that a 3+-way
// hand's history reports each opponent's own Won flag explicitly — before
// OpponentSummary.Won existed, a player's match history only let a client
// infer who won by elimination when there was exactly one opponent
// (heads-up); with 2+ opponents there was no way to tell which one(s) won.
func TestHandItemForMarksWinnerAmongMultipleOpponents(t *testing.T) {
	outcome := hand.HandOutcome{
		Winners:      []string{"p2"},
		Participants: []string{"p1", "p2", "p3"},
		Payouts:      map[string]int64{"p1": 0, "p2": 300, "p3": 0},
		Contributions: map[string]int64{
			"p1": 100, "p2": 100, "p3": 100,
		},
		PlayerHands: map[string]hand.PlayerHandInfo{
			"p1": {HoleCards: [2]string{"Ah", "Kh"}, Revealed: true},
			"p2": {HoleCards: [2]string{"2c", "2d"}, Revealed: true},
			"p3": {HoleCards: [2]string{"Qs", "Qc"}, Revealed: true},
		},
	}

	item := handItemFor(outcome, "p1", nil)
	if item.Outcome != "lost" {
		t.Fatalf("expected p1 outcome lost, got %q", item.Outcome)
	}
	if len(item.Opponents) != 2 {
		t.Fatalf("expected 2 opponents, got %d", len(item.Opponents))
	}
	var wonCount int
	for _, opp := range item.Opponents {
		if opp.Won {
			wonCount++
			if opp.PlayerID != "p2" {
				t.Fatalf("expected p2 marked as winner, got %q", opp.PlayerID)
			}
		}
	}
	if wonCount != 1 {
		t.Fatalf("expected exactly one opponent marked Won, got %d", wonCount)
	}
}

func TestHandItemForDoesNotCallDistinctSidePotWinnersTied(t *testing.T) {
	outcome := hand.HandOutcome{
		Winners:      []string{"main-winner", "side-winner"},
		Participants: []string{"main-winner", "side-winner", "loser"},
		ShowdownResults: map[string]hand.ShowdownResult{
			"main-winner": {Won: true, Category: "full_house"},
			"side-winner": {Won: true, Category: "three_of_a_kind"},
			"loser":       {Category: "pair"},
		},
	}
	for _, id := range []string{"main-winner", "side-winner"} {
		if got := handItemFor(outcome, id, nil).Outcome; got != "won" {
			t.Fatalf("%s won a distinct pot outright, got history outcome %q", id, got)
		}
	}
}

func TestHandItemForPreservesPartialOpponentReveal(t *testing.T) {
	outcome := hand.HandOutcome{
		Participants: []string{"viewer", "folder"},
		PlayerHands: map[string]hand.PlayerHandInfo{
			"viewer": {HoleCards: [2]string{"Ah", "Kh"}},
			"folder": {
				HoleCards:     [2]string{"Qs", "Qc"},
				RevealedCards: [2]bool{true, false},
			},
		},
	}
	item := handItemFor(outcome, "viewer", nil)
	if len(item.Opponents) != 1 ||
		len(item.Opponents[0].HoleCards) != 2 ||
		item.Opponents[0].HoleCards[0] != "Qs" ||
		item.Opponents[0].HoleCards[1] != "back" {
		t.Fatalf("partial reveal was not preserved in history: %+v", item.Opponents)
	}
}

func TestHandItemForDoesNotPersistSeedWithHiddenCards(t *testing.T) {
	outcome := hand.HandOutcome{
		ServerSeed:   "secret-seed",
		Participants: []string{"viewer", "folder"},
		PlayerHands: map[string]hand.PlayerHandInfo{
			"viewer": {HoleCards: [2]string{"Ah", "Kh"}, Revealed: true},
			"folder": {HoleCards: [2]string{"Qs", "Qc"}, Revealed: false},
		},
	}
	if seed := handItemFor(outcome, "viewer", nil).ServerSeed; seed != "" {
		t.Fatalf("hidden opponent cards must suppress persisted server seed, got %q", seed)
	}
	outcome.PlayerHands["folder"] = hand.PlayerHandInfo{
		HoleCards: [2]string{"Qs", "Qc"}, Revealed: true,
	}
	if seed := handItemFor(outcome, "viewer", nil).ServerSeed; seed != "secret-seed" {
		t.Fatalf("fully revealed showdown should retain server seed, got %q", seed)
	}
}

// A seed-less hand is still auditable: the per-position proof must survive into
// the player's history item, otherwise "verify your deck" is dead on any hand
// that ended without a showdown.
func TestHandItemForPersistsFairnessProofWithoutSeed(t *testing.T) {
	outcome := hand.HandOutcome{
		Participants: []string{"viewer", "folder"},
		PlayerHands: map[string]hand.PlayerHandInfo{
			"viewer": {HoleCards: [2]string{"Ah", "Kh"}},
			"folder": {HoleCards: [2]string{"Qs", "Qc"}},
		},
		FairnessProofs: map[string]hand.FairnessProof{
			"viewer": {
				RevealedCardSalts:    map[int]hand.RevealedSaltView{0: {Card: "Ah", SaltHex: "aa"}},
				UnrevealedCardHashes: map[int]string{1: "bb"},
			},
		},
	}
	item := handItemFor(outcome, "viewer", nil)
	if item.ServerSeed != "" {
		t.Fatalf("no-showdown hand must not publish the seed, got %q", item.ServerSeed)
	}
	if item.RevealedCardSalts["0"].Card != "Ah" || item.RevealedCardSalts["0"].SaltHex != "aa" {
		t.Fatalf("revealed salt not persisted: %+v", item.RevealedCardSalts)
	}
	if item.UnrevealedCardHashes["1"] != "bb" {
		t.Fatalf("unrevealed hash not persisted: %+v", item.UnrevealedCardHashes)
	}
	if got := handItemFor(outcome, "folder", nil); got.RevealedCardSalts != nil || got.UnrevealedCardHashes != nil {
		t.Fatalf("a player with no proof must not inherit another's: %+v", got)
	}
}

func testRoutes(app *fiber.App, cfg *config.Config) {
	verifier := jwtverify.NewVerifier("", "", "", cache.NewMemoryBackend(1))
	manager := tablemanager.NewManager(nil, nil, nil, nil)
	registerRoutes(app, cfg, nil, verifier, manager, ws.NewMemoryRegistry(), nil, nil, (*buyin.Service)(nil), (*player.Service)(nil), (*leaderboard.Service)(nil), (*dailyreward.Service)(nil), nil, nil, nil, nil, nil, nil)
}

func TestLivenessEndpointReturnsOK(t *testing.T) {
	app := fiber.New()
	testRoutes(app, &config.Config{AppVersion: "1.2.3"})

	req, _ := http.NewRequest(http.MethodGet, "/v1.0/health", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Status    string `json:"status"`
		ReleaseID string `json:"releaseId"`
		ServiceID string `json:"serviceId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "pass" {
		t.Fatalf("expected status pass, got %q", body.Status)
	}
	if body.ReleaseID != "1.2.3" {
		t.Fatalf("expected releaseId 1.2.3, got %q", body.ReleaseID)
	}
	if body.ServiceID == "" {
		t.Fatal("expected non-empty serviceId")
	}
}

func TestHealthCheckEndpointFailsWhenDynamoDBIsUnavailable(t *testing.T) {
	app := fiber.New()
	testRoutes(app, &config.Config{AppVersion: "1.2.3"})

	req, _ := http.NewRequest(http.MethodGet, "/v1.0/health-check", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 without DynamoDB, got %d", resp.StatusCode)
	}

	var body struct {
		Status      string                 `json:"status"`
		Version     string                 `json:"version"`
		ReleaseID   string                 `json:"releaseId"`
		ServiceID   string                 `json:"serviceId"`
		Description string                 `json:"description"`
		Checks      map[string]interface{} `json:"checks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Version != "/v1.0" {
		t.Fatalf("expected version /v1.0, got %q", body.Version)
	}
	if body.ReleaseID != "1.2.3" {
		t.Fatalf("expected releaseId 1.2.3, got %q", body.ReleaseID)
	}
	if body.Status != "fail" {
		t.Fatalf("expected fail without DynamoDB, got %q", body.Status)
	}
	for _, key := range []string{"uptime", "cpu", "memory", "dynamodb"} {
		if _, ok := body.Checks[key]; !ok {
			t.Fatalf("expected checks to contain %q, got %+v", key, body.Checks)
		}
	}
}
