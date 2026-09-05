package social

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
)

const (
	MaxFriends         = 200
	MaxPendingOutgoing = 50
	InviteLifetime     = 15 * time.Minute
)

var (
	ErrFeatureDisabled     = errors.New("social: graph is disabled")
	ErrFriendLimitReached  = errors.New("social: friend limit reached")
	ErrRequestLimitReached = errors.New("social: pending request limit reached")
	ErrInviteForbidden     = errors.New("social: invite is not allowed")
	ErrRoomClosed          = errors.New("social: room is closed")
	ErrRoomFull            = errors.New("social: room is full")
)

type inviteRoomStore interface {
	Get(context.Context, string) (*roomstore.Room, error)
}

type openSessionStore interface {
	FindOpenSession(context.Context, string, string) (*sessionlog.SessionItem, error)
}

type NotifyFunc func(context.Context, Event, int)

type Service struct {
	store    EdgeStore
	events   EventStore
	rooms    inviteRoomStore
	sessions openSessionStore
	notify   NotifyFunc
	enabled  bool
	now      func() time.Time
}

func NewService(store EdgeStore, enabled bool) *Service {
	return &Service{store: store, enabled: enabled, now: time.Now}
}

func (s *Service) WithInbox(events EventStore) *Service {
	s.events = events
	return s
}

func (s *Service) WithInvites(rooms inviteRoomStore, sessions openSessionStore) *Service {
	s.rooms, s.sessions = rooms, sessions
	return s
}

func (s *Service) WithNotifier(notify NotifyFunc) *Service {
	s.notify = notify
	return s
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
		count, err := s.store.Count(ctx, actorID, RelationshipOutgoing, MaxPendingOutgoing)
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
	edge, err := s.apply(ctx, actorID, targetID, TransitionRequest, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if s.events != nil {
		eventType := EventFriendRequest
		if edge.Relationship == RelationshipFriend {
			eventType = EventFriendAccepted
		}
		if _, err := s.createEvent(ctx, Event{RecipientPlayerID: targetID, ActorPlayerID: actorID, Type: eventType, Status: friendEventStatus(eventType)}, idempotencyKey); err != nil {
			return nil, err
		}
	}
	return edge, nil
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
	edge, err := s.apply(ctx, actorID, targetID, TransitionAccept, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if s.events != nil {
		if _, err := s.createEvent(ctx, Event{RecipientPlayerID: targetID, ActorPlayerID: actorID, Type: EventFriendAccepted, Status: EventStatusAccepted}, idempotencyKey); err != nil {
			return nil, err
		}
	}
	return edge, nil
}

func (s *Service) ListInbox(ctx context.Context, actorID string, limit int, startKey map[string]types.AttributeValue) ([]Event, map[string]types.AttributeValue, error) {
	if s.events == nil {
		return nil, nil, errors.New("social: inbox is not configured")
	}
	return s.events.List(ctx, actorID, limit, startKey)
}

func (s *Service) UnreadCount(ctx context.Context, actorID string) (int, error) {
	if s.events == nil {
		return 0, errors.New("social: inbox is not configured")
	}
	return s.events.UnreadCount(ctx, actorID)
}

func (s *Service) MarkInboxRead(ctx context.Context, actorID string, eventIDs []string) error {
	if s.events == nil {
		return errors.New("social: inbox is not configured")
	}
	if err := s.events.MarkRead(ctx, actorID, eventIDs); err != nil {
		return err
	}
	s.notifyUnread(ctx, actorID)
	return nil
}

func (s *Service) SendTableInvite(ctx context.Context, actorID, targetID, roomID, idempotencyKey string) (*Event, error) {
	if err := s.requireGraph(actorID, targetID); err != nil {
		return nil, err
	}
	if s.events == nil || s.rooms == nil || s.sessions == nil {
		return nil, errors.New("social: invitations are not configured")
	}
	actorEdge, err := s.store.Get(ctx, actorID, targetID)
	if err != nil {
		return nil, err
	}
	targetEdge, err := s.store.Get(ctx, targetID, actorID)
	if err != nil {
		return nil, err
	}
	if relationshipOf(actorEdge) != RelationshipFriend || relationshipOf(targetEdge) != RelationshipFriend || isBlocked(actorEdge) || isBlocked(targetEdge) {
		return nil, ErrInviteForbidden
	}
	room, err := s.rooms.Get(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if room == nil || (room.Status != "waiting" && room.Status != "active") {
		return nil, ErrRoomClosed
	}
	session, err := s.sessions.FindOpenSession(ctx, actorID, roomID)
	if err != nil {
		return nil, err
	}
	if session == nil || session.EndedAt != 0 {
		return nil, ErrInviteForbidden
	}
	now := s.now().UTC()
	event := Event{
		RecipientPlayerID: targetID, ActorPlayerID: actorID, Type: EventTableInvite,
		RoomID: roomID, Status: EventStatusPending, CreatedAt: now.UnixMilli(), ExpiresAt: now.Add(InviteLifetime).UnixMilli(),
	}
	created, err := s.events.CreateInvite(ctx, event, idempotencyKey)
	if errors.Is(err, ErrInviteAlreadySent) && created != nil && created.EventID == deterministicEventID(event, idempotencyKey) {
		err = nil
	}
	if err != nil {
		return nil, err
	}
	s.notifyEvent(ctx, *created)
	return created, nil
}

func (s *Service) AcceptTableInvite(ctx context.Context, actorID, eventID string) (*Event, *roomstore.Room, error) {
	if !s.enabled {
		return nil, nil, ErrFeatureDisabled
	}
	if s.events == nil || s.rooms == nil {
		return nil, nil, errors.New("social: invitations are not configured")
	}
	event, err := s.events.Get(ctx, actorID, eventID)
	if err != nil {
		return nil, nil, err
	}
	if event.Type != EventTableInvite {
		return nil, nil, ErrInviteNotPending
	}
	if event.ExpiresAt <= s.now().UTC().UnixMilli() {
		return nil, nil, ErrInviteExpired
	}
	if event.Status == EventStatusAccepted {
		room, err := s.rooms.Get(ctx, event.RoomID)
		if err != nil {
			return nil, nil, err
		}
		if room == nil || (room.Status != "waiting" && room.Status != "active") {
			return nil, nil, ErrRoomClosed
		}
		if room.SeatsTaken >= room.MaxSeats {
			return nil, nil, ErrRoomFull
		}
		return event, room, nil
	}
	actorEdge, err := s.store.Get(ctx, actorID, event.ActorPlayerID)
	if err != nil {
		return nil, nil, err
	}
	senderEdge, err := s.store.Get(ctx, event.ActorPlayerID, actorID)
	if err != nil {
		return nil, nil, err
	}
	if relationshipOf(actorEdge) != RelationshipFriend || relationshipOf(senderEdge) != RelationshipFriend || isBlocked(actorEdge) || isBlocked(senderEdge) {
		return nil, nil, ErrInviteForbidden
	}
	room, err := s.rooms.Get(ctx, event.RoomID)
	if err != nil {
		return nil, nil, err
	}
	if room == nil || (room.Status != "waiting" && room.Status != "active") {
		return nil, nil, ErrRoomClosed
	}
	if room.SeatsTaken >= room.MaxSeats {
		return nil, nil, ErrRoomFull
	}
	accepted, err := s.events.AcceptInvite(ctx, *event, s.now().UTC())
	if err != nil {
		return nil, nil, err
	}
	s.notifyUnread(ctx, actorID)
	return accepted, room, nil
}

func (s *Service) DeclineTableInvite(ctx context.Context, actorID, eventID string) (*Event, error) {
	if !s.enabled {
		return nil, ErrFeatureDisabled
	}
	if s.events == nil {
		return nil, errors.New("social: inbox is not configured")
	}
	event, err := s.events.Get(ctx, actorID, eventID)
	if err != nil {
		return nil, err
	}
	if event.Type != EventTableInvite {
		return nil, ErrInviteNotPending
	}
	if event.ExpiresAt <= s.now().UTC().UnixMilli() {
		return nil, ErrInviteExpired
	}
	if event.Status == EventStatusDeclined {
		return event, nil
	}
	declined, err := s.events.DeclineInvite(ctx, *event, s.now().UTC())
	if err != nil {
		return nil, err
	}
	s.notifyUnread(ctx, actorID)
	return declined, nil
}

// friend_accepted is a notice, not a prompt: nothing can be done to it, so it
// is born terminal. Only friend_request stays pending, answered from the
// requests tab rather than from the inbox row.
func friendEventStatus(eventType EventType) EventStatus {
	if eventType == EventFriendAccepted {
		return EventStatusAccepted
	}
	return EventStatusPending
}

func (s *Service) createEvent(ctx context.Context, event Event, idempotencyKey string) (*Event, error) {
	created, err := s.events.Create(ctx, event, idempotencyKey)
	if err == nil {
		s.notifyEvent(ctx, *created)
	}
	return created, err
}

func (s *Service) notifyEvent(ctx context.Context, event Event) {
	if s.notify == nil {
		return
	}
	count, err := s.events.UnreadCount(ctx, event.RecipientPlayerID)
	if err == nil {
		s.notify(ctx, event, count)
	}
}

func (s *Service) notifyUnread(ctx context.Context, recipientID string) {
	if s.notify == nil {
		return
	}
	count, err := s.events.UnreadCount(ctx, recipientID)
	if err == nil {
		s.notify(ctx, Event{RecipientPlayerID: recipientID}, count)
	}
}

func (s *Service) ListFriends(ctx context.Context, actorID string, limit int, startKey map[string]types.AttributeValue) ([]Edge, map[string]types.AttributeValue, error) {
	if !s.enabled {
		return nil, nil, ErrFeatureDisabled
	}
	return s.store.List(ctx, actorID, RelationshipFriend, false, limit, startKey)
}

// FriendIDs returns the complete bounded friend set for presence fan-out.
// MaxFriends caps this at 200, while pagination avoids depending on a single
// DynamoDB response page.
func (s *Service) FriendIDs(ctx context.Context, actorID string) ([]string, error) {
	if !s.enabled {
		return nil, nil
	}
	ids := make([]string, 0)
	var startKey map[string]types.AttributeValue
	for {
		edges, next, err := s.store.List(ctx, actorID, RelationshipFriend, false, 50, startKey)
		if err != nil {
			return nil, err
		}
		for _, edge := range edges {
			ids = append(ids, edge.OtherPlayerID)
		}
		if len(next) == 0 {
			return ids, nil
		}
		startKey = next
	}
}

// BlockedInEitherDirection returns opponent IDs that must be omitted from
// recent-player discovery. It deliberately does not expose which direction
// contained the block.
func (s *Service) BlockedInEitherDirection(ctx context.Context, actorID string, opponentIDs []string) (map[string]bool, error) {
	if batch, ok := s.store.(interface {
		BlockedInEitherDirection(context.Context, string, []string) (map[string]bool, error)
	}); ok {
		return batch.BlockedInEitherDirection(ctx, actorID, opponentIDs)
	}
	blocked := make(map[string]bool)
	for _, opponentID := range opponentIDs {
		forward, err := s.store.Get(ctx, actorID, opponentID)
		if err != nil {
			return nil, err
		}
		reverse, err := s.store.Get(ctx, opponentID, actorID)
		if err != nil {
			return nil, err
		}
		blocked[opponentID] = (forward != nil && forward.Blocked) || (reverse != nil && reverse.Blocked)
	}
	return blocked, nil
}

func (s *Service) Relationships(ctx context.Context, actorID string, otherIDs []string) (map[string]Edge, error) {
	if batch, ok := s.store.(interface {
		GetManyFromOwner(context.Context, string, []string) (map[string]Edge, error)
	}); ok {
		return batch.GetManyFromOwner(ctx, actorID, otherIDs)
	}
	result := make(map[string]Edge, len(otherIDs))
	for _, otherID := range otherIDs {
		edge, err := s.store.Get(ctx, actorID, otherID)
		if err != nil {
			return nil, err
		}
		if edge != nil {
			result[otherID] = *edge
		}
	}
	return result, nil
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
		count, err := s.store.Count(ctx, playerID, RelationshipFriend, MaxFriends)
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
