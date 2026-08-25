package table

import (
	"context"
	"errors"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

// fakeStreakStore stands in for internal/tablestreak: one map shared by every
// actor built with it, exactly like the Valkey key is in production.
type fakeStreakStore struct {
	shared  map[string]int
	loadErr error
	loads   int
}

func newFakeStreakStore() *fakeStreakStore {
	return &fakeStreakStore{shared: map[string]int{}}
}

func (f *fakeStreakStore) Load(context.Context, string) (map[string]int, error) {
	f.loads++
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	out := make(map[string]int, len(f.shared))
	for k, v := range f.shared {
		out[k] = v
	}
	return out, nil
}

func (f *fakeStreakStore) Merge(_ context.Context, _ string, streaks map[string]int) (map[string]int, error) {
	for k, v := range streaks {
		f.shared[k] = v
	}
	return f.Load(context.Background(), "")
}

func streakActor(t *testing.T, store StreakStore) (*Actor, *hand.Table) {
	t.Helper()
	table := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
	}, 10, 20)
	actor := New("table-1", nil, true, func(string, hand.Snapshot) {})
	t.Cleanup(func() { actor.afkSweepTimer.Stop() })
	actor.cached = table
	actor.SetStreakStoreForActor(store)
	return actor, table
}

// Two instances may run an Actor for the same table at once, and both
// broadcast to the same clients. The badge each publishes has to come from
// the shared store, or the number flips between them on consecutive
// snapshots of one hand (the reported "V2, V4, V2, V4").
func TestStreakBadgeIsSharedBetweenInstances(t *testing.T) {
	store := newFakeStreakStore()
	first, _ := streakActor(t, store)
	second, secondTable := streakActor(t, store)

	first.SetStreaksForActor(map[string]int{"p1": 3, "p2": -3})

	if err := second.ensureLoaded(context.Background(), false); err != nil {
		t.Fatalf("ensureLoaded: %v", err)
	}
	seats := secondTable.ViewFor("p1").Seats
	second.applyStreaks(seats)
	for _, seat := range seats {
		want := map[string]int32{"p1": 3, "p2": -3}[seat.PlayerID]
		if seat.CurrentStreak != want {
			t.Fatalf("seat %s streak = %d, want %d", seat.PlayerID, seat.CurrentStreak, want)
		}
	}
}

// A hand completing on the other instance must not be lost when this one
// publishes its own: the store merges, and the actor adopts what came back.
func TestStreakPublishAdoptsTheMergedResult(t *testing.T) {
	store := newFakeStreakStore()
	store.shared["p2"] = -4
	actor, _ := streakActor(t, store)

	actor.SetStreaksForActor(map[string]int{"p1": 1})

	if actor.streaks["p1"] != 1 || actor.streaks["p2"] != -4 {
		t.Fatalf("streaks = %v, want p1=1 and the other instance's p2=-4", actor.streaks)
	}
}

// An unreachable Valkey must degrade to the last known badge, never blank
// every seat mid-hand.
func TestStreakLoadFailureKeepsTheLastKnownBadge(t *testing.T) {
	store := newFakeStreakStore()
	actor, _ := streakActor(t, store)
	actor.SetStreaksForActor(map[string]int{"p1": 2})
	store.loadErr = errors.New("valkey down")

	if err := actor.ensureLoaded(context.Background(), false); err != nil {
		t.Fatalf("ensureLoaded: %v", err)
	}
	if actor.streaks["p1"] != 2 {
		t.Fatalf("streaks = %v, want the last known p1=2", actor.streaks)
	}
}

// Without a store wired (dev, tests) the in-process map is still the whole
// story, and nothing reaches for a nil interface.
func TestStreakWorksWithoutASharedStore(t *testing.T) {
	actor, _ := streakActor(t, nil)
	actor.SetStreaksForActor(map[string]int{"p1": 5})
	if err := actor.ensureLoaded(context.Background(), false); err != nil {
		t.Fatalf("ensureLoaded: %v", err)
	}
	if actor.streaks["p1"] != 5 {
		t.Fatalf("streaks = %v, want p1=5", actor.streaks)
	}
}
