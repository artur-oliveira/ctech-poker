package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/reconcile"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

type fakeStaleQuerier struct {
	stale       []tablestore.StoredTable
	archived    []string
	archiveErrs map[string]error
}

func (f *fakeStaleQuerier) QueryStaleActive(context.Context, int64, int) ([]tablestore.StoredTable, error) {
	return f.stale, nil
}
func (f *fakeStaleQuerier) MarkArchived(_ context.Context, tableID string, _ int) error {
	if err := f.archiveErrs[tableID]; err != nil {
		return err
	}
	f.archived = append(f.archived, tableID)
	return nil
}

type fakeRoomLookup struct {
	rooms   map[string]*roomstore.Room
	deleted []string
}

func (f *fakeRoomLookup) Get(_ context.Context, roomID string) (*roomstore.Room, error) {
	return f.rooms[roomID], nil
}

func (f *fakeRoomLookup) Delete(_ context.Context, roomID string) error {
	f.deleted = append(f.deleted, roomID)
	return nil
}

type fakeSandboxCredit struct {
	credits []struct {
		userID string
		amount int64
	}
}

func (f *fakeSandboxCredit) Credit(_ context.Context, userID string, amount int64, _, _ string) error {
	f.credits = append(f.credits, struct {
		userID string
		amount int64
	}{userID, amount})
	return nil
}

type fakeGameCashout struct {
	cashouts []struct {
		userID   string
		amount   int64
		tableRef string
		holdIDs  []string
	}
	failFor map[string]error
}

func (f *fakeGameCashout) CashoutGame(_ context.Context, userID string, amount int64, tableRef string, holdIDs []string, _, _ string) error {
	if f.failFor != nil {
		if err := f.failFor[userID]; err != nil {
			return err
		}
	}
	f.cashouts = append(f.cashouts, struct {
		userID   string
		amount   int64
		tableRef string
		holdIDs  []string
	}{userID, amount, tableRef, holdIDs})
	return nil
}

type fakePendingRecorder struct {
	recorded  []reconcile.PendingCashout
	resolved  []string
	recordErr error
}

func (f *fakePendingRecorder) Record(_ context.Context, p reconcile.PendingCashout) error {
	if f.recordErr != nil {
		return f.recordErr
	}
	f.recorded = append(f.recorded, p)
	return nil
}

func (f *fakePendingRecorder) MarkResolved(_ context.Context, id string) error {
	f.resolved = append(f.resolved, id)
	return nil
}

func TestRunRefundsSeatedSandboxPlayersAndArchives(t *testing.T) {
	stale := &fakeStaleQuerier{stale: []tablestore.StoredTable{
		{
			TableID: "table-1", Version: 3,
			State: hand.State{Players: []*hand.Player{{ID: "player-1", Stack: 500}}},
		},
	}}
	rooms := &fakeRoomLookup{rooms: map[string]*roomstore.Room{
		"table-1": {ID: "table-1", CurrencyMode: "sandbox"},
	}}
	wallet := &fakeSandboxCredit{}
	game := &fakeGameCashout{}
	pending := &fakePendingRecorder{}

	if err := run(context.Background(), stale, rooms, wallet, game, pending, time.Hour); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(wallet.credits) != 1 || wallet.credits[0].userID != "player-1" || wallet.credits[0].amount != 500 {
		t.Fatalf("expected a 500-chip refund to player-1, got %+v", wallet.credits)
	}
	if len(stale.archived) != 1 || stale.archived[0] != "table-1" {
		t.Fatalf("expected table-1 to be archived, got %v", stale.archived)
	}
	if len(rooms.deleted) != 1 || rooms.deleted[0] != "table-1" {
		t.Fatalf("expected table-1's room to be deleted, got %v", rooms.deleted)
	}
	if len(game.cashouts) != 0 {
		t.Fatalf("sandbox table must never call the real-money game wallet, got %+v", game.cashouts)
	}
}

func TestRunSettlesRealMoneyPlayersAndArchives(t *testing.T) {
	stale := &fakeStaleQuerier{stale: []tablestore.StoredTable{
		{
			TableID: "table-2", Version: 1,
			State: hand.State{Players: []*hand.Player{{ID: "player-2", Stack: 500, HoldID: "hold-abc"}}},
		},
	}}
	rooms := &fakeRoomLookup{rooms: map[string]*roomstore.Room{
		"table-2": {ID: "table-2", CurrencyMode: "real"},
	}}
	wallet := &fakeSandboxCredit{}
	game := &fakeGameCashout{}
	pending := &fakePendingRecorder{}

	if err := run(context.Background(), stale, rooms, wallet, game, pending, time.Hour); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(wallet.credits) != 0 {
		t.Fatalf("real-money table must never credit the sandbox ledger, got %+v", wallet.credits)
	}
	if len(pending.recorded) != 1 || pending.recorded[0].PlayerID != "player-2" || pending.recorded[0].Amount != 500 ||
		pending.recorded[0].CurrencyMode != "real" || len(pending.recorded[0].HoldIDs) != 1 || pending.recorded[0].HoldIDs[0] != "hold-abc" {
		t.Fatalf("expected a durable real-money recovery row for player-2's hold, got %+v", pending.recorded)
	}
	if len(game.cashouts) != 1 || game.cashouts[0].userID != "player-2" || game.cashouts[0].amount != 500 || game.cashouts[0].holdIDs[0] != "hold-abc" {
		t.Fatalf("expected the game-wallet hold to be cashed out, got %+v", game.cashouts)
	}
	if len(pending.resolved) != 1 {
		t.Fatalf("expected the recovery row to be marked resolved after a successful cash-out, got %v", pending.resolved)
	}
	if len(stale.archived) != 1 || stale.archived[0] != "table-2" {
		t.Fatalf("expected the real-money table to still be archived once holds are settled, got %v", stale.archived)
	}
	if len(rooms.deleted) != 1 {
		t.Fatalf("expected table-2's room to be deleted, got %v", rooms.deleted)
	}
}

func TestRunArchivesRealMoneyTableEvenWhenImmediateCashoutFails(t *testing.T) {
	stale := &fakeStaleQuerier{stale: []tablestore.StoredTable{
		{
			TableID: "table-3", Version: 1,
			State: hand.State{Players: []*hand.Player{{ID: "player-3", Stack: 200, HoldID: "hold-xyz"}}},
		},
	}}
	rooms := &fakeRoomLookup{rooms: map[string]*roomstore.Room{
		"table-3": {ID: "table-3", CurrencyMode: "real"},
	}}
	wallet := &fakeSandboxCredit{}
	game := &fakeGameCashout{failFor: map[string]error{"player-3": errors.New("wallet unavailable")}}
	pending := &fakePendingRecorder{}

	if err := run(context.Background(), stale, rooms, wallet, game, pending, time.Hour); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(pending.recorded) != 1 {
		t.Fatalf("expected the recovery row to be recorded before the cash-out attempt, got %+v", pending.recorded)
	}
	if len(pending.resolved) != 0 {
		t.Fatalf("must not mark the recovery row resolved when the immediate cash-out failed, got %v", pending.resolved)
	}
	// The obligation is durable (recorded) even though the immediate
	// cash-out failed, so cmd/reconcile's sweep can retry it — the table is
	// safe to archive right away instead of being left active forever.
	if len(stale.archived) != 1 {
		t.Fatalf("expected the table to still be archived once the recovery row is durably recorded, got %v", stale.archived)
	}
}

func TestRunLeavesRealMoneyTableActiveWhenRecordFails(t *testing.T) {
	stale := &fakeStaleQuerier{stale: []tablestore.StoredTable{
		{
			TableID: "table-4", Version: 1,
			State: hand.State{Players: []*hand.Player{{ID: "player-4", Stack: 200, HoldID: "hold-x"}}},
		},
	}}
	rooms := &fakeRoomLookup{rooms: map[string]*roomstore.Room{
		"table-4": {ID: "table-4", CurrencyMode: "real"},
	}}
	wallet := &fakeSandboxCredit{}
	game := &fakeGameCashout{}
	pending := &fakePendingRecorder{recordErr: errors.New("dynamo unavailable")}

	if err := run(context.Background(), stale, rooms, wallet, game, pending, time.Hour); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(game.cashouts) != 0 {
		t.Fatalf("must never attempt a cash-out without a durable recovery row first, got %+v", game.cashouts)
	}
	if len(stale.archived) != 0 {
		t.Fatalf("expected the table to stay active for retry when the recovery row can't be recorded, got %v", stale.archived)
	}
}

func TestRunSkipsTablesWithNoRoomRecord(t *testing.T) {
	stale := &fakeStaleQuerier{stale: []tablestore.StoredTable{
		{
			TableID: "table-5", Version: 1,
			State: hand.State{Players: []*hand.Player{{ID: "player-5", Stack: 500}}},
		},
	}}
	rooms := &fakeRoomLookup{rooms: map[string]*roomstore.Room{}}
	wallet := &fakeSandboxCredit{}
	game := &fakeGameCashout{}
	pending := &fakePendingRecorder{}

	if err := run(context.Background(), stale, rooms, wallet, game, pending, time.Hour); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(wallet.credits) != 0 {
		t.Fatalf("a missing room record must never be treated as sandbox, got %+v", wallet.credits)
	}
	if len(game.cashouts) != 0 {
		t.Fatalf("a missing room record must never be treated as real-money either, got %+v", game.cashouts)
	}
	if len(stale.archived) != 0 {
		t.Fatalf("a table with an unknown currency mode must not be archived, got %v", stale.archived)
	}
}

func TestRunSkipsEmptyTablesWithNoRefundNeeded(t *testing.T) {
	stale := &fakeStaleQuerier{stale: []tablestore.StoredTable{
		{TableID: "table-6", Version: 1, State: hand.State{Players: nil}},
	}}
	rooms := &fakeRoomLookup{rooms: map[string]*roomstore.Room{
		"table-6": {ID: "table-6", CurrencyMode: "sandbox"},
	}}
	wallet := &fakeSandboxCredit{}
	game := &fakeGameCashout{}
	pending := &fakePendingRecorder{}

	if err := run(context.Background(), stale, rooms, wallet, game, pending, time.Hour); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(wallet.credits) != 0 {
		t.Fatalf("a table with no chips at stake needs no refund, got %+v", wallet.credits)
	}
	if len(stale.archived) != 1 {
		t.Fatalf("expected the empty stale table to still be archived, got %v", stale.archived)
	}
}
