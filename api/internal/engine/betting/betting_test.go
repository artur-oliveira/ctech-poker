package betting

import "testing"

func TestRaiseBelowMinimumIsRejected(t *testing.T) {
	p1 := &PlayerState{ID: "P1", Stack: 1000}
	p2 := &PlayerState{ID: "P2", Stack: 1000}
	r := NewRound([]*PlayerState{p1, p2}, 0, 100)

	if err := r.Act(0, ActionBet, 100); err != nil {
		t.Fatalf("P1 bet 100 (full raise, opens at 0): %v", err)
	}
	if err := r.Act(1, ActionRaise, 150); err == nil {
		t.Fatal("expected raise to 150 (only +50, below the 100 minimum) to be rejected")
	}
	if err := r.Act(1, ActionRaise, 200); err != nil {
		t.Fatalf("raise to 200 (+100, meets minimum) should succeed: %v", err)
	}
}

func TestShortAllInDoesNotReopenActionForPlayersWhoAlreadyActed(t *testing.T) {
	p1 := &PlayerState{ID: "P1", Stack: 1000}
	p2 := &PlayerState{ID: "P2", Stack: 1000}
	p3 := &PlayerState{ID: "P3", Stack: 150}
	r := NewRound([]*PlayerState{p1, p2, p3}, 0, 100)

	if err := r.Act(0, ActionBet, 100); err != nil {
		t.Fatalf("P1 bets 100: %v", err)
	}
	if err := r.Act(1, ActionCall, 100); err != nil {
		t.Fatalf("P2 calls 100: %v", err)
	}
	// P3 shoves their entire 150-chip stack — a raise of only 50, below the
	// 100 minimum, but it's their whole stack so it's allowed as a short all-in.
	if err := r.Act(2, ActionRaise, 150); err != nil {
		t.Fatalf("P3's short all-in for 150 should be allowed: %v", err)
	}
	if !p3.AllIn {
		t.Fatal("P3 should be marked all-in")
	}
	if r.CurrentBet != 150 {
		t.Fatalf("CurrentBet should rise to 150, got %d", r.CurrentBet)
	}
	if r.MinRaise != 100 {
		t.Fatalf("MinRaise must stay at 100 — a short all-in does not reopen full-raise sizing, got %d", r.MinRaise)
	}

	// P1 already acted (bet) — the short all-in must NOT let them re-raise.
	if err := r.Act(0, ActionRaise, 300); err == nil {
		t.Fatal("P1 already acted; a short all-in must not reopen raising for them")
	}
	// P1 may still call the extra 50 to match the new CurrentBet.
	if err := r.Act(0, ActionCall, 150); err != nil {
		t.Fatalf("P1 should be able to call the extra 50: %v", err)
	}
	// P2 likewise may only call or fold, not re-raise.
	if err := r.Act(1, ActionRaise, 300); err == nil {
		t.Fatal("P2 already acted; a short all-in must not reopen raising for them either")
	}
	if err := r.Act(1, ActionCall, 150); err != nil {
		t.Fatalf("P2 should be able to call the extra 50: %v", err)
	}

	if !r.IsComplete() {
		t.Fatal("round should be complete: all non-folded, non-all-in players matched CurrentBet and have acted")
	}
}

func TestFullRaiseReopensActionForEveryone(t *testing.T) {
	p1 := &PlayerState{ID: "P1", Stack: 1000}
	p2 := &PlayerState{ID: "P2", Stack: 1000}
	r := NewRound([]*PlayerState{p1, p2}, 0, 100)

	if err := r.Act(0, ActionBet, 100); err != nil {
		t.Fatalf("P1 bets 100: %v", err)
	}
	if err := r.Act(1, ActionRaise, 300); err != nil { // +200, a full raise
		t.Fatalf("P2 raises to 300: %v", err)
	}
	if r.MinRaise != 200 {
		t.Fatalf("a full raise must update MinRaise to the new raise size (200), got %d", r.MinRaise)
	}
	// P1 already acted once, but P2's raise was full, so P1 may re-raise again.
	if err := r.Act(0, ActionRaise, 600); err != nil {
		t.Fatalf("P1 should be allowed to re-raise after a full raise reopened action: %v", err)
	}
}
