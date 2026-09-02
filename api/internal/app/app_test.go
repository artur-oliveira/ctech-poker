package app

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/jwtverify"
	"gopkg.aoctech.app/api-commons/ws"
	"gopkg.aoctech.app/poker/api/internal/buyin"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/cosmeticpurchase"
	"gopkg.aoctech.app/poker/api/internal/dailyreward"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/leaderboard"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/reactionpurchase"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/sandboxpurchase"
	"gopkg.aoctech.app/poker/api/internal/social"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
)

type fakeRoomModeReader struct{ room *roomstore.Room }

func (f fakeRoomModeReader) Get(context.Context, string) (*roomstore.Room, error) { return f.room, nil }

func TestTableCurrencyModeUsesRoomMode(t *testing.T) {
	mode, err := tableCurrencyMode(context.Background(), fakeRoomModeReader{
		room: &roomstore.Room{CurrencyMode: roomstore.CurrencyModeReal},
	}, "table-real")
	if err != nil || mode != roomstore.CurrencyModeReal {
		t.Fatalf("mode=%q err=%v", mode, err)
	}
}

type autoRebuyBuyInCall struct {
	roomID, playerID string
	amount           int64
	nonce            string
}

type fakeAutoRebuyBuyin struct {
	seats    map[string]buyin.SeatSummary
	balances map[string]int64
	buyIns   []autoRebuyBuyInCall
}

func (f *fakeAutoRebuyBuyin) SeatedSummary(_ context.Context, _, playerID string) (buyin.SeatSummary, error) {
	return f.seats[playerID], nil
}
func (f *fakeAutoRebuyBuyin) SandboxBalance(_ context.Context, playerID string) (int64, error) {
	return f.balances[playerID], nil
}
func (f *fakeAutoRebuyBuyin) BuyIn(_ context.Context, roomID, playerID string, amount int64, _ bool, nonce string) error {
	f.buyIns = append(f.buyIns, autoRebuyBuyInCall{roomID, playerID, amount, nonce})
	return nil
}

func TestAutoRebuySweepRebuysBustedAutoRebuySeatWithSufficientBalance(t *testing.T) {
	buyinSvc := &fakeAutoRebuyBuyin{
		seats:    map[string]buyin.SeatSummary{"player-1": {Seated: true, Stack: 0, AutoRebuy: true, BuyInAmount: 100}},
		balances: map[string]int64{"player-1": 500},
	}
	rooms := fakeRoomModeReader{room: &roomstore.Room{CurrencyMode: roomstore.CurrencyModeSandbox}}

	autoRebuySweep(context.Background(), buyinSvc, rooms, "table-1", "hand-1", hand.HandOutcome{Participants: []string{"player-1"}})

	if len(buyinSvc.buyIns) != 1 || buyinSvc.buyIns[0].amount != 100 {
		t.Fatalf("expected one 100-chip auto-rebuy, got %+v", buyinSvc.buyIns)
	}
}

// TestAutoRebuySweepNonceFitsWalletIdempotencyKeyLimit guards against a
// regression that shipped to production 2026-08-11: buyin.Service.buyIn
// builds the wallet idempotency key as "roomID#playerID#buyin#nonce". With
// realistic ULID table/hand IDs (26 chars) and a UUID player ID (36 chars),
// a nonce that repeats playerID pushed the compound key to 138 characters —
// over ctech-wallet's MovementOpRequest.IdempotencyKey max=128 — so every
// sandbox debit came back HTTP 422 and auto-rebuy silently never worked.
// Root-caused from a player.har capture plus prod wallet/poker logs; not
// caught by the other tests here because the fake BuyIn never enforces the
// real wallet's field-length validation.
func TestAutoRebuySweepNonceFitsWalletIdempotencyKeyLimit(t *testing.T) {
	const walletIdempotencyKeyMax = 128
	const roomID = "01KZRP3S675Y6RGT1W21M1K4MV"             // 26-char ULID
	const handID = "01KZRQ25994SJBRCKXEF6TGNVB"             // 26-char ULID
	const playerID = "78fd4d57-88a7-4eec-a997-7d7c09f58a1b" // 36-char UUID

	buyinSvc := &fakeAutoRebuyBuyin{
		seats:    map[string]buyin.SeatSummary{playerID: {Seated: true, Stack: 0, AutoRebuy: true, BuyInAmount: 100}},
		balances: map[string]int64{playerID: 500},
	}
	rooms := fakeRoomModeReader{room: &roomstore.Room{CurrencyMode: roomstore.CurrencyModeSandbox}}

	autoRebuySweep(context.Background(), buyinSvc, rooms, roomID, handID, hand.HandOutcome{Participants: []string{playerID}})

	if len(buyinSvc.buyIns) != 1 {
		t.Fatalf("expected one auto-rebuy call, got %+v", buyinSvc.buyIns)
	}
	nonce := buyinSvc.buyIns[0].nonce
	compoundKey := roomID + "#" + playerID + "#buyin#" + nonce
	if len(compoundKey) > walletIdempotencyKeyMax {
		t.Fatalf("compound idempotency key %q is %d chars, exceeds wallet's max=%d", compoundKey, len(compoundKey), walletIdempotencyKeyMax)
	}
}

func TestAutoRebuySweepSkipsInsufficientBalance(t *testing.T) {
	buyinSvc := &fakeAutoRebuyBuyin{
		seats:    map[string]buyin.SeatSummary{"player-1": {Seated: true, Stack: 0, AutoRebuy: true, BuyInAmount: 100}},
		balances: map[string]int64{"player-1": 50},
	}
	rooms := fakeRoomModeReader{room: &roomstore.Room{CurrencyMode: roomstore.CurrencyModeSandbox}}

	autoRebuySweep(context.Background(), buyinSvc, rooms, "table-1", "hand-1", hand.HandOutcome{Participants: []string{"player-1"}})

	if len(buyinSvc.buyIns) != 0 {
		t.Fatalf("expected no auto-rebuy for insufficient balance, got %+v", buyinSvc.buyIns)
	}
}

func TestAutoRebuySweepSkipsRealMoneyRooms(t *testing.T) {
	buyinSvc := &fakeAutoRebuyBuyin{
		seats:    map[string]buyin.SeatSummary{"player-1": {Seated: true, Stack: 0, AutoRebuy: true, BuyInAmount: 100}},
		balances: map[string]int64{"player-1": 500},
	}
	rooms := fakeRoomModeReader{room: &roomstore.Room{CurrencyMode: roomstore.CurrencyModeReal}}

	autoRebuySweep(context.Background(), buyinSvc, rooms, "table-1", "hand-1", hand.HandOutcome{Participants: []string{"player-1"}})

	if len(buyinSvc.buyIns) != 0 {
		t.Fatalf("expected no auto-rebuy sweep for real-money rooms, got %+v", buyinSvc.buyIns)
	}
}

func TestAutoRebuySweepSkipsSeatWithoutAutoRebuy(t *testing.T) {
	buyinSvc := &fakeAutoRebuyBuyin{
		seats:    map[string]buyin.SeatSummary{"player-1": {Seated: true, Stack: 0, AutoRebuy: false, BuyInAmount: 100}},
		balances: map[string]int64{"player-1": 500},
	}
	rooms := fakeRoomModeReader{room: &roomstore.Room{CurrencyMode: roomstore.CurrencyModeSandbox}}

	autoRebuySweep(context.Background(), buyinSvc, rooms, "table-1", "hand-1", hand.HandOutcome{Participants: []string{"player-1"}})

	if len(buyinSvc.buyIns) != 0 {
		t.Fatalf("expected no auto-rebuy when AutoRebuy is false, got %+v", buyinSvc.buyIns)
	}
}

func TestAutoRebuySweepSkipsSeatThatDidNotBust(t *testing.T) {
	buyinSvc := &fakeAutoRebuyBuyin{
		seats:    map[string]buyin.SeatSummary{"player-1": {Seated: true, Stack: 300, AutoRebuy: true, BuyInAmount: 100}},
		balances: map[string]int64{"player-1": 500},
	}
	rooms := fakeRoomModeReader{room: &roomstore.Room{CurrencyMode: roomstore.CurrencyModeSandbox}}

	autoRebuySweep(context.Background(), buyinSvc, rooms, "table-1", "hand-1", hand.HandOutcome{Participants: []string{"player-1"}})

	if len(buyinSvc.buyIns) != 0 {
		t.Fatalf("expected no auto-rebuy for a seat that still has chips, got %+v", buyinSvc.buyIns)
	}
}

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
		Board: []string{"Ah", "Kd", "Qc", "2s", "3h"}, BoardTwo: []string{"Ah", "Kd", "Qc", "4c", "5d"},
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
	if len(item.Board) != 5 || len(item.BoardTwo) != 5 {
		t.Fatalf("both runout boards must reach hand history: %+v", item)
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
	registerRoutes(app, cfg, nil, verifier, manager, ws.NewMemoryRegistry(), nil, nil, (*buyin.Service)(nil), (*player.Service)(nil), (*leaderboard.Service)(nil), (*dailyreward.Service)(nil), nil, nil, nil, nil, nil, nil, nil, nil, (*sandboxpurchase.Service)(nil), (*reactionpurchase.Service)(nil), (*cosmeticpurchase.Service)(nil), (*social.Service)(nil))
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

func TestFiberRejectsOversizedHTTPBodies(t *testing.T) {
	app := newFiberApp(&config.Config{Env: "test", AppVersion: "test", ReadTimeout: 10, WriteTimeout: 10, IdleTimeout: 10})
	app.Post("/upload", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	req, _ := http.NewRequest(http.MethodPost, "/upload", strings.NewReader(strings.Repeat("x", (1<<20)+1)))
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := app.Test(req)
	if err != nil {
		if !strings.Contains(err.Error(), "body size exceeds") {
			t.Fatalf("unexpected request error: %v", err)
		}
		return
	}
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", resp.StatusCode)
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

// TestDispatchGamificationPipelineDoesNotBlockCaller is the regression test
// for #61: onHandComplete used to run its entire gamification pipeline
// (achievements, leaderboard, matchup, sessionlog, ...) synchronously on the
// table actor's own goroutine, so a slow/throttled DynamoDB dependency froze
// the whole table for as long as the pipeline took. dispatchGamificationPipeline
// is the exact function onHandComplete now calls instead of running its body
// inline — this asserts the call returns immediately regardless of how long
// the pipeline itself takes, i.e. the caller (the actor) is never blocked on
// gamification I/O.
func TestDispatchGamificationPipelineDoesNotBlockCaller(t *testing.T) {
	const pipelineDelay = 200 * time.Millisecond
	started := make(chan struct{})
	finished := make(chan struct{})

	callStart := time.Now()
	dispatchGamificationPipeline("table-1", "hand-1", func() {
		close(started)
		time.Sleep(pipelineDelay)
		close(finished)
	})
	callDuration := time.Since(callStart)

	// The dispatch call itself must return long before the pipeline's own
	// artificial delay elapses — this is what "the actor is never blocked on
	// gamification I/O" means in practice.
	if callDuration >= pipelineDelay {
		t.Fatalf("dispatchGamificationPipeline blocked the caller for %v (pipeline delay %v) — the gamification pipeline is not detached", callDuration, pipelineDelay)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("pipeline never started running in its own goroutine")
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("pipeline never finished running in the background")
	}
}

// TestDispatchGamificationPipelineRecoversPanic ensures a panicking
// gamification write (e.g. a bad DynamoDB response) can never crash the
// process from the detached goroutine, since it runs outside any request's
// recover middleware.
func TestDispatchGamificationPipelineRecoversPanic(t *testing.T) {
	done := make(chan struct{})
	dispatchGamificationPipeline("table-1", "hand-1", func() {
		defer close(done)
		panic("boom")
	})
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("panicking pipeline never completed — process may have crashed instead of recovering")
	}
}
