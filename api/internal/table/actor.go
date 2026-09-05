// Package table drives one table's hand.Table from exactly one goroutine per
// instance — not because that instance owns write authority (it doesn't;
// ARCHITECTURE.md §2 makes DynamoDB's conditional writes the sole
// correctness mechanism), but because hand.Table has no internal lock, so
// two of this instance's own goroutines must still be serialized.
package table

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

var timeNowFunc = time.Now

const (
	maxPersistedChatMessages = 40
	maxPersistedReactions    = 8
	reactionLifetime         = 3400 * time.Millisecond
)

// Actor is the local serialization point for one table's hand.Table.
type Actor struct {
	id         string
	env        string
	store      *tablestore.Store
	trustCache bool // set once at construction — see New's doc comment
	broadcast  func(viewerID string, snap hand.Snapshot)

	cmds chan Command

	cached *hand.Table // nil until first loaded; never trusted when !trustCache
	// lastLoadedAt is when LoadTable last filled cached. It paces
	// handleSnapshot's authoritative read — see SnapshotReloadInterval.
	lastLoadedAt time.Time
	version      int
	handID       string
	activity     tablestore.TableActivity

	turnTimeout       time.Duration
	timeBankEnabled   bool
	disconnectedSince map[string]time.Time
	// streaks holds each seated player's current per-table win/loss streak
	// (positive = consecutive wins, negative = consecutive losses), overlaid
	// onto every ViewFor snapshot exactly like disconnectedSince/applyPresence.
	// It is only this command's read of streakStore below, never the source of
	// truth: any instance may run an Actor for any table, so a process-local
	// tally had two live actors publishing two different badges for one seat,
	// flipping between them on consecutive snapshots of the same hand.
	streaks map[string]int
	// streaksRefreshedAt paces the shared-store read that fills the map above
	// — see StreakRefreshInterval.
	streaksRefreshedAt time.Time
	// streakStore is the cross-instance home of that badge (Valkey, see
	// internal/tablestreak). nil in dev/tests without a cache, where the map
	// above is the whole story.
	streakStore StreakStore
	// activeConns tracks physical connection IDs, not just a count. Connect
	// and Disconnect are therefore idempotent when a live WS is re-registered
	// after actor replacement, and one tab closing cannot disconnect another.
	// It only ever knows about sockets terminating on THIS process, which is
	// why the seat dot also consults connStore below.
	activeConns map[string]map[string]struct{}
	// connStore shares the connection set with every other instance serving
	// this table (internal/tableconn). nil in dev/tests without a cache.
	connStore ConnStore
	// changeNotifier announces a successful commit to every sibling process
	// serving this table (internal/tablenotify), so their own Actor instances
	// reload and re-arm their real enforcement timers immediately instead of
	// only noticing on their own next unrelated reload trigger. nil in
	// dev/tests without a cache; see docs/specs/2026-09-04-cross-process-change-notify.md.
	changeNotifier ChangeNotifier
	// fleetConns is the last answer connStore gave — who the whole fleet
	// considers connected. nil means "never synced", which applyPresence reads
	// as "trust the local view". Display only, never a removal input.
	fleetConns map[string]bool
	// connSyncedAt paces the round trip; see tableconn.SyncInterval.
	connSyncedAt time.Time
	// kickGrace bounds how long a disconnected player can occupy a seat
	// before being auto-removed (Leave, cashing them out same as a manual
	// exit). kickTimers holds one AfterFunc per currently-disconnected
	// player — unlike turnTimer/nextHandTimer there can be several at once,
	// one per seat.
	kickGrace            time.Duration
	kickTimers           map[string]*time.Timer
	turnTimer            *time.Timer
	turnDeadline         time.Time
	turnBaseDeadline     time.Time
	turnDeadlineFor      string
	turnDeadlineForStage hand.Stage
	// pendingPersistedDeadline carries a just-loaded StoredTable's
	// TurnDeadlineUnixMs across to the next armTurnTimer call (set by
	// ensureLoaded, consumed and zeroed by armTurnTimer) so a freshly
	// (re)loaded actor resumes the true remaining time on the current turn
	// instead of computing a brand new turnTimeout window from now.
	pendingPersistedDeadline int64
	pendingDeadlineFor       string
	pendingDeadlineForStage  hand.Stage
	nextHandTimer            *time.Timer
	nextHandDeadline         time.Time
	nextHandArmedFor         string
	nextHandDelay            time.Duration
	// nextHandRetryDelay/nextHandRetries drive handleNextHand's bounded
	// re-arm after a transient (non-panic) load/commit failure — see
	// retryNextHand. Counted per stall, reset the moment a next-hand attempt
	// reaches a verdict.
	nextHandRetryDelay time.Duration
	nextHandRetries    int
	// nextHandArm{GuardHand,sForHand} cap armNextHandTimer's (re-)arms per hand
	// so a burst of rearmTimersFromCache calls (a client reconnect loop, a
	// keepalive storm) against a wedged table cannot become a storm of rejected
	// next-hand DynamoDB transactions — see MaxNextHandArmsPerHand and
	// docs/specs/2026-09-03-next-hand-rearm-storm.md.
	nextHandArmGuardHand string
	nextHandArmsForHand  int
	// pendingNextHandDeadline carries a just-loaded StoredTable's
	// NextHandDeadlineUnixMs across to the next armNextHandTimer call (set by
	// ensureLoaded, consumed and zeroed there) — the post-hand countdown's
	// exact analogue of pendingPersistedDeadline above, so every instance
	// rendering a Complete table publishes the same next_hand_unix_ms instead
	// of each starting a fresh 12s window from its own now.
	pendingNextHandDeadline int64
	winnerCardsTimer        *time.Timer
	winnerCardsArmedFor     string
	lastBroadcastStage      hand.Stage
	runoutTimer             *time.Timer
	runoutTimerHandID       string
	runoutTimerStage        hand.Stage
	runoutTimerPhase        int
	runoutStreetDelay       time.Duration
	escalationInterval      time.Duration
	escalationCfg           roomstore.BlindEscalation
	afkSweepTimer           *time.Timer
	afkSweepInterval        time.Duration
	done                    chan struct{}
	// equityCache memoizes the per-viewer Monte-Carlo estimate for the hand
	// in equityCacheHand. The estimate is a pure function of (hole cards,
	// board, opponent count) — all of which are keyed below — while
	// broadcastAll re-runs for every chat message, reconnect signal and act,
	// so without this a single street paid for equityIterations samples per
	// active seat over and over. Dropped whenever the hand changes.
	equityCache       map[string]float64
	equityCacheHand   string
	equityEnabled     atomic.Bool
	runItTwiceEnabled atomic.Bool
	onHandComplete    func(string, hand.HandOutcome, map[string]string)
	onHandUpdated     func(string, hand.HandOutcome, map[string]string)
	// completedHandNotified suppresses a repeat within THIS process only.
	// handHooks below is what stops a second instance from re-firing the same
	// hand's hooks — see notifyHandComplete.
	completedHandNotified string
	// handHooks grants the fleet-wide right to run one hand's post-hand hooks
	// (internal/handhook, Valkey SET NX). nil in dev/tests without a cache,
	// where one instance is the whole fleet and the field above suffices.
	handHooks            HandHookClaimer
	outcomeLoggedForHand string
	onSeatsChanged       func(int)
	// onPlayerRemoved fires only for a system-initiated removal (AFK sweep,
	// disconnect kick timeout) — never for a player-requested LeaveCmd, which
	// the client already knows about and navigates away for itself. It lets
	// the gateway push an explicit "removed" message to that player's own
	// socket, so the client stops silently reconnecting into a seat it no
	// longer holds and instead redirects to the lobby (see tablews.go/
	// useTableRealtime.ts). stack/holdID are the same values a player-initiated
	// CashOut would have received on its Stack/HoldID channels — a system
	// removal never goes through buyin.Service.CashOut itself (it fires from
	// inside the Actor's own goroutine), so without forwarding them here the
	// removed player's chips are never credited back to any wallet and their
	// sessionlog entry is never closed (buyin.SettleSystemRemoval does both).
	//
	// settlementNonce is a fresh per-removal token that both hooks forward
	// unchanged. It is what makes the co-committed poker_pending_cashouts row's
	// key unique to THIS removal: a player can be seated, system-removed, rebuy,
	// and be system-removed again from the same table for the same reason, and
	// without the nonce all of those collapse onto one key
	// (roomID#playerID#system_leave#reason). The row is written create-only and
	// co-committed atomically with the seat removal, so the second removal's
	// whole transaction failed its condition forever — the seat could never be
	// pulled and the player was wedged "leaving…" until an unrelated idle sweep
	// (different reason, different key) happened to catch them. Mirrors the
	// client nonce buyin.Service.CashOut already appends for the same reason.
	onPlayerRemoved        func(playerID, reason, settlementNonce string, stack int64, holdID string)
	systemSettlementIntent func(ctx context.Context, playerID, reason, settlementNonce string, stack int64, holdID string) (types.TransactWriteItem, error)
	// Premium reactions fail closed until both hooks are wired by the manager.
	reactionOwnership func(ctx context.Context, playerID, reactionID string) (bool, error)
	reactionMarkUsed  func(ctx context.Context, playerID, reactionID string) (*types.TransactWriteItem, error)
	// commandBudget/settlementBudget/queueBudget are the deadlines this actor
	// runs under — fields rather than bare constants only so tests can shrink
	// them. See budget.go for what each one bounds and why.
	commandBudget    time.Duration
	settlementBudget time.Duration
	queueBudget      time.Duration
	budget           budgetCounters
	// connCount mirrors the total size of activeConns across all players.
	// Maintained only inside Run (handleConnect/handleDisconnect) but read via
	// ActiveConnCount from any goroutine — same pattern as equityEnabled —
	// so the manager can decide whether a lease-less actor is idle without
	// dispatching a command to it.
	connCount atomic.Int32
}

// New returns an Actor for tableID. trustCache should be true only when the
// caller currently holds tableID's tablelease — it is read once here and
// never re-consulted; losing the lease later does not retroactively
// invalidate an in-flight Actor (ARCHITECTURE.md §2: the lease bounds
// latency, not correctness — a stale cache is always caught by
// CommitAction's version check regardless of trustCache).
func New(id string, store *tablestore.Store, trustCache bool, broadcast func(string, hand.Snapshot)) *Actor {
	a := &Actor{
		id: id, store: store, trustCache: trustCache, broadcast: broadcast, cmds: make(chan Command, 64),
		done:               make(chan struct{}),
		turnTimeout:        DefaultTurnTimeout,
		timeBankEnabled:    true,
		nextHandDelay:      NextHandDelay,
		nextHandRetryDelay: NextHandRetryDelay,
		runoutStreetDelay:  RunoutStreetDelay,
		disconnectedSince:  make(map[string]time.Time),
		streaks:            make(map[string]int),
		activeConns:        make(map[string]map[string]struct{}),
		kickGrace:          5 * time.Minute,
		kickTimers:         make(map[string]*time.Timer),
		afkSweepInterval:   AFKSweepInterval,
		commandBudget:      defaultCommandBudget,
		settlementBudget:   settlementCommandBudget,
		queueBudget:        defaultQueueBudget,
	}
	a.equityEnabled.Store(true)
	a.armAFKSweepTimer()
	return a
}

// ErrActorStopped is returned by Dispatch when the actor has stopped serving
// (e.g. it lost its table lease and Run exited) and will never read the
// command. Callers re-resolve a live actor via the manager.
var ErrActorStopped = errors.New("table: actor stopped")

// ErrNoSeatsAvailable lets the HTTP boundary return a stable, actionable
// problem type without parsing an internal error string. Buy-in wraps it after
// successfully compensating the wallet debit, so errors.Is remains usable.
var ErrNoSeatsAvailable = errors.New("table: no seats available")

func (a *Actor) Dispatch(cmd Command) error {
	// The mailbox is deliberately blocking rather than lossy: a full channel
	// backpressures the caller instead of dropping a command. That backpressure
	// is budgeted, though — see queueBudget: past it the caller is told the
	// table is unavailable instead of being parked on a wedged actor forever.
	select {
	case a.cmds <- cmd:
	case <-a.done:
		return ErrActorStopped
	default:
		if err := a.enqueueWithinBudget(cmd); err != nil {
			return err
		}
	}
	// Sent (channel is buffered). Wait for the reply, but bail if the actor
	// stops before Run reads/processes it — otherwise we'd block forever on a
	// dead actor. The wait itself is bounded by the command's own deadline
	// (handleWithBudget), never abandoned here: a caller that walked away from
	// an in-flight settlement would be exactly the silent-abandon failure this
	// budget exists to avoid.
	select {
	case err := <-cmd.reply():
		return err
	case <-a.done:
		return ErrActorStopped
	}
}

// enqueueWithinBudget waits out a full mailbox for at most queueBudget.
func (a *Actor) enqueueWithinBudget(cmd Command) error {
	timer := time.NewTimer(a.queueBudget)
	defer timer.Stop()
	started := time.Now()
	defer func() { a.budget.queueWaitNanos.Add(int64(time.Since(started))) }()
	select {
	case a.cmds <- cmd:
		return nil
	case <-a.done:
		return ErrActorStopped
	case <-timer.C:
		a.budget.queueSaturations.Add(1)
		slog.Warn("table command queue saturated, rejecting command",
			"table_id", a.id, "command", fmt.Sprintf("%T", cmd),
			"queued", len(a.cmds), "budget", a.queueBudget)
		return ErrQueueSaturated
	}
}

// Done exposes the actor's stop channel so the manager can detect a dead actor
// (after a lease loss) and recreate a live one.
func (a *Actor) Done() <-chan struct{} { return a.done }

// ActiveConnCount reports the number of live WS connections currently
// registered across all players at this table. Safe to call from any
// goroutine (see connCount's doc comment) — used by the manager to decide
// whether a lease-less actor is idle and can be evicted.
func (a *Actor) ActiveConnCount() int32 { return a.connCount.Load() }

// IsAlive reports whether Run is still serving commands.
func (a *Actor) IsAlive() bool {
	select {
	case <-a.done:
		return false
	default:
		return true
	}
}

func (a *Actor) Run(ctx context.Context) {
	defer close(a.done)
	for {
		select {
		case <-ctx.Done():
			return
		case cmd := <-a.cmds:
			cmd.reply() <- a.handleWithBudget(ctx, cmd)
		}
	}
}

// handleSafely runs one command's handler with panic recovery so a single
// engine panic (an out-of-bounds deal in hand.go, a malformed persisted
// State decode, a nil deref anywhere in the ~2000 lines of engine code)
// fails just that request instead of unwinding Run and taking the whole
// process — and every other table Actor on this instance — down with it.
// The two WebSocket handlers in tablews.go already recover their own
// goroutine panics; the actor goroutine, which runs all engine logic, must
// do the same.
func (a *Actor) handleSafely(ctx context.Context, cmd Command) (err error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		slog.ErrorContext(ctx, "table actor handler panic recovered",
			"table_id", a.id, "hand_id", a.handID, "command", fmt.Sprintf("%T", cmd),
			"panic", r, "stack", string(debug.Stack()))
		// A panic can land mid-mutation, after a.cached (or a.handID, a.activity,
		// the timers) was changed but before the matching conditional write
		// committed — exactly the poisoned-cache shape the 2026-09-01 duplicate
		// seat incident came from, with no rollback path from here. Drop the
		// cached state so the next command reloads authoritative state from the
		// store instead of trusting this fork. Nothing was persisted: the panic
		// unwound before commit.
		a.cached = nil
		a.version = 0
		a.handID = ""
		a.activity = tablestore.TableActivity{}
		// ErrUnavailable (not a bare error): the command reached no verdict about
		// the player's action, so the gateway answers "resync" rather than
		// blaming the action as invalid — see tablews.go/actionErrorCode.
		err = fmt.Errorf("%w: table actor recovered from an internal error", tablestore.ErrUnavailable)
	}()
	return a.handle(ctx, cmd)
}

func (a *Actor) handle(ctx context.Context, cmd Command) error {
	switch c := cmd.(type) {
	case ReadyCmd:
		return a.handleReady(ctx, c)
	case ActCmd:
		return a.handleAct(ctx, c)
	case ChatCmd:
		return a.handleChat(ctx, c)
	case ReactionCmd:
		return a.handleReaction(ctx, c)
	case PreselectCmd:
		return a.handlePreselect(ctx, c)
	case ConnectCmd:
		return a.handleConnect(c)
	case DisconnectCmd:
		return a.handleDisconnect(c)
	case ReconnectCmd:
		return a.handleReconnect(ctx, c)
	case ExternalChangeCmd:
		return a.handleExternalChange(ctx, c)
	case SitOutCmd:
		return a.handleSitOut(ctx, c)
	case ShowCardsCmd:
		return a.handleShowCards(ctx, c)
	case RequestRabbitHuntCmd:
		return a.handleRequestRabbitHunt(ctx, c)
	case RequestExitCmd:
		return a.handleRequestExit(ctx, c)
	case CancelExitCmd:
		return a.handleCancelExit(ctx, c)
	case RequestWinnerCardsCmd:
		return a.handleRequestWinnerCards(ctx, c)
	case AcceptWinnerCardsCmd:
		return a.handleAcceptWinnerCards(ctx, c)
	case DeclineWinnerCardsCmd:
		return a.handleDeclineWinnerCards(ctx, c)
	case expireWinnerCardsCmd:
		return a.handleExpireWinnerCards(ctx, c)
	case RabbitHuntVerifyFailedCmd:
		return a.handleRabbitHuntVerifyFailed(ctx, c)
	case SetRunItTwiceCmd:
		return a.handleSetRunItTwice(ctx, c)
	case KeepSeatCmd:
		return a.handleKeepSeat(ctx, c)
	case PeekCardsCmd:
		return a.handlePeekCards(ctx, c)
	case JoinCmd:
		return a.handleJoin(ctx, c)
	case LeaveCmd:
		return a.handleLeave(ctx, c)
	case PostBigBlindCmd:
		return a.handlePostBigBlind(ctx, c)
	case SnapshotCmd:
		return a.handleSnapshot(ctx, c)
	case SetIdentityCmd:
		return a.handleSetIdentity(ctx, c)
	case turnTimeoutCmd:
		return a.handleTurnTimeout(ctx, c)
	case nextHandCmd:
		return a.handleNextHand(ctx, c)
	case runoutStepCmd:
		return a.handleRunoutStep(ctx, c)
	case kickTimeoutCmd:
		return a.handleKickTimeout(ctx, c)
	case afkSweepCmd:
		return a.handleAFKSweep(ctx, c)
	case escalateCmd:
		return a.handleEscalate(ctx)
	default:
		return nil
	}
}
