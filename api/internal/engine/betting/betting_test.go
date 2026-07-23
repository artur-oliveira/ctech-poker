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

func TestFoldToOneRemainingPlayerCompletesRoundEvenIfThatPlayerNeverActed(t *testing.T) {
	// Heads-up pre-flop: P1 is SB (acts first), P2 is BB and has not acted
	// yet this round. P1 folds — P2 is the only player left, so the round
	// must be complete right away; there's no one left for P2 to act against.
	p1 := &PlayerState{ID: "P1", Stack: 990, Contributed: 10}
	p2 := &PlayerState{ID: "P2", Stack: 980, Contributed: 20}
	r := NewRound([]*PlayerState{p1, p2}, 20, 20)

	if err := r.Act(0, ActionFold, 0); err != nil {
		t.Fatalf("P1 folds: %v", err)
	}
	if !r.IsComplete() {
		t.Fatal("round must be complete once only one non-folded player remains, regardless of whether that player has acted")
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

// Regression (from task review): a short all-in sandwiched between two FULL
// raises from different players must still only lock out players who already
// acted since the most recent full raise — not reopen anything on its own.
func TestShortAllInBetweenTwoFullRaisesDoesNotReopenAction(t *testing.T) {
	p1 := &PlayerState{ID: "P1", Stack: 1000}
	p2 := &PlayerState{ID: "P2", Stack: 1000}
	p3 := &PlayerState{ID: "P3", Stack: 1000}
	p4 := &PlayerState{ID: "P4", Stack: 300}
	r := NewRound([]*PlayerState{p1, p2, p3, p4}, 0, 100)

	if err := r.Act(0, ActionBet, 100); err != nil { // full raise, MinRaise=100
		t.Fatalf("P1 bets 100: %v", err)
	}
	if err := r.Act(1, ActionCall, 100); err != nil {
		t.Fatalf("P2 calls 100: %v", err)
	}
	if err := r.Act(2, ActionRaise, 250); err != nil { // +150 >= 100, full raise, MinRaise=150
		t.Fatalf("P3 raises to 250: %v", err)
	}
	if r.MinRaise != 150 {
		t.Fatalf("P3's raise is full — MinRaise should become 150, got %d", r.MinRaise)
	}
	if err := r.Act(0, ActionCall, 250); err != nil {
		t.Fatalf("P1 calls 250: %v", err)
	}
	if err := r.Act(1, ActionCall, 250); err != nil {
		t.Fatalf("P2 calls 250: %v", err)
	}
	// P4 shoves their whole 300 stack — a raise of only 50, below the current
	// 150 minimum, but it's their entire stack so it's a short all-in.
	if err := r.Act(3, ActionRaise, 300); err != nil {
		t.Fatalf("P4's short all-in for 300 should be allowed: %v", err)
	}
	if r.MinRaise != 150 {
		t.Fatalf("short all-in must not change MinRaise, got %d", r.MinRaise)
	}
	// P1 and P2 already acted since P3's full raise (the most recent one) —
	// P4's short all-in must not grant them a new raise right.
	if err := r.Act(0, ActionRaise, 400); err == nil {
		t.Fatal("P1 already acted since the last full raise; P4's short all-in must not reopen raising")
	}
	if err := r.Act(1, ActionRaise, 400); err == nil {
		t.Fatal("P2 already acted since the last full raise; P4's short all-in must not reopen raising")
	}
	// All three remaining players can still call the extra 50 up to 300.
	if err := r.Act(0, ActionCall, 300); err != nil {
		t.Fatalf("P1 should be able to call the extra 50: %v", err)
	}
	if err := r.Act(1, ActionCall, 300); err != nil {
		t.Fatalf("P2 should be able to call the extra 50: %v", err)
	}
	if err := r.Act(2, ActionCall, 300); err != nil {
		t.Fatalf("P3 should be able to call the extra 50: %v", err)
	}
	if !r.IsComplete() {
		t.Fatal("round should be complete: all non-folded, non-all-in players matched CurrentBet and have acted")
	}
}

// Regression (from task review): a player who has genuinely never acted this
// round retains full raise rights even after a short all-in — and if they
// make a FULL raise, it retroactively reopens action for players a prior
// short all-in had locked out.
func TestNeverActedPlayerCanReopenActionAfterShortAllIn(t *testing.T) {
	p1 := &PlayerState{ID: "P1", Stack: 1000}
	p2 := &PlayerState{ID: "P2", Stack: 1000}
	p3 := &PlayerState{ID: "P3", Stack: 150}
	p4 := &PlayerState{ID: "P4", Stack: 1000}
	r := NewRound([]*PlayerState{p1, p2, p3, p4}, 0, 100)

	if err := r.Act(0, ActionBet, 100); err != nil { // full raise, MinRaise=100
		t.Fatalf("P1 bets 100: %v", err)
	}
	if err := r.Act(1, ActionCall, 100); err != nil {
		t.Fatalf("P2 calls 100: %v", err)
	}
	// P3 shoves their whole 150-chip stack — a short all-in (+50 < 100 minimum).
	if err := r.Act(2, ActionRaise, 150); err != nil {
		t.Fatalf("P3's short all-in for 150 should be allowed: %v", err)
	}
	if r.MinRaise != 100 {
		t.Fatalf("short all-in must not change MinRaise, got %d", r.MinRaise)
	}
	// P4 has never acted this round — their flag was never set true, so they
	// retain full raise rights regardless of P3's short all-in.
	if err := r.Act(3, ActionRaise, 350); err != nil { // +200 >= 100, a full raise
		t.Fatalf("P4, who never acted yet, should be allowed to raise: %v", err)
	}
	if r.MinRaise != 200 {
		t.Fatalf("P4's raise is full — MinRaise should become 200, got %d", r.MinRaise)
	}
	// P1 and P2 were locked out by P3's short all-in, but P4's FULL raise must
	// retroactively reopen action for them.
	if err := r.Act(0, ActionRaise, 600); err != nil {
		t.Fatalf("P4's full raise should have reopened raising for P1: %v", err)
	}
}
