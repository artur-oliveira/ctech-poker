package hand

import (
	"errors"
	"testing"
)

func TestBotPolicyPersistsAndTargetsEachFormat(t *testing.T) {
	for _, tc := range []struct{ seats, target int }{{2, 2}, {6, 4}, {9, 6}} {
		table := NewTable([]*Player{{ID: "human", Stack: 5000, Ready: true}}, 25, 50)
		table.ConfigureRake("sandbox")
		if err := table.ConfigureBotsForActor("human", 3500, tc.seats, 1234); err != nil {
			t.Fatal(err)
		}
		if got := table.BotSeatsNeededForActor(); got != tc.target-1 {
			t.Fatalf("%d seats: got %d", tc.seats, got)
		}
		if err := table.AddBotForActor("bot-1", "Lia", "tag"); err != nil {
			t.Fatal(err)
		}
		restored := NewTableFromState(table.ExportState())
		got := restored.BotPolicyForActor()
		if !got.Enabled || got.TargetOccupancy != tc.target || got.OwnerID != "human" {
			t.Fatalf("policy not restored: %+v", got)
		}
	}
}

func TestSecondHumanStopsBotFill(t *testing.T) {
	table := NewTable([]*Player{{ID: "h1", Stack: 5000, Ready: true}}, 25, 50)
	table.ConfigureRake("sandbox")
	if err := table.ConfigureBotsForActor("h1", 3500, 9, 1234); err != nil {
		t.Fatal(err)
	}
	if err := table.AddWaitingPlayer(&Player{ID: "h2", Stack: 3500, Ready: true}); err != nil {
		t.Fatal(err)
	}
	if got := table.BotSeatsNeededForActor(); got != 0 {
		t.Fatalf("got %d", got)
	}
}

func TestBotReservationPersistsAndBlocksRefill(t *testing.T) {
	table := NewTable([]*Player{{ID: "h1", Stack: 5000, Ready: true}}, 25, 50)
	table.ConfigureRake("sandbox")
	if err := table.ConfigureBotsForActor("h1", 3500, 2, 1); err != nil {
		t.Fatal(err)
	}
	if err := table.AddBotForActor("bot:h1:0", "Lia", "tag"); err != nil {
		t.Fatal(err)
	}
	reservation := BotReservation{ID: "r1", PlayerID: "h2", Amount: 3500, IdempotencyKey: "click-1", ExpiresAtUnixMs: 10_000}
	if err := table.ReserveBotSeatForActor(reservation, 1); err != nil {
		t.Fatal(err)
	}
	if got := table.BotSeatsNeededForActor(); got != 0 {
		t.Fatalf("reservation must block bot refill, got %d seats needed", got)
	}
	restored := NewTableFromState(table.ExportState())
	got := restored.BotReservationForActor()
	if got == nil || got.ID != reservation.ID || got.PlayerID != reservation.PlayerID {
		t.Fatalf("reservation not restored: %+v", got)
	}
	if !restored.CancelBotReservationForActor("r1", "h2") || restored.BotReservationForActor() != nil {
		t.Fatal("reservation was not cancelled")
	}
}

func TestBotReservationIsIdempotentForSameEntry(t *testing.T) {
	table := NewTable([]*Player{{ID: "h1", Stack: 5000, Ready: true}}, 25, 50)
	table.ConfigureRake("sandbox")
	_ = table.ConfigureBotsForActor("h1", 3500, 2, 1)
	_ = table.AddBotForActor("bot:h1:0", "Lia", "tag")
	first := BotReservation{ID: "r1", PlayerID: "h2", Amount: 3500, IdempotencyKey: "same-click", ExpiresAtUnixMs: 10_000}
	if err := table.ReserveBotSeatForActor(first, 1); err != nil {
		t.Fatal(err)
	}
	retry := first
	retry.ID = "r2"
	if err := table.ReserveBotSeatForActor(retry, 2); err != nil {
		t.Fatal(err)
	}
	if got := table.BotReservationForActor(); got.ID != "r1" {
		t.Fatalf("retry replaced stable reservation id: %+v", got)
	}
	other := first
	other.PlayerID, other.IdempotencyKey = "h3", "other-click"
	if err := table.ReserveBotSeatForActor(other, 2); !errors.Is(err, ErrBotSeatReserved) {
		t.Fatalf("got %v, want ErrBotSeatReserved", err)
	}
}
