package social

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const (
	MaxFriends         = 200
	MaxPendingOutgoing = 50
)

var (
	ErrFeatureDisabled     = errors.New("social: graph is disabled")
	ErrFriendLimitReached  = errors.New("social: friend limit reached")
	ErrRequestLimitReached = errors.New("social: pending request limit reached")
)

type Service struct {
	store   EdgeStore
	enabled bool
	now     func() time.Time
}

func NewService(store EdgeStore, enabled bool) *Service {
	return &Service{store: store, enabled: enabled, now: time.Now}
}

func (s *Service) Relationship(ctx context.Context, ownerID, otherID string) (*Edge, error) {
	if ownerID == otherID || ownerID == "" || otherID == "" {
		return nil, ErrSelfRelationship
	}
	return s.store.Get(ctx, ownerID, otherID)
}

func (s *Service) Request(ctx context.Context, actorID, targetID, idempotencyKey string) (*Edge, error) {
	if err := s.requireGraph(actorID, targetID); err != nil {
		return nil, err
	}
	current, err := s.store.Get(ctx, actorID, targetID)
	if err != nil {
		return nil, err
	}
	if relationshipOf(current) == RelationshipNone {
		count, err := s.store.Count(ctx, actorID, RelationshipOutgoing)
		if err != nil {
			return nil, err
		}
		if count >= MaxPendingOutgoing {
			return nil, ErrRequestLimitReached
		}
	}
	if relationshipOf(current) == RelationshipIncoming {
		if err := s.ensureFriendCapacity(ctx, actorID, targetID); err != nil {
			return nil, err
		}
	}
	return s.apply(ctx, actorID, targetID, TransitionRequest, idempotencyKey)
}

func (s *Service) Accept(ctx context.Context, actorID, targetID, idempotencyKey string) (*Edge, error) {
	if err := s.requireGraph(actorID, targetID); err != nil {
		return nil, err
	}
	current, err := s.store.Get(ctx, actorID, targetID)
	if err != nil {
		return nil, err
	}
	if relationshipOf(current) != RelationshipFriend {
		if err := s.ensureFriendCapacity(ctx, actorID, targetID); err != nil {
			return nil, err
		}
	}
	return s.apply(ctx, actorID, targetID, TransitionAccept, idempotencyKey)
}

func (s *Service) ListFriends(ctx context.Context, actorID string, limit int, startKey map[string]types.AttributeValue) ([]Edge, map[string]types.AttributeValue, error) {
	if !s.enabled {
		return nil, nil, ErrFeatureDisabled
	}
	return s.store.List(ctx, actorID, RelationshipFriend, false, limit, startKey)
}

func (s *Service) ListRequests(ctx context.Context, actorID string, direction Relationship, limit int, startKey map[string]types.AttributeValue) ([]Edge, map[string]types.AttributeValue, error) {
	if !s.enabled {
		return nil, nil, ErrFeatureDisabled
	}
	if direction != RelationshipIncoming && direction != RelationshipOutgoing {
		return nil, nil, ErrRelationshipConflict
	}
	return s.store.List(ctx, actorID, direction, false, limit, startKey)
}

func (s *Service) ListBlocked(ctx context.Context, actorID string, limit int, startKey map[string]types.AttributeValue) ([]Edge, map[string]types.AttributeValue, error) {
	return s.store.List(ctx, actorID, RelationshipNone, true, limit, startKey)
}

func (s *Service) ensureFriendCapacity(ctx context.Context, actorID, targetID string) error {
	for _, playerID := range []string{actorID, targetID} {
		count, err := s.store.Count(ctx, playerID, RelationshipFriend)
		if err != nil {
			return err
		}
		if count >= MaxFriends {
			return ErrFriendLimitReached
		}
	}
	return nil
}

func (s *Service) Decline(ctx context.Context, actorID, targetID, idempotencyKey string) (*Edge, error) {
	if err := s.requireGraph(actorID, targetID); err != nil {
		return nil, err
	}
	return s.apply(ctx, actorID, targetID, TransitionDecline, idempotencyKey)
}

func (s *Service) Cancel(ctx context.Context, actorID, targetID, idempotencyKey string) (*Edge, error) {
	if err := s.requireGraph(actorID, targetID); err != nil {
		return nil, err
	}
	return s.apply(ctx, actorID, targetID, TransitionCancel, idempotencyKey)
}

func (s *Service) RemoveFriend(ctx context.Context, actorID, targetID, idempotencyKey string) (*Edge, error) {
	if err := s.requireGraph(actorID, targetID); err != nil {
		return nil, err
	}
	return s.apply(ctx, actorID, targetID, TransitionRemove, idempotencyKey)
}

func (s *Service) Mute(ctx context.Context, actorID, targetID, idempotencyKey string) (*Edge, error) {
	return s.applySafety(ctx, actorID, targetID, TransitionMute, idempotencyKey)
}

func (s *Service) Unmute(ctx context.Context, actorID, targetID, idempotencyKey string) (*Edge, error) {
	return s.applySafety(ctx, actorID, targetID, TransitionUnmute, idempotencyKey)
}

func (s *Service) Block(ctx context.Context, actorID, targetID, idempotencyKey string) (*Edge, error) {
	return s.applySafety(ctx, actorID, targetID, TransitionBlock, idempotencyKey)
}

func (s *Service) Unblock(ctx context.Context, actorID, targetID, idempotencyKey string) (*Edge, error) {
	return s.applySafety(ctx, actorID, targetID, TransitionUnblock, idempotencyKey)
}

func (s *Service) applySafety(ctx context.Context, actorID, targetID string, kind TransitionKind, key string) (*Edge, error) {
	if actorID == "" || targetID == "" || actorID == targetID {
		return nil, ErrSelfRelationship
	}
	return s.apply(ctx, actorID, targetID, kind, key)
}

func (s *Service) apply(ctx context.Context, actorID, targetID string, kind TransitionKind, key string) (*Edge, error) {
	return s.store.Apply(ctx, Transition{
		ActorPlayerID: actorID, TargetPlayerID: targetID, Kind: kind,
		IdempotencyKey: key, OccurredAt: s.now().UTC(),
	})
}

func (s *Service) requireGraph(actorID, targetID string) error {
	if !s.enabled {
		return ErrFeatureDisabled
	}
	if actorID == "" || targetID == "" || actorID == targetID {
		return ErrSelfRelationship
	}
	return nil
}
