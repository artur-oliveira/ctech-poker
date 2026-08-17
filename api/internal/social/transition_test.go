package social

import (
	"errors"
	"testing"
	"time"
)

var transitionTime = time.Date(2026, 8, 16, 21, 0, 0, 0, time.UTC)

func TestRequestCreatesMirroredRowsAndCrossedRequestConverges(t *testing.T) {
	request := Transition{ActorPlayerID: "a", TargetPlayerID: "b", Kind: TransitionRequest, OccurredAt: transitionTime}
	a, b, err := PlanTransition(nil, nil, request)
	if err != nil {
		t.Fatal(err)
	}
	assertPair(t, a, b, RelationshipOutgoing, RelationshipIncoming)

	crossed := Transition{ActorPlayerID: "b", TargetPlayerID: "a", Kind: TransitionRequest, OccurredAt: transitionTime.Add(time.Second)}
	b, a, err = PlanTransition(b, a, crossed)
	if err != nil {
		t.Fatal(err)
	}
	assertPair(t, a, b, RelationshipFriend, RelationshipFriend)
	if a.FriendsSince == 0 || a.FriendsSince != b.FriendsSince {
		t.Fatalf("friends_since not mirrored: a=%d b=%d", a.FriendsSince, b.FriendsSince)
	}
}

func TestAcceptIsIdempotentAndNeverProducesUnilateralFriendship(t *testing.T) {
	a := &Edge{OwnerPlayerID: "a", OtherPlayerID: "b", Relationship: RelationshipIncoming, Version: 2}
	b := &Edge{OwnerPlayerID: "b", OtherPlayerID: "a", Relationship: RelationshipOutgoing, Version: 3}
	transition := Transition{ActorPlayerID: "a", TargetPlayerID: "b", Kind: TransitionAccept, OccurredAt: transitionTime}
	afterA, afterB, err := PlanTransition(a, b, transition)
	if err != nil {
		t.Fatal(err)
	}
	assertPair(t, afterA, afterB, RelationshipFriend, RelationshipFriend)

	retriedA, retriedB, err := PlanTransition(afterA, afterB, transition)
	if err != nil {
		t.Fatal(err)
	}
	if retriedA.Version != afterA.Version || retriedB.Version != afterB.Version {
		t.Fatalf("idempotent accept changed versions: %d/%d -> %d/%d", afterA.Version, afterB.Version, retriedA.Version, retriedB.Version)
	}
}

func TestBlockClearsRelationshipButPreservesLocalSafetyFlags(t *testing.T) {
	a := &Edge{OwnerPlayerID: "a", OtherPlayerID: "b", Relationship: RelationshipFriend, Version: 1}
	b := &Edge{OwnerPlayerID: "b", OtherPlayerID: "a", Relationship: RelationshipFriend, Muted: true, Version: 4}
	afterA, afterB, err := PlanTransition(a, b, Transition{ActorPlayerID: "a", TargetPlayerID: "b", Kind: TransitionBlock, OccurredAt: transitionTime})
	if err != nil {
		t.Fatal(err)
	}
	if afterA == nil || !afterA.Blocked || !afterA.Muted || afterA.Relationship != RelationshipNone {
		t.Fatalf("unexpected blocker edge: %+v", afterA)
	}
	if afterB == nil || !afterB.Muted || afterB.Blocked || afterB.Relationship != RelationshipNone {
		t.Fatalf("target safety flags were not preserved: %+v", afterB)
	}

	unblocked, _, err := PlanTransition(afterA, afterB, Transition{ActorPlayerID: "a", TargetPlayerID: "b", Kind: TransitionUnblock, OccurredAt: transitionTime.Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if unblocked.Blocked || !unblocked.Muted {
		t.Fatalf("unblock must preserve mute: %+v", unblocked)
	}
}

func TestBlockedPlayersCannotRequestOrUnmute(t *testing.T) {
	blocked := &Edge{OwnerPlayerID: "a", OtherPlayerID: "b", Blocked: true, Muted: true, Version: 1}
	for _, kind := range []TransitionKind{TransitionRequest, TransitionUnmute} {
		_, _, err := PlanTransition(blocked, nil, Transition{ActorPlayerID: "a", TargetPlayerID: "b", Kind: kind})
		if !errors.Is(err, ErrRelationshipConflict) {
			t.Fatalf("kind=%s err=%v", kind, err)
		}
	}
}

func TestInconsistentPairFailsClosed(t *testing.T) {
	a := &Edge{OwnerPlayerID: "a", OtherPlayerID: "b", Relationship: RelationshipFriend, Version: 1}
	_, _, err := PlanTransition(a, nil, Transition{ActorPlayerID: "a", TargetPlayerID: "b", Kind: TransitionRemove})
	if !errors.Is(err, ErrRelationshipConflict) {
		t.Fatalf("expected relationship conflict, got %v", err)
	}
}

func TestBlockRepairsInconsistentPair(t *testing.T) {
	a := &Edge{OwnerPlayerID: "a", OtherPlayerID: "b", Relationship: RelationshipFriend, Version: 3}
	b := &Edge{OwnerPlayerID: "b", OtherPlayerID: "a", Relationship: RelationshipNone, Version: 2}

	afterA, afterB, err := PlanTransition(a, b, Transition{
		ActorPlayerID: "a", TargetPlayerID: "b", Kind: TransitionBlock, OccurredAt: transitionTime,
	})
	if err != nil {
		t.Fatalf("block should repair an inconsistent relationship pair: %v", err)
	}
	if afterA == nil || !afterA.Blocked || !afterA.Muted || afterA.Relationship != RelationshipNone {
		t.Fatalf("unexpected blocker edge: %+v", afterA)
	}
	if afterB != nil {
		t.Fatalf("target edge should be removed after relationship repair: %+v", afterB)
	}
}

func assertPair(t *testing.T, a, b *Edge, wantA, wantB Relationship) {
	t.Helper()
	if a == nil || b == nil || a.Relationship != wantA || b.Relationship != wantB {
		t.Fatalf("pair=%+v / %+v want=%s/%s", a, b, wantA, wantB)
	}
}
