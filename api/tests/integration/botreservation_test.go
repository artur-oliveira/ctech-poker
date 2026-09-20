//go:build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

func TestConcurrentBotReservationsAcrossServersChooseOneHuman(t *testing.T) {
	ctx := context.Background()
	db := testDynamoClient(t)
	env := "flow_test"
	mustCreatePokerTables(t, db, env)
	store := tablestore.NewStore(db, env)
	tableID := uniqueTableID(t)
	// Deliberately do not share a lease cache: leases may reduce conflicts,
	// but only DynamoDB's version guard may decide who owns the seat.
	mgrA := tablemanager.NewManager(tablelease.NewService(cache.NewMemoryBackend(16)), store, nil, nil)
	mgrB := tablemanager.NewManager(tablelease.NewService(cache.NewMemoryBackend(16)), store, nil, nil)
	seed := func() *hand.Table {
		game := hand.NewTable([]*hand.Player{{ID: "owner", Stack: 5_000, Ready: true}}, 25, 50)
		game.ConfigureRake("sandbox")
		if err := game.ConfigureBotsForActor("owner", 4_000, 2, time.Now().UnixMilli()); err != nil {
			t.Fatal(err)
		}
		if err := game.AddBotForActor("bot:owner:0", "Lia", "tag"); err != nil {
			t.Fatal(err)
		}
		return game
	}
	actorA, err := mgrA.GetOrCreateActor(ctx, tableID, seed)
	if err != nil {
		t.Fatal(err)
	}
	actorB, err := mgrB.GetOrCreateActor(ctx, tableID, seed)
	if err != nil {
		t.Fatal(err)
	}
	actors := []*table.Actor{actorA, actorB}
	ids := []string{"reservation-a", "reservation-b"}
	players := []string{"human-a", "human-b"}
	start := make(chan struct{})
	var wg sync.WaitGroup
	var results [2]error
	for i := range actors {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			reservation := hand.BotReservation{ID: ids[i], PlayerID: players[i], Amount: 4_000,
				IdempotencyKey: "entry-" + players[i], ExpiresAtUnixMs: time.Now().Add(time.Minute).UnixMilli()}
			results[i] = actors[i].Dispatch(table.ReserveBotSeatCmd{
				Reservation: reservation, Result: make(chan hand.BotReservation, 1), Reply: make(chan error, 1),
			})
		}(i)
	}
	close(start)
	wg.Wait()

	winner := -1
	for i, err := range results {
		if err == nil {
			if winner >= 0 {
				t.Fatalf("both servers reserved the same bot seat: results=%v", results)
			}
			winner = i
		} else if !errors.Is(err, hand.ErrBotSeatReserved) {
			t.Fatalf("unexpected losing reservation error: %v", err)
		}
	}
	if winner < 0 {
		t.Fatalf("neither server reserved the bot seat: results=%v", results)
	}
	stored, err := store.LoadTable(ctx, tableID)
	if err != nil || stored == nil {
		t.Fatalf("load committed reservation: stored=%+v err=%v", stored, err)
	}
	reservation := stored.State.BotReservation
	if reservation == nil || reservation.ID != ids[winner] || reservation.PlayerID != players[winner] {
		t.Fatalf("persisted reservation differs from winning response: %+v winner=%d", reservation, winner)
	}
}

func TestBotReservationSurvivesReconnectOnAnotherServer(t *testing.T) {
	ctx := context.Background()
	db := testDynamoClient(t)
	env := "flow_test"
	mustCreatePokerTables(t, db, env)
	store := tablestore.NewStore(db, env)
	tableID := uniqueTableID(t)
	seed := func() *hand.Table {
		game := hand.NewTable([]*hand.Player{{ID: "owner", Stack: 5_000, Ready: true}}, 25, 50)
		game.ConfigureRake("sandbox")
		if err := game.ConfigureBotsForActor("owner", 4_000, 2, time.Now().UnixMilli()); err != nil {
			t.Fatal(err)
		}
		if err := game.AddBotForActor("bot:owner:0", "Lia", "tag"); err != nil {
			t.Fatal(err)
		}
		return game
	}
	mgrA := tablemanager.NewManager(tablelease.NewService(cache.NewMemoryBackend(16)), store, nil, nil)
	actorA, err := mgrA.GetOrCreateActor(ctx, tableID, seed)
	if err != nil {
		t.Fatal(err)
	}
	want := hand.BotReservation{ID: "reconnect-reservation", PlayerID: "visitor", Amount: 4_000,
		IdempotencyKey: "visitor-entry", ExpiresAtUnixMs: time.Now().Add(time.Minute).UnixMilli()}
	if err := actorA.Dispatch(table.ReserveBotSeatCmd{Reservation: want,
		Result: make(chan hand.BotReservation, 1), Reply: make(chan error, 1)}); err != nil {
		t.Fatal(err)
	}

	// A fresh manager has no in-memory actor or lease state. This is the path
	// used when a browser reconnects through another API instance.
	mgrB := tablemanager.NewManager(tablelease.NewService(cache.NewMemoryBackend(16)), store, nil, nil)
	actorB, err := mgrB.GetOrCreateActor(ctx, tableID, seed)
	if err != nil {
		t.Fatal(err)
	}
	matchCh := make(chan table.BotMatchStatus, 1)
	if err := actorB.Dispatch(table.BotMatchStatusCmd{Status: matchCh, Reply: make(chan error, 1)}); err != nil {
		t.Fatal(err)
	}
	match := <-matchCh
	if match.Reservation == nil || match.Reservation.ID != want.ID || match.BotPolicy.OwnerID != "owner" {
		t.Fatalf("reconnected actor lost bot policy or reservation: %+v", match)
	}
	statusCh := make(chan table.BotReservationStatus, 1)
	if err := actorB.Dispatch(table.BotReservationStatusCmd{PlayerID: "visitor", Status: statusCh, Reply: make(chan error, 1)}); err != nil {
		t.Fatal(err)
	}
	status := <-statusCh
	if status.Reservation == nil || status.Reservation.ID != want.ID || status.Reservation.Amount != want.Amount {
		t.Fatalf("reconnected visitor lost pending reservation: %+v", status)
	}
}
