package social

import "context"

// EventStore is the durable in-app inbox boundary implemented in PR 3.
type EventStore interface {
	Create(ctx context.Context, event Event, idempotencyKey string) (*Event, error)
	Get(ctx context.Context, recipientPlayerID, eventID string) (*Event, error)
	MarkRead(ctx context.Context, recipientPlayerID string, eventIDs []string) error
}
