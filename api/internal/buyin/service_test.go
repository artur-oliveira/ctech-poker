//go:build integration

package buyin

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/reconcile"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

type fakeWallet struct {
	credits   []call
	debits    []call
	feeDebits []call
	holds     []holdCall
	cashouts  []cashoutCall
}
type call struct {
	userID string
	amount int64
	key    string
}
type holdCall struct {
	userID         string
	amount         int64
	tableRef       string
	idempotencyKey string
	reason         string
	holdID         string
}
type cashoutCall struct {
	userID         string
	amount         int64
	tableRef       string
	holdIDs        []string
	idempotencyKey string
	reason         string
}

type failingPendingStore struct{ err error }

func (s failingPendingStore) BuildRecordTx(reconcile.PendingCashout) (types.TransactWriteItem, error) {
	return types.TransactWriteItem{}, s.err
}
func (s failingPendingStore) Record(context.Context, reconcile.PendingCashout) error { return s.err }
func (s failingPendingStore) MarkResolved(context.Context, string) error             { return s.err }

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
func (f *fakeWallet) DebitReal(_ context.Context, userID string, amount int64, key, _ string) error {
	f.feeDebits = append(f.feeDebits, call{userID, amount, key})
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

// raceWallet lets a test force two BuyIn calls to interleave: the first
// caller to Debit blocks until released, so a second, concurrent BuyIn for
// the same seat can win the race and commit its JoinCmd first.
type raceWallet struct {
	fakeWallet
	mu           sync.Mutex
	debitN       int
	firstStarted chan struct{}
	release      chan struct{}
}

func (f *raceWallet) Debit(ctx context.Context, userID string, amount int64, key, reason string) error {
	f.mu.Lock()
	f.debitN++
	first := f.debitN == 1
	f.mu.Unlock()
	if first {
		close(f.firstStarted)
		<-f.release
	}
	return f.fakeWallet.Debit(ctx, userID, amount, key, reason)
}

// TestBuyInRefundsLoserOfConcurrentSeatRace guards against the money-loss bug
// found in the 2026-07-27 audit: two near-simultaneous BuyIn calls for the
// same (room, player) — double-click, two devices — each carry a distinct
// client nonce, so isSeated's early-return can't dedupe them and both debit
// real money before either seats. Only one JoinCmd can win the seat; the
// loser must be refunded, never silently swallowed as a no-op.
func TestBuyInRefundsLoserOfConcurrentSeatRace(t *testing.T) {
	wallet := &raceWallet{firstStarted: make(chan struct{}), release: make(chan struct{})}
	mgr := testManager(t)
	rooms := testRoomLookup()
	svc := NewService(wallet, mgr, rooms)
	ctx := context.Background()

	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	if _, err := mgr.GetOrCreateActor(ctx, "room-1", seed); err != nil {
		t.Fatalf("get or create actor: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.BuyIn(ctx, "room-1", "user-1", 400, false, "nonce-a")
	}()

	<-wallet.firstStarted
	if err := svc.BuyIn(ctx, "room-1", "user-1", 400, false, "nonce-b"); err != nil {
		t.Fatalf("winning buyin should succeed: %v", err)
	}
	close(wallet.release)

	if errA := <-errCh; errA == nil {
		t.Fatal("losing buyin should return an error (refunded), not silent success")
	}

	if len(wallet.debits) != 2 {
		t.Fatalf("expected 2 debits (one per concurrent attempt), got %+v", wallet.debits)
	}
	if len(wallet.credits) != 1 || wallet.credits[0].amount != 400 {
		t.Fatalf("expected exactly one 400-chip refund credit for the race loser, got %+v", wallet.credits)
	}
}

func TestBuyInFastFailsAlreadyFullTableBeforeDebit(t *testing.T) {
	wallet := &fakeWallet{}
	mgr := testManager(t)
	rooms := &fakeRoomLookup{room: &roomstore.Room{
		ID: "room-full", CurrencyMode: "sandbox", BigBlind: 20,
		BuyInMin: 40, BuyInMax: 400, MaxSeats: 1,
	}}
	svc := NewService(wallet, mgr, rooms)
	ctx := context.Background()

	if err := svc.BuyIn(ctx, "room-full", "player-1", 100, false, "first"); err != nil {
		t.Fatalf("first buy-in: %v", err)
	}
	err := svc.BuyIn(ctx, "room-full", "player-2", 100, false, "second")
	if !errors.Is(err, table.ErrNoSeatsAvailable) {
		t.Fatalf("second buy-in error=%v, want ErrNoSeatsAvailable", err)
	}
	if len(wallet.debits) != 1 {
		t.Fatalf("full-table fast fail made an avoidable debit: %+v", wallet.debits)
	}
}

func TestBuyInAllowsBustedSeatedPlayerToRebuyAtFullTable(t *testing.T) {
	wallet := &fakeWallet{}
	mgr := testManager(t)
	rooms := &fakeRoomLookup{room: &roomstore.Room{
		ID: "room-full-rebuy", CurrencyMode: "sandbox", BigBlind: 20,
		BuyInMin: 40, BuyInMax: 400, MaxSeats: 1,
	}}
	svc := NewService(wallet, mgr, rooms)
	ctx := context.Background()

	seed := func() *hand.Table {
		return hand.NewTable([]*hand.Player{{
			ID: "player-1", Stack: 0, State: hand.SittingOut,
		}}, 10, 20)
	}
	if _, err := mgr.GetOrCreateActor(ctx, "room-full-rebuy", seed); err != nil {
		t.Fatalf("seed full table: %v", err)
	}

	if err := svc.BuyIn(ctx, "room-full-rebuy", "player-1", 100, false, "rebuy"); err != nil {
		t.Fatalf("busted seated player rebuy: %v", err)
	}
	if len(wallet.debits) != 1 || wallet.debits[0].amount != 100 {
		t.Fatalf("expected one rebuy debit, got %+v", wallet.debits)
	}
}

func TestCashOutRemovesThenCredits(t *testing.T) {
	wallet := &fakeWallet{}
	mgr := testManager(t)
	rooms := testRoomLookup()
	svc := NewService(wallet, mgr, rooms).WithPendingStore(testPendingStore(t))
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

func TestCashOutKeepsSeatWhenSettlementIntentCannotBePersisted(t *testing.T) {
	wallet := &fakeWallet{}
	mgr := testManager(t)
	svc := NewService(wallet, mgr, testRoomLookup()).WithPendingStore(failingPendingStore{err: errors.New("dynamodb unavailable")})
	ctx := context.Background()

	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	actor, err := mgr.GetOrCreateActor(ctx, "room-cashout-failure", seed)
	if err != nil {
		t.Fatalf("get or create actor: %v", err)
	}
	if err := svc.BuyIn(ctx, "room-cashout-failure", "user-1", 400, false, ""); err != nil {
		t.Fatalf("buyin: %v", err)
	}

	if _, err := svc.CashOut(ctx, "room-cashout-failure", "user-1", "nonce"); err == nil {
		t.Fatal("expected cashout to fail when the durable settlement intent cannot be built")
	}
	if len(wallet.credits) != 0 {
		t.Fatalf("wallet must not be credited after failed seat transaction, got %+v", wallet.credits)
	}
	found := false
	for _, seat := range actor.TableForTest().ViewFor("user-1").Seats {
		if seat.PlayerID == "user-1" && seat.Stack == 400 {
			found = true
		}
	}
	if !found {
		t.Fatal("player seat must be restored when the settlement intent cannot be committed")
	}
}

func TestBuyInRecordsAnOpenSessionWhenSessionStoreIsSet(t *testing.T) {
	wallet := &fakeWallet{}
	mgr := testManager(t)
	rooms := testRoomLookup()
	svc := NewService(wallet, mgr, rooms)
	sessions := testSessionStore(t)
	svc.WithSessionStore(sessions)
	ctx := context.Background()

	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	if _, err := mgr.GetOrCreateActor(ctx, "room-3", seed); err != nil {
		t.Fatalf("get or create actor: %v", err)
	}
	if err := svc.BuyIn(ctx, "room-3", "user-1", 400, false, ""); err != nil {
		t.Fatalf("buyin: %v", err)
	}

	open, err := sessions.FindOpenSession(ctx, "user-1", "room-3")
	if err != nil || open == nil {
		t.Fatalf("expected an open session recorded, got %+v err=%v", open, err)
	}
	if open.BuyinAmount != 400 {
		t.Fatalf("expected buyin amount 400 recorded, got %d", open.BuyinAmount)
	}
}

func TestBuyInRebuyIncrementsExistingSession(t *testing.T) {
	wallet := &fakeWallet{}
	mgr := testManager(t)
	rooms := testRoomLookup()
	svc := NewService(wallet, mgr, rooms)
	sessions := testSessionStore(t)
	svc.WithSessionStore(sessions)
	ctx := context.Background()

	// Seed table with user-1 already seated but having 0 chips (busted rebuy situation)
	seed := func() *hand.Table {
		p := &hand.Player{ID: "user-1", Stack: 0, State: hand.SittingOut}
		return hand.NewTable([]*hand.Player{p}, 10, 20)
	}
	if _, err := mgr.GetOrCreateActor(ctx, "room-rebuy", seed); err != nil {
		t.Fatalf("get or create actor: %v", err)
	}

	// First, record an open session manually to simulate the initial session being open
	_ = sessions.RecordSession(ctx, sessionlog.SessionItem{PK: "user-1", TableID: "room-rebuy", BuyinAmount: 400, JoinedAt: 1})

	// Perform buyin (rebuy) of 200 chips
	if err := svc.BuyIn(ctx, "room-rebuy", "user-1", 200, false, ""); err != nil {
		t.Fatalf("buyin: %v", err)
	}

	// Verify that the existing open session has been updated (BuyinAmount updated to 400 + 200 = 600)
	open, err := sessions.FindOpenSession(ctx, "user-1", "room-rebuy")
	if err != nil || open == nil {
		t.Fatalf("expected an open session to exist, got %+v err=%v", open, err)
	}
	if open.BuyinAmount != 600 {
		t.Fatalf("expected cumulative buyin amount of 600, got %d", open.BuyinAmount)
	}
}

func TestCashOutClosesTheOpenSession(t *testing.T) {
	wallet := &fakeWallet{}
	mgr := testManager(t)
	rooms := testRoomLookup()
	svc := NewService(wallet, mgr, rooms).WithPendingStore(testPendingStore(t))
	sessions := testSessionStore(t)
	svc.WithSessionStore(sessions)
	ctx := context.Background()

	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	if _, err := mgr.GetOrCreateActor(ctx, "room-4", seed); err != nil {
		t.Fatalf("get or create actor: %v", err)
	}
	if err := svc.BuyIn(ctx, "room-4", "user-1", 400, false, ""); err != nil {
		t.Fatalf("buyin: %v", err)
	}
	if _, err := svc.CashOut(ctx, "room-4", "user-1", ""); err != nil {
		t.Fatalf("cashout: %v", err)
	}

	open, _ := sessions.FindOpenSession(ctx, "user-1", "room-4")
	if open != nil {
		t.Fatal("expected the session to be closed after cash-out")
	}
}

// TestSettleSkipsWalletCallForZeroStack pins down that a busted player
// (stack 0 at cash-out/removal) never reaches wallet.Credit/CashoutGame —
// ctech-wallet rejects a zero amount as a validation error, which used to
// surface as "buyin: cash-out credit failed after seat removal" even though
// there was nothing to credit. The session must still close normally.
func TestSettleSkipsWalletCallForZeroStack(t *testing.T) {
	wallet := &fakeWallet{}
	mgr := testManager(t)
	rooms := testRoomLookup()
	svc := NewService(wallet, mgr, rooms).WithPendingStore(testPendingStore(t))
	sessions := testSessionStore(t)
	svc.WithSessionStore(sessions)
	ctx := context.Background()

	_ = sessions.RecordSession(ctx, sessionlog.SessionItem{PK: "user-1", TableID: "room-5", JoinedAt: 1})

	if err := svc.settle(ctx, "room-5", "user-1", 0, "", wallet, "room-5#user-1#zero"); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if len(wallet.credits) != 0 {
		t.Fatalf("expected no wallet credit for a zero stack, got %+v", wallet.credits)
	}

	open, _ := sessions.FindOpenSession(ctx, "user-1", "room-5")
	if open != nil {
		t.Fatal("expected the session to still be closed after a zero-stack settle")
	}
}

type fakeActivation struct{ activated map[string]bool }

func (f *fakeActivation) IsGamblingActivated(_ context.Context, userID string) (bool, error) {
	return f.activated[userID], nil
}

type fakeRoomLookup struct{ room *roomstore.Room }

func (f *fakeRoomLookup) Get(_ context.Context, _ string) (*roomstore.Room, error) {
	return f.room, nil
}

func TestSeatedReportsExistingSeatAndStack(t *testing.T) {
	wallet := &fakeWallet{}
	mgr := testManager(t)
	rooms := testRoomLookup()
	svc := NewService(wallet, mgr, rooms)
	ctx := context.Background()

	if err := svc.BuyIn(ctx, "test-room", "player-1", 100, false, "idem-1"); err != nil {
		t.Fatalf("BuyIn: %v", err)
	}

	seated, stack, err := svc.Seated(ctx, "test-room", "player-1")
	if err != nil {
		t.Fatalf("Seated: %v", err)
	}
	if !seated || stack != 100 {
		t.Fatalf("expected seated=true stack=100, got seated=%v stack=%d", seated, stack)
	}
}

func TestSeatedReportsFalseForNeverJoinedPlayer(t *testing.T) {
	wallet := &fakeWallet{}
	mgr := testManager(t)
	rooms := testRoomLookup()
	svc := NewService(wallet, mgr, rooms)
	ctx := context.Background()

	if err := svc.BuyIn(ctx, "test-room", "player-1", 100, false, "idem-1"); err != nil {
		t.Fatalf("BuyIn: %v", err)
	}

	seated, stack, err := svc.Seated(ctx, "test-room", "player-2")
	if err != nil {
		t.Fatalf("Seated: %v", err)
	}
	if seated || stack != 0 {
		t.Fatalf("expected seated=false stack=0 for a player who never joined, got seated=%v stack=%d", seated, stack)
	}
}

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

func TestBuyInChargesFixedEntryFeeForRealRoomsAfterSeating(t *testing.T) {
	sandbox := &fakeWallet{}
	game := &fakeWallet{}
	mgr := testManager(t)
	rooms := &fakeRoomLookup{room: &roomstore.Room{
		ID: "room-real-fee", CurrencyMode: "real", BigBlind: 20, BuyInMin: 40, BuyInMax: 400, MaxSeats: 9,
		EntryFeeCents: 100,
	}}
	svc := NewServiceWithGame(sandbox, game, mgr, rooms, &fakeActivation{activated: map[string]bool{"user-1": true}})
	ctx := context.Background()

	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	if _, err := mgr.GetOrCreateActor(ctx, "room-real-fee", seed); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	if err := svc.BuyIn(ctx, "room-real-fee", "user-1", 400, false, "nonce-1"); err != nil {
		t.Fatalf("buyin: %v", err)
	}
	if len(game.feeDebits) != 1 || game.feeDebits[0].amount != 100 || game.feeDebits[0].userID != "user-1" {
		t.Fatalf("expected one 100-cent fee debit for user-1, got %+v", game.feeDebits)
	}
	if len(game.holds) != 1 || game.holds[0].amount != 400 {
		t.Fatalf("expected the stake hold to remain 400 (fee is separate), got %+v", game.holds)
	}
}

func TestBuyInChargesFeeAgainOnRebuyAfterLeaving(t *testing.T) {
	sandbox := &fakeWallet{}
	game := &fakeWallet{}
	mgr := testManager(t)
	rooms := &fakeRoomLookup{room: &roomstore.Room{
		ID: "room-real-rebuy", CurrencyMode: "real", BigBlind: 20, BuyInMin: 40, BuyInMax: 400, MaxSeats: 9,
		EntryFeeCents: 100,
	}}
	svc := NewServiceWithGame(sandbox, game, mgr, rooms, &fakeActivation{activated: map[string]bool{"user-1": true}}).WithPendingStore(testPendingStore(t))
	ctx := context.Background()

	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	if _, err := mgr.GetOrCreateActor(ctx, "room-real-rebuy", seed); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	if err := svc.BuyIn(ctx, "room-real-rebuy", "user-1", 400, false, "nonce-1"); err != nil {
		t.Fatalf("first buyin: %v", err)
	}
	if _, err := svc.CashOut(ctx, "room-real-rebuy", "user-1", ""); err != nil {
		t.Fatalf("cashout: %v", err)
	}
	// A fresh nonce, exactly like the UI's rebuy flow (RebuyDialog calls the
	// same joinRoom as a first-time join, with a new crypto.randomUUID()).
	if err := svc.BuyIn(ctx, "room-real-rebuy", "user-1", 400, false, "nonce-2"); err != nil {
		t.Fatalf("rebuy: %v", err)
	}
	if len(game.feeDebits) != 2 {
		t.Fatalf("expected the fee to be charged again on rebuy after leaving, got %+v", game.feeDebits)
	}
}

func TestBuyInSkipsFeeForSandboxRooms(t *testing.T) {
	wallet := &fakeWallet{}
	mgr := testManager(t)
	rooms := testRoomLookup() // sandbox room, EntryFeeCents unset (0)
	svc := NewService(wallet, mgr, rooms)
	ctx := context.Background()

	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	if _, err := mgr.GetOrCreateActor(ctx, "room-sandbox-fee", seed); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := svc.BuyIn(ctx, "room-sandbox-fee", "user-1", 400, false, ""); err != nil {
		t.Fatalf("buyin: %v", err)
	}
	if len(wallet.feeDebits) != 0 {
		t.Fatalf("expected no fee debit for a sandbox room, got %+v", wallet.feeDebits)
	}
}
