//go:build integration

package buyin

import (
	"context"
	"errors"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/responsible"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
)

// rulesFixture is a real-money room with the real responsible-gambling service
// (memory store) in front of the buy-in, the way app.go wires it.
type rulesFixture struct {
	svc     *Service
	game    *fakeWallet
	sandbox *fakeWallet
	rules   *responsible.Service
	room    string
}

func newRulesFixture(t *testing.T, roomID string) *rulesFixture {
	t.Helper()
	mgr := testManager(t)
	game, sandbox := &fakeWallet{}, &fakeWallet{}
	rooms := &fakeRoomLookup{room: &roomstore.Room{
		ID: roomID, CurrencyMode: "real", BigBlind: 20, BuyInMin: 40, BuyInMax: 400, MaxSeats: 9,
	}}
	rules := responsible.NewService(responsible.NewMemoryStore())
	if _, err := mgr.GetOrCreateActor(context.Background(), roomID, func() *hand.Table { return hand.NewTable(nil, 10, 20) }); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	return &rulesFixture{svc: NewServiceWithGame(sandbox, game, mgr, rooms, rules), game: game, sandbox: sandbox, rules: rules, room: roomID}
}

func (f *rulesFixture) ready(t *testing.T, user string, daily, weekly, monthly int64) {
	t.Helper()
	if err := f.rules.AcceptGambling(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	if _, err := f.rules.SetLimits(context.Background(), user, daily, weekly, monthly); err != nil {
		t.Fatal(err)
	}
}

func (f *rulesFixture) usage(t *testing.T, user string) int64 {
	t.Helper()
	st, err := f.rules.Status(context.Background(), user)
	if err != nil {
		t.Fatal(err)
	}
	return st.Usage.Daily
}

func TestRealBuyInWithoutConsentIsRefusedBeforeAnyHold(t *testing.T) {
	f := newRulesFixture(t, "room-rules-1")
	err := f.svc.BuyIn(context.Background(), f.room, "user-1", 400, false, "")
	if !errors.Is(err, responsible.ErrConsentRequired) {
		t.Fatalf("err = %v, want consent required", err)
	}
	if len(f.game.holds) != 0 || len(f.game.feeDebits) != 0 {
		t.Fatalf("a refusal moved money: holds=%d fees=%d", len(f.game.holds), len(f.game.feeDebits))
	}
}

func TestSelfExcludedPlayerCannotBuyInEvenWithEverythingElseInOrder(t *testing.T) {
	f := newRulesFixture(t, "room-rules-2")
	f.ready(t, "user-1", 100000, 500000, 1000000)
	if _, err := f.rules.SelfExclude(context.Background(), "user-1", "30d"); err != nil {
		t.Fatal(err)
	}
	var excluded *responsible.ExcludedError
	if err := f.svc.BuyIn(context.Background(), f.room, "user-1", 400, false, ""); !errors.As(err, &excluded) {
		t.Fatalf("err = %v, want self-excluded", err)
	}
	if len(f.game.holds) != 0 {
		t.Fatalf("holds = %d", len(f.game.holds))
	}
}

func TestBuyInOverTheDailyLimitIsRefusedBeforeTheHold(t *testing.T) {
	f := newRulesFixture(t, "room-rules-3")
	f.ready(t, "user-1", 300, 5000, 10000)
	var breach *responsible.BreachError
	err := f.svc.BuyIn(context.Background(), f.room, "user-1", 400, false, "")
	if !errors.As(err, &breach) || breach.Breach.Window != "daily" || breach.Breach.ResetsAt.IsZero() {
		t.Fatalf("err = %v, want a daily breach carrying its reset time", err)
	}
	if len(f.game.holds) != 0 {
		t.Fatalf("holds = %d", len(f.game.holds))
	}
}

func TestAcceptedBuyInMetersTheLimitOnceEvenWhenRetried(t *testing.T) {
	f := newRulesFixture(t, "room-rules-4")
	f.ready(t, "user-1", 100000, 500000, 1000000)
	if err := f.svc.BuyIn(context.Background(), f.room, "user-1", 400, false, "nonce-1"); err != nil {
		t.Fatalf("buyin: %v", err)
	}
	if len(f.game.holds) != 1 || len(f.sandbox.debits) != 0 {
		t.Fatalf("holds=%d sandbox debits=%d", len(f.game.holds), len(f.sandbox.debits))
	}
	if got := f.usage(t, "user-1"); got != 400 {
		t.Fatalf("metered %d, want 400", got)
	}
	// The player is seated with chips, so a retry is a no-op; the meter must not move.
	if err := f.svc.BuyIn(context.Background(), f.room, "user-1", 400, false, "nonce-1"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if got := f.usage(t, "user-1"); got != 400 {
		t.Fatalf("a retry was metered again: %d", got)
	}
}

// A racing buy-in took the last of the window between Eligibility and the meter:
// the hold goes back and nobody is seated.
func TestLosingTheMeterRaceReleasesTheHold(t *testing.T) {
	f := newRulesFixture(t, "room-rules-5")
	f.ready(t, "user-1", 500, 5000, 10000)
	racing := &racingRules{Service: f.rules, before: func() {
		if b, err := f.rules.RecordInflow(context.Background(), "user-1", 300, "other-buyin"); err != nil || b != nil {
			t.Fatalf("setup inflow: %v %v", b, err)
		}
	}}
	f.svc.rules = racing
	err := f.svc.BuyIn(context.Background(), f.room, "user-1", 400, false, "")
	var breach *responsible.BreachError
	if !errors.As(err, &breach) {
		t.Fatalf("err = %v, want a breach", err)
	}
	if len(f.game.holds) != 1 || len(f.game.released) != 1 || f.game.released[0] != f.game.holds[0].holdID {
		t.Fatalf("holds=%v released=%v", f.game.holds, f.game.released)
	}
	if got := f.usage(t, "user-1"); got != 300 {
		t.Fatalf("the refused buy-in was counted: %d", got)
	}
}

// racingRules lets a test move the counters after Eligibility said yes.
type racingRules struct {
	*responsible.Service
	before func()
}

func (r *racingRules) RecordInflow(ctx context.Context, userID string, amount int64, key string) (*responsible.LimitBreach, error) {
	r.before()
	return r.Service.RecordInflow(ctx, userID, amount, key)
}

func TestSandboxBuyInIgnoresTheRules(t *testing.T) {
	mgr := testManager(t)
	wallet := &fakeWallet{}
	rooms := testRoomLookup()
	rules := responsible.NewService(responsible.NewMemoryStore())
	svc := NewServiceWithGame(wallet, &fakeWallet{}, mgr, rooms, rules)
	if _, err := mgr.GetOrCreateActor(context.Background(), "test-room", func() *hand.Table { return hand.NewTable(nil, 10, 20) }); err != nil {
		t.Fatal(err)
	}
	if err := rules.AcceptGambling(context.Background(), "user-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := rules.SelfExclude(context.Background(), "user-1", "indefinite"); err != nil {
		t.Fatal(err)
	}
	if err := svc.BuyIn(context.Background(), "test-room", "user-1", 400, false, ""); err != nil {
		t.Fatalf("a self-excluded player may still play Sandbox: %v", err)
	}
}
