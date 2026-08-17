package presence

import (
	"context"
	"time"
)

// Store abstracts the Valkey-backed multi-connection presence state. The bool
// results identify real online/offline transitions eligible for friend fanout.
type Store interface {
	Open(ctx context.Context, playerID, connectionID string, expiresAt time.Time) (becameOnline bool, err error)
	Heartbeat(ctx context.Context, playerID, connectionID string, expiresAt time.Time) error
	Close(ctx context.Context, playerID, connectionID string) (becameOffline bool, err error)
	SetInTable(ctx context.Context, playerID string, inTable bool) error
	GetMany(ctx context.Context, playerIDs []string) (map[string]Status, error)
}
