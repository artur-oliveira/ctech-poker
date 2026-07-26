package table

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func completedTable(t *testing.T) *hand.Table {
	t.Helper()
	table := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
	}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}
	if err := table.Act(table.CurrentPlayerIDForActor(), betting.ActionFold, 0); err != nil {
		t.Fatal(err)
	}
	return table
}

func TestReadyAndJoinPreserveCompleteRevealWindow(t *testing.T) {
	table := completedTable(t)
	actor := New("table-1", nil, true, func(string, hand.Snapshot) {})
	t.Cleanup(func() {
		actor.afkSweepTimer.Stop()
		if actor.nextHandTimer != nil {
			actor.nextHandTimer.Stop()
		}
	})
	actor.cached = table
	actor.handID = "completed-hand"
	actor.nextHandDelay = time.Hour
	actor.broadcastAll()
	deadline := actor.nextHandDeadline

	if err := actor.handleReady(context.Background(), ReadyCmd{
		PlayerID: "p1", ActionID: "ready-after-hand", Ready: true,
	}); err != nil {
		t.Fatal(err)
	}
	if table.Stage() != hand.Complete || actor.handID != "completed-hand" {
		t.Fatalf("ready skipped result window: stage=%v hand=%s", table.Stage(), actor.handID)
	}
	if !actor.nextHandDeadline.Equal(deadline) {
		t.Fatal("ready restarted the post-hand countdown")
	}

	if err := actor.handleJoin(context.Background(), JoinCmd{PlayerID: "p3", Stack: 1000}); err != nil {
		t.Fatal(err)
	}
	if table.Stage() != hand.Complete || actor.handID != "completed-hand" {
		t.Fatalf("join skipped result window: stage=%v hand=%s", table.Stage(), actor.handID)
	}
}

func TestUnseatedViewerCannotMutateReadyOrBlindState(t *testing.T) {
	table := hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}}, 10, 20)
	actor := New("table-1", nil, true, func(string, hand.Snapshot) {})
	t.Cleanup(func() { actor.afkSweepTimer.Stop() })
	actor.cached = table
	version := actor.version

	if err := actor.handleReady(context.Background(), ReadyCmd{
		PlayerID: "spectator", ActionID: "bad-ready", Ready: true,
	}); err == nil {
		t.Fatal("unseated viewer unexpectedly changed readiness")
	}
	if err := actor.handlePostBigBlind(context.Background(), PostBigBlindCmd{
		PlayerID: "spectator", ActionID: "bad-blind",
	}); err == nil {
		t.Fatal("unseated viewer unexpectedly posted a blind")
	}
	if actor.version != version {
		t.Fatalf("rejected spectator commands advanced version from %d to %d", version, actor.version)
	}
}

func TestKeepSeatRefreshesActivityAndSnapshotDeadline(t *testing.T) {
	old := timeNowFunc
	now := time.Unix(1_800_000_000, 0)
	timeNowFunc = func() time.Time { return now }
	t.Cleanup(func() { timeNowFunc = old })

	table := hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000, LastActionAt: now.Add(-4 * time.Minute).UnixMilli()}}, 10, 20)
	var latest hand.Snapshot
	actor := New("table-1", nil, true, func(_ string, snapshot hand.Snapshot) { latest = snapshot })
	t.Cleanup(func() { actor.afkSweepTimer.Stop() })
	actor.cached = table
	actor.kickGrace = 5 * time.Minute

	if err := actor.handleKeepSeat(context.Background(), KeepSeatCmd{PlayerID: "p1", ActionID: "stay"}); err != nil {
		t.Fatal(err)
	}
	want := now.Add(5 * time.Minute).UnixMilli()
	if latest.IdleRemovalUnixMs != want {
		t.Fatalf("idle deadline=%d, want %d", latest.IdleRemovalUnixMs, want)
	}
}
