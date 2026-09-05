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

const (
	// closeWriteWait bounds how long a single close-frame write may block on a
	// peer that has stopped reading. Deliberately short — a stalled socket must
	// not eat into the caller's grace window.
	closeWriteWait = 2 * time.Second

	// closeMaxFanOut is the ceiling on how many close frames are written
	// concurrently. It is high on purpose: a *narrow* worker pool reproduces
	// the very bug this guards against, because every worker can be parked on a
	// peer that stopped reading while healthy sockets queue behind them, so the
	// fan-out has to be at least as wide as the number of peers that can stall
	// at once — which is all of them. One goroutine per socket for the length
	// of one control-frame write is cheap next to the read-loop goroutine each
	// of those sockets already owns. The cap only exists so a pathological
	// tracker size cannot turn shutdown into a goroutine storm; past it the
	// remainder is written by the caller, which is no worse than the fully
	// sequential behaviour this replaced.
	closeMaxFanOut = 8192
)

// Conn is the write-control half of a live socket. *v1.wsConnAdapter
// satisfies it, and its mutex is what keeps this write from racing the
// registry's data-frame broadcasts.
type Conn interface {
	WriteControl(messageType int, data []byte, deadline time.Time) error
}

var (
	mu       sync.Mutex
	conns    = make(map[Conn]struct{})
	byConnID = make(map[string]Conn)
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

// TrackByID indexes a connection by its application-level connID, in
// addition to the identity-keyed Track above — CloseByConnID is the only
// consumer, for the session-handoff feature (#353), where the caller only
// knows the connID, never the Conn's Go identity. Call alongside Track, from
// the same place tablews.go registers the socket with ws.Registry.
func TrackByID(connID string, c Conn) {
	mu.Lock()
	defer mu.Unlock()
	byConnID[connID] = c
}

func UntrackByID(connID string) {
	mu.Lock()
	defer mu.Unlock()
	delete(byConnID, connID)
}

// CloseByConnID sends a 1001 close frame to each of connIDs that this
// process actually holds, ignoring any it doesn't recognize (the normal case
// for a handoff broadcast fleet-wide — most instances own none of the named
// IDs). Returns how many were signalled. Unlike CloseAll this is not a
// shutdown path, so there is no grace window to wait out: the caller (a
// Pub/Sub subscriber) must not block on slow peers, so each write gets its
// own goroutine exactly like CloseAll's fan-out, and this function returns
// immediately after dispatching them.
func CloseByConnID(connIDs []string) int {
	mu.Lock()
	var targets []Conn
	for _, id := range connIDs {
		if c, ok := byConnID[id]; ok {
			targets = append(targets, c)
		}
	}
	mu.Unlock()
	if len(targets) == 0 {
		return 0
	}
	frame := fws.FormatCloseMessage(fws.CloseGoingAway, "session handoff to another device")
	deadline := time.Now().Add(closeWriteWait)
	for _, c := range targets {
		go func(c Conn) {
			if err := c.WriteControl(fws.CloseMessage, frame, deadline); err != nil {
				slog.Debug("ws handoff close frame failed", "err", err)
			}
		}(c)
	}
	return len(targets)
}

// CloseAll sends a 1001 close frame to every tracked connection — on a
// bounded pool of writers, so one stalled peer cannot delay the rest — and then
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

	// grace is the budget for this whole phase, the writes included. They used
	// to run sequentially under one shared deadline, so a handful of peers that
	// had stopped reading serialised closeWriteWait each and blew the caller's
	// grace before a single healthy socket queued behind them was signalled
	// (issue #226). They now run on a bounded pool, all still sharing the one
	// deadline below — a late worker inherits an already-elapsed deadline and
	// fails fast rather than extending the phase.
	start := time.Now()
	writeWait := closeWriteWait
	if grace < writeWait {
		writeWait = grace
	}
	deadline := start.Add(writeWait)

	frame := fws.FormatCloseMessage(fws.CloseGoingAway, "server restarting")
	write := func(c Conn) {
		if err := c.WriteControl(fws.CloseMessage, frame, deadline); err != nil {
			slog.Debug("ws going-away frame failed", "err", err)
		}
	}
	for i, c := range snapshot {
		if i >= closeMaxFanOut {
			write(c)
			continue
		}
		go write(c)
	}

	// Deliberately not joining the writers. A Conn whose own write mutex is
	// held by an in-flight broadcast can block past its write deadline, and
	// waiting on that would put the sequential stall straight back into the
	// shutdown path. What the clients need is the rest of the grace window to
	// react to the frames that did go out; the process is exiting either way.
	timer := time.NewTimer(grace - time.Since(start))
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
	return len(snapshot)
}
