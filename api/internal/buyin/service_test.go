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
	"gopkg.aoctech.app/poker/api/internal/entitlement"
	"gopkg.aoctech.app/poker/api/internal/reconcile"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

type fakeWallet struct {
	credits   []call
	debits    []call
	feeDebits []call
	holds     []holdCall
	cashouts  []cashoutCall
	balances  map[string]int64 // playerID -> sandbox balance, for the auto-rebuy tests
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
func (s failingPendingStore) Get(context.Context, string) (*reconcile.PendingCashout, error) {
	return nil, s.err
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
func (f *fakeWallet) DebitReal(_ context.Context, userID string, amount int64, key, _ string) error {
	f.feeDebits = append(f.feeDebits, call{userID, amount, key})
	return nil
}

func (f *fakeWallet) Balances(_ context.Context, userID string) (*walletclient.Balances, error) {
	return &walletclient.Balances{SandboxBalance: f.balances[userID]}, nil
}

func testManager(t *testing.T) *tablemanager.Manager {
	t.Helper()
	mgr, _ := testManagerAndStore(t)
	return mgr
}

func testManagerAndStore(t *testing.T) (*tablemanager.Manager, *tablestore.Store) {
	t.Helper()
	db := testClient(t)
	env := fmt.Sprintf("buyin_test_%d", time.Now().UnixNano())
	mustCreateTestTables(t, db, env)
	store := tablestore.NewStore(db, env)
	return tablemanager.NewManager(tablelease.NewService(cache.NewMemoryBackend(16)), store, nil, nil, nil), store
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

// multiRoomLookup is fakeRoomLookup's multi-table counterpart, for tests that
// need distinct rooms (entitlement rebind across tables) instead of a single
// fixture returned regardless of the requested ID. A nil result for an
// unknown ID mirrors roomstore.Store.Get on a deleted/nonexistent room.
type multiRoomLookup map[string]*roomstore.Room

func (f multiRoomLookup) Get(_ context.Context, roomID string) (*roomstore.Room, error) {
	return f[roomID], nil
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

func TestSandboxBalanceReturnsWalletBalance(t *testing.T) {
	wallet := &fakeWallet{balances: map[string]int64{"player-1": 500}}
	mgr := testManager(t)
	rooms := testRoomLookup()
	svc := NewService(wallet, mgr, rooms)

	balance, err := svc.SandboxBalance(context.Background(), "player-1")
	if err != nil {
		t.Fatalf("SandboxBalance: %v", err)
	}
	if balance != 500 {
		t.Fatalf("expected balance=500, got %d", balance)
	}
}

func TestSeatedSummaryReportsAutoRebuyAndBuyInAmount(t *testing.T) {
	wallet := &fakeWallet{}
	mgr := testManager(t)
	rooms := testRoomLookup()
	svc := NewService(wallet, mgr, rooms)
	ctx := context.Background()

	if err := svc.BuyInWithAutoRebuy(ctx, "test-room", "player-1", 100, false, true, "idem-1"); err != nil {
		t.Fatalf("BuyInWithAutoRebuy: %v", err)
	}

	seat, err := svc.SeatedSummary(ctx, "test-room", "player-1")
	if err != nil {
		t.Fatalf("SeatedSummary: %v", err)
	}
	if !seat.Seated || seat.Stack != 100 || !seat.AutoRebuy || seat.BuyInAmount != 100 {
		t.Fatalf("expected seated=true stack=100 autoRebuy=true buyInAmount=100, got %+v", seat)
	}
}

func TestSeatedSummaryReportsFalseAutoRebuyWhenNotOptedIn(t *testing.T) {
	wallet := &fakeWallet{}
	mgr := testManager(t)
	rooms := testRoomLookup()
	svc := NewService(wallet, mgr, rooms)
	ctx := context.Background()

	if err := svc.BuyIn(ctx, "test-room", "player-1", 100, false, "idem-1"); err != nil {
		t.Fatalf("BuyIn: %v", err)
	}

	seat, err := svc.SeatedSummary(ctx, "test-room", "player-1")
	if err != nil {
		t.Fatalf("SeatedSummary: %v", err)
	}
	if seat.AutoRebuy {
		t.Fatalf("expected AutoRebuy=false for a plain BuyIn, got %+v", seat)
	}
}

// TestSeatedSummaryKeepsOriginalBuyInAmountAcrossManualRebuy exercises the
// same invariant as hand_test.go's
// TestAddWaitingPlayerRebuyKeepsOriginalAutoRebuyAndBuyInAmount, but through
// the buyin.Service seam the auto-rebuy sweep actually calls.
func TestSeatedSummaryKeepsOriginalBuyInAmountAcrossManualRebuy(t *testing.T) {
	wallet := &fakeWallet{}
	mgr := testManager(t)
	rooms := &fakeRoomLookup{room: &roomstore.Room{
		ID: "room-rebuy-amount", CurrencyMode: "sandbox", BigBlind: 20, BuyInMin: 40, BuyInMax: 400, MaxSeats: 9,
	}}
	svc := NewService(wallet, mgr, rooms)
	ctx := context.Background()

	seed := func() *hand.Table {
		return hand.NewTable([]*hand.Player{{
			ID: "player-1", Stack: 0, State: hand.SittingOut, AutoRebuy: true, BuyInAmount: 100,
		}}, 10, 20)
	}
	if _, err := mgr.GetOrCreateActor(ctx, "room-rebuy-amount", seed); err != nil {
		t.Fatalf("seed busted seat: %v", err)
	}

	if err := svc.BuyIn(ctx, "room-rebuy-amount", "player-1", 200, false, "rebuy"); err != nil {
		t.Fatalf("manual rebuy: %v", err)
	}

	seat, err := svc.SeatedSummary(ctx, "room-rebuy-amount", "player-1")
	if err != nil {
		t.Fatalf("SeatedSummary: %v", err)
	}
	if seat.Stack != 200 {
		t.Fatalf("expected post-rebuy stack=200, got %d", seat.Stack)
	}
	if !seat.AutoRebuy || seat.BuyInAmount != 100 {
		t.Fatalf("expected AutoRebuy/BuyInAmount pinned to original join (true/100), got auto_rebuy=%v buy_in_amount=%d", seat.AutoRebuy, seat.BuyInAmount)
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

func TestBuyInChargesFixedEntryFeeForRealRoomsBeforeSeating(t *testing.T) {
	sandbox := &fakeWallet{}
	game := &fakeWallet{}
	mgr := testManager(t)
	rooms := &fakeRoomLookup{room: &roomstore.Room{
		ID: "room-real-fee", CurrencyMode: "real", Tier: "micro", BigBlind: 20, BuyInMin: 40, BuyInMax: 400, MaxSeats: 9,
		EntryFeeCents: 100,
	}}
	svc := NewServiceWithGame(sandbox, game, mgr, rooms, &fakeActivation{activated: map[string]bool{"user-1": true}}).
		WithPendingStore(testPendingStore(t)).WithEntitlements(testEntitlementStore(t))
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

// failingFeeWallet lets a test force every DebitReal call to fail while
// still recording each attempt, so a test can assert on how many times (and
// with what idempotency key) the entry fee was actually attempted.
type failingFeeWallet struct {
	fakeWallet
	err error
}

func (f *failingFeeWallet) DebitReal(ctx context.Context, userID string, amount int64, key, reason string) error {
	if err := f.fakeWallet.DebitReal(ctx, userID, amount, key, reason); err != nil {
		return err
	}
	return f.err
}

// TestBuyInEntitlementClaimRaceNeverSeatsForFreeWhenFeeChargeFails is the
// regression test for issue #40's Claim-race free seat: two concurrent
// buy-ins for the same (player, room) both reach entitlement.Store.Claim —
// one wins (persists the row), the other loses (ErrAlreadyClaimed). The old
// buyin.Service.chargeEntryFee treated ErrAlreadyClaimed alone as "the fee
// is covered" and seated the loser unconditionally, even though the
// winner's own DebitReal might never actually succeed. This drives that
// exact sequence — winner's charge fails, then a second (losing) claim
// attempt for the same table — and asserts neither call ever seats the
// player nor records a game-wallet stake hold: the fee genuinely never
// cleared, so nobody should be sitting at the table for it.
func TestBuyInEntitlementClaimRaceNeverSeatsForFreeWhenFeeChargeFails(t *testing.T) {
	sandbox := &fakeWallet{}
	game := &failingFeeWallet{err: errors.New("wallet unavailable")}
	mgr := testManager(t)
	rooms := &fakeRoomLookup{room: &roomstore.Room{
		ID: "room-real-fee-race", CurrencyMode: "real", Tier: "micro", BigBlind: 20, BuyInMin: 40, BuyInMax: 400, MaxSeats: 9,
		EntryFeeCents: 100,
	}}
	svc := NewServiceWithGame(sandbox, game, mgr, rooms, &fakeActivation{activated: map[string]bool{"user-1": true}}).
		WithPendingStore(testPendingStore(t)).WithEntitlements(testEntitlementStore(t))
	ctx := context.Background()

	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	if _, err := mgr.GetOrCreateActor(ctx, "room-real-fee-race", seed); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	// "Winner": claims the entitlement, then its fee charge fails — must not
	// seat the player (this alone is not the race; it's the setup for it).
	if err := svc.BuyIn(ctx, "room-real-fee-race", "user-1", 400, false, "nonce-winner"); err == nil {
		t.Fatal("expected the winning claim's failed fee charge to return an error")
	}

	// "Loser": a second, concurrent-in-spirit buy-in for the same
	// (player, room) — a different client nonce, same as a double-click or
	// second device — arrives while the entitlement row the winner claimed
	// is still valid. Before the fix this returned nil (ErrAlreadyClaimed
	// alone treated as "covered") and seated the player for free.
	if err := svc.BuyIn(ctx, "room-real-fee-race", "user-1", 400, false, "nonce-loser"); err == nil {
		t.Fatal("expected the losing claim to also fail: the fee was never actually collected")
	}

	if len(game.holds) != 0 {
		t.Fatalf("player must never be seated (no stake hold) when the entry fee never cleared, got %+v", game.holds)
	}
	if len(game.feeDebits) != 2 {
		t.Fatalf("expected both the winner and the loser to each attempt the fee charge, got %+v", game.feeDebits)
	}
	if game.feeDebits[0].key != game.feeDebits[1].key {
		t.Fatalf("expected both racers to share one idempotency key derived from the persisted claim, got %q and %q", game.feeDebits[0].key, game.feeDebits[1].key)
	}
}

// TestBuyInDoesNotChargeFeeAgainOnRebuyOrReentryWithinWindow is the
// regression test for Problem 1 (docs/plans/2026-08-21-entry-fee-entitlement.md):
// the fee is a table reservation good for entitlement.Window, not a
// per-buy-in charge. Bust-and-rebuy and cash-out-and-return within the
// window must both be free.
func TestBuyInDoesNotChargeFeeAgainOnRebuyOrReentryWithinWindow(t *testing.T) {
	sandbox := &fakeWallet{}
	game := &fakeWallet{}
	mgr := testManager(t)
	rooms := &fakeRoomLookup{room: &roomstore.Room{
		ID: "room-real-rebuy", CurrencyMode: "real", Tier: "micro", BigBlind: 20, BuyInMin: 40, BuyInMax: 400, MaxSeats: 9,
		EntryFeeCents: 100,
	}}
	svc := NewServiceWithGame(sandbox, game, mgr, rooms, &fakeActivation{activated: map[string]bool{"user-1": true}}).
		WithPendingStore(testPendingStore(t)).WithEntitlements(testEntitlementStore(t))
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
	if len(game.feeDebits) != 1 {
		t.Fatalf("expected the fee to be charged exactly once within the reservation window, got %+v", game.feeDebits)
	}
}

// TestBuyInChargesFeeAgainForADifferentTable is the multi-table counterpart:
// an entitlement only covers the table it was bound to (or its rebind
// target), so a second, still-live table of the same tier is a second
// reservation.
func TestBuyInChargesFeeAgainForADifferentTable(t *testing.T) {
	sandbox := &fakeWallet{}
	game := &fakeWallet{}
	mgr := testManager(t)
	rooms := multiRoomLookup{
		"room-a": {ID: "room-a", CurrencyMode: "real", Tier: "micro", BigBlind: 20, BuyInMin: 40, BuyInMax: 400, MaxSeats: 9, EntryFeeCents: 100},
		"room-b": {ID: "room-b", CurrencyMode: "real", Tier: "micro", BigBlind: 20, BuyInMin: 40, BuyInMax: 400, MaxSeats: 9, EntryFeeCents: 100},
	}
	svc := NewServiceWithGame(sandbox, game, mgr, rooms, &fakeActivation{activated: map[string]bool{"user-1": true}}).
		WithPendingStore(testPendingStore(t)).WithEntitlements(testEntitlementStore(t))
	ctx := context.Background()

	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	if _, err := mgr.GetOrCreateActor(ctx, "room-a", seed); err != nil {
		t.Fatalf("acquire room-a: %v", err)
	}
	if _, err := mgr.GetOrCreateActor(ctx, "room-b", seed); err != nil {
		t.Fatalf("acquire room-b: %v", err)
	}

	if err := svc.BuyIn(ctx, "room-a", "user-1", 400, false, "nonce-a"); err != nil {
		t.Fatalf("buyin room-a: %v", err)
	}
	if err := svc.BuyIn(ctx, "room-b", "user-1", 400, false, "nonce-b"); err != nil {
		t.Fatalf("buyin room-b: %v", err)
	}
	if len(game.feeDebits) != 2 {
		t.Fatalf("expected a separate fee for a second still-live table, got %+v", game.feeDebits)
	}
}

// TestBuyInRebindsEntitlementWhenBoundTableIsFull covers the second half of
// "O rebind": the originally-paid table is still live but full, so the
// reservation moves to a same-tier table with room instead of charging again.
func TestBuyInRebindsEntitlementWhenBoundTableIsFull(t *testing.T) {
	sandbox := &fakeWallet{}
	game := &fakeWallet{}
	mgr := testManager(t)
	rooms := multiRoomLookup{
		"room-full": {ID: "room-full", CurrencyMode: "real", Tier: "micro", BigBlind: 20, BuyInMin: 40, BuyInMax: 400, MaxSeats: 1, EntryFeeCents: 100},
		"room-open": {ID: "room-open", CurrencyMode: "real", Tier: "micro", BigBlind: 20, BuyInMin: 40, BuyInMax: 400, MaxSeats: 9, EntryFeeCents: 100},
	}
	svc := NewServiceWithGame(sandbox, game, mgr, rooms, &fakeActivation{activated: map[string]bool{"user-1": true, "user-2": true}}).
		WithPendingStore(testPendingStore(t)).WithEntitlements(testEntitlementStore(t))
	ctx := context.Background()

	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	if _, err := mgr.GetOrCreateActor(ctx, "room-full", seed); err != nil {
		t.Fatalf("acquire room-full: %v", err)
	}
	if _, err := mgr.GetOrCreateActor(ctx, "room-open", seed); err != nil {
		t.Fatalf("acquire room-open: %v", err)
	}

	// user-1 pays for and occupies the only seat at room-full.
	if err := svc.BuyIn(ctx, "room-full", "user-1", 400, false, "nonce-1"); err != nil {
		t.Fatalf("buyin room-full: %v", err)
	}
	// user-2 tries to buy into room-full (now full — this call fast-fails
	// before charging), then, holding no entitlement anywhere, buys into
	// room-open fresh. Neither of these is the rebind under test; the rebind
	// happens for user-1 below once room-full is genuinely occupied by
	// someone else's stack. To isolate the rebind, user-1 leaves and a
	// different player fills room-full's one seat, so room-full is
	// unavailable to user-1 (full) without user-1 itself vacating it.
	if _, err := svc.CashOut(ctx, "room-full", "user-1", ""); err != nil {
		t.Fatalf("cashout user-1: %v", err)
	}
	if err := svc.BuyIn(ctx, "room-full", "user-2", 400, false, "nonce-2"); err != nil {
		t.Fatalf("buyin room-full user-2: %v", err)
	}

	// user-1 still holds a valid (unexpired) entitlement bound to room-full,
	// which is now full with user-2 in the one seat. Buying into room-open
	// (same tier) must rebind for free instead of charging again.
	if err := svc.BuyIn(ctx, "room-open", "user-1", 400, false, "nonce-3"); err != nil {
		t.Fatalf("buyin room-open (rebind): %v", err)
	}
	if len(game.feeDebits) != 2 {
		t.Fatalf("expected exactly 2 fee debits (user-1's original + user-2's), rebind must be free, got %+v", game.feeDebits)
	}
}

// TestBuyInRebindsEntitlementWhenBoundTableIsArchived covers the first half
// of "O rebind": the table cmd/tablecleanup archived (and deleted the room
// row for) is unavailable regardless of how many seats it had.
func TestBuyInRebindsEntitlementWhenBoundTableIsArchived(t *testing.T) {
	sandbox := &fakeWallet{}
	game := &fakeWallet{}
	mgr, store := testManagerAndStore(t)
	rooms := multiRoomLookup{
		"room-live": {ID: "room-live", CurrencyMode: "real", Tier: "micro", BigBlind: 20, BuyInMin: 40, BuyInMax: 400, MaxSeats: 9, EntryFeeCents: 100},
		// room-gone is intentionally absent from rooms: cmd/tablecleanup
		// deletes the room row only after confirming the table archived.
	}
	svc := NewServiceWithGame(sandbox, game, mgr, rooms, &fakeActivation{activated: map[string]bool{"user-1": true}}).
		WithPendingStore(testPendingStore(t)).WithEntitlements(testEntitlementStore(t))
	ctx := context.Background()

	// Seed and claim an entitlement bound to room-gone, then archive it —
	// mirroring cmd/tablecleanup's own sequence (archive the table, then
	// delete the room row) without going through the real sandbox-only sweep.
	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	if _, err := mgr.GetOrCreateActor(ctx, "room-gone", seed); err != nil {
		t.Fatalf("seed room-gone: %v", err)
	}
	claimed, err := svc.entitlements.Claim(ctx, entitlement.Entitlement{
		PlayerID: "user-1", OriginTableID: "room-gone", BoundTableID: "room-gone",
		Tier: "micro", FeeCents: 100, ExpiresAt: time.Now().Add(entitlement.Window),
	})
	if err != nil {
		t.Fatalf("seed entitlement: %v", err)
	}
	// confirmFeeCharged (buyin.Service.resolveEntitlement's rebind path) only
	// treats a matching entitlement as fee-covered once its own recovery row
	// reports resolved — a real successful buy-in always leaves one via
	// chargeEntryFee's Record+MarkResolved. Seed it here too, since this test
	// claims the entitlement directly rather than through a full BuyIn.
	feeKey := fmt.Sprintf("room-gone#user-1#buyinfee#%d", claimed.CreatedAt.Unix())
	if err := svc.pending.Record(ctx, reconcile.PendingCashout{
		ID: feeKey, PlayerID: "user-1", Amount: 100, CurrencyMode: roomstore.CurrencyModeReal,
		Kind: reconcile.KindFeeDebit, TableRef: "room-gone", IdempotencyKey: feeKey,
	}); err != nil {
		t.Fatalf("seed fee recovery row: %v", err)
	}
	if err := svc.pending.MarkResolved(ctx, feeKey); err != nil {
		t.Fatalf("resolve fee recovery row: %v", err)
	}
	stored, err := store.LoadTable(ctx, "room-gone")
	if err != nil || stored == nil {
		t.Fatalf("load room-gone: %v", err)
	}
	if err := store.MarkArchived(ctx, "room-gone", stored.Version); err != nil {
		t.Fatalf("archive room-gone: %v", err)
	}

	if _, err := mgr.GetOrCreateActor(ctx, "room-live", seed); err != nil {
		t.Fatalf("acquire room-live: %v", err)
	}
	if err := svc.BuyIn(ctx, "room-live", "user-1", 400, false, "nonce-1"); err != nil {
		t.Fatalf("buyin room-live (rebind): %v", err)
	}
	if len(game.feeDebits) != 0 {
		t.Fatalf("expected the archived-table rebind to be free, got %+v", game.feeDebits)
	}
}

// TestBuyInChargesAgainAfterEntitlementWindowExpires guards the "janela é
// absoluta" rule: an entitlement past its ExpiresAt is invisible to
// resolveEntitlement (ActiveFor filters it out), even at the exact table it
// was bound to, and gets charged again rather than silently renewed.
func TestBuyInChargesAgainAfterEntitlementWindowExpires(t *testing.T) {
	sandbox := &fakeWallet{}
	game := &fakeWallet{}
	mgr := testManager(t)
	rooms := &fakeRoomLookup{room: &roomstore.Room{
		ID: "room-real-expiry", CurrencyMode: "real", Tier: "micro", BigBlind: 20, BuyInMin: 40, BuyInMax: 400, MaxSeats: 9,
		EntryFeeCents: 100,
	}}
	ents := testEntitlementStore(t)
	svc := NewServiceWithGame(sandbox, game, mgr, rooms, &fakeActivation{activated: map[string]bool{"user-1": true}}).
		WithPendingStore(testPendingStore(t)).WithEntitlements(ents)
	ctx := context.Background()

	// Seed an already-expired entitlement directly, bypassing BuyIn's normal
	// now()+Window — nothing in resolveEntitlement ever extends ExpiresAt on
	// later activity, so this is equivalent to (and faster than) actually
	// waiting out the real 3-hour window.
	if _, err := ents.Claim(ctx, entitlement.Entitlement{
		PlayerID: "user-1", OriginTableID: "room-real-expiry", BoundTableID: "room-real-expiry",
		Tier: "micro", FeeCents: 100, ExpiresAt: time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("seed expired entitlement: %v", err)
	}

	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	if _, err := mgr.GetOrCreateActor(ctx, "room-real-expiry", seed); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := svc.BuyIn(ctx, "room-real-expiry", "user-1", 400, false, "nonce-1"); err != nil {
		t.Fatalf("buyin: %v", err)
	}
	if len(game.feeDebits) != 1 {
		t.Fatalf("expected the fee to be charged since the seeded entitlement had expired, got %+v", game.feeDebits)
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
