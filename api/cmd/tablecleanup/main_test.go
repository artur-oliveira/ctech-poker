package main

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

type fakeStaleQuerier struct {
	stale    []tablestore.StoredTable
	archived []string
}

func (f *fakeStaleQuerier) QueryStaleActive(context.Context, int64, int) ([]tablestore.StoredTable, error) {
	return f.stale, nil
}
func (f *fakeStaleQuerier) MarkArchived(_ context.Context, tableID string, _ int) error {
	f.archived = append(f.archived, tableID)
	return nil
}

type fakeRoomLookup struct {
	rooms map[string]*roomstore.Room
}

func (f *fakeRoomLookup) Get(_ context.Context, roomID string) (*roomstore.Room, error) {
	return f.rooms[roomID], nil
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

	if err := run(context.Background(), stale, rooms, wallet, time.Hour); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(wallet.credits) != 1 || wallet.credits[0].userID != "player-1" || wallet.credits[0].amount != 500 {
		t.Fatalf("expected a 500-chip refund to player-1, got %+v", wallet.credits)
	}
	if len(stale.archived) != 1 || stale.archived[0] != "table-1" {
		t.Fatalf("expected table-1 to be archived, got %v", stale.archived)
	}
}

func TestRunSkipsRealMoneyTables(t *testing.T) {
	stale := &fakeStaleQuerier{stale: []tablestore.StoredTable{
		{
			TableID: "table-2", Version: 1,
			State: hand.State{Players: []*hand.Player{{ID: "player-2", Stack: 500}}},
		},
	}}
	rooms := &fakeRoomLookup{rooms: map[string]*roomstore.Room{
		"table-2": {ID: "table-2", CurrencyMode: "real"},
	}}
	wallet := &fakeSandboxCredit{}

	if err := run(context.Background(), stale, rooms, wallet, time.Hour); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(wallet.credits) != 0 {
		t.Fatalf("real-money tables must never be refunded/archived by this sandbox-only job, got %+v", wallet.credits)
	}
	if len(stale.archived) != 0 {
		t.Fatalf("real-money tables must not be archived by this job, got %v", stale.archived)
	}
}

func TestRunSkipsEmptyTablesWithNoRefundNeeded(t *testing.T) {
	stale := &fakeStaleQuerier{stale: []tablestore.StoredTable{
		{TableID: "table-3", Version: 1, State: hand.State{Players: nil}},
	}}
	rooms := &fakeRoomLookup{rooms: map[string]*roomstore.Room{
		"table-3": {ID: "table-3", CurrencyMode: "sandbox"},
	}}
	wallet := &fakeSandboxCredit{}

	if err := run(context.Background(), stale, rooms, wallet, time.Hour); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(wallet.credits) != 0 {
		t.Fatalf("a table with no chips at stake needs no refund, got %+v", wallet.credits)
	}
	if len(stale.archived) != 1 {
		t.Fatalf("expected the empty stale table to still be archived, got %v", stale.archived)
	}
}
