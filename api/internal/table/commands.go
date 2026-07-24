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

// ConnectCmd is dispatched exactly once per physical WS connection, right
// after the gateway registers it — so the actor can count concurrently open
// connections per player (e.g. two browser tabs) and only treat the player
// as disconnected once the LAST one closes. See ReconnectCmd, which instead
// fires on every inbound frame and cannot be used for counting.
type ConnectCmd struct {
	PlayerID string
	Reply    chan error
}

func (c ConnectCmd) reply() chan error { return c.Reply }

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

type ShowCardsCmd struct {
	PlayerID string
	Reply    chan error
}

func (c ShowCardsCmd) reply() chan error { return c.Reply }

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

// turnTimeoutCmd is dispatched by the universal per-turn timer (a
// time.AfterFunc goroutine) so that all actor-map/state mutations happen
// inside Run, never from the timer goroutine (see armTurnTimer). Fires for
// WHOEVER currently must act, connected or not — a disconnected player who
// times out here still falls through to the existing grace/consecutive-hands
// check inside handleTurnTimeout before deciding fold vs. sit-out.
// nextHandCmd is dispatched by the 5s post-hand timer (a time.AfterFunc
// goroutine) so the actual StartHand attempt happens inside Run, never from
// the timer goroutine (see armNextHandTimer). A stale command (the table is
// no longer Complete, or a new hand already started through some other path)
// is a silent no-op — handleNextHand re-checks the stage before acting.
type nextHandCmd struct {
	Reply chan error
}

func (c nextHandCmd) reply() chan error { return c.Reply }

type turnTimeoutCmd struct {
	PlayerID string
	Reply    chan error
}

func (c turnTimeoutCmd) reply() chan error { return c.Reply }

// runoutStepCmd is dispatched by the paced all-in-runout timer (a
// time.AfterFunc armed in armRunoutTimer) — runoutStreetDelay after the
// previous street was dealt, dealing exactly the next one.
type runoutStepCmd struct{ Reply chan error }

func (c runoutStepCmd) reply() chan error { return c.Reply }

// kickTimeoutCmd is dispatched by the per-player auto-kick timer (a
// time.AfterFunc goroutine, see armKickTimer) once a disconnected player has
// been gone for kickGrace. A stale command (they reconnected or left in the
// meantime) is a silent no-op — handleKickTimeout re-checks disconnectedSince
// first.
type kickTimeoutCmd struct {
	PlayerID string
	Reply    chan error
}

func (c kickTimeoutCmd) reply() chan error { return c.Reply }

// afkSweepCmd is dispatched by the self-perpetuating AFK sweep timer (a
// time.AfterFunc armed in armAFKSweepTimer, re-armed every AFKSweepInterval
// regardless of outcome) — checks every seated player's LastActionAt for
// staleness, independent of whose turn it currently is.
type afkSweepCmd struct{ Reply chan error }

func (c afkSweepCmd) reply() chan error { return c.Reply }
