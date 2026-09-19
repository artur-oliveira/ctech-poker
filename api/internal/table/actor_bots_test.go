package table

import (
	"context"
	"errors"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func sandboxBotTable(players ...*hand.Player) *hand.Table {
	t := hand.NewTable(players, 25, 50)
	t.ConfigureRake("sandbox")
	return t
}

func TestFillBotsReachesFormatTargetAndStartsHand(t *testing.T) {
	game := sandboxBotTable(&hand.Player{ID: "human", Stack: 5_000, Ready: true})
	if err := game.ConfigureBotsForActor("human", 4_000, 6, 1); err != nil {
		t.Fatal(err)
	}
	a := New("table-1", nil, true, func(string, hand.Snapshot) {})
	a.SetCachedForTest(game)

	if err := a.handleFillBots(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := len(a.cached.PlayersForActor()); got != 4 {
		t.Fatalf("got %d seats, want four", got)
	}
	if a.cached.Stage() != hand.PreFlop {
		t.Fatalf("got stage %v, want pre-flop", a.cached.Stage())
	}
	for _, p := range a.cached.PlayersForActor() {
		if p.ID != "human" && (!p.IsBot || p.BotProfile == "") {
			t.Fatalf("bot identity/profile missing: %+v", p)
		}
	}
}

func TestStartBotsNowRequiresOwnerAndFillsBeforeDeadline(t *testing.T) {
	game := sandboxBotTable(&hand.Player{ID: "human", Stack: 5_000, Ready: true})
	if err := game.ConfigureBotsForActor("human", 4_000, 2, timeNowFunc().Add(time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	a := New("table-1", nil, true, func(string, hand.Snapshot) {})
	a.SetCachedForTest(game)
	if err := a.handleStartBotsNow(context.Background(), StartBotsNowCmd{OwnerID: "other"}); !errors.Is(err, hand.ErrBotFillUnavailable) {
		t.Fatalf("unauthorized start returned %v", err)
	}
	if len(a.cached.PlayersForActor()) != 1 {
		t.Fatal("unauthorized start added a bot")
	}
	if err := a.handleStartBotsNow(context.Background(), StartBotsNowCmd{OwnerID: "human"}); err != nil {
		t.Fatal(err)
	}
	if got := a.cached.PlayersForActor(); len(got) != 2 || !got[1].IsBot || a.cached.Stage() != hand.PreFlop {
		t.Fatalf("start did not fill table: stage=%v players=%+v", a.cached.Stage(), got)
	}
}

func TestBotReactionUsesNormalActivityWithoutPremiumOwnership(t *testing.T) {
	game := sandboxBotTable(&hand.Player{ID: "human", Stack: 5_000, Ready: true})
	if err := game.ConfigureBotsForActor("human", 4_000, 2, 1); err != nil {
		t.Fatal(err)
	}
	if err := game.AddBotForActor("bot:human:0", "Lia", "tag"); err != nil {
		t.Fatal(err)
	}
	if err := game.StartHand(); err != nil {
		t.Fatal(err)
	}
	if err := game.Act(game.CurrentPlayerIDForActor(), betting.ActionFold, 0); err != nil {
		t.Fatal(err)
	}
	a := New("table-1", nil, true, func(string, hand.Snapshot) {})
	a.SetCachedForTest(game)
	a.handID = "hand-1"
	command := ReactionCmd{PlayerID: "bot:human:0", ActionID: "bot-reaction-hand-1",
		ReactionID: "poop", TargetPlayerID: "human", BotGenerated: true, BotHandID: "hand-1"}
	if err := a.handleReaction(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	if len(a.activity.Reactions) != 1 || a.activity.Reactions[0].ReactionID != "poop" ||
		a.cached.BotPolicyForActor().LastReactionHandID != "hand-1" {
		t.Fatalf("bot reaction was not recorded: %+v", a.activity.Reactions)
	}
	if err := a.handleReaction(context.Background(), command); err == nil {
		t.Fatal("same bot reacted twice in one hand")
	}
}

func TestDirectHumanJoinCannotBypassHeadsUpReservation(t *testing.T) {
	game := sandboxBotTable(&hand.Player{ID: "human-1", Stack: 5_000, Ready: true})
	if err := game.ConfigureBotsForActor("human-1", 4_000, 2, 1); err != nil {
		t.Fatal(err)
	}
	if err := game.AddBotForActor("bot:human-1:0", "Lia", "tag"); err != nil {
		t.Fatal(err)
	}
	if err := game.StartHand(); err != nil {
		t.Fatal(err)
	}
	a := New("table-1", nil, true, func(string, hand.Snapshot) {})
	a.SetCachedForTest(game)

	if err := a.applyJoinAndCommit(context.Background(), JoinCmd{PlayerID: "human-2", Stack: 5_000, MaxSeats: 2}); err != ErrBotReservationRequired {
		t.Fatalf("direct join should require reservation, got %v", err)
	}
	for _, p := range a.cached.PlayersForActor() {
		if p.ID == "human-2" || p.IsBot && p.PendingExit {
			t.Fatalf("direct join changed bot hand: %+v", p)
		}
	}
}

func TestReservationRemovesWaitingBotAndBecomesReadyWithoutDebit(t *testing.T) {
	game := sandboxBotTable(&hand.Player{ID: "human-1", Stack: 5_000, Ready: true})
	if err := game.ConfigureBotsForActor("human-1", 4_000, 2, 1); err != nil {
		t.Fatal(err)
	}
	if err := game.AddBotForActor("bot:human-1:0", "Lia", "tag"); err != nil {
		t.Fatal(err)
	}
	a := New("table-1", nil, true, func(string, hand.Snapshot) {})
	a.SetCachedForTest(game)
	result := make(chan hand.BotReservation, 1)
	reservation := hand.BotReservation{ID: "r1", PlayerID: "human-2", Amount: 4_000,
		IdempotencyKey: "click", ExpiresAtUnixMs: timeNowFunc().Add(time.Minute).UnixMilli()}
	if err := a.handleReserveBotSeat(context.Background(), ReserveBotSeatCmd{Reservation: reservation, Result: result}); err != nil {
		t.Fatal(err)
	}
	if got := <-result; got.ID != "r1" {
		t.Fatalf("unexpected reservation: %+v", got)
	}
	for _, p := range a.cached.PlayersForActor() {
		if p.IsBot {
			t.Fatalf("waiting bot was not removed: %+v", p)
		}
	}
	if !a.cached.BotReservationReadyForActor() {
		t.Fatal("reservation should be ready after all bots leave")
	}
	if err := a.applyJoinAndCommit(context.Background(), JoinCmd{PlayerID: "human-3", Stack: 4_000, MaxSeats: 2}); err != ErrBotReservationRequired {
		t.Fatalf("released bot seat must stay reserved for human-2, got %v", err)
	}
	if err := a.applyJoinAndCommit(context.Background(), JoinCmd{PlayerID: "human-2", Stack: 4_000,
		MaxSeats: 2, ReservationID: "r1"}); err != nil {
		t.Fatalf("reserved player should take the seat: %v", err)
	}
	if a.cached.BotReservationForActor() != nil {
		t.Fatal("reservation must be consumed with the seat commit")
	}
	if err := a.applyJoinAndCommit(context.Background(), JoinCmd{PlayerID: "human-2", Stack: 4_000,
		MaxSeats: 2, ReservationID: "r1"}); err != nil {
		t.Fatalf("concurrent retry of committed reserved seat must be idempotent: %v", err)
	}
}
