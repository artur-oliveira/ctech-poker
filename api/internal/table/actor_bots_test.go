package table

import (
	"context"
	"testing"

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

func TestHumanJoinRetiresBotsAndHeadsUpMayWaitForNextHand(t *testing.T) {
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

	if err := a.applyJoinAndCommit(context.Background(), JoinCmd{PlayerID: "human-2", Stack: 5_000, MaxSeats: 2}); err != nil {
		t.Fatal(err)
	}
	var bot, entrant *hand.Player
	for _, p := range a.cached.PlayersForActor() {
		switch p.ID {
		case "bot:human-1:0":
			bot = p
		case "human-2":
			entrant = p
		}
	}
	if bot == nil || !bot.PendingExit || bot.Ready {
		t.Fatalf("bot was not retired safely: %+v", bot)
	}
	if entrant == nil || a.cached.DealtIntoCurrentHandForActor(entrant.ID) {
		t.Fatalf("human must stay outside the hand that was live on arrival: %+v", entrant)
	}
}
