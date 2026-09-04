package table

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeClock drives timeNowFunc so the pacing window can be crossed without
// sleeping. Restores the real clock when the test ends.
func fakeClock(t *testing.T) *time.Time {
	t.Helper()
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	timeNowFunc = func() time.Time { return now }
	t.Cleanup(func() { timeNowFunc = time.Now })
	return &now
}

// refreshStreaks used to run on every single command through ensureLoaded,
// putting a synchronous Valkey read in front of every action (#222). One read
// per window is enough for a badge that only changes when a hand completes.
func TestStreakRefreshIsPacedToOneReadPerWindow(t *testing.T) {
	now := fakeClock(t)
	store := newFakeStreakStore()
	actor, _ := streakActor(t, store)
	ctx := context.Background()

	for range 20 {
		actor.refreshStreaks(ctx)
		*now = now.Add(time.Second)
	}
	if store.loads != 1 {
		t.Fatalf("streak loads within one window = %d, want 1", store.loads)
	}

	*now = now.Add(StreakRefreshInterval)
	actor.refreshStreaks(ctx)
	if store.loads != 2 {
		t.Fatalf("streak loads after the window lapsed = %d, want 2", store.loads)
	}
}

// A degraded store must not be retried on every command either: the failure
// costs a full streakStoreTimeout on the actor goroutine, and the last known
// badge is still displayed.
func TestStreakRefreshBacksOffAfterAFailure(t *testing.T) {
	now := fakeClock(t)
	store := newFakeStreakStore()
	actor, _ := streakActor(t, store)
	actor.SetStreaksForActor(map[string]int{"p1": 2})
	store.loads = 0
	store.loadErr = errors.New("valkey down")

	*now = now.Add(StreakRefreshInterval)
	for range 5 {
		actor.refreshStreaks(context.Background())
		*now = now.Add(time.Second)
	}
	if store.loads != 1 {
		t.Fatalf("streak loads after a failure = %d, want 1", store.loads)
	}
	if actor.streaks["p1"] != 2 {
		t.Fatalf("streaks = %v, want the last known p1=2", actor.streaks)
	}
}

// Publishing a just-completed hand's streaks is itself a fresh value, so it
// restarts the window instead of leaving a read due immediately after.
func TestStreakPublishRestartsTheWindow(t *testing.T) {
	now := fakeClock(t)
	store := newFakeStreakStore()
	actor, _ := streakActor(t, store)

	*now = now.Add(StreakRefreshInterval)
	actor.SetStreaksForActor(map[string]int{"p1": 1})
	store.loads = 0
	actor.refreshStreaks(context.Background())

	if store.loads != 0 {
		t.Fatalf("streak loads right after a publish = %d, want 0", store.loads)
	}
}
