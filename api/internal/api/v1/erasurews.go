package v1

import (
	"context"
	"io"

	"gopkg.aoctech.app/api-commons/ws"
)

// erasureLockKey is the ws.Registry key every socket of a player also listens
// on. A broadcast there reaches the player's sockets on every instance
// (Valkey pub/sub fan-out), which is what lets the replica that consumed the
// lock close sockets held by its siblings (plan ruling R6).
func erasureLockKey(sub string) string { return "erasure-lock#" + sub }

// lockCloser is the ws.Conn registered under erasureLockKey: anything
// broadcast there closes its socket. The gateway's read loop then fails and
// runs its usual cleanup (unregister, presence, table disconnect).
type lockCloser struct{ conn io.Closer }

func (l lockCloser) WriteMessage(int, []byte) error { return l.conn.Close() }

// watchErasureLock registers conn to be closed when sub is locked and
// returns the matching unregister.
func watchErasureLock(reg ws.Registry, sub, connID string, conn io.Closer) func() {
	key := erasureLockKey(sub)
	reg.Register(key, connID, lockCloser{conn: conn})
	return func() { reg.Unregister(key, connID) }
}

// CloseLockedSockets is the erasure.Consumer OnLock hook: it closes every
// table socket (seated or spectating) and gateway socket of sub, fleet-wide.
func CloseLockedSockets(reg ws.Registry) func(ctx context.Context, sub string) {
	return func(ctx context.Context, sub string) {
		reg.Broadcast(ctx, erasureLockKey(sub), []byte("account_locked"))
	}
}
