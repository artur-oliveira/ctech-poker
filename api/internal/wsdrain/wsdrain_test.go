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

	// Frames are written concurrently now, so a cancelled context can return
	// before the (unbounded, fire-and-forget) writers have all run.
	waitFrames(t, tracked, 1)
	if got := untracked.frameCount(); got != 0 {
		t.Fatalf("untracked conn got %d frames, want 0", got)
	}
	untracked.mu.Lock()
	defer untracked.mu.Unlock()
	tracked.mu.Lock()
	defer tracked.mu.Unlock()
	if tracked.types[0] != fws.CloseMessage {
		t.Fatalf("tracked conn got types=%v, want one CloseMessage", tracked.types)
	}
	// A close payload is a 2-byte big-endian status code, then the reason.
	if code := int(tracked.frames[0][0])<<8 | int(tracked.frames[0][1]); code != fws.CloseGoingAway {
		t.Fatalf("close code = %d, want %d (going away)", code, fws.CloseGoingAway)
	}
}

func (f *fakeConn) frameCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.frames)
}

func waitFrames(t *testing.T, c *fakeConn, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if c.frameCount() == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("conn got %d frames, want %d", c.frameCount(), want)
}

// blockedConn is a peer that has stopped reading: its write parks until the
// test releases it, exactly the shape that used to serialise closeWriteWait
// in front of every socket queued behind it.
type blockedConn struct{ release chan struct{} }

func (b *blockedConn) WriteControl(int, []byte, time.Time) error {
	<-b.release
	return nil
}

// TestCloseAllStalledPeersDoNotDelayHealthySockets pins the ceiling issue #226
// asks for: the whole phase costs the caller its grace budget and no more, and
// every healthy socket is signalled inside it even with more stalled peers than
// the fan-out has workers.
func TestCloseAllStalledPeersDoNotDelayHealthySockets(t *testing.T) {
	const (
		healthy = 200
		stalled = 64
		grace   = 150 * time.Millisecond
	)
	release := make(chan struct{})
	defer close(release)

	good := make([]*fakeConn, healthy)
	for i := range good {
		good[i] = &fakeConn{}
		Track(good[i])
	}
	blocked := make([]*blockedConn, stalled)
	for i := range blocked {
		blocked[i] = &blockedConn{release: release}
		Track(blocked[i])
	}
	t.Cleanup(func() {
		for _, c := range good {
			Untrack(c)
		}
		for _, c := range blocked {
			Untrack(c)
		}
	})

	start := time.Now()
	if n := CloseAll(context.Background(), grace); n != healthy+stalled {
		t.Fatalf("CloseAll signalled %d conns, want %d", n, healthy+stalled)
	}
	// Sequentially, stalled peers alone cost stalled*closeWriteWait (over a
	// minute). The budget is the grace, plus scheduling slack.
	if elapsed := time.Since(start); elapsed > grace+2*time.Second {
		t.Fatalf("CloseAll took %s, want at most the %s grace budget", elapsed, grace)
	}
	for i, c := range good {
		if frames := c.frameCount(); frames != 1 {
			t.Fatalf("healthy conn %d got %d frames, want 1 — a stalled peer starved it", i, frames)
		}
	}
}

func TestCloseAllNoConnsIsANoop(t *testing.T) {
	if n := CloseAll(context.Background(), time.Minute); n != 0 {
		t.Fatalf("CloseAll on an empty registry signalled %d conns, want 0", n)
	}
}
