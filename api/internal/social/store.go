package social

import "context"

// EdgeStore owns all mirrored relationship transitions. Implementations must
// use one conditional DynamoDB transaction for both directed rows.
type EdgeStore interface {
	Get(ctx context.Context, ownerPlayerID, otherPlayerID string) (*Edge, error)
	GetMany(ctx context.Context, ownerPlayerID string, otherPlayerIDs []string) (map[string]Edge, error)
	List(ctx context.Context, ownerPlayerID string, relationship Relationship, cursor string, limit int) (Page[Edge], error)
	Apply(ctx context.Context, transition Transition) (*Edge, error)
}

// EventStore is the durable in-app inbox boundary. It intentionally exposes no
// cross-recipient scan to the public API layer.
type EventStore interface {
	Create(ctx context.Context, event Event, idempotencyKey string) (*Event, error)
	Get(ctx context.Context, recipientPlayerID, eventID string) (*Event, error)
	List(ctx context.Context, recipientPlayerID, cursor string, limit int) (Page[Event], error)
	MarkRead(ctx context.Context, recipientPlayerID string, eventIDs []string) error
}
