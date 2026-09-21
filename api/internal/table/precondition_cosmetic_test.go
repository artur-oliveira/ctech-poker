package table

import (
	"context"
	"strings"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

// A table's version is a single optimistic-concurrency counter bumped by every
// commit, including ones that change nothing any player can see: peek_cards
// never broadcasts at all, and a reaction only fans out its own dedicated
// frame. Clients therefore hold a version the server has already moved past
// through no gameplay event, and their next act was rejected as
// "stale action state" — 23 times in 40 minutes in the 2026-09-21 capture of
// table 01M327ZR25JS10AMJ7NWMK5YSE.
func TestCosmeticCommitsDoNotStaleOutAPlayersAction(t *testing.T) {
	for _, tc := range []struct {
		name string
		bump func(*Actor, string) error
	}{
		{"peek_cards", func(a *Actor, playerID string) error {
			return a.handlePeekCards(context.Background(), PeekCardsCmd{PlayerID: playerID, ActionID: "peek-1"})
		}},
		{"chat", func(a *Actor, playerID string) error {
			return a.handleChat(context.Background(), ChatCmd{PlayerID: playerID, ActionID: "chat-1", Message: "oi"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, current, other := actorMidHand(t)
			seen := uint64(a.version)

			if err := tc.bump(a, other); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if uint64(a.version) == seen {
				t.Skipf("%s did not advance the version; nothing to guard against", tc.name)
			}

			if err := a.handleAct(context.Background(), ActCmd{
				PlayerID: current, ActionID: "act-1", Action: betting.ActionFold,
				ExpectedSnapshotVersion: seen, ExpectedHandID: a.handID,
			}); err != nil {
				t.Fatalf("act rejected after a purely cosmetic version bump: %v", err)
			}
		})
	}
}

// The tolerance is scoped strictly to cosmetic drift: a real gameplay commit
// still invalidates whatever the client thought it was acting on.
func TestGameplayCommitStillStalesOutAnOlderAction(t *testing.T) {
	a, current, _ := actorMidHand(t)
	seen := uint64(a.version)

	if err := a.handleAct(context.Background(), ActCmd{
		PlayerID: current, ActionID: "act-1", Action: betting.ActionFold,
		ExpectedSnapshotVersion: seen, ExpectedHandID: a.handID,
	}); err != nil {
		t.Fatalf("first act: %v", err)
	}
	next := a.cached.CurrentPlayerIDForActor()
	err := a.handleAct(context.Background(), ActCmd{
		PlayerID: next, ActionID: "act-2", Action: betting.ActionFold,
		ExpectedSnapshotVersion: seen, ExpectedHandID: a.handID,
	})
	if err == nil || !strings.Contains(err.Error(), "stale action state") {
		t.Fatalf("expected stale-state rejection across a gameplay commit, got %v", err)
	}
}

// actorMidHand returns a store-less actor on a live three-handed pre-flop
// round, plus the player on the clock and one who is not.
func actorMidHand(t *testing.T) (*Actor, string, string) {
	t.Helper()
	tableState := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
		{ID: "p3", Stack: 1000, Ready: true},
	}, 10, 20)
	if err := tableState.StartHand(); err != nil {
		t.Fatal(err)
	}
	a := New("cosmetic", nil, true, nil)
	a.SetCachedForTest(tableState)
	a.version = 7
	a.gameplayVersion = 7
	a.handID = "hand-current"
	current := tableState.CurrentPlayerIDForActor()
	other := "p1"
	if current == other {
		other = "p2"
	}
	return a, current, other
}
