package v1

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"gopkg.aoctech.app/api-commons/ws"
)

type fakeCloser struct{ closed atomic.Bool }

func (f *fakeCloser) Close() error {
	f.closed.Store(true)
	return nil
}

// A lock closes every socket of that player (table + gateway), and only those.
func TestCloseLockedSocketsClosesOnlyThatPlayersSockets(t *testing.T) {
	reg := ws.NewMemoryRegistry()
	table, gateway, other, gone := &fakeCloser{}, &fakeCloser{}, &fakeCloser{}, &fakeCloser{}
	defer watchErasureLock(reg, "u1", "table-conn", table)()
	defer watchErasureLock(reg, "u1", "gateway-conn", gateway)()
	defer watchErasureLock(reg, "u2", "other-conn", other)()
	watchErasureLock(reg, "u1", "closed-conn", gone)() // disconnected before the lock: unregistered

	CloseLockedSockets(reg)(context.Background(), "u1")

	deadline := time.Now().Add(time.Second)
	for !(table.closed.Load() && gateway.closed.Load()) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !table.closed.Load() || !gateway.closed.Load() {
		t.Fatal("every socket of the locked player must be closed")
	}
	if other.closed.Load() {
		t.Fatal("another player's socket was closed")
	}
	if gone.closed.Load() {
		t.Fatal("an unregistered socket was touched")
	}
}
