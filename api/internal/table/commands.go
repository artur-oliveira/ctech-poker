package table

import (
	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

// Command is anything the Actor's Run loop can process.
type Command interface {
	reply() chan error
}

type ReadyCmd struct {
	PlayerID string
	Ready    bool
	Reply    chan error
}

func (c ReadyCmd) reply() chan error { return c.Reply }

type ActCmd struct {
	PlayerID string
	ActionID string
	Action   betting.Action
	Amount   int64
	Reply    chan error
}

func (c ActCmd) reply() chan error { return c.Reply }

type DisconnectCmd struct {
	PlayerID string
	Reply    chan error
}

func (c DisconnectCmd) reply() chan error { return c.Reply }

type ReconnectCmd struct {
	PlayerID string
	Reply    chan error
}

func (c ReconnectCmd) reply() chan error { return c.Reply }

type SitOutCmd struct {
	PlayerID string
	Reply    chan error
}

func (c SitOutCmd) reply() chan error { return c.Reply }

type JoinCmd struct {
	PlayerID string
	Stack    int64
	MaxSeats int
	// MidHand is retained for wire compatibility with the Phase 3 service.
	// The actor derives pending-entry status from its authoritative hand state
	// instead of trusting this potentially stale lobby hint.
	MidHand bool
	HoldID  string
	Reply   chan error
}

func (c JoinCmd) reply() chan error { return c.Reply }

type LeaveCmd struct {
	PlayerID string
	Stack    chan int64  // receives the player's final stack, only after the removal commits
	HoldID   chan string // receives the player's holdID, only after the removal commits
	Reply    chan error
}

func (c LeaveCmd) reply() chan error { return c.Reply }

type PostBigBlindCmd struct {
	PlayerID string
	Reply    chan error
}

func (c PostBigBlindCmd) reply() chan error { return c.Reply }

// SnapshotCmd asks the actor for the current viewer-specific table state. It
// is how the WS gateway pushes the initial snapshot to a freshly connected
// socket: broadcasts only fire on a state mutation (broadcastAll), so without
// this a new connection would sit on ping/pong until the next action. The
// snapshot is built inside Run (hand.Table has no lock) and handed back on the
// Snapshot channel; Reply carries the usual command error.
type SnapshotCmd struct {
	PlayerID string
	Snapshot chan hand.Snapshot
	Reply    chan error
}

func (c SnapshotCmd) reply() chan error { return c.Reply }

// SetNameCmd caches a player's persisted display name (looked up from
// player.Service by the WS gateway on connect, not client-supplied) for
// broadcastAll to attach to their SeatView. Cosmetic only — playerID (JWT
// sub) stays the sole identity (IDOR safety is unaffected since Name never
// gates any action).
type SetNameCmd struct {
	PlayerID string
	Name     string
	Reply    chan error
}

func (c SetNameCmd) reply() chan error { return c.Reply }

// autoFoldCheckCmd is dispatched by the auto-fold timer (a time.AfterFunc
// goroutine) so that all actor-map mutations happen inside Run, never from the
// timer goroutine (see armActionDeadlineIfTheirTurn).
type autoFoldCheckCmd struct {
	PlayerID string
	Reply    chan error
}

func (c autoFoldCheckCmd) reply() chan error { return c.Reply }
