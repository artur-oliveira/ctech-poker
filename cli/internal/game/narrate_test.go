package game

import (
	"strings"
	"testing"

	"gopkg.aoctech.app/poker/cli/internal/proto"
)

func TestNarratorStreetChange(t *testing.T) {
	n := NewNarrator("you")
	n.OnSnapshot(&proto.TableSnapshot{Stage: "preflop"})
	lines := n.OnSnapshot(&proto.TableSnapshot{Stage: "flop", Board: []string{"Ah", "7c", "Kd"}})
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "FLOP") || !strings.Contains(joined, "♥") {
		t.Fatalf("want a flop line with cards, got %q", joined)
	}
}

func TestNarratorPlayerActions(t *testing.T) {
	n := NewNarrator("you")
	base := &proto.TableSnapshot{
		Stage: "flop",
		Seats: []*proto.Seat{
			{PlayerId: "caio", Name: "Caio", State: "active", Contributed: 0},
			{PlayerId: "duda", Name: "Duda", State: "active", Contributed: 0},
		},
	}
	n.OnSnapshot(base)

	next := &proto.TableSnapshot{
		Stage: "flop",
		Seats: []*proto.Seat{
			{PlayerId: "caio", Name: "Caio", State: "active", Contributed: 8},
			{PlayerId: "duda", Name: "Duda", State: "folded", Contributed: 0},
		},
	}
	lines := strings.Join(n.OnSnapshot(next), "\n")
	if !strings.Contains(lines, "Caio") || !strings.Contains(lines, "8") {
		t.Errorf("expected Caio's bet, got %q", lines)
	}
	if !strings.Contains(lines, "Duda") || !strings.Contains(lines, "desiste") {
		t.Errorf("expected Duda's fold, got %q", lines)
	}
}

func TestNarratorShowdownEmitsWinnerAndPayout(t *testing.T) {
	n := NewNarrator("you")
	n.OnSnapshot(&proto.TableSnapshot{Stage: "river", Seats: []*proto.Seat{{PlayerId: "edu", Name: "Edu"}}})
	lines := strings.Join(n.OnSnapshot(&proto.TableSnapshot{
		Stage:   "complete",
		Board:   []string{"Ah", "7c", "Kd", "2s", "9h"},
		Winners: []string{"edu"},
		Seats: []*proto.Seat{
			{PlayerId: "edu", Name: "Edu", HoleCards: []string{"As", "Ad"}},
		},
		PotResults: []*proto.PotResult{{Amount: 24, PayoutAmount: 24, WinnerPlayerIds: []string{"edu"}}},
	}), "\n")
	if !strings.Contains(lines, "Edu") || !strings.Contains(lines, "24") {
		t.Fatalf("want a winner+payout line, got %q", lines)
	}
}

func TestNarratorShowdownSidePotBreakdown(t *testing.T) {
	n := NewNarrator("you")
	n.OnSnapshot(&proto.TableSnapshot{Stage: "river", Seats: []*proto.Seat{
		{PlayerId: "a", Name: "Ana"}, {PlayerId: "b", Name: "Bru"}, {PlayerId: "c", Name: "Caio"},
	}})
	lines := n.OnSnapshot(&proto.TableSnapshot{
		Stage:   "complete",
		Board:   []string{"Ah", "7c", "Kd", "2s", "9h"},
		Winners: []string{"a", "c"},
		Seats: []*proto.Seat{
			{PlayerId: "a", Name: "Ana"}, {PlayerId: "b", Name: "Bru"}, {PlayerId: "c", Name: "Caio"},
		},
		PotResults: []*proto.PotResult{
			{PayoutAmount: 300, Payouts: map[string]int64{"a": 300}},
			{PayoutAmount: 120, Payouts: map[string]int64{"c": 120}},
			{PayoutAmount: 40, Refund: true, Payouts: map[string]int64{"b": 40}},
		},
	})
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"pote principal 300 · Ana", "pote lateral 1 120 · Caio", "devolvido 40 · Bru"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in:\n%s", want, joined)
		}
	}
	// History still totals contested winnings only (refund excluded).
	if w := n.LastWinners(2); len(w) != 2 || !strings.Contains(strings.Join(w, ","), "Ana (+300)") {
		t.Fatalf("history: %v", w)
	}
}

func TestNarratorShowdownSplitPot(t *testing.T) {
	n := NewNarrator("you")
	n.OnSnapshot(&proto.TableSnapshot{Stage: "river"})
	lines := strings.Join(n.OnSnapshot(&proto.TableSnapshot{
		Stage: "complete", Winners: []string{"a", "b"},
		Seats: []*proto.Seat{{PlayerId: "a", Name: "Ana"}, {PlayerId: "b", Name: "Bru"}},
		PotResults: []*proto.PotResult{
			{PayoutAmount: 200, Refund: true, Payouts: map[string]int64{"c": 200}},
			{PayoutAmount: 101, Payouts: map[string]int64{"a": 51, "b": 50}},
		},
	}), "\n")
	if !strings.Contains(lines, "Ana 51 e Bru 50") {
		t.Fatalf("split pot not shown: %s", lines)
	}
}

func TestNarratorOnMessageChatAndReaction(t *testing.T) {
	n := NewNarrator("you")
	chat := n.OnMessage(&proto.ServerMessage{Type: "chat", PlayerId: "ana", Message: "gg"})
	if len(chat) != 1 || !strings.Contains(chat[0], "gg") {
		t.Errorf("chat line: %v", chat)
	}
	ach := n.OnMessage(&proto.ServerMessage{Type: "achievement_unlocked", Key: "first_win", Stars: 1})
	if len(ach) != 1 || !strings.Contains(ach[0], "first_win") {
		t.Errorf("achievement line: %v", ach)
	}
	rem := n.OnMessage(&proto.ServerMessage{Type: "removed", Message: "idle", Amount: 1500})
	if len(rem) != 1 || !strings.Contains(rem[0], "1500") {
		t.Errorf("removed line: %v", rem)
	}
}

func TestNarratorLastWinnersTracksHistory(t *testing.T) {
	n := NewNarrator("you")
	for _, w := range []string{"ana", "bruno", "caio"} {
		n.OnSnapshot(&proto.TableSnapshot{Stage: "turn"})
		n.OnSnapshot(&proto.TableSnapshot{
			Stage: "complete", Winners: []string{w},
			Seats:      []*proto.Seat{{PlayerId: w, Name: strings.ToUpper(w[:1]) + w[1:]}},
			PotResults: []*proto.PotResult{{PayoutAmount: 10, WinnerPlayerIds: []string{w}}},
		})
	}
	got := n.LastWinners(2)
	if len(got) != 2 || !strings.Contains(got[0], "Caio") || !strings.Contains(got[1], "Bruno") {
		t.Fatalf("last winners (newest first): %v", got)
	}
}

func TestNarratorNoPanicOnRepeatedIdenticalSnapshots(t *testing.T) {
	n := NewNarrator("you")
	s := &proto.TableSnapshot{Stage: "flop", Seats: []*proto.Seat{{PlayerId: "a", Name: "A", Contributed: 4}}}
	for i := 0; i < 5; i++ {
		if lines := n.OnSnapshot(s); i > 0 && len(lines) != 0 {
			t.Fatalf("identical snapshot %d produced lines: %v", i, lines)
		}
	}
}
