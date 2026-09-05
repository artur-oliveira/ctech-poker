package table

import (
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

// Command is anything the Actor's Run loop can process.
type Command interface {
	reply() chan error
}

type ReadyCmd struct {
	PlayerID string
	ActionID string
	Ready    bool
	Reply    chan error
}

func (c ReadyCmd) reply() chan error { return c.Reply }

type ActCmd struct {
	PlayerID                string
	ActionID                string
	ExpectedSnapshotVersion uint64
	ExpectedHandID          string
	Action                  betting.Action
	Amount                  int64
	Reply                   chan error
}

func (c ActCmd) reply() chan error { return c.Reply }

type ChatCmd struct {
	PlayerID string
	ActionID string
	Message  string
	Reply    chan error
}

func (c ChatCmd) reply() chan error { return c.Reply }

type ReactionCmd struct {
	PlayerID       string
	ActionID       string
	ReactionID     string
	TargetPlayerID string
	Reply          chan error
}

func (c ReactionCmd) reply() chan error { return c.Reply }

type PreselectCmd struct {
	PlayerID                string
	ActionID                string
	Selection               string
	Amount                  int64
	ExpectedSnapshotVersion uint64
	ExpectedHandID          string
	ExpectedStage           string
	Reply                   chan error
}

func (c PreselectCmd) reply() chan error { return c.Reply }

// ConnectCmd is dispatched exactly once per physical WS connection, right
// after the gateway registers it — so the actor can count concurrently open
// connections per player (e.g. two browser tabs) and only treat the player
// as disconnected once the LAST one closes. See ReconnectCmd, which instead
// fires on every inbound frame and cannot be used for counting.
type ConnectCmd struct {
	PlayerID string
	ConnID   string
	Reply    chan error
}

func (c ConnectCmd) reply() chan error { return c.Reply }

type DisconnectCmd struct {
	PlayerID string
	ConnID   string
	Reply    chan error
}

func (c DisconnectCmd) reply() chan error { return c.Reply }

type ReconnectCmd struct {
	PlayerID string
	Reply    chan error
}

func (c ReconnectCmd) reply() chan error { return c.Reply }

// RequestHandoffCmd is dispatched when a player explicitly confirms "continue
// here, disconnect the other device." NewConnID is the connection asking to
// take over — every OTHER connID this player currently holds anywhere in the
// fleet (per tableconn, via Actor.fleetConnIDs) gets a deliberate server close,
// never left to expire by TTL. See internal/tablehandoff.
type RequestHandoffCmd struct {
	PlayerID  string
	NewConnID string
	Reply     chan error
}

func (c RequestHandoffCmd) reply() chan error { return c.Reply }

// ExternalChangeCmd is dispatched by tablemanager when a ChangeNotifier
// signal reports that a sibling process just committed for this table. It
// carries no player scope: unlike ReconnectCmd (which only broadcasts when
// clearing a stale disconnect mark, to avoid flooding on routine pings), this
// always forces a fresh reload and re-broadcast, because it only ever fires
// when something genuinely changed elsewhere.
type ExternalChangeCmd struct {
	Reply chan error
}

func (c ExternalChangeCmd) reply() chan error { return c.Reply }

type SitOutCmd struct {
	PlayerID string
	Reply    chan error
}

func (c SitOutCmd) reply() chan error { return c.Reply }

type ShowCardsCmd struct {
	PlayerID  string
	ActionID  string
	CardIndex *int32
	Reply     chan error
}

type SetRunItTwiceCmd struct {
	PlayerID string
	Enabled  bool
	Reply    chan error
}

func (c SetRunItTwiceCmd) reply() chan error { return c.Reply }

func (c ShowCardsCmd) reply() chan error { return c.Reply }

// RequestRabbitHuntCmd charges the player the table's big blind to reveal
// the rabbit-hunt runout for the just-completed hand.
type RequestRabbitHuntCmd struct {
	PlayerID string
	ActionID string
	Reply    chan error
}

func (c RequestRabbitHuntCmd) reply() chan error { return c.Reply }

// RequestExitCmd asks to leave the table. It always pauses the player
// (no future hands) and, if currently their turn, folds them out of the
// live round — see Table.RequestExit. If they are not currently dealt into
// a hand, this resolves as an immediate removal+cash-out, same latency as
// the plain HTTP leave path it replaces.
type RequestExitCmd struct {
	PlayerID string
	ActionID string
	Reply    chan error
}

func (c RequestExitCmd) reply() chan error { return c.Reply }

// CancelExitCmd reverses a still-pending RequestExitCmd. Errors if the
// player has no pending exit (either never requested one, or the sweep
// already removed them).
type CancelExitCmd struct {
	PlayerID string
	ActionID string
	Reply    chan error
}

func (c CancelExitCmd) reply() chan error { return c.Reply }

// RequestWinnerCardsCmd charges a dealt-in opponent to see the sole
// uncontested winner's otherwise-mucked hole cards for this hand.
type RequestWinnerCardsCmd struct {
	PlayerID string
	ActionID string
	Reply    chan error
}

func (c RequestWinnerCardsCmd) reply() chan error { return c.Reply }

// AcceptWinnerCardsCmd is the winner agreeing to the pending paid-reveal
// request, which is what actually moves the fee and reveals the cards.
type AcceptWinnerCardsCmd struct {
	PlayerID string
	ActionID string
	Reply    chan error
}

func (c AcceptWinnerCardsCmd) reply() chan error { return c.Reply }

// DeclineWinnerCardsCmd is the winner refusing; the requester is refunded and
// nothing is revealed.
type DeclineWinnerCardsCmd struct {
	PlayerID string
	ActionID string
	Reply    chan error
}

func (c DeclineWinnerCardsCmd) reply() chan error { return c.Reply }

// expireWinnerCardsCmd is the consent window closing with no answer. Internal
// (timer-driven), never dispatched by a client.
type expireWinnerCardsCmd struct {
	Reply chan error
}

func (c expireWinnerCardsCmd) reply() chan error { return c.Reply }

// RabbitHuntVerifyFailedCmd refunds a RequestRabbitHuntCmd charge when the
// client couldn't locally verify the revealed runout.
type RabbitHuntVerifyFailedCmd struct {
	PlayerID string
	ActionID string
	Reply    chan error
}

func (c RabbitHuntVerifyFailedCmd) reply() chan error { return c.Reply }

// KeepSeatCmd is an explicit human-presence signal used by the idle-removal
// warning. Transport heartbeats deliberately do not count as activity.
type KeepSeatCmd struct {
	PlayerID string
	ActionID string
	Reply    chan error
}

func (c KeepSeatCmd) reply() chan error { return c.Reply }

// PeekCardsCmd records that a player looked at their own hole cards this
// hand. It changes no game state — it exists only so the post-hand action
// log (achievements.Service.RecordHand) can tell a genuinely blind all-in or
// win from one the client simply never reported.
type PeekCardsCmd struct {
	PlayerID string
	ActionID string
	Reply    chan error
}

func (c PeekCardsCmd) reply() chan error { return c.Reply }

type JoinCmd struct {
	PlayerID string
	Stack    int64
	MaxSeats int
	// MidHand is retained for wire compatibility with the Phase 3 service.
	// The actor derives pending-entry status from its authoritative hand state
	// instead of trusting this potentially stale lobby hint.
	MidHand          bool
	HoldID           string
	AutoRebuy        bool
	SettlementIntent func() (types.TransactWriteItem, error)
	Reply            chan error
}

func (c JoinCmd) reply() chan error { return c.Reply }

type LeaveCmd struct {
	PlayerID         string
	Stack            chan int64 // receives the player's final stack, only after the removal commits
	SettlementIntent func(stack int64, holdID string) (types.TransactWriteItem, error)
	HoldID           chan string // receives the player's holdID, only after the removal commits
	Reply            chan error
}

func (c LeaveCmd) reply() chan error { return c.Reply }

type PostBigBlindCmd struct {
	PlayerID string
	ActionID string
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
	// AllowCached lets the actor answer from its own cache when that cache
	// was loaded less than SnapshotReloadInterval ago, instead of forcing a
	// DynamoDB read for every snapshot. Set by the WS gateway, whose
	// sync_state frames arrive at up to the connection rate limit (10/s) and
	// whose staleness window is already bounded by ChangeNotifier (a sibling
	// commit forces a reload) plus the version precondition every real action
	// carries. Left false by the seat/buy-in read paths, where an
	// authoritative answer is worth the read.
	AllowCached bool
}

func (c SnapshotCmd) reply() chan error { return c.Reply }

// SetIdentityCmd caches a player's persisted display identity (looked up from
// player.Service by the WS gateway on connect, not client-supplied) for
// broadcastAll to attach to their SeatView. Cosmetic only — playerID (JWT
// sub) stays the sole identity (IDOR safety is unaffected since Name never
// gates any action).
type SetIdentityCmd struct {
	PlayerID       string
	Name           string
	AvatarURL      string
	PlaystyleBadge string
	Reply          chan error
}

func (c SetIdentityCmd) reply() chan error { return c.Reply }

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
