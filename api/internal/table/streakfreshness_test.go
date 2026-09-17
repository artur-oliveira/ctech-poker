package table

import (
	"context"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

// Only the instance that wins claimHandHooks publishes the new badges. Every
// other instance learns about them through the same ChangeNotifier channel it
// already uses for commits — otherwise it keeps serving the previous hand's
// numbers for up to StreakRefreshInterval, which is exactly the 6-21s windows
// the 2026-09-15 capture shows.
// See docs/specs/2026-09-17-table-snapshot-divergence-and-highlight-winner.md.
func TestPublishingStreaksNotifiesSiblingInstances(t *testing.T) {
	store := newFakeStreakStore()
	actor, _ := streakActor(t, store)
	notifier := newFakeChangeNotifier()
	actor.SetChangeNotifierForActor(notifier)

	actor.SetStreaksForActor(map[string]int{"p1": 2, "p2": -1})

	if got := notifier.awaitOne(t); got != "table-1" {
		t.Fatalf("notified table_id = %q, want %q", got, "table-1")
	}
}

// The sibling's side of that signal: a reload landing on a completed hand
// re-reads the badge even though the pacing window has not lapsed. Gated on
// Complete so this stays a handful of reads per hand and never one per
// command — the regression StreakRefreshInterval exists to prevent (#222).
func TestExternalChangeOnACompletedHandRefreshesTheBadge(t *testing.T) {
	fakeClock(t)
	store := newFakeStreakStore()
	actor, table := streakActor(t, store)
	actor.refreshStreaks(context.Background()) // opens the pacing window
	store.shared["p1"] = 3
	store.loads = 0
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	foldToOnePlayer(t, table)

	if err := actor.handleExternalChange(context.Background(), ExternalChangeCmd{}); err != nil {
		t.Fatalf("handleExternalChange: %v", err)
	}

	if store.loads != 1 {
		t.Fatalf("streak loads on a completed-hand external change = %d, want 1", store.loads)
	}
	if actor.streaks["p1"] != 3 {
		t.Fatalf("streaks = %v, want the freshly published p1=3", actor.streaks)
	}
}

// Mid-hand external changes are the common case (every action commits), so
// they must not each cost a Valkey read.
func TestExternalChangeMidHandKeepsThePacingWindow(t *testing.T) {
	fakeClock(t)
	store := newFakeStreakStore()
	actor, table := streakActor(t, store)
	actor.refreshStreaks(context.Background())
	store.loads = 0
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	for range 10 {
		if err := actor.handleExternalChange(context.Background(), ExternalChangeCmd{}); err != nil {
			t.Fatalf("handleExternalChange: %v", err)
		}
	}

	if store.loads != 0 {
		t.Fatalf("streak loads on mid-hand external changes = %d, want 0", store.loads)
	}
}

// foldToOnePlayer drives the table to hand.Complete by folding everyone but
// one player, which is the same path an all-in nobody calls takes.
func foldToOnePlayer(t *testing.T, table *hand.Table) {
	t.Helper()
	for range 20 {
		if table.Stage() == hand.Complete {
			return
		}
		current := table.CurrentPlayerIDForActor()
		if current == "" {
			break
		}
		if err := table.Act(current, betting.ActionFold, 0); err != nil {
			t.Fatalf("fold for %s: %v", current, err)
		}
	}
	if table.Stage() != hand.Complete {
		t.Fatalf("table never reached Complete, stuck at stage %v", table.Stage())
	}
}
