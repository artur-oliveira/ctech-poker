// Package wsdrain tracks this process's live WebSocket connections so a
// rolling deploy can hand every client a clean 1001 "going away" close frame
// before the HTTP server force-closes the transport underneath it.
//
// Without this, `rc-service restart` on one unit drops roughly half of all
// connected players abruptly: the client only learns the socket is dead when
// its own read fails or a TCP-level timeout fires, which lags. A 1001 close
// reaches the browser's onclose handler immediately, so the client's existing
// bounded-backoff reconnect starts measurably earlier.
// See docs/specs/2026-08-24-graceful-ws-shutdown-on-deploy.md.
//
// The tracker is a package-level singleton on purpose: it is process-wide
// transport state with exactly one consumer (the OnStop hook), and threading
// it through v1.Register's already-30-argument signature would buy nothing.
package wsdrain

import (
	"context"
	"log/slog"
	"sync"
	"time"

	fws "github.com/fasthttp/websocket"
)

// closeWriteWait bounds how long a single close-frame write may block on a
// peer that has stopped reading. Deliberately short — a stalled socket must
// not eat into the caller's grace window.
const closeWriteWait = 2 * time.Second

// Conn is the write-control half of a live socket. *v1.wsConnAdapter
// satisfies it, and its mutex is what keeps this write from racing the
// registry's data-frame broadcasts.
type Conn interface {
	WriteControl(messageType int, data []byte, deadline time.Time) error
}

var (
	mu    sync.Mutex
	conns = make(map[Conn]struct{})
)

func Track(c Conn) {
	mu.Lock()
	defer mu.Unlock()
	conns[c] = struct{}{}
}

func Untrack(c Conn) {
	mu.Lock()
	defer mu.Unlock()
	delete(conns, c)
}

// Live reports how many connections this process currently holds.
func Live() int {
	mu.Lock()
	defer mu.Unlock()
	return len(conns)
}

// CloseAll sends a 1001 close frame to every tracked connection and then
// waits up to grace for clients to process it and start reconnecting, so the
// caller's subsequent force-close lands on sockets the client already knows
// are gone. Returns how many connections were signalled.
//
// It does not close the connections itself: the read loop that owns each one
// unwinds on its own once the peer echoes the close, and the server's
// Shutdown is the backstop for the ones that don't.
func CloseAll(ctx context.Context, grace time.Duration) int {
	mu.Lock()
	snapshot := make([]Conn, 0, len(conns))
	for c := range conns {
		snapshot = append(snapshot, c)
	}
	mu.Unlock()
	if len(snapshot) == 0 {
		return 0
	}

	frame := fws.FormatCloseMessage(fws.CloseGoingAway, "server restarting")
	deadline := time.Now().Add(closeWriteWait)
	for _, c := range snapshot {
		if err := c.WriteControl(fws.CloseMessage, frame, deadline); err != nil {
			slog.Debug("ws going-away frame failed", "err", err)
		}
	}

	select {
	case <-time.After(grace):
	case <-ctx.Done():
	}
	return len(snapshot)
}
