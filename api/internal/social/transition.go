package social

import (
	"errors"
	"time"
)

var (
	ErrRelationshipConflict = errors.New("social: relationship conflict")
	ErrSelfRelationship     = errors.New("social: cannot create a relationship with self")
)

func PlanTransition(actorBefore, targetBefore *Edge, transition Transition) (*Edge, *Edge, error) {
	if transition.ActorPlayerID == "" || transition.TargetPlayerID == "" || transition.ActorPlayerID == transition.TargetPlayerID {
		return nil, nil, ErrSelfRelationship
	}
	// Safety actions must remain available even if an older write left the
	// mirrored relationship in an inconsistent state. Blocking repairs both
	// relationship sides; mute/unmute/unblock only change the actor's flags.
	if !isSafetyTransition(transition.Kind) && !relationshipPairValid(actorBefore, targetBefore) {
		return nil, nil, ErrRelationshipConflict
	}
	now := transition.OccurredAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	nowMillis := now.UnixMilli()
	actor := cloneEdge(actorBefore)
	target := cloneEdge(targetBefore)
	ensureActor := func() {
		if actor == nil {
			actor = &Edge{OwnerPlayerID: transition.ActorPlayerID, OtherPlayerID: transition.TargetPlayerID}
		}
	}
	ensureTarget := func() {
		if target == nil {
			target = &Edge{OwnerPlayerID: transition.TargetPlayerID, OtherPlayerID: transition.ActorPlayerID}
		}
	}

	switch transition.Kind {
	case TransitionRequest:
		if isBlocked(actor) || isBlocked(target) {
			return nil, nil, ErrRelationshipConflict
		}
		switch relationshipOf(actor) {
		case RelationshipFriend, RelationshipOutgoing:
			// Idempotent retry.
		case RelationshipIncoming:
			ensureActor()
			ensureTarget()
			makeFriends(actor, target, nowMillis)
		case RelationshipNone:
			if relationshipOf(target) != RelationshipNone {
				return nil, nil, ErrRelationshipConflict
			}
			ensureActor()
			ensureTarget()
			actor.Relationship = RelationshipOutgoing
			target.Relationship = RelationshipIncoming
			actor.RequestedAt, target.RequestedAt = nowMillis, nowMillis
		default:
			return nil, nil, ErrRelationshipConflict
		}

	case TransitionAccept:
		if isBlocked(actor) || isBlocked(target) {
			return nil, nil, ErrRelationshipConflict
		}
		if relationshipOf(actor) == RelationshipFriend {
			break
		}
		if relationshipOf(actor) != RelationshipIncoming || relationshipOf(target) != RelationshipOutgoing {
			return nil, nil, ErrRelationshipConflict
		}
		makeFriends(actor, target, nowMillis)

	case TransitionDecline:
		if relationshipOf(actor) == RelationshipNone && relationshipOf(target) == RelationshipNone {
			break
		}
		if relationshipOf(actor) != RelationshipIncoming || relationshipOf(target) != RelationshipOutgoing {
			return nil, nil, ErrRelationshipConflict
		}
		clearRelationship(actor)
		clearRelationship(target)

	case TransitionCancel:
		if relationshipOf(actor) == RelationshipNone && relationshipOf(target) == RelationshipNone {
			break
		}
		if relationshipOf(actor) != RelationshipOutgoing || relationshipOf(target) != RelationshipIncoming {
			return nil, nil, ErrRelationshipConflict
		}
		clearRelationship(actor)
		clearRelationship(target)

	case TransitionRemove:
		if relationshipOf(actor) == RelationshipNone && relationshipOf(target) == RelationshipNone {
			break
		}
		if relationshipOf(actor) != RelationshipFriend || relationshipOf(target) != RelationshipFriend {
			return nil, nil, ErrRelationshipConflict
		}
		clearRelationship(actor)
		clearRelationship(target)

	case TransitionMute:
		ensureActor()
		actor.Muted = true

	case TransitionUnmute:
		if actor == nil {
			break
		}
		if actor.Blocked {
			return nil, nil, ErrRelationshipConflict
		}
		actor.Muted = false

	case TransitionBlock:
		ensureActor()
		actor.Blocked, actor.Muted = true, true
		clearRelationship(actor)
		if target != nil {
			clearRelationship(target)
		}

	case TransitionUnblock:
		if actor == nil {
			break
		}
		actor.Blocked = false

	default:
		return nil, nil, ErrRelationshipConflict
	}

	actor = finalizeEdge(actorBefore, actor, nowMillis)
	target = finalizeEdge(targetBefore, target, nowMillis)
	return actor, target, nil
}

func isSafetyTransition(kind TransitionKind) bool {
	switch kind {
	case TransitionMute, TransitionUnmute, TransitionBlock, TransitionUnblock:
		return true
	default:
		return false
	}
}

func relationshipPairValid(actor, target *Edge) bool {
	a, b := relationshipOf(actor), relationshipOf(target)
	switch a {
	case RelationshipNone:
		return b == RelationshipNone
	case RelationshipOutgoing:
		return b == RelationshipIncoming
	case RelationshipIncoming:
		return b == RelationshipOutgoing
	case RelationshipFriend:
		return b == RelationshipFriend
	default:
		return false
	}
}

func relationshipOf(edge *Edge) Relationship {
	if edge == nil {
		return RelationshipNone
	}
	return edge.Relationship
}

func isBlocked(edge *Edge) bool { return edge != nil && edge.Blocked }

func cloneEdge(edge *Edge) *Edge {
	if edge == nil {
		return nil
	}
	copy := *edge
	return &copy
}

func makeFriends(actor, target *Edge, now int64) {
	actor.Relationship, target.Relationship = RelationshipFriend, RelationshipFriend
	actor.RequestedAt, target.RequestedAt = 0, 0
	actor.FriendsSince, target.FriendsSince = now, now
}

func clearRelationship(edge *Edge) {
	if edge == nil {
		return
	}
	edge.Relationship = RelationshipNone
	edge.RequestedAt = 0
	edge.FriendsSince = 0
}

func finalizeEdge(before, after *Edge, now int64) *Edge {
	if after != nil && after.Relationship == RelationshipNone && !after.Muted && !after.Blocked {
		after = nil
	}
	if edgeEqual(before, after) {
		return after
	}
	if after != nil {
		after.UpdatedAt = now
		if before == nil {
			after.Version = 1
		} else {
			after.Version = before.Version + 1
		}
	}
	return after
}

func edgeEqual(left, right *Edge) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
