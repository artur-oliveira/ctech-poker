// Package table drives one table's hand.Table from exactly one goroutine per
// instance — not because that instance owns write authority (it doesn't;
// ARCHITECTURE.md §2 makes DynamoDB's conditional writes the sole
// correctness mechanism), but because hand.Table has no internal lock, so
// two of this instance's own goroutines must still be serialized.
package table

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"sort"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/oklog/ulid/v2"
	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/equity"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/reactions"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/tableconn"
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

	cached   *hand.Table // nil until first loaded; never trusted when !trustCache
	version  int
	handID   string
	activity tablestore.TableActivity

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
	equityEnabled           atomic.Bool
	runItTwiceEnabled       atomic.Bool
	onHandComplete          func(string, hand.HandOutcome, map[string]string)
	onHandUpdated           func(string, hand.HandOutcome, map[string]string)
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
	onPlayerRemoved        func(playerID, reason string, stack int64, holdID string)
	systemSettlementIntent func(context.Context, string, string, int64, string) (types.TransactWriteItem, error)
	// Premium reactions fail closed until both hooks are wired by the manager.
	reactionOwnership func(ctx context.Context, playerID, reactionID string) (bool, error)
	reactionMarkUsed  func(ctx context.Context, playerID, reactionID string) (*types.TransactWriteItem, error)
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
		done:              make(chan struct{}),
		turnTimeout:       DefaultTurnTimeout,
		timeBankEnabled:   true,
		nextHandDelay:     NextHandDelay,
		runoutStreetDelay: RunoutStreetDelay,
		disconnectedSince: make(map[string]time.Time),
		streaks:           make(map[string]int),
		activeConns:       make(map[string]map[string]struct{}),
		kickGrace:         5 * time.Minute,
		kickTimers:        make(map[string]*time.Timer),
		afkSweepInterval:  AFKSweepInterval,
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
	if len(a.cmds) >= (cap(a.cmds)*3)/4 {
		// The mailbox is deliberately blocking rather than lossy. This signal
		// detects sustained pressure without emitting one metric per command.
	}
	select {
	case a.cmds <- cmd:
		// Sent (channel is buffered). Wait for the reply, but bail if the
		// actor stops before Run reads/processes it — otherwise we'd block
		// forever on a dead actor.
		select {
		case err := <-cmd.reply():
			return err
		case <-a.done:
			return ErrActorStopped
		}
	case <-a.done:
		return ErrActorStopped
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
			err := a.handle(ctx, cmd)
			cmd.reply() <- err
		}
	}
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

func (a *Actor) handlePostBigBlind(ctx context.Context, c PostBigBlindCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	apply := func() error {
		if !a.isSeated(c.PlayerID) {
			return fmt.Errorf("table: player %s is not seated", c.PlayerID)
		}
		a.cached.MarkReadyToPost(c.PlayerID)
		return a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
			PlayerID: c.PlayerID, ActionID: c.ActionID, Action: "post_big_blind",
		})
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		if !errors.Is(err, tablestore.ErrDuplicateAction) {
			return err
		}
		if err := a.ensureLoaded(ctx, true); err != nil {
			return err
		}
	}
	a.broadcastAll()
	return nil
}

func (a *Actor) handleChat(ctx context.Context, c ChatCmd) error {
	if c.ActionID == "" {
		return errors.New("table: action_id is required")
	}
	if c.Message == "" {
		return errors.New("table: chat message is required")
	}
	return a.commitActivity(ctx, true, func() error {
		if !a.isSeated(c.PlayerID) {
			return fmt.Errorf("table: player %s is not seated", c.PlayerID)
		}
		a.markLastAction(c.PlayerID)
		now := timeNowFunc().UnixMilli()
		a.activity.Chat = append(a.activity.Chat, tablestore.ChatMessage{
			ID: c.ActionID, PlayerID: c.PlayerID, Message: c.Message, Timestamp: now,
		})
		if len(a.activity.Chat) > maxPersistedChatMessages {
			a.activity.Chat = append([]tablestore.ChatMessage(nil), a.activity.Chat[len(a.activity.Chat)-maxPersistedChatMessages:]...)
		}
		return a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
			PlayerID: c.PlayerID, ActionID: c.ActionID, Action: "chat", Message: c.Message,
		})
	})
}

func (a *Actor) handleReaction(ctx context.Context, c ReactionCmd) error {
	if c.ActionID == "" {
		return errors.New("table: action_id is required")
	}
	if c.ReactionID == "" {
		return errors.New("table: reaction_id is required")
	}
	if !reactions.IsKnown(c.ReactionID) {
		return errors.New("table: unknown reaction_id")
	}
	if reactions.IsPremium(c.ReactionID) {
		if a.reactionOwnership == nil {
			return errors.New("table: reaction ownership check unavailable")
		}
		owned, err := a.reactionOwnership(ctx, c.PlayerID, c.ReactionID)
		if err != nil {
			return err
		}
		if !owned {
			return errors.New("table: reaction not owned")
		}
	}
	// Reactions already have a dedicated fan-out frame. Persist them so a
	// reconnect can restore the short-lived effect, but do not also broadcast
	// a full table snapshot for the same cosmetic action.
	return a.commitActivity(ctx, false, func() error {
		if !a.isSeated(c.PlayerID) {
			return fmt.Errorf("table: player %s is not seated", c.PlayerID)
		}
		if c.TargetPlayerID != "" && (c.TargetPlayerID == c.PlayerID || !a.isSeated(c.TargetPlayerID)) {
			return errors.New("table: invalid reaction target")
		}
		var extra []types.TransactWriteItem
		if reactions.IsPremium(c.ReactionID) {
			if a.reactionMarkUsed == nil {
				return errors.New("table: reaction usage recorder unavailable")
			}
			// This conditional write commits atomically with the reaction and is
			// the serialization point against refunds: exactly one can win.
			usedIntent, err := a.reactionMarkUsed(ctx, c.PlayerID, c.ReactionID)
			if err != nil {
				return fmt.Errorf("table: build premium reaction usage: %w", err)
			}
			if usedIntent != nil {
				extra = append(extra, *usedIntent)
			}
		}
		a.markLastAction(c.PlayerID)
		now := timeNowFunc().UnixMilli()
		a.activity.Reactions = append(a.activity.Reactions, tablestore.Reaction{
			ID: c.ActionID, PlayerID: c.PlayerID, ReactionID: c.ReactionID,
			TargetPlayerID: c.TargetPlayerID, Timestamp: now, ExpiresAt: now + reactionLifetime.Milliseconds(),
		})
		if len(a.activity.Reactions) > maxPersistedReactions {
			a.activity.Reactions = append([]tablestore.Reaction(nil), a.activity.Reactions[len(a.activity.Reactions)-maxPersistedReactions:]...)
		}
		return a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
			PlayerID: c.PlayerID, ActionID: c.ActionID, Action: "reaction",
			ReactionID: c.ReactionID, TargetPlayerID: c.TargetPlayerID,
		}, extra...)
	})
}

func (a *Actor) handlePreselect(ctx context.Context, c PreselectCmd) error {
	if c.ActionID == "" {
		return errors.New("table: action_id is required")
	}
	if c.Selection != "" && c.Selection != "check_fold" && c.Selection != "fold" &&
		c.Selection != "call" && c.Selection != "call_any" && c.Selection != "all_in" {
		return errors.New("table: invalid action preselection")
	}
	return a.commitActivity(ctx, true, func() error {
		if !a.isSeated(c.PlayerID) {
			return fmt.Errorf("table: player %s is not seated", c.PlayerID)
		}
		stage := a.cached.ViewFor("").Stage
		if c.ExpectedHandID == "" || a.handID != c.ExpectedHandID {
			return errors.New("table: stale action state")
		}
		// New clients scope this harmless future intent to hand+street instead
		// of the whole table version. Activity, presence and another player's
		// action may legitimately advance version while the frame is in flight.
		// Keep exact-version validation only as a rolling-deploy fallback for
		// older clients that do not send expected_stage yet.
		if c.ExpectedStage != "" {
			if c.ExpectedStage != stage {
				return errors.New("table: stale action state")
			}
		} else if c.ExpectedSnapshotVersion == 0 || uint64(a.version) != c.ExpectedSnapshotVersion {
			return errors.New("table: stale action state")
		}
		if c.Selection == "call" && (c.Amount <= 0 || c.Amount != a.cached.ProspectiveCallAmountForActor(c.PlayerID)) {
			return errors.New("table: fixed call amount changed")
		}
		if c.Selection == "check_fold" {
			c.Amount = a.cached.ProspectiveCallAmountForActor(c.PlayerID)
		} else if c.Selection != "call" {
			c.Amount = 0
		}
		if a.activity.Preselections == nil {
			a.activity.Preselections = make(map[string]tablestore.Preselection)
		}
		if c.Selection == "" {
			delete(a.activity.Preselections, c.PlayerID)
		} else {
			a.activity.Preselections[c.PlayerID] = tablestore.Preselection{
				Selection: c.Selection, Amount: c.Amount, HandID: a.handID, Stage: stage,
			}
		}
		a.markLastAction(c.PlayerID)
		action := "preselect_action"
		if c.Selection == "" {
			action = "clear_preselection"
		}
		return a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
			PlayerID: c.PlayerID, ActionID: c.ActionID, Action: action, Selection: c.Selection, Amount: c.Amount,
		})
	})
}

// prunePreselections enforces the lifetime promised on the wire: a prepared
// action belongs to exactly one hand and one betting street. Keeping stale
// entries hidden only at snapshot time is insufficient because the inline
// executor would otherwise find and execute them when that player becomes
// current on a later street or hand.
func (a *Actor) prunePreselections() {
	if a.cached == nil || a.activity.Preselections == nil {
		return
	}
	stage := a.cached.ViewFor("").Stage
	for playerID, preselection := range a.activity.Preselections {
		if preselection.HandID != a.handID || preselection.Stage != stage {
			delete(a.activity.Preselections, playerID)
		}
	}
}

func (a *Actor) commitActivity(ctx context.Context, broadcast bool, apply func() error) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	err := apply()
	if errors.Is(err, tablestore.ErrVersionConflict) {
		if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
			return reloadErr
		}
		err = apply()
	}
	if errors.Is(err, tablestore.ErrDuplicateAction) {
		if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
			return reloadErr
		}
		err = nil
	}
	if err != nil {
		return err
	}
	if broadcast {
		a.broadcastAll()
	}
	return nil
}

// handleSnapshot loads the table (seeding on first touch) and returns the
// viewer-specific snapshot. Built inside Run so it never races broadcastAll's
// concurrent ViewFor calls over a.cached.
func (a *Actor) handleSnapshot(ctx context.Context, c SnapshotCmd) error {
	// A snapshot is an explicit synchronization boundary. Always read the
	// authoritative item: another fleet actor is allowed to commit without
	// owning this actor's cache-affinity lease.
	if err := a.ensureLoaded(ctx, true); err != nil {
		return err
	}
	snapshot := a.cached.ViewFor(c.PlayerID)
	snapshot.SnapshotVersion = uint64(a.version)
	snapshot.HandID = a.handID
	current := a.cached.CurrentPlayerIDForActor()
	if current != "" && current == a.turnDeadlineFor {
		snapshot.ActionDeadlineUnixMs = a.turnDeadline.UnixMilli()
		snapshot.ActionBaseDeadlineUnixMs = a.turnBaseDeadline.UnixMilli()
	}
	if a.cached.Stage() == hand.Complete && a.handID == a.nextHandArmedFor {
		snapshot.NextHandUnixMs = a.nextHandDeadline.UnixMilli()
	}
	for _, p := range a.cached.PlayersForActor() {
		if p.ID == c.PlayerID && p.LastActionAt > 0 {
			snapshot.IdleRemovalUnixMs = p.LastActionAt + a.kickGrace.Milliseconds()
			break
		}
	}
	a.applyPresence(snapshot.Seats)
	a.applyStreaks(snapshot.Seats)
	a.applyActivity(c.PlayerID, &snapshot)
	c.Snapshot <- snapshot
	return nil
}

// handleSetIdentity persists display identity with the seat so every actor in the
// fleet produces the same snapshot. A no-op name does not bump table version.
func (a *Actor) handleSetIdentity(ctx context.Context, c SetIdentityCmd) error {
	if c.Name == "" && c.AvatarURL == "" && c.PlaystyleBadge == "" {
		return nil
	}
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	apply := func() error {
		if !a.cached.SetPlayerIdentityForActor(c.PlayerID, c.Name, c.AvatarURL, c.PlaystyleBadge) {
			return nil
		}
		return a.commit(ctx, "", &tablestore.ActionLogEntry{
			PlayerID: c.PlayerID, Action: "set_identity",
		})
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		return err
	}
	a.broadcastAll()
	return nil
}

func (a *Actor) handleEscalate(ctx context.Context) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	apply := func() error {
		a.cached.EscalateBlindsForActor(a.escalationCfg.Multiplier, a.escalationCfg.Max)
		return a.commit(ctx, "", &tablestore.ActionLogEntry{Action: "escalate_blinds"})
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		return err
	}
	a.broadcastAll()
	return nil
}

// ensureLoaded reads current state from the store the first time this Actor
// is used, or whenever force is true (a prior commit proved the local cache
// stale). force never mutates a.trustCache — trustCache reflects only
// whether this Actor's own lease-affinity was granted at construction; a
// version conflict is evidence about staleness at this moment, not a
// permanent downgrade or upgrade of that grant.
func (a *Actor) ensureLoaded(ctx context.Context, force bool) error {
	// A duplicate seat can only exist in a.cached if some earlier mutation
	// (e.g. applyJoinAndCommit's append) was never actually committed — commit's
	// own DuplicateSeatIDForActor guard is what stopped it — yet the handler
	// that produced it had no before/rollback to undo it locally (unlike
	// applyLeaveAndCommit/applyJoinAndCommit, applyReadyAndCommit mutates
	// a.cached in place with no snapshot to restore). Trusting this cache
	// forever after that (trustCache skips every future reload) is exactly
	// how the 2026-09-01 incident's ghost seat kept surviving until an
	// unrelated commit finally persisted it for real. Force a genuine reload
	// the moment this is detected, regardless of trustCache, so the next
	// command starts from DynamoDB's still-clean state instead of this fork.
	if a.cached != nil && a.trustCache && !force {
		if _, dup := a.cached.DuplicateSeatIDForActor(); dup {
			force = true
		}
	}
	if a.cached != nil && a.trustCache && !force {
		a.cached.ConfigureRunItTwice(a.runItTwiceEnabled.Load())
		a.refreshStreaks(ctx)
		a.syncFleetConns(false)
		return nil
	}
	if a.store == nil {
		return nil
	}
	stored, err := a.store.LoadTable(ctx, a.id)
	if err != nil {
		return err
	}
	if stored == nil {
		// tablestore.ErrUnavailable, not a plain error: this aborts the command
		// before any of its own validation runs, so the gateway must answer
		// "table unavailable" rather than blaming the player's action.
		return fmt.Errorf("%w: no state seeded for this table yet", tablestore.ErrUnavailable)
	}
	a.cached = hand.NewTableFromState(stored.State)
	a.cached.ConfigureRunItTwice(a.runItTwiceEnabled.Load())
	a.version = stored.Version
	a.handID = stored.HandID
	a.activity = stored.Activity
	a.pendingPersistedDeadline = stored.TurnDeadlineUnixMs
	a.pendingDeadlineFor = a.cached.CurrentPlayerIDForActor()
	a.pendingDeadlineForStage = a.cached.Stage()
	a.pendingNextHandDeadline = stored.NextHandDeadlineUnixMs
	a.pruneStalePresence()
	a.rearmTimersFromCache()
	a.refreshStreaks(ctx)
	a.syncFleetConns(false)
	return nil
}

// pruneStalePresence drops disconnect/kick bookkeeping for anyone no longer
// in the freshly reloaded cache — e.g. another actor instance already
// completed their leave/cash-out before this reload. Without this, a kick
// timer armed against the old, now-stale player list keeps firing and
// retrying forever against a player RemovePlayerForActor will never find
// again (same self-healing guarantee rearmTimersFromCache gives the other
// timers on every reload).
func (a *Actor) pruneStalePresence() {
	for playerID, timer := range a.kickTimers {
		if !a.isSeated(playerID) {
			timer.Stop()
			delete(a.kickTimers, playerID)
			delete(a.disconnectedSince, playerID)
		}
	}
	for playerID := range a.disconnectedSince {
		if !a.isSeated(playerID) {
			delete(a.disconnectedSince, playerID)
		}
	}
}

// rearmTimersFromCache re-derives and re-arms the turn/runout/next-hand
// timers from whatever is now cached. Runout timers are idempotent per
// (handID, phase, stage); the other timers use their corresponding hand/stage
// keys (see armTurnTimer/armRunoutTimer/armNextHandTimer), so calling this on
// every fresh load is safe and cheap. This is what lets a
// hand self-heal from the fleet's core failure mode: these timers are bare
// in-process time.AfterFunc calls with no persisted counterpart, so if the
// actor instance that armed one dies before it fires (node restart, lease
// handoff, autoscale-in), nothing else ever resumes it — the hand (most
// visibly an all-in runout) stays frozen forever even though the table's
// persisted state is fine. ensureLoaded runs on every command a
// non-lease-holding actor handles and on first construction everywhere, so
// the next bit of traffic this table sees from ANY node — even a bare
// ping's ReconnectCmd — re-derives these timers from durable state instead
// of trusting whatever the previous instance's memory (now gone) intended.
// Reveal grace is a live-broadcast pacing nicety, not a correctness
// requirement, so this always arms with zero grace — broadcastAll still
// applies the real grace on its own next call for anyone actually watching.
func (a *Actor) rearmTimersFromCache() {
	if a.cached == nil {
		return
	}
	stage := a.cached.Stage()
	a.armTurnTimer(a.cached.CurrentPlayerIDForActor(), stage, 0)
	a.armRunoutTimer(a.cached.IsAwaitingRunoutForActor(), stage)
	a.armNextHandTimer(stage == hand.Complete)
	a.armWinnerCardsTimer(a.cached.PendingWinnerCards())
}

func (a *Actor) handleReady(ctx context.Context, c ReadyCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	apply := func() error { return a.applyReadyAndCommit(ctx, c) }
	if err := a.retryOnConflict(ctx, apply); err != nil {
		if !errors.Is(err, tablestore.ErrDuplicateAction) {
			return err
		}
		if err := a.ensureLoaded(ctx, true); err != nil {
			return err
		}
	}
	a.broadcastAll()
	return nil
}

func (a *Actor) applyReadyAndCommit(ctx context.Context, c ReadyCmd) error {
	if !a.isSeated(c.PlayerID) {
		return fmt.Errorf("table: player %s is not seated", c.PlayerID)
	}
	// Snapshot before any in-place mutation, same discipline as
	// applyLeaveAndCommit/applyJoinAndCommit: c.Ready can drive tryStartHand
	// (a full StartHand() — dealing, dealer rotation, blind posting) straight
	// into a.cached, and a.handID alongside it. A commit failure below that
	// isn't a version conflict (a transient store error, not just something
	// retryOnConflict already reloads on) must not leave that uncommitted
	// mutation trusted in this actor's cache with no matching
	// poker_action_log entry — that is exactly what let the 2026-09-01
	// incident's ghost seat and dropped player survive to be persisted for
	// real by a later, unrelated successful commit.
	before := a.cached.ExportState()
	beforeHandID := a.handID
	a.markLastAction(c.PlayerID)
	for _, p := range a.cached.PlayersForActor() {
		if p.ID == c.PlayerID {
			p.Ready = c.Ready
		}
	}
	action := "not_ready"
	if c.Ready {
		a.cached.RequestReturnFromSitOut(c.PlayerID)
		action = "ready"
		// Sit-out (ready:false) never raises the ready-player count, so it must not
		// trigger tryStartHand: doing so during Stage==Complete forced the "not enough
		// ready players" fallback early, snapping the table back to WaitingForPlayers
		// and clearing payouts before next_hand_unix_ms elapsed — killing the other
		// player's win banner mid-countdown. armNextHandTimer still starts the next
		// hand once the grace period actually ends.
		if a.cached.Stage() == hand.WaitingForPlayers {
			a.tryStartHand(ctx)
		}
	} else {
		a.cached.SitOutForActor(c.PlayerID)
	}
	if err := a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
		PlayerID: c.PlayerID, ActionID: c.ActionID, Action: action,
	}); err != nil {
		a.cached = hand.NewTableFromState(before)
		a.handID = beforeHandID
		return err
	}
	return nil
}

// tryStartHand attempts to start a new hand if the table is between hands.
// "need at least 2 ready players" is not a caller error — the table just
// keeps waiting; StartHand's own error is swallowed here on purpose. Called
// from both a Ready toggle and a fresh Join, since a join alone can now bring
// the table to 2+ ready players (auto-ready on join). Complete is deliberately
// excluded: nextHandCmd owns that transition and preserves the reveal window.
func (a *Actor) tryStartHand(ctx context.Context) {
	if a.cached.Stage() == hand.WaitingForPlayers {
		if err := a.cached.StartHand(); err == nil {
			a.handID = newHandID()
			a.prunePreselections()
		}
	}
}

// saveHandHistorySnapshot persists the table's current (pre-reset) state to
// poker_table_state_history for audit purposes. Best-effort: this is an
// append-only audit copy, not the authoritative item, so a failure here never
// blocks the hand transition — it only emits a metric. A version-conflict
// retry re-running tryStartHand is harmless: once another instance has
// already advanced the stage past Complete, the reloaded cache no longer
// satisfies the Complete guard above, so the snapshot is not repeated.
func (a *Actor) saveHandHistorySnapshot(ctx context.Context) {
	if a.store == nil {
		return
	}
	if err := a.store.SaveTableStateHistory(ctx, a.id, timeNowFunc().Unix(), a.cached.ExportState()); err != nil {
	}
}

func (a *Actor) handleAct(ctx context.Context, c ActCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	if err := a.validateActionPrecondition(ctx, c); err != nil {
		return err
	}
	a.markLastAction(c.PlayerID)
	_, err := a.applyActAndCommit(ctx, c)
	if err != nil && !errors.Is(err, tablestore.ErrDuplicateAction) {
		// Two distinct reasons to reload and retry exactly once:
		//   - ErrVersionConflict: another instance committed first: definite
		//     staleness.
		//   - trustCache==true and any other error: this instance normally
		//     trusts a.cached between commits (ensureLoaded(ctx,false) is a
		//     no-op once cached is set) and only re-reads on a version
		//     conflict from ITS OWN write attempts. But ARCHITECTURE.md §2 /
		//     tablews.go's RegisterTableWS explicitly allow any instance to
		//     accept any table's connections directly, with no proxying to
		//     the lease holder — so another instance can commit actions this
		//     one never observes, and its next local Act() call (e.g. a
		//     turn-order check) can reject a genuinely legal action against
		//     stale data without ever reaching a conditional write to
		//     conflict on. Retrying here can't mask a truly invalid action:
		//     if it's genuinely invalid, re-running it against freshly
		//     loaded state reproduces the identical rejection.
		if errors.Is(err, tablestore.ErrVersionConflict) || a.trustCache {
			if errors.Is(err, tablestore.ErrVersionConflict) {
			}
			if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
				return reloadErr
			}
			a.markLastAction(c.PlayerID)
			_, err = a.applyActAndCommit(ctx, c)
			if errors.Is(err, tablestore.ErrVersionConflict) {
			} else {
			}
		}
	}
	if errors.Is(err, tablestore.ErrDuplicateAction) {
		// The guard proves another commit already won. Discard the speculative
		// local mutation before outcome logging or broadcasting.
		if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
			return reloadErr
		}
	}
	if err != nil && !errors.Is(err, tablestore.ErrDuplicateAction) {
		return err
	}
	if err := a.commitOutcomeLogEntries(ctx); err != nil {
		return err
	}
	a.broadcastAll()
	return nil
}

func (a *Actor) validateActionPrecondition(ctx context.Context, c ActCmd) error {
	// Internal/system commands and older direct unit tests omit preconditions.
	// The WebSocket boundary requires both for every user-originated act.
	if c.ExpectedSnapshotVersion == 0 && c.ExpectedHandID == "" {
		return nil
	}
	if c.ExpectedSnapshotVersion == 0 || c.ExpectedHandID == "" {
		return fmt.Errorf("table: incomplete action precondition")
	}
	if uint64(a.version) != c.ExpectedSnapshotVersion || a.handID != c.ExpectedHandID {
		if err := a.ensureLoaded(ctx, true); err != nil {
			return err
		}
	}
	if uint64(a.version) != c.ExpectedSnapshotVersion || a.handID != c.ExpectedHandID {
		return fmt.Errorf("table: stale action state")
	}
	return nil
}

// HandHookClaimer answers whether this instance is the one allowed to run a
// given hand's post-hand hooks. See internal/handhook for why the claim must
// be atomic and cross-instance.
type HandHookClaimer interface {
	Claim(ctx context.Context, tableID, handID string) (bool, error)
}

// handHookTimeout bounds the claim round trip so an unreachable Valkey cannot
// stall the table's actor goroutine.
const handHookTimeout = 2 * time.Second

// SetHandHookClaimerForActor wires the shared hook claim. Set once, right
// after construction, by tablemanager.
func (a *Actor) SetHandHookClaimerForActor(c HandHookClaimer) { a.handHooks = c }

// claimHandHooks reports whether this instance may run the current hand's
// post-hand hooks. A claim error fails OPEN: an unreachable Valkey degrades
// back to the previous at-least-once behaviour, because silently skipping the
// hooks would permanently lose a hand's achievements, streak and auto-rebuy
// while a double credit is at least visible and bounded.
func (a *Actor) claimHandHooks() bool {
	if a.handHooks == nil {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), handHookTimeout)
	defer cancel()
	claimed, err := a.handHooks.Claim(ctx, a.id, a.handID)
	if err != nil {
		slog.Warn("table hand hook claim failed", "table_id", a.id, "hand_id", a.handID, "err", err)
		return true
	}
	return claimed
}

// notifyHandComplete runs one completed hand's gamification hooks. It is
// reached from broadcastAll, which any instance can call for a table already
// sitting on Complete (chat, reactions and connect/disconnect all broadcast),
// so the local completedHandNotified guard is not enough on its own — the
// hooks downstream are non-idempotent counters. handHooks is what makes this
// once per hand for the whole fleet.
func (a *Actor) notifyHandComplete() {
	if a.cached == nil || a.cached.Stage() != hand.Complete || a.handID == "" || a.completedHandNotified == a.handID {
		return
	}
	if outcome := a.cached.LastOutcomeForActor(); outcome != nil {
		// Mark before claiming: a lost claim means another instance owns this
		// hand, and re-asking on every later broadcast of the same hand would
		// be one wasted round trip per chat message.
		a.completedHandNotified = a.handID
		if !a.claimHandHooks() {
			return
		}
		if a.onHandComplete != nil {
			names := make(map[string]string)
			for _, p := range a.cached.PlayersForActor() {
				if p.Name != "" {
					names[p.ID] = p.Name
				}
			}
			hookOutcome := *outcome
			hookOutcome.FairnessProofs = a.cached.FairnessProofsForActor()
			a.onHandComplete(a.handID, hookOutcome, names)
		}
	}
}

// SetOnHandCompleteForActor installs the post-commit gamification hook.
// The actor invokes it at most once per local hand ID. The map param is the
// persisted seat-name map (player_id -> display name), handed along so
// downstream writers can denormalize a name without a fresh profile read.
func (a *Actor) SetOnHandCompleteForActor(fn func(string, hand.HandOutcome, map[string]string)) {
	a.onHandComplete = fn
}

// SetOnHandUpdatedForActor installs the history-only hook used when a
// participant voluntarily reveals cards after completion.
func (a *Actor) SetOnHandUpdatedForActor(fn func(string, hand.HandOutcome, map[string]string)) {
	a.onHandUpdated = fn
}

// SetOnSeatsChangedForActor installs the post-commit occupancy write-through
// hook, invoked with the new occupied-seat count after every committed join
// or leave.
func (a *Actor) SetOnSeatsChangedForActor(fn func(int)) { a.onSeatsChanged = fn }

// SetOnPlayerRemovedForActor installs the system-removal notification hook —
// see onPlayerRemoved's doc comment.
func (a *Actor) SetOnPlayerRemovedForActor(fn func(playerID, reason string, stack int64, holdID string)) {
	a.onPlayerRemoved = fn
}

func (a *Actor) SetSystemSettlementIntentForActor(fn func(context.Context, string, string, int64, string) (types.TransactWriteItem, error)) {
	a.systemSettlementIntent = fn
}

func (a *Actor) SetReactionOwnershipForActor(fn func(ctx context.Context, playerID, reactionID string) (bool, error)) {
	a.reactionOwnership = fn
}

func (a *Actor) SetReactionMarkUsedForActor(fn func(ctx context.Context, playerID, reactionID string) (*types.TransactWriteItem, error)) {
	a.reactionMarkUsed = fn
}

func (a *Actor) notifySeatsChanged() {
	if a.onSeatsChanged != nil && a.cached != nil {
		a.onSeatsChanged(len(a.cached.PlayersForActor()))
	}
}

// applyActAndCommit returns completed=true only when this Actor successfully
// committed the transition to Complete. A duplicate observed after another
// instance won the conditional write therefore cannot emit gamification a
// second time from this process.

func (a *Actor) applyActAndCommit(ctx context.Context, c ActCmd) (bool, error) {
	bettingAction := a.cached.NormalizedActionForActor(c.PlayerID, c.Action)
	applied, err := a.cached.ActIdempotent(c.ActionID, c.PlayerID, c.Action, c.Amount)
	if err != nil {
		return false, err
	}
	if !applied {
		return false, nil
	}
	if a.activity.Preselections != nil {
		delete(a.activity.Preselections, c.PlayerID)
		// A fixed call means exactly the amount visible when it was selected.
		// Any raise that changes what another player owes cancels it atomically
		// with the action, so reconnecting clients never revive a stale call.
		for playerID, preselection := range a.activity.Preselections {
			current := a.cached.ProspectiveCallAmountForActor(playerID)
			if preselection.Selection == "call" && preselection.Amount != current {
				delete(a.activity.Preselections, playerID)
			}
			if preselection.Selection == "check_fold" && current > preselection.Amount {
				delete(a.activity.Preselections, playerID)
			}
		}
		a.prunePreselections()
	}
	timeBankMs := a.consumeTimeBank(c.PlayerID)
	action := string(c.Action)
	if a.cached.PlayerAllInForActor(c.PlayerID) {
		action = "all_in"
	}
	entry := tablestore.ActionLogEntry{
		PlayerID: c.PlayerID, ActionID: c.ActionID, Action: action,
		BettingAction: string(bettingAction), Amount: c.Amount, TimeBankMs: timeBankMs,
	}
	if err := a.commit(ctx, c.ActionID, &entry); err != nil {
		return false, err
	}
	return a.cached.Stage() == hand.Complete, nil
}

// commitOutcomeLogEntries appends one "won" or "tie" ActionLogEntry per
// winner once a hand reaches Complete, so the hand-history timeline shows
// final results alongside the actions that led to them. Both handleAct (the
// final betting action) and handleRunoutStep (an all-in runout's last dealt
// street) can be the call that pushes a hand to Complete, so this guards on
// handID to log each hand's outcome exactly once regardless of which one
// got there.
func (a *Actor) commitOutcomeLogEntries(ctx context.Context) error {
	if a.cached == nil || a.cached.Stage() != hand.Complete || a.handID == "" || a.outcomeLoggedForHand == a.handID {
		return nil
	}
	outcome := a.cached.LastOutcomeForActor()
	if outcome == nil {
		return nil
	}
	entries := make([]tablestore.ActionLogEntry, 0)
	if outcome.WonWithoutShowdown {
		for _, id := range outcome.Winners {
			entries = append(entries, tablestore.ActionLogEntry{PlayerID: id, Action: "won", Amount: outcome.Payouts[id]})
		}
	} else {
		ids := make([]string, 0, len(outcome.ShowdownResults))
		for id := range outcome.ShowdownResults {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			result := outcome.ShowdownResults[id]
			entries = append(entries, tablestore.ActionLogEntry{PlayerID: id, Action: result.Action(), Amount: outcome.Payouts[id]})
		}
	}
	for _, entry := range entries {
		actionID := "outcome:" + a.handID + ":" + entry.PlayerID
		entry.ActionID = actionID
		for {
			err := a.commit(ctx, actionID, &entry)
			if err == nil {
				break
			}
			if !errors.Is(err, tablestore.ErrDuplicateAction) && !errors.Is(err, tablestore.ErrVersionConflict) {
				return err
			}
			// A prior partial attempt may already have written this row and
			// advanced the table version. Reload before continuing so the next
			// missing outcome entry does not conflict against that stale
			// version. A version conflict retries this same deterministic row.
			if err := a.ensureLoaded(ctx, true); err != nil {
				return err
			}
			if errors.Is(err, tablestore.ErrDuplicateAction) {
				break
			}
		}
	}
	// Do not suppress retries after a partial write. Deterministic action IDs
	// make already-written rows safe to replay.
	a.outcomeLoggedForHand = a.handID
	return nil
}

func (a *Actor) commit(ctx context.Context, actionID string, entry *tablestore.ActionLogEntry, extra ...types.TransactWriteItem) error {
	if a.store == nil {
		// Mirrors ensureLoaded's nil-store no-op: unit tests construct an
		// Actor with a nil store to exercise engine-level handler logic
		// without a real (DynamoDB Local) backing store. Never nil in
		// production — the manager always supplies a real *tablestore.Store.
		a.version++
		return nil
	}
	// Last-line backstop (2026-09-01 incident: a player seated twice at
	// 01M1C5GQR7HWXSNSSX8Q49XN9X after a non-version-conflict commit failure
	// left a.cached poisoned with an uncommitted duplicate seat that a later,
	// unrelated successful commit then persisted for real). Never persist a
	// state with two seats sharing one player ID, no matter which upstream
	// handler produced it — refuse loudly instead of writing corrupted state
	// that every subsequent read/broadcast/settlement would then trust.
	if dupID, dup := a.cached.DuplicateSeatIDForActor(); dup {
		// Never persist it — whatever mutation produced this was never
		// committed, so DynamoDB is still clean. Leave recovery to the next
		// ensureLoaded call (see its own duplicate check) rather than forcing
		// a reload here, which would race this function's own callers: both
		// applyLeaveAndCommit and applyJoinAndCommit unconditionally restore
		// a.cached to their pre-mutation snapshot on any commit error,
		// immediately after this returns — a reload performed here would
		// just get overwritten by that restore.
		return fmt.Errorf("table: refusing to commit duplicate seat for player %s", dupID)
	}
	newState := a.cached.ExportState()
	entry.TableID, entry.HandID, entry.Version = a.id, a.handID, a.version+1
	entry.Frame = replayFrameFor(a.cached.ViewFor(""))
	deadline := a.turnDeadlineForPersist()
	if err := a.store.CommitAction(ctx, a.id, a.handID, actionID, a.version, newState, a.activity,
		deadline, a.nextHandDeadlineForPersist(), *entry, extra...); err != nil {
		return err
	}
	a.version++
	return nil
}

// replayFrameFor deliberately copies only public gameplay state. In
// particular, SeatView.HoleCards is never persisted in the shared action log:
// the participant-scoped hand record remains the sole source of cards a
// replay viewer is allowed to see.
func replayFrameFor(snapshot hand.Snapshot) *tablestore.ReplayFrame {
	frame := &tablestore.ReplayFrame{
		Stage: snapshot.Stage, Board: append([]string(nil), snapshot.Board...),
		BoardTwo: append([]string(nil), snapshot.BoardTwo...), BoardSplitAt: snapshot.BoardSplitAt,
		CurrentPlayerID:    snapshot.CurrentPlayerID,
		DealerPlayerID:     snapshot.DealerPlayerID,
		SmallBlindPlayerID: snapshot.SmallBlindPlayerID,
		BigBlindPlayerID:   snapshot.BigBlindPlayerID,
		Payouts:            snapshot.Payouts, Winners: append([]string(nil), snapshot.Winners...),
		Seats: make([]tablestore.ReplaySeat, 0, len(snapshot.Seats)),
	}
	for _, pot := range snapshot.Pots {
		frame.Pot += pot.Amount
	}
	if frame.Pot == 0 {
		for _, seat := range snapshot.Seats {
			frame.Pot += seat.Contributed
		}
	}
	for _, seat := range snapshot.Seats {
		frame.Seats = append(frame.Seats, tablestore.ReplaySeat{
			PlayerID: seat.PlayerID, Name: seat.Name, Stack: seat.Stack,
			State: seat.State, Contributed: seat.Contributed, DealtIn: seat.DealtIn,
		})
	}
	return frame
}

// turnDeadlineForPersist returns the deadline to commit alongside this
// state: unchanged (already-armed) if this action didn't change whose turn
// it is or what stage they're acting in, freshly computed one turnTimeout
// from now if it did, 0 if no one is on the clock. armTurnTimer (called from
// broadcastAll right after every commit) is the one source of truth for
// actually scheduling the timeout and for a.turnDeadlineFor/ForStage — this
// only has to agree closely enough that a later reload resumes the same
// instant instead of granting a fresh window (see StoredTable.TurnDeadlineUnixMs).
func (a *Actor) turnDeadlineForPersist() int64 {
	current := a.cached.CurrentPlayerIDForActor()
	if current == "" {
		return 0
	}
	if current == a.turnDeadlineFor && a.cached.Stage() == a.turnDeadlineForStage {
		return a.turnDeadline.UnixMilli()
	}
	return timeNowFunc().Add(a.turnTimeout + a.timeBankFor(current)).UnixMilli()
}

// nextHandDeadlineForPersist returns the post-hand countdown to commit
// alongside this state: the already-armed expiry when this actor is the one
// that armed it for this hand, a fresh one nextHandDelay from now when this
// commit is what completed the hand, and 0 whenever the table is not on
// Complete (so leaving Complete clears the stored value instead of leaving
// the previous hand's expiry behind). Like turnDeadlineForPersist above this
// only has to agree with armNextHandTimer closely enough that a reload
// resumes the same instant — armNextHandTimer, which runs from broadcastAll
// immediately after every commit, stays the one scheduler of record.
func (a *Actor) nextHandDeadlineForPersist() int64 {
	if a.cached == nil || a.cached.Stage() != hand.Complete {
		return 0
	}
	if a.handID != "" && a.handID == a.nextHandArmedFor {
		return a.nextHandDeadline.UnixMilli()
	}
	return timeNowFunc().Add(a.nextHandDelay).UnixMilli()
}

func (a *Actor) timeBankFor(playerID string) time.Duration {
	// Production room validation never permits a clock below five seconds.
	// Integration tests historically assign tiny clocks directly (rather
	// than through SetTurnTimeoutForActor), so treat those as test clocks and
	// do not silently append the real 30-second reserve.
	if !a.timeBankEnabled || a.turnTimeout < 5*time.Second || a.cached == nil {
		return 0
	}
	return time.Duration(a.cached.TimeBankForActor(playerID)) * time.Millisecond
}

// consumeTimeBank charges only the part of a decision made after the normal
// room clock expired. The total deadline and the durable balance are
// committed in the same conditionally-written table state, so a losing
// multi-server attempt is discarded and recomputed after reload.
func (a *Actor) consumeTimeBank(playerID string) int64 {
	if !a.timeBankEnabled || a.turnTimeout < 5*time.Second || playerID == "" || playerID != a.turnDeadlineFor || a.turnBaseDeadline.IsZero() {
		return 0
	}
	elapsed := timeNowFunc().Sub(a.turnBaseDeadline).Milliseconds()
	if elapsed <= 0 {
		return 0
	}
	before := a.cached.TimeBankForActor(playerID)
	after := a.cached.ConsumeTimeBankForActor(playerID, elapsed)
	slog.Info("table time bank consumed",
		"table", a.id, "hand", a.handID, "stage", a.cached.ViewFor("").Stage,
		"turn_player", a.turnDeadlineFor, "charged_player", playerID,
		"bank_before_ms", before, "bank_elapsed_ms", elapsed, "bank_after_ms", after,
		"base_deadline_unix_ms", a.turnBaseDeadline.UnixMilli(),
		"action_deadline_unix_ms", a.turnDeadline.UnixMilli())
	// The bank can run out mid-decision: charge what was actually deducted,
	// never the raw elapsed time.
	return before - after
}

// retryOnConflict runs apply once. If a version conflict is detected (another
// instance committed first), it reloads fresh state and applies once more.
// Handlers whose apply needs a return value beyond error (Act, Leave) keep
// their specialized retry; this covers the simple mutating handlers.
func (a *Actor) retryOnConflict(ctx context.Context, apply func() error) error {
	if err := apply(); err == nil {
		return nil
	} else if !errors.Is(err, tablestore.ErrVersionConflict) {
		return err
	}
	if err := a.ensureLoaded(ctx, true); err != nil {
		return err
	}
	return apply()
}

func (a *Actor) SetEnv(env string) { a.env = env }

// handleConnect fires exactly once per physical WS connection, right after
// the gateway registers it — unlike ReconnectCmd (fired on every inbound
// frame from every connection), this is the only place safe to count
// connections. A player with a second tab already open bumps this to 2; only
// the LAST connection to close (handleDisconnect dropping the count to 0)
// ever marks the player disconnected.
func (a *Actor) handleConnect(c ConnectCmd) error {
	if c.ConnID == "" {
		return errors.New("table: conn_id is required")
	}
	if a.activeConns[c.PlayerID] == nil {
		a.activeConns[c.PlayerID] = make(map[string]struct{})
	}
	if _, already := a.activeConns[c.PlayerID][c.ConnID]; !already {
		a.connCount.Add(1)
	}
	a.activeConns[c.PlayerID][c.ConnID] = struct{}{}
	// Force the publish: every other instance's dot for this player depends on
	// it, and a connect is rare enough to always be worth the round trip.
	a.syncFleetConns(true)
	if a.clearDisconnectMark(c.PlayerID) {
		a.broadcastAll()
	}
	return nil
}

func (a *Actor) handleDisconnect(c DisconnectCmd) error {
	if conns := a.activeConns[c.PlayerID]; conns != nil {
		if _, registered := conns[c.ConnID]; !registered {
			if c.ConnID != "" {
				return nil
			}
		} else {
			a.connCount.Add(-1)
		}
		delete(conns, c.ConnID)
		if len(conns) == 0 {
			delete(a.activeConns, c.PlayerID)
		}
	} else if c.ConnID != "" {
		return nil
	}
	if len(a.activeConns[c.PlayerID]) > 0 {
		return nil // another connection (another tab) for this player is still live
	}
	a.disconnectedSince[c.PlayerID] = timeNowFunc()
	a.armKickTimer(c.PlayerID)
	// activeConns no longer lists this player, so the forced sync is what
	// retracts them from the fleet set instead of waiting out EntryTTL.
	a.syncFleetConns(true)
	a.broadcastAll()
	return nil
}

func (a *Actor) handleReconnect(ctx context.Context, c ReconnectCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	// This runs on EVERY inbound frame (tablews.go's read loop dispatches it
	// ahead of every message, including plain keepalive pings) so any traffic
	// clears a stale disconnect mark. Broadcasting unconditionally here means
	// every ping from every seat re-pushes the snapshot to the whole table —
	// with N seats pinging independently that's an O(N) snapshot flood with
	// no state change behind it. Only broadcast when this player was actually
	// marked disconnected.
	if !a.clearDisconnectMark(c.PlayerID) {
		return nil
	}
	a.broadcastAll()
	return nil
}

// clearDisconnectMark deletes playerID's stale disconnect bookkeeping and
// reports whether anything was actually cleared, so callers only broadcast
// (or otherwise react) when this genuinely changed something.
func (a *Actor) clearDisconnectMark(playerID string) bool {
	if t, armed := a.kickTimers[playerID]; armed {
		t.Stop()
		delete(a.kickTimers, playerID)
	}
	if _, wasDisconnected := a.disconnectedSince[playerID]; !wasDisconnected {
		return false
	}
	delete(a.disconnectedSince, playerID)
	return true
}

// armKickTimer (re-)arms the auto-kick clock for a just-disconnected player.
// Only handleDisconnect calls this, exactly once per disconnect episode (the
// same invariant handleConnect/handleReconnect's clearDisconnectMark relies
// on), so unlike armTurnTimer there's no same-player no-op check needed.
func (a *Actor) armKickTimer(playerID string) {
	if t, armed := a.kickTimers[playerID]; armed {
		t.Stop()
	}
	a.kickTimers[playerID] = time.AfterFunc(a.kickGrace, func() {
		reply := make(chan error, 1)
		if err := a.Dispatch(kickTimeoutCmd{PlayerID: playerID, Reply: reply}); err != nil {
			slog.Warn("table kick timeout dispatch failed", "table_id", a.id, "player_id", playerID, "err", err)
		}
	})
}

// handleKickTimeout fires 5 minutes after a player disconnects and removes
// them from the table (same cash-out path as a manual Leave), freeing the
// seat for someone else. Stale if they reconnected since (clearDisconnectMark
// already stopped this timer, but a fire can still be in flight on the cmds
// channel when that happens) or already left.
func (a *Actor) handleKickTimeout(ctx context.Context, c kickTimeoutCmd) error {
	if _, disconnected := a.disconnectedSince[c.PlayerID]; !disconnected {
		return nil
	}
	if err := a.ensureLoaded(ctx, false); err != nil {
		return nil // transient load failure; the AFK sweep retries this seat
	}
	// The disconnect mark is in-memory and therefore instance-local, while any
	// instance may run an Actor for any table: a player whose socket dropped
	// here and reconnected through another instance leaves this mark uncleared
	// forever, because this actor never sees that connect. Removing on the
	// mark alone cashed out players who were sitting at the table playing.
	// LastActionAt is persisted by whichever instance they are actually on, so
	// it is the one piece of evidence that crosses instances: recent activity
	// means alive somewhere, and the mark is the stale side. A seat with no
	// LastActionAt at all (never acted since it was seeded) has no such
	// evidence and stays kickable. Silence past kickGrace is removed either
	// way — that is exactly what handleAFKSweep does to connected players too.
	if p := a.seatedPlayer(c.PlayerID); p != nil && p.LastActionAt > 0 &&
		timeNowFunc().Sub(time.UnixMilli(p.LastActionAt)) < a.kickGrace {
		a.clearDisconnectMark(c.PlayerID)
		a.broadcastAll()
		return nil
	}
	delete(a.kickTimers, c.PlayerID)
	// RemovePlayerForActor rejects removal while the player is still dealt
	// into a hand in progress (including after they folded). This is a normal
	// race at a busy table, so a failed removal must not consume the only kick
	// attempt. The durable between-hands sweep in handleNextHand is the primary
	// guarantee across actor replacement; this retry covers the same actor.
	stackCh := make(chan int64, 1)
	holdIDCh := make(chan string, 1)
	if err := a.handleLeave(ctx, a.systemLeaveCmd(ctx, c.PlayerID, "disconnected", stackCh, holdIDCh)); err != nil {
		if errors.Is(err, hand.ErrPlayerNotFound) {
			// Terminal, not a race: the player is already gone (removed by
			// another actor instance, or table cleanup), so there is nothing
			// left to retry — retrying forever just spams this WARN on an
			// otherwise-healthy, possibly now-empty table.
			delete(a.disconnectedSince, c.PlayerID)
			return nil
		}
		a.armKickRetry(c.PlayerID)
		return err
	}
	if a.onPlayerRemoved != nil {
		a.onPlayerRemoved(c.PlayerID, "disconnected", <-stackCh, <-holdIDCh)
	}
	return nil
}

func (a *Actor) armKickRetry(playerID string) {
	a.kickTimers[playerID] = time.AfterFunc(a.afkSweepInterval, func() {
		reply := make(chan error, 1)
		if err := a.Dispatch(kickTimeoutCmd{PlayerID: playerID, Reply: reply}); err != nil {
			slog.Warn("table kick retry dispatch failed", "table_id", a.id, "player_id", playerID, "err", err)
		}
	})
}

// markLastAction stamps playerID's LastActionAt with now — called only from
// handlers driven by a genuine inbound client command (Act, Ready, SitOut),
// never from a server-synthesized one (turn-timeout fold, disconnect
// auto-sit-out, the AFK sweep's own removal), so it reflects actual human
// presence rather than the system acting on the player's behalf. A no-op if
// the player isn't seated (defensive; every caller already resolved them).
func (a *Actor) markLastAction(playerID string) {
	for _, p := range a.cached.PlayersForActor() {
		if p.ID == playerID {
			p.LastActionAt = timeNowFunc().UnixMilli()
			return
		}
	}
}

func (a *Actor) isSeated(playerID string) bool {
	return a.seatedPlayer(playerID) != nil
}

// seatedPlayer returns playerID's entry in the currently cached state, or nil
// when they aren't seated (or nothing is loaded yet). Read-only — callers that
// mutate go through the engine so the change is committed.
func (a *Actor) seatedPlayer(playerID string) *hand.Player {
	if a.cached == nil {
		return nil
	}
	for _, p := range a.cached.PlayersForActor() {
		if p.ID == playerID {
			return p
		}
	}
	return nil
}

// armAFKSweepTimer (re-)arms the periodic AFK sweep. Unlike every other timer
// in this file, it isn't tied to a game-state transition — it re-arms itself
// unconditionally after every fire (from handleAFKSweep), so it keeps
// checking every AFKSweepInterval for as long as the actor is alive. Once the
// actor dies (Run's ctx cancelled), the next fire's Dispatch hits a.done and
// returns ErrActorStopped without re-arming, so this doesn't outlive the
// actor.
func (a *Actor) armAFKSweepTimer() {
	a.afkSweepTimer = time.AfterFunc(a.afkSweepInterval, func() {
		reply := make(chan error, 1)
		if err := a.Dispatch(afkSweepCmd{Reply: reply}); err != nil {
			slog.Warn("table AFK sweep dispatch failed", "table_id", a.id, "err", err)
		}
	})
}

// handleAFKSweep exists because the per-turn timer only ever checks whoever
// the engine currently prompts (armTurnTimer/handleTurnTimeout) — a player
// who never becomes "current" for a long stretch, or one whose disconnect
// bookkeeping was lost to an actor restart (disconnectedSince/kickTimers are
// in-memory only; LastActionAt is persisted specifically so this check
// survives that), would otherwise occupy a seat forever. Reuses kickGrace as
// the staleness threshold — same "gone" semantics whether detected via
// transport disconnect or via activity silence, both flowing into the exact
// same removal path (handleLeave) as the disconnect-driven kick, including
// its existing has-a-live-hand guard (a stale player mid-hand is retried on
// the next sweep instead of being force-removed there).
func (a *Actor) handleAFKSweep(ctx context.Context, c afkSweepCmd) error {
	defer a.armAFKSweepTimer()
	if err := a.ensureLoaded(ctx, false); err != nil {
		return nil // transient load failure; next sweep retries
	}
	// The sweep is the only unconditional, self-perpetuating tick this actor
	// has, which makes it the natural watchdog for every timer — not just the
	// idle seats it was written for. ensureLoaded's trust-cache path skips
	// rearmTimersFromCache, and the sweep only broadcasts when it actually
	// removes someone, so without this a timer lost to a transient failure
	// (see handleNextHand) was never re-derived on a quiet table. Every arm
	// function is idempotent per hand/stage, so this is a no-op whenever the
	// timers are already correct.
	a.rearmTimersFromCache()
	now := timeNowFunc()
	var stale []string
	for _, p := range a.cached.PlayersForActor() {
		if p.LastActionAt == 0 {
			continue
		}
		if now.Sub(time.UnixMilli(p.LastActionAt)) >= a.kickGrace {
			stale = append(stale, p.ID)
		}
	}
	for _, id := range stale {
		stackCh := make(chan int64, 1)
		holdIDCh := make(chan string, 1)
		if err := a.handleLeave(ctx, a.systemLeaveCmd(ctx, id, "idle", stackCh, holdIDCh)); err != nil {
			continue // still dealt into a hand in progress; next sweep retries
		}
		if a.onPlayerRemoved != nil {
			a.onPlayerRemoved(id, "idle", <-stackCh, <-holdIDCh)
		}
	}
	return nil
}

// removeIdlePlayersBetweenHands closes the gap left by timer-based removal:
// a player cannot be removed safely while dealt into a live hand, and a
// periodic timer can repeatedly land inside live hands. LastActionAt is
// persisted, so checking it immediately before the next deal also survives
// actor replacement and guarantees a stale seat cannot enter another hand.
func (a *Actor) removeIdlePlayersBetweenHands(ctx context.Context) {
	now := timeNowFunc()
	var stale []string
	for _, p := range a.cached.PlayersForActor() {
		if p.LastActionAt > 0 && now.Sub(time.UnixMilli(p.LastActionAt)) >= a.kickGrace {
			stale = append(stale, p.ID)
		}
	}
	for _, id := range stale {
		stackCh := make(chan int64, 1)
		holdIDCh := make(chan string, 1)
		if err := a.handleLeave(ctx, a.systemLeaveCmd(ctx, id, "idle", stackCh, holdIDCh)); err != nil {
			continue
		}
		if a.onPlayerRemoved != nil {
			a.onPlayerRemoved(id, "idle", <-stackCh, <-holdIDCh)
		}
	}
}

// removeEligiblePendingExits removes and cashes out every PendingExit
// player no longer dealt into the current hand. Not gated behind
// claimHandHooks (that guard fleet-dedupes optional gamification side
// effects) — RemovePlayerForActor's own conditional commit already makes a
// duplicate attempt from another instance a safe, cheap no-op, the same
// protection handleLeave's ErrVersionConflict retry already relies on.
func (a *Actor) removeEligiblePendingExits(ctx context.Context) {
	var exiting []string
	for _, p := range a.cached.PlayersForActor() {
		if p.PendingExit && !a.cached.DealtIntoCurrentHandForActor(p.ID) {
			exiting = append(exiting, p.ID)
		}
	}
	for _, id := range exiting {
		stackCh := make(chan int64, 1)
		holdIDCh := make(chan string, 1)
		if err := a.handleLeave(ctx, a.systemLeaveCmd(ctx, id, "exit_requested", stackCh, holdIDCh)); err != nil {
			continue
		}
		if a.onPlayerRemoved != nil {
			a.onPlayerRemoved(id, "exit_requested", <-stackCh, <-holdIDCh)
		}
	}
}

func (a *Actor) handleSitOut(ctx context.Context, c SitOutCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	apply := func() error {
		if !a.isSeated(c.PlayerID) {
			return fmt.Errorf("table: player %s is not seated", c.PlayerID)
		}
		a.markLastAction(c.PlayerID)
		a.cached.SitOutForActor(c.PlayerID)
		return a.commit(ctx, "", &tablestore.ActionLogEntry{PlayerID: c.PlayerID, Action: "sit_out"})
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		return err
	}
	a.broadcastAll()
	return nil
}

func (a *Actor) handleKeepSeat(ctx context.Context, c KeepSeatCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	apply := func() error {
		if !a.isSeated(c.PlayerID) {
			return fmt.Errorf("table: player %s is not seated", c.PlayerID)
		}
		a.markLastAction(c.PlayerID)
		return a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
			PlayerID: c.PlayerID, ActionID: c.ActionID, Action: "keep_seat",
		})
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		if !errors.Is(err, tablestore.ErrDuplicateAction) {
			return err
		}
		if err := a.ensureLoaded(ctx, true); err != nil {
			return err
		}
	}
	a.broadcastAll()
	return nil
}

// handlePeekCards logs a breadcrumb only — no seat or hand state changes, so
// unlike its siblings it never broadcasts (nothing any viewer's snapshot
// depends on actually changed).
func (a *Actor) handlePeekCards(ctx context.Context, c PeekCardsCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	apply := func() error {
		if !a.isSeated(c.PlayerID) {
			return fmt.Errorf("table: player %s is not seated", c.PlayerID)
		}
		return a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
			PlayerID: c.PlayerID, ActionID: c.ActionID, Action: "peek_cards",
		})
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		if !errors.Is(err, tablestore.ErrDuplicateAction) {
			return err
		}
		if err := a.ensureLoaded(ctx, true); err != nil {
			return err
		}
	}
	return nil
}

func (a *Actor) handleShowCards(ctx context.Context, c ShowCardsCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	changed := false
	apply := func() error {
		applied, err := a.cached.RevealHoleCard(c.PlayerID, c.CardIndex)
		if err != nil {
			return err
		}
		if !applied {
			return nil
		}
		changed = true
		return a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
			PlayerID: c.PlayerID, ActionID: c.ActionID, Action: "show_cards",
		})
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		if !errors.Is(err, tablestore.ErrDuplicateAction) {
			return err
		}
		if err := a.ensureLoaded(ctx, true); err != nil {
			return err
		}
	}
	if changed {
		a.broadcastAll()
		if outcome := a.cached.LastOutcomeForActor(); outcome != nil && a.onHandUpdated != nil {
			names := make(map[string]string)
			for _, p := range a.cached.PlayersForActor() {
				if p.Name != "" {
					names[p.ID] = p.Name
				}
			}
			hookOutcome := *outcome
			hookOutcome.FairnessProofs = a.cached.FairnessProofsForActor()
			a.onHandUpdated(a.handID, hookOutcome, names)
		}
	}
	return nil
}

func (a *Actor) handleRequestRabbitHunt(ctx context.Context, c RequestRabbitHuntCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	changed := false
	apply := func() error {
		if _, err := a.cached.RequestRabbitHunt(c.PlayerID); err != nil {
			return err
		}
		changed = true
		return a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
			PlayerID: c.PlayerID, ActionID: c.ActionID, Action: "request_rabbit_hunt",
		})
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		if !errors.Is(err, tablestore.ErrDuplicateAction) {
			return err
		}
		if err := a.ensureLoaded(ctx, true); err != nil {
			return err
		}
	}
	if changed {
		a.broadcastAll()
	}
	return nil
}

func (a *Actor) handleRequestExit(ctx context.Context, c RequestExitCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	changed := false
	apply := func() error {
		if !a.isSeated(c.PlayerID) {
			return fmt.Errorf("table: player %s is not seated", c.PlayerID)
		}
		a.markLastAction(c.PlayerID)
		if err := a.cached.RequestExit(c.PlayerID); err != nil {
			return err
		}
		changed = true
		return a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
			PlayerID: c.PlayerID, ActionID: c.ActionID, Action: "request_exit",
		})
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		if !errors.Is(err, tablestore.ErrDuplicateAction) {
			return err
		}
		if err := a.ensureLoaded(ctx, true); err != nil {
			return err
		}
	}
	if changed {
		a.broadcastAll()
	}
	return nil
}

func (a *Actor) handleCancelExit(ctx context.Context, c CancelExitCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	changed := false
	apply := func() error {
		if !a.isSeated(c.PlayerID) {
			return fmt.Errorf("table: player %s is not seated", c.PlayerID)
		}
		a.markLastAction(c.PlayerID)
		if err := a.cached.CancelExit(c.PlayerID); err != nil {
			return err
		}
		changed = true
		if a.cached.Stage() == hand.WaitingForPlayers {
			a.tryStartHand(ctx)
		}
		return a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
			PlayerID: c.PlayerID, ActionID: c.ActionID, Action: "cancel_exit",
		})
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		if !errors.Is(err, tablestore.ErrDuplicateAction) {
			return err
		}
		if err := a.ensureLoaded(ctx, true); err != nil {
			return err
		}
	}
	if changed {
		a.broadcastAll()
	}
	return nil
}

func (a *Actor) handleRequestWinnerCards(ctx context.Context, c RequestWinnerCardsCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	changed := false
	apply := func() error {
		if _, err := a.cached.RequestWinnerCards(c.PlayerID, timeNowFunc()); err != nil {
			return err
		}
		changed = true
		return a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
			PlayerID: c.PlayerID, ActionID: c.ActionID, Action: "request_winner_cards",
		})
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		// A rejected request can still have mutated a.cached on the way to the
		// rejection: RequestWinnerCards expires (and refunds) a stale pending
		// request before it validates the rest. Reload so that refund is never
		// left sitting uncommitted in memory for the next command to trust —
		// same discipline as handleTurnTimeout/handleNextHand.
		if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
			return reloadErr
		}
		if !errors.Is(err, tablestore.ErrDuplicateAction) {
			return err
		}
	}
	if changed {
		a.broadcastAll()
	}
	return nil
}

// handleAcceptWinnerCards and handleDeclineWinnerCards mirror
// handleRequestWinnerCards exactly — same conflict retry, same duplicate-action
// reload — because all three mutate the same per-hand request and must commit
// through the same conditional-write path.
func (a *Actor) handleAcceptWinnerCards(ctx context.Context, c AcceptWinnerCardsCmd) error {
	return a.applyWinnerCardsAnswer(ctx, c.PlayerID, c.ActionID, "accept_winner_cards",
		func() error { return a.cached.AcceptWinnerCards(c.PlayerID, timeNowFunc()) })
}

func (a *Actor) handleDeclineWinnerCards(ctx context.Context, c DeclineWinnerCardsCmd) error {
	return a.applyWinnerCardsAnswer(ctx, c.PlayerID, c.ActionID, "decline_winner_cards",
		func() error { return a.cached.DeclineWinnerCards(c.PlayerID) })
}

func (a *Actor) applyWinnerCardsAnswer(ctx context.Context, playerID, actionID, action string, mutate func() error) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	changed := false
	apply := func() error {
		if err := mutate(); err != nil {
			return err
		}
		changed = true
		return a.commit(ctx, actionID, &tablestore.ActionLogEntry{
			PlayerID: playerID, ActionID: actionID, Action: action,
		})
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		// AcceptWinnerCards expires a stale request and refunds the requester
		// before reporting that the window closed or the winner has left, so a
		// failed answer can still have moved chips in memory. Discard it.
		if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
			return reloadErr
		}
		if !errors.Is(err, tablestore.ErrDuplicateAction) {
			return err
		}
	}
	if changed {
		a.broadcastAll()
	}
	return nil
}

// handleExpireWinnerCards closes an unanswered consent window and refunds the
// requester. A stale timer (already answered, or the next hand already
// started) is a silent no-op.
func (a *Actor) handleExpireWinnerCards(ctx context.Context, _ expireWinnerCardsCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	changed := false
	apply := func() error {
		if !a.cached.ExpireWinnerCards(timeNowFunc()) {
			return nil
		}
		changed = true
		return a.commit(ctx, "", &tablestore.ActionLogEntry{Action: "expire_winner_cards"})
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
			return reloadErr
		}
		if !errors.Is(err, tablestore.ErrVersionConflict) {
			return err
		}
	}
	if changed {
		a.broadcastAll()
	}
	return nil
}

func (a *Actor) handleRabbitHuntVerifyFailed(ctx context.Context, c RabbitHuntVerifyFailedCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	changed := false
	apply := func() error {
		if err := a.cached.RefundRabbitHunt(c.PlayerID); err != nil {
			return err
		}
		changed = true
		return a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
			PlayerID: c.PlayerID, ActionID: c.ActionID, Action: "rabbit_hunt_verify_failed",
		})
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		if !errors.Is(err, tablestore.ErrDuplicateAction) {
			return err
		}
		if err := a.ensureLoaded(ctx, true); err != nil {
			return err
		}
	}
	if changed {
		a.broadcastAll()
	}
	return nil
}

func (a *Actor) handleSetRunItTwice(ctx context.Context, c SetRunItTwiceCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	changed := false
	apply := func() error {
		if !a.isSeated(c.PlayerID) {
			return fmt.Errorf("table: player %s is not seated", c.PlayerID)
		}
		if !a.cached.SetPlayerRunItTwiceForActor(c.PlayerID, c.Enabled) {
			return nil
		}
		changed = true
		return a.commit(ctx, "", &tablestore.ActionLogEntry{
			PlayerID: c.PlayerID, Action: "set_run_it_twice",
		})
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		return err
	}
	if changed {
		a.broadcastAll()
	}
	return nil
}

// handleTurnTimeout runs inside Run (dispatched by the universal per-turn
// timer) so it can safely read/write the actor's disconnect bookkeeping maps.
// It fires for whoever currently must act, regardless of connection state. A
// stale timer (the player already acted through the normal path before this
// fired) is a silent no-op — CurrentPlayerCanActForActor is false by then.
func (a *Actor) handleTurnTimeout(ctx context.Context, c turnTimeoutCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	// A canceled timer can already have queued its command while another
	// action advances the turn. Merely being eligible to act later in this
	// round is not enough: a raise may have reopened this player's action even
	// though somebody else is currently on the clock. Only the authoritative
	// current player may be folded by this particular deadline.
	if a.cached.CurrentPlayerIDForActor() != c.PlayerID {
		return nil
	}
	// A disconnected player who lets the clock run out is out of the game
	// immediately: auto-checking on their behalf keeps them in every street of
	// every hand at the full turn timeout, which stalls the whole table (worst
	// at a checked-down table, where nothing ever folds them out). SitOutForActor
	// folds them out of the live round itself (not just a bare state flip), so
	// the round can actually complete and, if this was the last decision
	// pending, advance the hand to Complete — broadcastAll's notifyHandComplete
	// call picks that up same as a normal Act would. They keep their seat and
	// chips until the kick timer, and reconnecting plus "sit in" brings them
	// straight back.
	if _, disconnected := a.disconnectedSince[c.PlayerID]; disconnected {
		timeBankMs := a.consumeTimeBank(c.PlayerID)
		a.cached.SitOutForActor(c.PlayerID)
		if err := a.commit(ctx, "", &tablestore.ActionLogEntry{
			PlayerID: c.PlayerID, Action: "disconnect_sit_out", TimeBankMs: timeBankMs,
		}); err != nil {
			// a.cached now holds an uncommitted SitOutForActor mutation
			// layered on stale state -- discard it by reloading fresh,
			// authoritative state instead of leaving this fabricated,
			// never-persisted table in memory for whatever this actor does
			// next (e.g. a later kick-timeout removal computing
			// handInProgress/dealtIntoCurrentHand off of it, which could
			// wrongly allow removing a player still dealt into the REAL
			// hand and leave a stale handOrder entry for a since-removed
			// player — runShowdown's playerByID lookup on that entry would
			// then panic). Unconditional: an ErrVersionConflict genuinely
			// means someone else already advanced and this reload is enough
			// to reconcile, but any OTHER commit error (a dropped extra
			// item, a throttle) left the exact same kind of uncommitted
			// mutation behind and must be purged the same way — the error
			// itself still propagates for anything but ErrVersionConflict.
			if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
				return reloadErr
			}
			if !errors.Is(err, tablestore.ErrVersionConflict) {
				return err
			}
		}
		a.broadcastAll()
		return nil
	}
	timeoutActionID := fmt.Sprintf("turn-timeout-%s-%d", c.PlayerID, a.version)
	timeoutAction := betting.ActionFold
	if a.cached.ProspectiveCallAmountForActor(c.PlayerID) == 0 {
		timeoutAction = betting.ActionCheck
	}
	_, err := a.applyActAndCommit(ctx, ActCmd{
		PlayerID: c.PlayerID, ActionID: timeoutActionID, Action: timeoutAction, Amount: 0, Reply: c.Reply,
	})
	if errors.Is(err, tablestore.ErrVersionConflict) {
		if err := a.ensureLoaded(ctx, true); err != nil {
			return err
		}
		if a.cached.CurrentPlayerIDForActor() != c.PlayerID {
			a.broadcastAll()
			return nil
		}
		timeoutActionID = fmt.Sprintf("turn-timeout-%s-%d", c.PlayerID, a.version)
		timeoutAction = betting.ActionFold
		if a.cached.ProspectiveCallAmountForActor(c.PlayerID) == 0 {
			timeoutAction = betting.ActionCheck
		}
		_, err = a.applyActAndCommit(ctx, ActCmd{
			PlayerID: c.PlayerID, ActionID: timeoutActionID, Action: timeoutAction, Amount: 0, Reply: c.Reply,
		})
	}
	if err != nil && !errors.Is(err, tablestore.ErrDuplicateAction) {
		return err
	}
	a.broadcastAll()
	return nil
}

func (a *Actor) handleJoin(ctx context.Context, c JoinCmd) error {
	// Force a fresh reload here (unlike every other handler's ensureLoaded(ctx,
	// false)): a join is the highest-risk moment for a new viewer to be folded
	// into a stale in-memory cache — e.g. a lease-holding actor whose local
	// state is behind because it never observed another instance's commit (see
	// ensureLoaded's doc comment). A fresh read costs one extra DynamoDB read
	// per join and guarantees a joining player never sees leftover
	// LastOutcome/board/deadlines from before they existed.
	if err := a.ensureLoaded(ctx, true); err != nil {
		return err
	}
	apply := func() error { return a.applyJoinAndCommit(ctx, c) }
	if err := a.retryOnConflict(ctx, apply); err != nil {
		return err
	}
	a.notifySeatsChanged()
	a.broadcastAll()
	return nil
}

func (a *Actor) applyJoinAndCommit(ctx context.Context, c JoinCmd) error {
	players := a.cached.PlayersForActor()
	alreadySeated := false
	for _, player := range players {
		if player.ID == c.PlayerID {
			alreadySeated = true
			break
		}
	}
	// A busted player still occupies their original seat. Capacity only
	// rejects a genuinely new player; an existing player must reach the hand
	// engine below, which restores a Stack<=0 seat and rejects a duplicate
	// join when chips are still present.
	if !alreadySeated && c.MaxSeats > 0 && len(players) >= c.MaxSeats {
		return ErrNoSeatsAvailable
	}
	// Snapshot before any in-place mutation, mirroring applyLeaveAndCommit —
	// AddMidHandJoiner/AddWaitingPlayer append straight into a.cached.players,
	// so a commit failure below (a transient store error, not just a version
	// conflict retryOnConflict already reloads on) must not leave a phantom
	// seated player — already carrying their debited buy-in stack — trusted
	// in this actor's cache with no matching poker_action_log entry. Without
	// this, the next unrelated successful commit persists the ghost seat for
	// real the first time any other player's action commits.
	before := a.cached.ExportState()
	beforeHandID := a.handID
	p := &hand.Player{ID: c.PlayerID, Stack: c.Stack, HoldID: c.HoldID, LastActionAt: timeNowFunc().UnixMilli(), AutoRebuy: c.AutoRebuy, BuyInAmount: c.Stack}
	stage := a.cached.Stage()
	if stage != hand.WaitingForPlayers && stage != hand.Complete {
		if err := a.cached.AddMidHandJoiner(p); err != nil {
			return err
		}
	} else if err := a.cached.AddWaitingPlayer(p); err != nil {
		return err
	}
	if stage == hand.WaitingForPlayers {
		a.tryStartHand(ctx)
	}
	var extra []types.TransactWriteItem
	if c.SettlementIntent != nil {
		intent, err := c.SettlementIntent()
		if err != nil {
			a.cached = hand.NewTableFromState(before)
			a.handID = beforeHandID
			return err
		}
		extra = append(extra, intent)
	}
	if err := a.commit(ctx, "", &tablestore.ActionLogEntry{PlayerID: c.PlayerID, Action: "join"}, extra...); err != nil {
		a.cached = hand.NewTableFromState(before)
		a.handID = beforeHandID
		return err
	}
	return nil
}

func (a *Actor) systemLeaveCmd(ctx context.Context, playerID, reason string, stack chan int64, holdID chan string) LeaveCmd {
	cmd := LeaveCmd{PlayerID: playerID, Stack: stack, HoldID: holdID}
	if a.systemSettlementIntent != nil {
		cmd.SettlementIntent = func(amount int64, hold string) (types.TransactWriteItem, error) {
			return a.systemSettlementIntent(ctx, playerID, reason, amount, hold)
		}
	}
	return cmd
}

// handleLeave removes the player and reports their final stack on c.Stack —
// but only after the removal has actually committed, so a caller (buyin's
// CashOut) never credits a wallet for a leave that a version conflict or
// store error ultimately rolled back. The stack is recomputed from the
// freshly-reloaded a.cached on the retry (see applyLeaveAndCommit), never
// carried over from the stale pre-conflict attempt.
func (a *Actor) handleLeave(ctx context.Context, c LeaveCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	stack, holdID, err := a.applyLeaveAndCommit(ctx, c)
	if errors.Is(err, tablestore.ErrVersionConflict) {
		if err := a.ensureLoaded(ctx, true); err != nil {
			return err
		}
		stack, holdID, err = a.applyLeaveAndCommit(ctx, c)
	}
	if err != nil {
		return err
	}
	delete(a.disconnectedSince, c.PlayerID)
	delete(a.activeConns, c.PlayerID)
	if t, armed := a.kickTimers[c.PlayerID]; armed {
		t.Stop()
		delete(a.kickTimers, c.PlayerID)
	}
	if c.Stack != nil {
		c.Stack <- stack
	}
	if c.HoldID != nil {
		c.HoldID <- holdID
	}
	a.notifySeatsChanged()
	a.broadcastAll()
	return nil
}

func (a *Actor) applyLeaveAndCommit(ctx context.Context, c LeaveCmd) (int64, string, error) {
	before := a.cached.ExportState()
	stack, holdID, err := a.cached.RemovePlayerForActor(c.PlayerID)
	if err != nil {
		return 0, "", err
	}
	var extra []types.TransactWriteItem
	if c.SettlementIntent != nil {
		intent, err := c.SettlementIntent(stack, holdID)
		if err != nil {
			a.cached = hand.NewTableFromState(before)
			return 0, "", fmt.Errorf("table: build settlement intent: %w", err)
		}
		extra = append(extra, intent)
	}
	if err := a.commit(ctx, "", &tablestore.ActionLogEntry{PlayerID: c.PlayerID, Action: "leave"}, extra...); err != nil {
		a.cached = hand.NewTableFromState(before)
		return 0, "", err
	}
	return stack, holdID, nil
}

// armTurnTimer (re-)arms the universal per-turn timer for current — the
// player who must act right now, connected or not (empty string when no
// decision is pending). Idempotent per (current, stage) pair: re-arming for
// the SAME current player on the SAME street is a no-op (does not restart
// their clock), matching "the timer counts down from when the turn actually
// began," not from every subsequent broadcast. stage is part of the key
// (not just current) because currentPlayerToAct always resolves to the
// earliest non-folded active player in table order at the start of a fresh
// betting round — the very same player ID can easily be "current" again on
// the next street (trivially so in heads-up), which must still count as a
// brand new turn. grace is added on top of the normal turnTimeout —
// broadcastAll passes RevealGrace for the first arm after a stage transition
// into Flop/Turn/River, and 0 otherwise.
func (a *Actor) armTurnTimer(current string, stage hand.Stage, grace time.Duration) {
	if current == a.turnDeadlineFor && stage == a.turnDeadlineForStage {
		// A reload on the same actor may carry the persisted deadline for the
		// already-armed turn. It has served its synchronization purpose; never
		// leave it pending where the next player's arm could inherit it.
		a.pendingPersistedDeadline = 0
		a.pendingDeadlineFor = ""
		a.pendingDeadlineForStage = hand.WaitingForPlayers
		return
	}
	if a.turnTimer != nil {
		a.turnTimer.Stop()
	}
	a.turnDeadlineFor = current
	a.turnDeadlineForStage = stage
	if current == "" {
		a.pendingPersistedDeadline = 0
		a.turnBaseDeadline = time.Time{}
		return
	}
	bank := a.timeBankFor(current)
	a.turnBaseDeadline = timeNowFunc().Add(a.turnTimeout + grace)
	deadline := a.turnBaseDeadline.Add(bank)
	// A freshly (re)loaded actor (ensureLoaded set this from
	// StoredTable.TurnDeadlineUnixMs) has never armed this exact turn before,
	// so the guard above never matches on its first call here — reuse the
	// persisted deadline verbatim (even if already past — the timer below
	// then fires ~immediately, correctly enforcing an overdue auto-fold)
	// instead of granting a brand new full window just because this
	// instance's own bookkeeping started from zero values.
	if a.pendingPersistedDeadline > 0 &&
		a.pendingDeadlineFor == current && a.pendingDeadlineForStage == stage {
		deadline = time.UnixMilli(a.pendingPersistedDeadline)
		a.turnBaseDeadline = deadline.Add(-bank)
	}
	a.pendingPersistedDeadline = 0
	a.pendingDeadlineFor = ""
	a.pendingDeadlineForStage = hand.WaitingForPlayers
	a.turnDeadline = deadline
	remaining := time.Until(deadline)
	if remaining < 0 {
		remaining = 0
	}
	// The timer only dispatches a command; all map/state mutations happen
	// inside Run (handleTurnTimeout), so there is no data race with the Run
	// goroutine.
	a.turnTimer = time.AfterFunc(remaining, func() {
		reply := make(chan error, 1)
		if err := a.Dispatch(turnTimeoutCmd{PlayerID: current, Reply: reply}); err != nil {
			slog.Warn("table turn timeout dispatch failed", "table_id", a.id, "player_id", current, "err", err)
		}
	})
}

// isRevealStreet reports whether stage is one of the three streets whose
// arrival deals new board cards and therefore plays a reveal animation —
// PreFlop's hole cards use a different (faster) animation and are excluded.
func isRevealStreet(stage hand.Stage) bool {
	return stage == hand.Flop || stage == hand.Turn || stage == hand.River
}

// armNextHandTimer (re-)arms the 12s post-hand countdown when the table is
// Complete. Idempotent per handID: re-arming for the SAME hand does not
// restart the countdown (matches armTurnTimer's convention). complete is
// passed in by broadcastAll (already knows the current stage) so this stays
// a plain bool check, no engine dependency beyond what's already cached.
func (a *Actor) armNextHandTimer(complete bool) {
	if !complete {
		if a.nextHandTimer != nil {
			a.nextHandTimer.Stop()
		}
		a.nextHandArmedFor = ""
		a.pendingNextHandDeadline = 0
		return
	}
	if a.handID == a.nextHandArmedFor {
		// Already counting down for this hand. The persisted value describes
		// the very countdown already running, so drop it — left set, the NEXT
		// hand's arm would consume this hand's (by then expired) deadline and
		// start immediately instead of giving players their 12 seconds.
		a.pendingNextHandDeadline = 0
		return
	}
	if a.nextHandTimer != nil {
		a.nextHandTimer.Stop()
	}
	a.nextHandArmedFor = a.handID
	// Resume the persisted expiry when this table was loaded mid-countdown,
	// so a second instance publishes the same next_hand_unix_ms rather than
	// granting a fresh full window from its own now (see
	// StoredTable.NextHandDeadlineUnixMs). Consumed once, exactly like
	// armTurnTimer treats pendingPersistedDeadline.
	delay := a.nextHandDelay
	if persisted := a.pendingNextHandDeadline; persisted > 0 {
		a.nextHandDeadline = time.UnixMilli(persisted)
		if delay = a.nextHandDeadline.Sub(timeNowFunc()); delay < 0 {
			delay = 0
		}
	} else {
		a.nextHandDeadline = timeNowFunc().Add(delay)
	}
	a.pendingNextHandDeadline = 0
	a.nextHandTimer = time.AfterFunc(delay, func() {
		reply := make(chan error, 1)
		if err := a.Dispatch(nextHandCmd{Reply: reply}); err != nil {
			slog.Warn("table next hand dispatch failed", "table_id", a.id, "err", err)
		}
	})
}

// handleNextHand attempts to start the next hand once the post-hand
// countdown expires. A stale timer (the table isn't Complete anymore) is a
// silent no-op. StartHand's "fewer than 2 ready
// players" — StartHand falls the table back to WaitingForPlayers in that
// case, so it doesn't stay stuck on Complete; a ReadyCmd(true) later starts
// the next hand normally.
func (a *Actor) handleNextHand(ctx context.Context, c nextHandCmd) error {
	// The timer that dispatched this command has already fired, so the
	// invariant nextHandArmedFor stands for — "a countdown is pending for this
	// hand" — is false the moment we get here. Clear it before anything that
	// can fail: an error exit below consumed the only timer, and leaving the
	// flag set made armNextHandTimer's idempotence check suppress every later
	// re-arm, so the table sat on Complete forever. Nothing on this instance
	// could recover it — a keepalive ping's ReconnectCmd, the AFK sweep and a
	// reconnect all reach armNextHandTimer and hit that same early return —
	// only another instance that had never armed for this hand. Under load the
	// trigger is ordinary: ensureLoaded or commit failing with a cancelled
	// DynamoDB context.
	a.nextHandArmedFor = ""
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	if a.cached.Stage() != hand.Complete {
		return nil
	}
	a.saveHandHistorySnapshot(ctx)
	a.removeIdlePlayersBetweenHands(ctx)
	// A concurrent actor may have advanced the table while an idle-player
	// removal retried/reloaded. Never start from a stage that is no longer the
	// completed hand this timer was responsible for.
	if a.cached.Stage() != hand.Complete {
		return nil
	}
	if err := a.cached.StartHand(); err == nil {
		a.handID = newHandID()
		a.prunePreselections()
	}
	if err := a.commit(ctx, "", &tablestore.ActionLogEntry{Action: "next_hand"}); err != nil {
		// a.cached now holds an uncommitted tryStartHand mutation (possibly a
		// whole fabricated next hand, dealt from a stale player roster) layered
		// on stale state -- discard it by reloading fresh, authoritative state
		// instead of leaving it in memory for this actor's next command to
		// trust (see handleTurnTimeout's identical fix for the full story).
		// Unconditional, same reasoning: any commit error leaves this exact
		// kind of fabricated state behind, not just ErrVersionConflict.
		if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
			return reloadErr
		}
		if !errors.Is(err, tablestore.ErrVersionConflict) {
			return err
		}
	}
	a.broadcastAll()
	return nil
}

// armWinnerCardsTimer (re-)arms the consent-window expiry for a pending
// paid-reveal request. Idempotent per requester (at most one request exists
// per hand) and driven off the request's persisted ExpiresAt, so a reload on
// any node resumes the same deadline rather than restarting it — without
// this, an actor handoff mid-request would strand the requester's fee.
func (a *Actor) armWinnerCardsTimer(req *hand.WinnerCardsRequest) {
	if req == nil {
		if a.winnerCardsTimer != nil {
			a.winnerCardsTimer.Stop()
		}
		a.winnerCardsArmedFor = ""
		return
	}
	key := a.handID + "#" + req.RequesterID
	if key == a.winnerCardsArmedFor {
		return
	}
	if a.winnerCardsTimer != nil {
		a.winnerCardsTimer.Stop()
	}
	a.winnerCardsArmedFor = key
	delay := time.UnixMilli(req.ExpiresAt).Sub(timeNowFunc())
	if delay < 0 {
		delay = 0
	}
	a.winnerCardsTimer = time.AfterFunc(delay, func() {
		reply := make(chan error, 1)
		if err := a.Dispatch(expireWinnerCardsCmd{Reply: reply}); err != nil {
			slog.Warn("table winner cards expiry dispatch failed", "table_id", a.id, "err", err)
		}
	})
}

// armRunoutTimer (re-)arms the paced all-in-runout timer while
// IsAwaitingRunoutForActor is true. Idempotent per (handID, phase, stage) —
// re-arming for the same point in the runout does not restart the delay,
// matching armTurnTimer/armNextHandTimer's convention. stage is passed in by
// broadcastAll (already knows the current stage) so this stays a plain
// comparison, no extra engine call.
func (a *Actor) armRunoutTimer(awaiting bool, stage hand.Stage) {
	if !awaiting {
		if a.runoutTimer != nil {
			a.runoutTimer.Stop()
		}
		a.runoutTimerHandID = ""
		return
	}
	phase := a.cached.RunoutPhaseForActor()
	if a.handID == a.runoutTimerHandID && stage == a.runoutTimerStage && phase == a.runoutTimerPhase {
		return
	}
	if a.runoutTimer != nil {
		a.runoutTimer.Stop()
	}
	a.runoutTimerHandID = a.handID
	a.runoutTimerStage = stage
	a.runoutTimerPhase = phase
	a.runoutTimer = time.AfterFunc(a.runoutStreetDelay, func() {
		reply := make(chan error, 1)
		if err := a.Dispatch(runoutStepCmd{Reply: reply}); err != nil {
			slog.Warn("table runout dispatch failed", "table_id", a.id, "err", err)
		}
	})
}

// handleRunoutStep fires runoutStreetDelay after the previous runout street
// was dealt and deals exactly the next one, pacing an all-in board reveal
// instead of showing the whole runout in a single broadcast. A stale fire
// (the awaited state no longer holds, e.g. this table already finished the
// runout through another path) is a silent no-op.
func (a *Actor) handleRunoutStep(ctx context.Context, c runoutStepCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	if !a.cached.IsAwaitingRunoutForActor() {
		return nil
	}
	a.cached.AdvanceRunoutStreetForActor()
	if err := a.commit(ctx, "", &tablestore.ActionLogEntry{Action: "runout_step"}); err != nil {
		// Same discard-and-reload fix as handleTurnTimeout/handleNextHand:
		// a.cached currently holds an uncommitted AdvanceRunoutStreetForActor
		// mutation layered on stale state. Unconditional for the same reason
		// — any commit error leaves it behind, not just ErrVersionConflict.
		if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
			return reloadErr
		}
		if !errors.Is(err, tablestore.ErrVersionConflict) {
			return err
		}
	}
	if err := a.commitOutcomeLogEntries(ctx); err != nil {
		return err
	}
	a.broadcastAll()
	return nil
}

func (a *Actor) processInlinePreselections(ctx context.Context) {
	a.prunePreselections()
	for len(a.activity.Preselections) > 0 &&
		a.cached != nil && a.cached.Stage() != hand.Complete {
		current := a.cached.CurrentPlayerIDForActor()
		if current == "" {
			return
		}
		preselection, ok := a.activity.Preselections[current]
		if !ok {
			return
		}
		callAmount := a.cached.ProspectiveCallAmountForActor(current)
		var action betting.Action
		var amount int64

		switch preselection.Selection {
		case "check_fold":
			if callAmount == 0 {
				action = betting.ActionCheck
			} else {
				action = betting.ActionFold
			}
		case "fold":
			action = betting.ActionFold
		case "call":
			if preselection.Amount == callAmount && callAmount > 0 {
				action = betting.ActionCall
				amount = callAmount
			} else if callAmount == 0 {
				action = betting.ActionCheck
			} else {
				delete(a.activity.Preselections, current)
				continue
			}
		case "call_any":
			if callAmount == 0 {
				action = betting.ActionCheck
			} else {
				action = betting.ActionCall
				amount = callAmount
			}
		case "all_in":
			if amt, ok := a.cached.AllInAmountForActor(current); ok {
				action = betting.ActionRaise
				amount = amt
			} else {
				delete(a.activity.Preselections, current)
				continue
			}
		default:
			delete(a.activity.Preselections, current)
			continue
		}

		delete(a.activity.Preselections, current)
		autoActionID := fmt.Sprintf("auto-preselect-%s-%s-%d", current, preselection.Selection, a.version)
		applied, err := a.applyActAndCommit(ctx, ActCmd{
			PlayerID: current,
			ActionID: autoActionID,
			Action:   action,
			Amount:   amount,
		})
		if err != nil || !applied {
			if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
				slog.Error("table reload after preselection failed", "table_id", a.id, "err", reloadErr)
			}
			return
		}
		if err := a.commitOutcomeLogEntries(ctx); err != nil {
			slog.Error("table preselection outcome log commit failed", "table_id", a.id, "err", err)
		}
	}
}

// processPendingExitAutoFolds folds out, one at a time, whoever is
// currently on the clock and has a pending exit request — the moment their
// turn actually arrives, not when RequestExit was called (an uncontested
// win owed to them before their turn comes back around must still pay
// out — see Table.RequestExit's doc comment). Mirrors
// processInlinePreselections's loop shape exactly (same applyActAndCommit +
// commitOutcomeLogEntries tail), and runs immediately before it from the
// same broadcastAll call site so a pending exit always takes priority over
// a stale preselection for the same turn.
func (a *Actor) processPendingExitAutoFolds(ctx context.Context) {
	for a.cached != nil && a.cached.Stage() != hand.Complete && a.cached.CurrentPlayerHasPendingExitForActor() {
		current := a.cached.CurrentPlayerIDForActor()
		autoActionID := fmt.Sprintf("auto-exit-fold-%s-%d", current, a.version)
		applied, err := a.applyActAndCommit(ctx, ActCmd{
			PlayerID: current, ActionID: autoActionID, Action: betting.ActionFold,
		})
		if err != nil || !applied {
			if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
				slog.Error("table reload after pending-exit auto-fold failed", "table_id", a.id, "err", reloadErr)
			}
			return
		}
		if err := a.commitOutcomeLogEntries(ctx); err != nil {
			slog.Error("table pending-exit auto-fold outcome log commit failed", "table_id", a.id, "err", err)
		}
	}
}

func (a *Actor) broadcastAll() {
	if a.broadcast == nil || a.cached == nil {
		return
	}
	a.processPendingExitAutoFolds(context.Background())
	a.processInlinePreselections(context.Background())
	a.removeEligiblePendingExits(context.Background())
	stage := a.cached.Stage()
	current := a.cached.CurrentPlayerIDForActor()
	grace := time.Duration(0)
	if stage != a.lastBroadcastStage && isRevealStreet(stage) {
		grace = RevealGrace
	}
	a.armTurnTimer(current, stage, grace)
	a.armRunoutTimer(a.cached.IsAwaitingRunoutForActor(), stage)
	a.armNextHandTimer(stage == hand.Complete)
	a.armWinnerCardsTimer(a.cached.PendingWinnerCards())
	a.lastBroadcastStage = stage
	doEquity := a.equityEnabled.Load() && equityStage(stage)
	for _, p := range a.cached.PlayersForActor() {
		snapshot := a.cached.ViewFor(p.ID)
		snapshot.SnapshotVersion = uint64(a.version)
		snapshot.HandID = a.handID
		if current != "" && current == a.turnDeadlineFor {
			snapshot.ActionDeadlineUnixMs = a.turnDeadline.UnixMilli()
			snapshot.ActionBaseDeadlineUnixMs = a.turnBaseDeadline.UnixMilli()
		}
		if stage == hand.Complete && a.handID == a.nextHandArmedFor {
			snapshot.NextHandUnixMs = a.nextHandDeadline.UnixMilli()
		}
		if p.LastActionAt > 0 {
			snapshot.IdleRemovalUnixMs = p.LastActionAt + a.kickGrace.Milliseconds()
		}
		a.applyPresence(snapshot.Seats)
		a.applyStreaks(snapshot.Seats)
		a.applyActivity(p.ID, &snapshot)
		if doEquity {
			if hole, board, ok := a.cached.HoleAndBoardForActor(p.ID); ok {
				opponents := 0
				for _, seat := range snapshot.Seats {
					if seat.PlayerID != p.ID && (seat.State == "active" || seat.State == "all_in") {
						opponents++
					}
				}
				if opponents > 0 {
					estimate, _, err := equity.EstimateWithStats(hole, board, nil, opponents, 200)
					if err == nil {
						for i := range snapshot.Seats {
							if snapshot.Seats[i].PlayerID == p.ID {
								snapshot.Seats[i].Equity = &estimate
								break
							}
						}
					}
				}
			}
		}
		a.broadcast(p.ID, snapshot)
	}
	a.notifyHandComplete()
}

func (a *Actor) applyActivity(viewerID string, snapshot *hand.Snapshot) {
	snapshot.ChatMessages = make([]hand.ChatMessageView, 0, len(a.activity.Chat))
	for _, message := range a.activity.Chat {
		snapshot.ChatMessages = append(snapshot.ChatMessages, hand.ChatMessageView{
			ID: message.ID, PlayerID: message.PlayerID, Message: message.Message, Timestamp: message.Timestamp,
		})
	}
	now := timeNowFunc().UnixMilli()
	for _, reaction := range a.activity.Reactions {
		if reaction.ExpiresAt <= now {
			continue
		}
		snapshot.Reactions = append(snapshot.Reactions, hand.ReactionView{
			ID: reaction.ID, PlayerID: reaction.PlayerID, ReactionID: reaction.ReactionID,
			TargetPlayerID: reaction.TargetPlayerID, Timestamp: reaction.Timestamp, ExpiresAt: reaction.ExpiresAt,
		})
	}
	if preselection, ok := a.activity.Preselections[viewerID]; ok &&
		preselection.HandID == a.handID && preselection.Stage == snapshot.Stage {
		snapshot.ActionPreselection = preselection.Selection
		snapshot.ActionPreselectionAmount = preselection.Amount
	}
}

// applyPresence keeps transport presence separate from poker state: a folded
// or all-in player can simultaneously be disconnected without losing either
// piece of information.
//
// The local disconnect mark is only this instance's view. Once the fleet-wide
// set is known (fleetConns, refreshed by syncFleetConns) it decides instead:
// a player whose socket lives on another instance is connected even though
// this one holds a stale mark, and a player this instance never saw is
// disconnected rather than defaulting to connected.
func (a *Actor) applyPresence(seats []hand.SeatView) {
	for i := range seats {
		playerID := seats[i].PlayerID
		_, locallyConnected := a.activeConns[playerID]
		_, locallyDisconnected := a.disconnectedSince[playerID]
		connected := locallyConnected || !locallyDisconnected
		if a.fleetConns != nil {
			connected = locallyConnected || a.fleetConns[playerID]
		}
		seats[i].ConnectionState = "disconnected"
		if connected {
			seats[i].ConnectionState = "connected"
		}
	}
}

// ConnStore shares which players hold a live table socket anywhere in the
// fleet. See internal/tableconn.
type ConnStore interface {
	Sync(ctx context.Context, tableID string, localPlayerIDs []string) (map[string]bool, error)
}

// SetConnStoreForActor wires the shared connection set. Set once, right after
// construction, by tablemanager.
func (a *Actor) SetConnStoreForActor(s ConnStore) { a.connStore = s }

// syncFleetConns republishes this instance's connected players and refreshes
// the fleet-wide set. force is set by the connect/disconnect handlers, whose
// whole point is to make the change visible immediately; the paced caller is
// ensureLoaded, so ANY traffic — down to a keepalive ping's ReconnectCmd —
// keeps this instance's entries from lapsing, while tableconn.SyncInterval
// keeps a busy table to one round trip per interval instead of one per
// action. Pacing it off broadcastAll alone was not enough: a table where
// everyone is connected and quiet broadcasts nothing, so the shared key
// expired and every other instance showed the whole seat row as
// disconnected. A failure keeps the previous answer rather than blanking
// every dot.
func (a *Actor) syncFleetConns(force bool) {
	if a.connStore == nil {
		return
	}
	now := timeNowFunc()
	if !force && !a.connSyncedAt.IsZero() && now.Sub(a.connSyncedAt) < tableconn.SyncInterval {
		return
	}
	a.connSyncedAt = now
	local := make([]string, 0, len(a.activeConns))
	for playerID := range a.activeConns {
		local = append(local, playerID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), connStoreTimeout)
	defer cancel()
	connected, err := a.connStore.Sync(ctx, a.id, local)
	if err != nil {
		slog.Warn("table conn sync failed", "table_id", a.id, "err", err)
		return
	}
	if connected != nil {
		a.fleetConns = connected
	}
}

// connStoreTimeout bounds the shared-cache round trip, so an unreachable
// Valkey degrades the dot instead of stalling the actor goroutine.
const connStoreTimeout = 2 * time.Second

// StreakStore is the cross-instance home of the hot/cold streak badge
// (internal/tablestreak, Valkey-backed). Load answers with the whole table's
// map; Merge folds one completed hand's values in and answers with the merged
// result, so no actor ever treats its own process as authoritative.
type StreakStore interface {
	Load(ctx context.Context, tableID string) (map[string]int, error)
	Merge(ctx context.Context, tableID string, streaks map[string]int) (map[string]int, error)
}

// streakStoreTimeout bounds every shared-cache round trip, so an unreachable
// Valkey degrades the badge instead of stalling this table's actor goroutine
// (which would stall the table itself).
const streakStoreTimeout = 2 * time.Second

// SetStreakStoreForActor wires the shared badge store. Set once, right after
// construction, by tablemanager.
func (a *Actor) SetStreakStoreForActor(s StreakStore) { a.streakStore = s }

// refreshStreaks re-reads the badge from the shared store so every instance
// rendering this table publishes the same number. Called from ensureLoaded —
// once per command, the same point at which everything else in the cache
// heals. A read failure keeps the last known values rather than blanking
// every seat.
func (a *Actor) refreshStreaks(ctx context.Context) {
	if a.streakStore == nil {
		return
	}
	loadCtx, cancel := context.WithTimeout(ctx, streakStoreTimeout)
	defer cancel()
	streaks, err := a.streakStore.Load(loadCtx, a.id)
	if err != nil {
		slog.Warn("table streak load failed", "table_id", a.id, "err", err)
		return
	}
	if streaks != nil {
		a.streaks = streaks
	}
}

// applyStreaks overlays the cached per-table win/loss streak onto every seat,
// same idiom as applyPresence above.
func (a *Actor) applyStreaks(seats []hand.SeatView) {
	for i := range seats {
		seats[i].CurrentStreak = int32(a.streaks[seats[i].PlayerID])
	}
}

// SetStreaksForActor publishes a just-completed hand's freshly persisted
// streak values to the shared store and adopts the merged result. Called
// synchronously from the same table-actor goroutine that just ran the
// post-hand hooks (tablemanager.Manager's onHandComplete wrapper) — never
// via Dispatch, which would deadlock against that same in-flight call.
func (a *Actor) SetStreaksForActor(streaks map[string]int) {
	maps.Copy(a.streaks, streaks)
	if a.streakStore == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), streakStoreTimeout)
	defer cancel()
	merged, err := a.streakStore.Merge(ctx, a.id, streaks)
	if err != nil {
		slog.Warn("table streak publish failed", "table_id", a.id, "err", err)
		return
	}
	if merged != nil {
		a.streaks = merged
	}
}

func equityStage(stage hand.Stage) bool {
	return stage == hand.PreFlop || stage == hand.Flop || stage == hand.Turn || stage == hand.River
}

func (a *Actor) SetEquityEnabledForActor(enabled bool) { a.equityEnabled.Store(enabled) }

func (a *Actor) SetRunItTwiceEnabledForActor(enabled bool) { a.runItTwiceEnabled.Store(enabled) }

// SetTurnTimeoutForActor sets the per-turn action deadline from the room's
// configured turn_timeout_seconds (0 handled by table.TurnTimeoutFor before
// this is called).
func (a *Actor) SetTurnTimeoutForActor(d time.Duration) {
	if d > 0 {
		a.turnTimeout = d
		// Room creation rejects values below five seconds. Sub-five-second
		// values are therefore test-only clocks; disable the real 30-second
		// reserve so timeout tests stay fast without weakening production.
		if d < 5*time.Second {
			a.timeBankEnabled = false
		}
	}
}

// SetKickGraceForActor overrides how long a disconnected player can occupy a
// seat before armKickTimer auto-removes them. Test-only knob — no room
// config exposes this today.
func (a *Actor) SetKickGraceForActor(d time.Duration) {
	if d > 0 {
		a.kickGrace = d
	}
}

func newHandID() string {
	return ulid.MustNew(ulid.Timestamp(timeNowFunc()), rand.Reader).String()
}

// TableForTest exposes the cached hand.Table for integration-test assertions.
func (a *Actor) TableForTest() *hand.Table { return a.cached }

// SetCachedForTest seeds the cached hand.Table when running without a store.
func (a *Actor) SetCachedForTest(t *hand.Table) { a.cached = t }
