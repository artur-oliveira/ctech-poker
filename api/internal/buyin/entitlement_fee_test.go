package buyin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"gopkg.aoctech.app/poker/api/internal/entitlement"
	"gopkg.aoctech.app/poker/api/internal/reconcile"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

// feeWallet records every real-money entry-fee debit and can be made to fail
// them, so a test can reproduce "the reservation exists but its charge never
// landed" — the state that used to hand out a free seat (issue #40).
type feeWallet struct {
	debits   []string // idempotency keys, in order
	failNext bool
}

func (w *feeWallet) Credit(context.Context, string, int64, string, string) error { return nil }
func (w *feeWallet) Debit(context.Context, string, int64, string, string) error  { return nil }
func (w *feeWallet) HoldGame(context.Context, string, int64, string, string, string) (string, error) {
	return "h1", nil
}
func (w *feeWallet) ReleaseHold(context.Context, string) error { return nil }
func (w *feeWallet) CashoutGame(context.Context, string, int64, string, []string, string, string) error {
	return nil
}
func (w *feeWallet) DebitReal(_ context.Context, _ string, _ int64, key, _ string) error {
	if w.failNext {
		w.failNext = false
		return errors.New("wallet unavailable")
	}
	// ctech-wallet dedupes on the idempotency key; mirror that so a repeated
	// key is recorded once and a test can assert "charged at most once".
	for _, seen := range w.debits {
		if seen == key {
			return nil
		}
	}
	w.debits = append(w.debits, key)
	return nil
}
func (w *feeWallet) Balances(context.Context, string) (*walletclient.Balances, error) {
	return &walletclient.Balances{}, nil
}

// memPending is reconcile.PendingStore's in-memory stand-in: Record is
// immutable-first-write and MarkResolved is monotonic, like the real one.
type memPending struct{ rows map[string]*reconcile.PendingCashout }

func newMemPending() *memPending { return &memPending{rows: map[string]*reconcile.PendingCashout{}} }

func (m *memPending) BuildRecordTx(reconcile.PendingCashout) (types.TransactWriteItem, error) {
	return types.TransactWriteItem{}, nil
}
func (m *memPending) Record(_ context.Context, p reconcile.PendingCashout) error {
	if _, ok := m.rows[p.ID]; ok {
		return nil
	}
	row := p
	m.rows[p.ID] = &row
	return nil
}
func (m *memPending) Get(_ context.Context, id string) (*reconcile.PendingCashout, error) {
	row, ok := m.rows[id]
	if !ok {
		return nil, nil
	}
	copied := *row
	return &copied, nil
}
func (m *memPending) MarkResolved(_ context.Context, id string) error {
	if row, ok := m.rows[id]; ok {
		row.Resolved = true
	}
	return nil
}

// memEntitlements mirrors entitlement.Store's Claim condition ("absent, or
// present but expired") and its Rebind compare-and-swap.
type memEntitlements struct {
	rows map[string]entitlement.Entitlement // keyed playerID + "#" + originTableID
	now  func() time.Time
}

func newMemEntitlements() *memEntitlements {
	return &memEntitlements{rows: map[string]entitlement.Entitlement{}, now: time.Now}
}

func (m *memEntitlements) key(playerID, originTableID string) string {
	return playerID + "#" + originTableID
}

func (m *memEntitlements) ActiveFor(_ context.Context, playerID string) ([]entitlement.Entitlement, error) {
	out := []entitlement.Entitlement{}
	for _, e := range m.rows {
		if e.PlayerID == playerID && e.ExpiresAt.After(m.now()) {
			out = append(out, e)
		}
	}
	return out, nil
}

func (m *memEntitlements) Claim(_ context.Context, e entitlement.Entitlement) error {
	if existing, ok := m.rows[m.key(e.PlayerID, e.OriginTableID)]; ok && existing.ExpiresAt.After(m.now()) {
		return entitlement.ErrAlreadyClaimed
	}
	m.rows[m.key(e.PlayerID, e.OriginTableID)] = e
	return nil
}

func (m *memEntitlements) Rebind(_ context.Context, playerID, originTableID, expectedBoundTableID, newTableID string) error {
	e, ok := m.rows[m.key(playerID, originTableID)]
	if !ok || !e.ExpiresAt.After(m.now()) || e.BoundTableID != expectedBoundTableID {
		return entitlement.ErrNotFound
	}
	e.BoundTableID = newTableID
	m.rows[m.key(playerID, originTableID)] = e
	return nil
}

func feeService(wallet *feeWallet, pending *memPending, ents *memEntitlements, rooms roomLookup) *Service {
	return NewServiceWithGame(wallet, wallet, nil, rooms, nil).
		WithPendingStore(pending).WithEntitlements(ents)
}

func realRoom(id string, maxSeats, seatsTaken int) *roomstore.Room {
	return &roomstore.Room{
		ID: id, CurrencyMode: roomstore.CurrencyModeReal, Tier: "micro", BigBlind: 20,
		BuyInMin: 40, BuyInMax: 400, MaxSeats: maxSeats, SeatsTaken: seatsTaken, EntryFeeCents: 100,
	}
}

// TestUnpaidReservationIsChargedNotTreatedAsCovered is the regression test for
// issue #40's free-seat window: a claim commits before its debit, so an
// entitlement row whose DebitReal never cleared must NOT admit the player for
// free — the charge is completed first, under the reservation's own key.
func TestUnpaidReservationIsChargedNotTreatedAsCovered(t *testing.T) {
	wallet, pending, ents := &feeWallet{}, newMemPending(), newMemEntitlements()
	room := realRoom("room-a", 9, 0)
	svc := feeService(wallet, pending, ents, &gateRoomLookup{room: room})
	ctx := context.Background()

	// The claim race winner got as far as writing the reservation, then its
	// wallet call failed: reservation present, nothing paid.
	wallet.failNext = true
	if err := svc.resolveEntitlement(ctx, room, "u1"); err == nil {
		t.Fatal("expected the failed fee debit to abort the buy-in, got nil")
	}
	if len(wallet.debits) != 0 {
		t.Fatalf("no debit should have been recorded, got %v", wallet.debits)
	}
	if len(ents.rows) != 1 {
		t.Fatalf("the reservation must be left in place for the idempotent retry, got %d rows", len(ents.rows))
	}

	// A second buy-in attempt sees that reservation. It must charge it rather
	// than read "a row exists" as "the fee is covered".
	if err := svc.resolveEntitlement(ctx, room, "u1"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if len(wallet.debits) != 1 {
		t.Fatalf("expected exactly one fee debit after the retry, got %v", wallet.debits)
	}

	// And once it is genuinely paid, rebuys inside the window are free again.
	if err := svc.resolveEntitlement(ctx, room, "u1"); err != nil {
		t.Fatalf("rebuy: %v", err)
	}
	if len(wallet.debits) != 1 {
		t.Fatalf("rebuy within the window must be free, got %v", wallet.debits)
	}
}

// TestFeeKeyIsStableAcrossRequestsWithinTheSameReservation guards the property
// the retry above depends on: the fee's idempotency key comes from the
// reservation (table + player + window), never from a per-request nonce, so
// two different requests recovering the same unpaid reservation reproduce one
// key and ctech-wallet charges once.
func TestFeeKeyIsStableAcrossRequestsWithinTheSameReservation(t *testing.T) {
	e := entitlement.Entitlement{
		PlayerID: "u1", OriginTableID: "room-a", BoundTableID: "room-a", Tier: "micro",
		FeeCents: 100, ExpiresAt: time.Unix(1800000000, 0),
	}
	first := entitlementFeeKey("u1", e)
	rebound := e
	rebound.BoundTableID = "room-b" // a rebind must not re-key the paid fee
	if second := entitlementFeeKey("u1", rebound); first != second {
		t.Fatalf("rebind changed the fee key: %q vs %q", first, second)
	}
	renewed := e
	renewed.ExpiresAt = e.ExpiresAt.Add(entitlement.Window) // a genuinely new window is a new charge
	if third := entitlementFeeKey("u1", renewed); first == third {
		t.Fatalf("a fresh reservation window reused the previous fee key %q", first)
	}
}

// TestClaimRaceLoserConfirmsTheWinnersCharge covers the loser's branch
// directly: ErrAlreadyClaimed must not short-circuit to "covered".
func TestClaimRaceLoserConfirmsTheWinnersCharge(t *testing.T) {
	wallet, pending, ents := &feeWallet{}, newMemPending(), newMemEntitlements()
	room := realRoom("room-a", 9, 0)
	svc := feeService(wallet, pending, ents, &gateRoomLookup{room: room})
	ctx := context.Background()

	// Winner's reservation, written directly: nothing charged yet.
	if err := ents.Claim(ctx, entitlement.Entitlement{
		PlayerID: "u1", OriginTableID: room.ID, BoundTableID: room.ID, Tier: room.Tier,
		FeeCents: room.EntryFeeCents, ExpiresAt: time.Now().Add(entitlement.Window),
	}); err != nil {
		t.Fatalf("seed claim: %v", err)
	}

	if err := svc.chargeEntryFee(ctx, room, "u1"); err != nil {
		t.Fatalf("claim-race loser: %v", err)
	}
	if len(wallet.debits) != 1 {
		t.Fatalf("the loser must complete the unpaid charge exactly once, got %v", wallet.debits)
	}
	if row, _ := pending.Get(ctx, wallet.debits[0]); row == nil || !row.Resolved {
		t.Fatalf("the fee recovery row must end up resolved, got %+v", row)
	}
}

// TestFeeStatusReportsDueWhileTheChargeIsUnsettled: GET /rooms/:id/seated
// exists so a player is never surprised by a charge at buy-in, so an unpaid
// reservation must read as fee_due.
func TestFeeStatusReportsDueWhileTheChargeIsUnsettled(t *testing.T) {
	wallet, pending, ents := &feeWallet{}, newMemPending(), newMemEntitlements()
	room := realRoom("room-a", 9, 0)
	svc := feeService(wallet, pending, ents, &gateRoomLookup{room: room})
	ctx := context.Background()

	wallet.failNext = true
	if err := svc.resolveEntitlement(ctx, room, "u1"); err == nil {
		t.Fatal("expected the seeded debit failure to abort")
	}
	if due, _, err := svc.FeeStatus(ctx, room, "u1"); err != nil || !due {
		t.Fatalf("unpaid reservation must read as fee_due: due=%v err=%v", due, err)
	}
	if err := svc.resolveEntitlement(ctx, room, "u1"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if due, expires, err := svc.FeeStatus(ctx, room, "u1"); err != nil || due || expires == 0 {
		t.Fatalf("paid reservation must read as covered: due=%v expires=%d err=%v", due, expires, err)
	}
}

// TestTableUnavailableReadsSeatsTakenWithoutAnActor is issue #57: the rebind
// availability check must decide from the room's write-through seat count.
// svc.manager is nil here, so any attempt to materialise an actor panics —
// which is exactly the assertion.
func TestTableUnavailableReadsSeatsTakenWithoutAnActor(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		room *roomstore.Room
		want bool
	}{
		{"full", realRoom("room-a", 2, 2), true},
		{"over-full", realRoom("room-a", 2, 3), true},
		{"has room", realRoom("room-a", 2, 1), false},
		{"archived and room row deleted", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := feeService(&feeWallet{}, newMemPending(), newMemEntitlements(), &gateRoomLookup{room: tc.room})
			got, err := svc.tableUnavailable(ctx, "room-a")
			if err != nil {
				t.Fatalf("tableUnavailable: %v", err)
			}
			if got != tc.want {
				t.Fatalf("tableUnavailable = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestRebindOnlyCoversASettledFee: rebinding an unpaid reservation to another
// table must not carry a free seat with it.
func TestRebindOnlyCoversASettledFee(t *testing.T) {
	wallet, pending, ents := &feeWallet{}, newMemPending(), newMemEntitlements()
	roomA, roomB := realRoom("room-a", 1, 1), realRoom("room-b", 9, 0)
	rooms := multiRoom{"room-a": roomA, "room-b": roomB}
	svc := feeService(wallet, pending, ents, rooms)
	ctx := context.Background()

	// An unpaid reservation bound to the now-full room-a.
	if err := ents.Claim(ctx, entitlement.Entitlement{
		PlayerID: "u1", OriginTableID: "room-a", BoundTableID: "room-a", Tier: "micro",
		FeeCents: 100, ExpiresAt: time.Now().Add(entitlement.Window),
	}); err != nil {
		t.Fatalf("seed claim: %v", err)
	}

	if err := svc.resolveEntitlement(ctx, roomB, "u1"); err != nil {
		t.Fatalf("rebind: %v", err)
	}
	if len(wallet.debits) != 1 {
		t.Fatalf("the rebound reservation's unpaid fee must be charged once, got %v", wallet.debits)
	}
	if ents.rows[ents.key("u1", "room-a")].BoundTableID != "room-b" {
		t.Fatal("expected the reservation to be rebound to room-b")
	}
	// The now-paid reservation makes the second buy-in at room-b free.
	if err := svc.resolveEntitlement(ctx, roomB, "u1"); err != nil {
		t.Fatalf("rebuy: %v", err)
	}
	if len(wallet.debits) != 1 {
		t.Fatalf("rebuy at the rebound table must be free, got %v", wallet.debits)
	}
}

type multiRoom map[string]*roomstore.Room

func (m multiRoom) Get(_ context.Context, roomID string) (*roomstore.Room, error) {
	return m[roomID], nil
}
