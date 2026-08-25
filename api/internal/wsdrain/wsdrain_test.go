package wsdrain

import (
	"context"
	"sync"
	"testing"
	"time"

	fws "github.com/fasthttp/websocket"
)

type fakeConn struct {
	mu     sync.Mutex
	frames [][]byte
	types  []int
}

func (f *fakeConn) WriteControl(messageType int, data []byte, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.types = append(f.types, messageType)
	f.frames = append(f.frames, append([]byte(nil), data...))
	return nil
}

func TestCloseAllSendsGoingAwayToTrackedConnsOnly(t *testing.T) {
	tracked, untracked := &fakeConn{}, &fakeConn{}
	Track(tracked)
	Track(untracked)
	Untrack(untracked)
	t.Cleanup(func() { Untrack(tracked) })

	if got := Live(); got != 1 {
		t.Fatalf("Live() = %d, want 1", got)
	}

	// ctx already cancelled: CloseAll must still write the frames and return
	// immediately instead of burning the grace window.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if n := CloseAll(ctx, time.Minute); n != 1 {
		t.Fatalf("CloseAll signalled %d conns, want 1", n)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("CloseAll waited %s despite a cancelled context", elapsed)
	}

	if len(untracked.frames) != 0 {
		t.Fatalf("untracked conn got %d frames, want 0", len(untracked.frames))
	}
	if len(tracked.frames) != 1 || tracked.types[0] != fws.CloseMessage {
		t.Fatalf("tracked conn got types=%v frames=%d, want one CloseMessage", tracked.types, len(tracked.frames))
	}
	// A close payload is a 2-byte big-endian status code, then the reason.
	if code := int(tracked.frames[0][0])<<8 | int(tracked.frames[0][1]); code != fws.CloseGoingAway {
		t.Fatalf("close code = %d, want %d (going away)", code, fws.CloseGoingAway)
	}
}

func TestCloseAllNoConnsIsANoop(t *testing.T) {
	if n := CloseAll(context.Background(), time.Minute); n != 0 {
		t.Fatalf("CloseAll on an empty registry signalled %d conns, want 0", n)
	}
}
