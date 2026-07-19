package table

import "gopkg.aoctech.app/poker/api/internal/engine/betting"

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
	Reply   chan error
}

func (c JoinCmd) reply() chan error { return c.Reply }

type LeaveCmd struct {
	PlayerID string
	Stack    chan int64 // receives the player's final stack, only after the removal commits
	Reply    chan error
}

func (c LeaveCmd) reply() chan error { return c.Reply }

type PostBigBlindCmd struct {
	PlayerID string
	Reply    chan error
}

func (c PostBigBlindCmd) reply() chan error { return c.Reply }
