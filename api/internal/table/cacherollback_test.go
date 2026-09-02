package table

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

// playerStack reads playerID's stack out of the actor's cache, failing the
// test if the player is not seated.
func playerStack(t *testing.T, a *Actor, playerID string) int64 {
	t.Helper()
	for _, p := range a.cached.PlayersForActor() {
		if p.ID == playerID {
			return p.Stack
		}
	}
	t.Fatalf("player %s not seated", playerID)
	return 0
}

func playerIDs(a *Actor) []string {
	var ids []string
	for _, p := range a.cached.PlayersForActor() {
		ids = append(ids, p.ID)
	}
	return ids
}

// TestMutateRestoresCacheHandIDAndActivityOnError is a direct, handler-agnostic
// proof of the structural guard introduced for #51: a.mutate must restore
// a.cached, a.handID AND a.activity to their exact pre-call values whenever
// the wrapped function returns any error, regardless of how far into the
// mutation it got before failing. Before this guard, each handler was
// individually trusted to perform this snapshot/restore dance by convention
// (and several simply never did — see applyActAndCommit, handleSitOut,
// handleShowCards, etc. before this change), so the actor's cache could
// silently diverge from what was actually persisted.
//
// Deliberately checks a mutation that changes an EXISTING player's field in
// place (p.Stack), not just a slice append/remove: a naive
// `before := a.cached.ExportState()` snapshot — the very convention every
// apply*AndCommit handler used before this change — aliases the live
// table's *Player pointers and its players slice's backing array, so it
// fails to undo either kind of in-place mutation. mutate's snapshot instead
// round-trips through the same DynamoDB attribute-value encoding a real
// commit uses, which is what makes the restored table a genuine deep copy.
func TestMutateRestoresCacheHandIDAndActivityOnError(t *testing.T) {
	game := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000},
		{ID: "p2", Stack: 1000},
	}, 10, 20)

	actor := New("mutate-rollback", nil, true, nil)
	// Route through NewTableFromState once, exactly like ensureLoaded always
	// does, so this starts from the same normalized shape a.cached actually
	// has in production (e.g. TimeBankMs defaulting).
	actor.cached = hand.NewTableFromState(game.ExportState())
	actor.handID = "hand-before"
	actor.activity = tablestore.TableActivity{
		Chat:          []tablestore.ChatMessage{{ID: "c1", PlayerID: "p1", Message: "hi"}},
		Preselections: map[string]tablestore.Preselection{"p2": {Selection: "fold", HandID: "hand-before"}},
	}
	beforeActivity := cloneActivity(actor.activity)

	sentinel := errors.New("boom: partial mutation must not survive")
	err := actor.mutate(func() error {
		// Mutate all three fields the guard is responsible for, then fail —
		// simulating a handler whose engine mutation (or the commit call
		// itself) fails after already touching a.cached/a.handID/a.activity.
		actor.cached.PlayersForActor()[0].Stack = 999999
		actor.handID = "hand-after-fabricated"
		actor.activity.Chat = append(actor.activity.Chat, tablestore.ChatMessage{ID: "c2", PlayerID: "p2", Message: "never persisted"})
		delete(actor.activity.Preselections, "p2")
		return sentinel
	})

	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if got := playerStack(t, actor, "p1"); got != 1000 {
		t.Fatalf("p1 stack = %d, want restored 1000 (mutation must not survive a failed handler)", got)
	}
	if actor.handID != "hand-before" {
		t.Fatalf("a.handID = %q, want restored %q", actor.handID, "hand-before")
	}
	if !reflect.DeepEqual(actor.activity, beforeActivity) {
		t.Fatalf("a.activity was not restored to its pre-mutation snapshot:\n got=%+v\nwant=%+v", actor.activity, beforeActivity)
	}
}

// TestHandleJoinRollsBackCacheWhenSettlementIntentFails proves the guard on a
// real production handler, not just the helper in isolation:
// applyJoinAndCommit seats the new player straight into a.cached before its
// settlement intent is built, so a failure building that intent (a wallet
// error surfacing after the seat was already added, never reaching
// a.commit) must not leave the phantom seat trusted in the actor's cache —
// exactly the 2026-09-01 duplicate/ghost-seat incident's failure shape.
func TestHandleJoinRollsBackCacheWhenSettlementIntentFails(t *testing.T) {
	game := hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}}, 10, 20)
	actor := New("rollback-join", nil, true, nil)
	actor.cached = hand.NewTableFromState(game.ExportState())
	actor.handID = "hand-1"
	actor.version = 1

	wantErr := errors.New("settlement intent build failed")
	err := actor.handleJoin(context.Background(), JoinCmd{
		PlayerID: "p2", Stack: 500,
		SettlementIntent: func() (types.TransactWriteItem, error) {
			return types.TransactWriteItem{}, wantErr
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected settlement error, got %v", err)
	}
	if got := playerIDs(actor); !reflect.DeepEqual(got, []string{"p1"}) {
		t.Fatalf("seated players = %v, want [p1] (join never committed)", got)
	}
	if actor.version != 1 {
		t.Fatalf("version changed despite no successful commit: %d", actor.version)
	}
}

// TestHandleLeaveRollsBackCacheWhenSettlementIntentFails is
// TestHandleJoinRollsBackCacheWhenSettlementIntentFails's leave-side mirror:
// applyLeaveAndCommit removes the player from a.cached before building the
// settlement intent, so a failure there must restore the seat rather than
// leaving the player silently vanished from the actor's cache with nothing
// ever persisted.
func TestHandleLeaveRollsBackCacheWhenSettlementIntentFails(t *testing.T) {
	game := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000},
		{ID: "p2", Stack: 1000},
	}, 10, 20)
	actor := New("rollback-leave", nil, true, nil)
	actor.cached = hand.NewTableFromState(game.ExportState())
	actor.handID = "hand-1"
	actor.version = 1

	wantErr := errors.New("settlement intent build failed")
	err := actor.handleLeave(context.Background(), LeaveCmd{
		PlayerID: "p1",
		SettlementIntent: func(stack int64, holdID string) (types.TransactWriteItem, error) {
			return types.TransactWriteItem{}, wantErr
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected settlement error, got %v", err)
	}
	got := playerIDs(actor)
	if len(got) != 2 || got[0] != "p1" || got[1] != "p2" {
		t.Fatalf("seated players = %v, want [p1 p2] (leave never committed)", got)
	}
	if actor.version != 1 {
		t.Fatalf("version changed despite no successful commit: %d", actor.version)
	}
}
