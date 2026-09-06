package game

import (
	"testing"

	"gopkg.aoctech.app/poker/cli/internal/proto"
)

func TestPositionsHeadsUp(t *testing.T) {
	// Heads-up: the dealer is also the small blind.
	got := Positions([]string{"a", "b"}, "a", "a", "b")
	if got["a"] != "D/SB" || got["b"] != "BB" {
		t.Fatalf("got %+v", got)
	}
}

func TestPositionsSixMax(t *testing.T) {
	seats := []string{"btn", "sb", "bb", "utg", "mp", "co"}
	got := Positions(seats, "btn", "sb", "bb")
	want := map[string]string{"btn": "D", "sb": "SB", "bb": "BB", "utg": "UTG", "mp": "MP", "co": "CO"}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s: got %q want %q", id, got[id], w)
		}
	}
}

func TestPositionsNineMax(t *testing.T) {
	seats := []string{"btn", "sb", "bb", "utg", "utg1", "utg2", "mp", "hj", "co"}
	got := Positions(seats, "btn", "sb", "bb")
	for id, w := range map[string]string{
		"btn": "D", "sb": "SB", "bb": "BB", "utg": "UTG", "utg1": "UTG+1",
		"utg2": "UTG+2", "mp": "MP", "hj": "HJ", "co": "CO",
	} {
		if got[id] != w {
			t.Errorf("%s: got %q want %q", id, got[id], w)
		}
	}
}

func snapshotFixture() *proto.TableSnapshot {
	dealtIn := true
	eq := 0.62
	return &proto.TableSnapshot{
		Stage:                    "flop",
		Board:                    []string{"Ah", "7c", "Kd"},
		CurrentPlayerId:          "you",
		SmallBlindPlayerId:       "duda",
		BigBlindPlayerId:         "edu",
		DealerPlayerId:           "caio",
		Pots:                     []*proto.Pot{{Amount: 24}},
		ActionDeadlineUnixMs:     1_000_000,
		ActionBaseDeadlineUnixMs: 990_000,
		LegalActions: &proto.LegalActions{
			Actions: []string{"fold", "call", "raise"}, CallAmount: 8, MinRaiseTo: 16, MaxRaiseTo: 246,
			PotRaiseTo: 32,
		},
		Seats: []*proto.Seat{
			{PlayerId: "caio", Name: "Caio", Stack: 297, State: "active", Contributed: 8, DealtIn: &dealtIn},
			{PlayerId: "duda", Name: "Duda", Stack: 100, State: "folded", DealtIn: &dealtIn},
			{PlayerId: "edu", Name: "Edu", Stack: 402, State: "active", Contributed: 8, DealtIn: &dealtIn},
			{
				PlayerId: "you", Name: "VOCÊ", Stack: 246, State: "active", DealtIn: &dealtIn,
				HoleCards: []string{"As", "Qh"}, Equity: &eq,
			},
		},
	}
}

func TestNewTableViewMapsHeaderFields(t *testing.T) {
	v := NewTableView(snapshotFixture(), "you", "room-1", false, [2]int64{1, 2}, 6, CardASCII)

	if v.Pot != 24 || v.Stage != "flop" {
		t.Errorf("pot/stage: %d %q", v.Pot, v.Stage)
	}
	if v.Seated != 4 { // duda folded but still occupies a seat
		t.Errorf("seated = %d, want 4", v.Seated)
	}
	if !v.IsYourTurn {
		t.Error("IsYourTurn should be true for current_player_id == you")
	}
	if v.CurrentPlayer.ID != "you" || !v.CurrentPlayer.IsYou {
		t.Errorf("current player not mapped: %+v", v.CurrentPlayer)
	}
	if len(v.YourHole) != 2 || v.YourStrength != "par de ases" {
		t.Errorf("your hand: %v %q", v.YourHole, v.YourStrength)
	}
	if v.YourEquity < 0.61 || v.YourEquity > 0.63 {
		t.Errorf("equity = %v", v.YourEquity)
	}
	if v.Legal == nil || v.Legal.CallAmount != 8 || v.Legal.PotRaiseTo != 32 {
		t.Errorf("legal actions not carried: %v", v.Legal)
	}
}

func TestNewTableViewPositionsAndFoldFlags(t *testing.T) {
	v := NewTableView(snapshotFixture(), "you", "room-1", false, [2]int64{1, 2}, 6, CardASCII)
	byID := map[string]PlayerView{}
	for _, p := range v.Players {
		byID[p.ID] = p
	}
	if byID["caio"].Position != "D" || byID["duda"].Position != "SB" || byID["edu"].Position != "BB" {
		t.Errorf("positions: caio=%q duda=%q edu=%q", byID["caio"].Position, byID["duda"].Position, byID["edu"].Position)
	}
	if !byID["duda"].Folded {
		t.Error("duda should be folded")
	}
	if !byID["you"].IsYou {
		t.Error("you flag missing")
	}
}

func TestNewTableViewEquityAbsentIsNegativeOne(t *testing.T) {
	s := snapshotFixture()
	s.Seats[3].Equity = nil
	v := NewTableView(s, "you", "room-1", false, [2]int64{1, 2}, 6, CardASCII)
	if v.YourEquity != -1 {
		t.Errorf("absent equity should map to -1, got %v", v.YourEquity)
	}
}

func TestNewTableViewStrengthEmptyWithoutTwoHoleCards(t *testing.T) {
	s := snapshotFixture()
	s.Seats[3].HoleCards = []string{"As"}
	v := NewTableView(s, "you", "room-1", false, [2]int64{1, 2}, 6, CardASCII)
	if v.YourStrength != "" {
		t.Errorf("strength should be empty with one hole card, got %q", v.YourStrength)
	}
}

func TestNewTableViewRealMoneyFlagIsCarried(t *testing.T) {
	v := NewTableView(snapshotFixture(), "you", "room-1", true, [2]int64{1, 2}, 6, CardASCII)
	if !v.RealMoney {
		t.Error("RealMoney flag not carried")
	}
}
