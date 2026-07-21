//go:build integration

package buyin

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

type fakeWallet struct {
	credits  []call
	debits   []call
	holds    []holdCall
	cashouts []cashoutCall
}
type call struct {
	userID string
	amount int64
	key    string
}
type holdCall struct {
	userID       string
	amount       int64
	tableRef     string
	idempotencyKey string
	reason       string
	holdID       string
}
type cashoutCall struct {
	userID      string
	amount      int64
	tableRef    string
	holdIDs     []string
	idempotencyKey string
	reason      string
}

func (f *fakeWallet) Credit(_ context.Context, userID string, amount int64, key, _ string) error {
	f.credits = append(f.credits, call{userID, amount, key})
	return nil
}
func (f *fakeWallet) Debit(_ context.Context, userID string, amount int64, key, _ string) error {
	f.debits = append(f.debits, call{userID, amount, key})
	return nil
}
func (f *fakeWallet) HoldGame(_ context.Context, userID string, amount int64, tableRef, idempotencyKey, reason string) (string, error) {
	id := fmt.Sprintf("hold-%d", len(f.holds))
	f.holds = append(f.holds, holdCall{userID, amount, tableRef, idempotencyKey, reason, id})
	return id, nil
}
func (f *fakeWallet) ReleaseHold(_ context.Context, holdID string) error {
	return nil
}
func (f *fakeWallet) CashoutGame(_ context.Context, userID string, amount int64, tableRef string, holdIDs []string, key, reason string) error {
	f.cashouts = append(f.cashouts, cashoutCall{userID, amount, tableRef, holdIDs, key, reason})
	return nil
}

func testManager(t *testing.T) *tablemanager.Manager {
	t.Helper()
	db := testClient(t)
	env := fmt.Sprintf("buyin_test_%d", time.Now().UnixNano())
	mustCreateTestTables(t, db, env)
	store := tablestore.NewStore(db, env)
	return tablemanager.NewManager(tablelease.NewService(cache.NewMemoryBackend(16)), store, nil, nil, nil)
}

func testRoomLookup() *fakeRoomLookup {
	return &fakeRoomLookup{room: &roomstore.Room{
		ID: "test-room", CurrencyMode: "sandbox", BigBlind: 20, BuyInMin: 40, BuyInMax: 400, MaxSeats: 9,
	}}
}

func TestBuyInDebitsThenSeats(t *testing.T) {
	wallet := &fakeWallet{}
	mgr := testManager(t)
	rooms := testRoomLookup()
	svc := NewService(wallet, mgr, rooms)
	ctx := context.Background()

	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	actor, err := mgr.GetOrCreateActor(ctx, "room-1", seed)
	if err != nil {
		t.Fatalf("get or create actor: %v", err)
	}

	if err := svc.BuyIn(ctx, "room-1", "user-1", 400, false, ""); err != nil {
		t.Fatalf("buyin: %v", err)
	}
	if len(wallet.debits) != 1 || wallet.debits[0].amount != 400 {
		t.Fatalf("expected one 400-chip debit, got %+v", wallet.debits)
	}
	found := false
	for _, s := range actor.TableForTest().ViewFor("user-1").Seats {
		if s.PlayerID == "user-1" && s.Stack == 400 {
			found = true
		}
	}
	if !found {
		t.Fatal("expected user-1 seated with a 400-chip stack after buy-in")
	}
}

func TestCashOutRemovesThenCredits(t *testing.T) {
	wallet := &fakeWallet{}
	mgr := testManager(t)
	rooms := testRoomLookup()
	svc := NewService(wallet, mgr, rooms)
	ctx := context.Background()

	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	if _, err := mgr.GetOrCreateActor(ctx, "room-2", seed); err != nil {
		t.Fatalf("get or create actor: %v", err)
	}
	if err := svc.BuyIn(ctx, "room-2", "user-1", 400, false, ""); err != nil {
		t.Fatalf("buyin: %v", err)
	}

	stack, err := svc.CashOut(ctx, "room-2", "user-1", "")
	if err != nil {
		t.Fatalf("cashout: %v", err)
	}
	if stack != 400 {
		t.Fatalf("expected cash-out amount 400, got %d", stack)
	}
	if len(wallet.credits) != 1 || wallet.credits[0].amount != 400 {
		t.Fatalf("expected one 400-chip credit, got %+v", wallet.credits)
	}
}

type fakeActivation struct{ activated map[string]bool }
func (f *fakeActivation) IsGamblingActivated(_ context.Context, userID string) (bool, error) {
	return f.activated[userID], nil
}

type fakeRoomLookup struct{ room *roomstore.Room }
func (f *fakeRoomLookup) Get(_ context.Context, _ string) (*roomstore.Room, error) { return f.room, nil }

func TestBuyInRejectsRealRoomWithoutGamblingActivation(t *testing.T) {
	sandbox := &fakeWallet{}
	game := &fakeWallet{}
	mgr := testManager(t)
	rooms := &fakeRoomLookup{room: &roomstore.Room{
		ID: "room-real-1", CurrencyMode: "real", BigBlind: 20, BuyInMin: 40, BuyInMax: 400, MaxSeats: 9,
	}}
	svc := NewServiceWithGame(sandbox, game, mgr, rooms, &fakeActivation{activated: map[string]bool{}})
	ctx := context.Background()

	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	if _, err := mgr.GetOrCreateActor(ctx, "room-real-1", seed); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	if err := svc.BuyIn(ctx, "room-real-1", "user-1", 400, false, ""); err == nil {
		t.Fatal("expected buy-in to be rejected for a non-activated user in a real room")
	}
}

func TestBuyInUsesGameWalletForRealRooms(t *testing.T) {
	sandbox := &fakeWallet{}
	game := &fakeWallet{}
	mgr := testManager(t)
	rooms := &fakeRoomLookup{room: &roomstore.Room{
		ID: "room-real-2", CurrencyMode: "real", BigBlind: 20, BuyInMin: 40, BuyInMax: 400, MaxSeats: 9,
	}}
	svc := NewServiceWithGame(sandbox, game, mgr, rooms, &fakeActivation{activated: map[string]bool{"user-1": true}})
	ctx := context.Background()

	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	if _, err := mgr.GetOrCreateActor(ctx, "room-real-2", seed); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	if err := svc.BuyIn(ctx, "room-real-2", "user-1", 400, false, ""); err != nil {
		t.Fatalf("buyin: %v", err)
	}
	if len(game.holds) != 1 {
		t.Fatalf("expected one game-wallet hold, got %d", len(game.holds))
	}
	if len(sandbox.debits) != 0 {
		t.Fatalf("expected zero sandbox debits, got %d", len(sandbox.debits))
	}
}
