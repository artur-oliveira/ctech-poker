// Package social defines the persistent friendship, safety and in-app inbox
// contracts. Implementations arrive in the graph and inbox delivery slices.
package social

import "time"

type Relationship string

const (
	RelationshipNone     Relationship = ""
	RelationshipOutgoing Relationship = "outgoing"
	RelationshipIncoming Relationship = "incoming"
	RelationshipFriend   Relationship = "friend"
)

type Edge struct {
	OwnerPlayerID string       `dynamodbav:"pk"`
	OtherPlayerID string       `dynamodbav:"sk"`
	Relationship  Relationship `dynamodbav:"relationship,omitempty"`
	Muted         bool         `dynamodbav:"muted,omitempty"`
	Blocked       bool         `dynamodbav:"blocked,omitempty"`
	RequestedAt   int64        `dynamodbav:"requested_at,omitempty"`
	FriendsSince  int64        `dynamodbav:"friends_since,omitempty"`
	UpdatedAt     int64        `dynamodbav:"updated_at"`
}

type TransitionKind string

const (
	TransitionRequest TransitionKind = "request"
	TransitionAccept  TransitionKind = "accept"
	TransitionDecline TransitionKind = "decline"
	TransitionCancel  TransitionKind = "cancel"
	TransitionRemove  TransitionKind = "remove"
	TransitionMute    TransitionKind = "mute"
	TransitionUnmute  TransitionKind = "unmute"
	TransitionBlock   TransitionKind = "block"
	TransitionUnblock TransitionKind = "unblock"
)

type Transition struct {
	ActorPlayerID  string
	TargetPlayerID string
	Kind           TransitionKind
	IdempotencyKey string
	OccurredAt     time.Time
}

type EventType string

const (
	EventFriendRequest  EventType = "friend_request"
	EventFriendAccepted EventType = "friend_accepted"
	EventTableInvite    EventType = "table_invite"
)

type Event struct {
	RecipientPlayerID string    `dynamodbav:"pk"`
	EventID           string    `dynamodbav:"sk"`
	Type              EventType `dynamodbav:"type"`
	ActorPlayerID     string    `dynamodbav:"actor_id"`
	Status            string    `dynamodbav:"status"`
	RoomID            string    `dynamodbav:"room_id,omitempty"`
	Unread            bool      `dynamodbav:"unread,omitempty"`
	CreatedAt         int64     `dynamodbav:"created_at"`
	ExpiresAt         int64     `dynamodbav:"expires_at,omitempty"`
	InboxPartition    string    `dynamodbav:"gsi_inbox_pk"`
	InboxSort         string    `dynamodbav:"gsi_inbox_sk"`
	UnreadPartition   string    `dynamodbav:"gsi_unread_pk,omitempty"`
	UnreadSort        string    `dynamodbav:"gsi_unread_sk,omitempty"`
	TTL               int64     `dynamodbav:"ttl"`
}

type Page[T any] struct {
	Items      []T
	NextCursor string
}
