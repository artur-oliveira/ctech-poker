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
	"sort"
	"sync/atomic"
	"time"

	"github.com/oklog/ulid/v2"
	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/deck"
	"gopkg.aoctech.app/poker/api/internal/engine/equity"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/metrics"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

var timeNowFunc = time.Now

const (
	maxPersistedChatMessages = 40
	maxPersistedReactions    = 8
	reactionLifetime         = 2400 * time.Millisecond
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

	turnTimeout                  time.Duration
	timeBankEnabled              bool
	disconnectGrace              time.Duration
	disconnectedSince            map[string]time.Time
	consecutiveDisconnectedHands map[string]int
	// activeConns tracks physical connection IDs, not just a count. Connect
	// and Disconnect are therefore idempotent when a live WS is re-registered
	// after actor replacement, and one tab closing cannot disconnect another.
	activeConns map[string]map[string]struct{}
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
	nextHandTimer            *time.Timer
	nextHandDeadline         time.Time
	nextHandArmedFor         string
	nextHandDelay            time.Duration
	lastBroadcastStage       hand.Stage
	runoutTimer              *time.Timer
	runoutTimerHandID        string
	runoutTimerStage         hand.Stage
	runoutStreetDelay        time.Duration
	escalationInterval       time.Duration
	escalationCfg            roomstore.BlindEscalation
	afkSweepTimer            *time.Timer
	afkSweepInterval         time.Duration
	done                     chan struct{}
	equityEnabled            atomic.Bool
	onHandComplete           func(string, hand.HandOutcome, map[string]string)
	onHandUpdated            func(string, hand.HandOutcome, map[string]string)
	completedHandNotified    string
	outcomeLoggedForHand     string
	onSeatsChanged           func(int)
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
	onPlayerRemoved func(playerID, reason string, stack int64, holdID string)
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
		done:                         make(chan struct{}),
		turnTimeout:                  DefaultTurnTimeout,
		timeBankEnabled:              true,
		nextHandDelay:                NextHandDelay,
		runoutStreetDelay:            RunoutStreetDelay,
		disconnectGrace:              45 * time.Second,
		disconnectedSince:            make(map[string]time.Time),
		consecutiveDisconnectedHands: make(map[string]int),
		activeConns:                  make(map[string]map[string]struct{}),
		kickGrace:                    5 * time.Minute,
		kickTimers:                   make(map[string]*time.Timer),
		afkSweepInterval:             AFKSweepInterval,
	}
	a.equityEnabled.Store(true)
	a.armAFKSweepTimer()
	return a
}

// ErrActorStopped is returned by Dispatch when the actor has stopped serving
// (e.g. it lost its table lease and Run exited) and will never read the
// command. Callers re-resolve a live actor via the manager.
var ErrActorStopped = errors.New("table: actor stopped")

func (a *Actor) Dispatch(cmd Command) error {
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
	case KeepSeatCmd:
		return a.handleKeepSeat(ctx, c)
	case JoinCmd:
		return a.handleJoin(ctx, c)
	case LeaveCmd:
		return a.handleLeave(ctx, c)
	case PostBigBlindCmd:
		return a.handlePostBigBlind(ctx, c)
	case SnapshotCmd:
		return a.handleSnapshot(ctx, c)
	case SetNameCmd:
		return a.handleSetName(ctx, c)
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
	return a.commitActivity(ctx, func() error {
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
	return a.commitActivity(ctx, func() error {
		if !a.isSeated(c.PlayerID) {
			return fmt.Errorf("table: player %s is not seated", c.PlayerID)
		}
		if c.TargetPlayerID != "" && (c.TargetPlayerID == c.PlayerID || !a.isSeated(c.TargetPlayerID)) {
			return errors.New("table: invalid reaction target")
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
		})
	})
}

func (a *Actor) handlePreselect(ctx context.Context, c PreselectCmd) error {
	if c.ActionID == "" {
		return errors.New("table: action_id is required")
	}
	if c.Selection != "" && c.Selection != "check_fold" && c.Selection != "fold" &&
		c.Selection != "call" && c.Selection != "call_any" {
		return errors.New("table: invalid action preselection")
	}
	return a.commitActivity(ctx, func() error {
		if !a.isSeated(c.PlayerID) {
			return fmt.Errorf("table: player %s is not seated", c.PlayerID)
		}
		if c.ExpectedSnapshotVersion == 0 || c.ExpectedHandID == "" ||
			uint64(a.version) != c.ExpectedSnapshotVersion || a.handID != c.ExpectedHandID {
			return errors.New("table: stale action state")
		}
		if c.Selection == "call" && (c.Amount <= 0 || c.Amount != a.cached.ProspectiveCallAmountForActor(c.PlayerID)) {
			return errors.New("table: fixed call amount changed")
		}
		if c.Selection != "call" {
			c.Amount = 0
		}
		if a.activity.Preselections == nil {
			a.activity.Preselections = make(map[string]tablestore.Preselection)
		}
		if c.Selection == "" {
			delete(a.activity.Preselections, c.PlayerID)
		} else {
			a.activity.Preselections[c.PlayerID] = tablestore.Preselection{
				Selection: c.Selection, Amount: c.Amount, HandID: a.handID, Stage: a.cached.ViewFor("").Stage,
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

func (a *Actor) commitActivity(ctx context.Context, apply func() error) error {
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
	a.broadcastAll()
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
	a.applyActivity(c.PlayerID, &snapshot)
	c.Snapshot <- snapshot
	return nil
}

// handleSetName persists display identity with the seat so every actor in the
// fleet produces the same snapshot. A no-op name does not bump table version.
func (a *Actor) handleSetName(ctx context.Context, c SetNameCmd) error {
	if c.Name == "" {
		return nil
	}
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	apply := func() error {
		if !a.cached.SetPlayerNameForActor(c.PlayerID, c.Name) {
			return nil
		}
		return a.commit(ctx, "", &tablestore.ActionLogEntry{
			PlayerID: c.PlayerID, Action: "set_name",
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
	if a.cached != nil && a.trustCache && !force {
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
		return errors.New("table: no state seeded for this table yet")
	}
	a.cached = hand.NewTableFromState(stored.State)
	a.version = stored.Version
	a.handID = stored.HandID
	a.activity = stored.Activity
	a.pendingPersistedDeadline = stored.TurnDeadlineUnixMs
	a.rearmTimersFromCache()
	return nil
}

// rearmTimersFromCache re-derives and re-arms the turn/runout/next-hand
// timers from whatever is now cached. All three are idempotent per
// (handID, stage) — see armTurnTimer/armRunoutTimer/armNextHandTimer — so
// calling this on every fresh load is safe and cheap. This is what lets a
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
	return a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
		PlayerID: c.PlayerID, ActionID: c.ActionID, Action: action,
	})
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
		metrics.EmitTableMetric(a.env, "TableStateHistorySaveError", 1, map[string]string{"table_id": a.id})
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
	start := timeNowFunc()
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
			if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
				return reloadErr
			}
			a.markLastAction(c.PlayerID)
			_, err = a.applyActAndCommit(ctx, c)
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
	metrics.EmitTableMetric(a.env, "ActionLatencyMs", float64(timeNowFunc().Sub(start).Milliseconds()), map[string]string{"table_id": a.id})
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

func (a *Actor) notifyHandComplete() {
	if a.cached == nil || a.cached.Stage() != hand.Complete || a.handID == "" || a.completedHandNotified == a.handID {
		return
	}
	if outcome := a.cached.LastOutcomeForActor(); outcome != nil {
		a.completedHandNotified = a.handID
		metrics.EmitTableMetric(a.env, "HandsCompleted", 1, map[string]string{"table_id": a.id})
		if a.onHandComplete != nil {
			names := make(map[string]string)
			for _, p := range a.cached.PlayersForActor() {
				if p.Name != "" {
					names[p.ID] = p.Name
				}
			}
			a.onHandComplete(a.handID, *outcome, names)
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
			if preselection.Selection == "call" &&
				preselection.Amount != a.cached.ProspectiveCallAmountForActor(playerID) {
				delete(a.activity.Preselections, playerID)
			}
		}
	}
	a.consumeTimeBank(c.PlayerID)
	action := string(c.Action)
	if a.cached.PlayerAllInForActor(c.PlayerID) {
		action = "all_in"
	}
	entry := tablestore.ActionLogEntry{
		PlayerID: c.PlayerID, ActionID: c.ActionID, Action: action,
		BettingAction: string(bettingAction), Amount: c.Amount,
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

func (a *Actor) commit(ctx context.Context, actionID string, entry *tablestore.ActionLogEntry) error {
	if a.store == nil {
		// Mirrors ensureLoaded's nil-store no-op: unit tests construct an
		// Actor with a nil store to exercise engine-level handler logic
		// without a real (DynamoDB Local) backing store. Never nil in
		// production — the manager always supplies a real *tablestore.Store.
		a.version++
		return nil
	}
	newState := a.cached.ExportState()
	entry.TableID, entry.HandID, entry.Version = a.id, a.handID, a.version+1
	entry.Frame = replayFrameFor(a.cached.ViewFor(""))
	deadline := a.turnDeadlineForPersist()
	if err := a.store.CommitAction(ctx, a.id, a.handID, actionID, a.version, newState, a.activity, deadline, *entry); err != nil {
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
func (a *Actor) consumeTimeBank(playerID string) {
	if !a.timeBankEnabled || a.turnTimeout < 5*time.Second || playerID == "" || playerID != a.turnDeadlineFor || a.turnBaseDeadline.IsZero() {
		return
	}
	elapsed := timeNowFunc().Sub(a.turnBaseDeadline).Milliseconds()
	if elapsed > 0 {
		a.cached.ConsumeTimeBankForActor(playerID, elapsed)
	}
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
	metrics.EmitTableMetric(a.env, "Disconnects", 1, map[string]string{"table_id": a.id})
	a.disconnectedSince[c.PlayerID] = timeNowFunc()
	a.armKickTimer(c.PlayerID)
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
	delete(a.consecutiveDisconnectedHands, playerID)
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
		_ = a.Dispatch(kickTimeoutCmd{PlayerID: playerID, Reply: reply})
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
	delete(a.kickTimers, c.PlayerID)
	// ponytail: RemovePlayerForActor rejects removal while the player is
	// still dealt into a hand in progress (any state, including Folded — see
	// RemovePlayerForActor's doc comment). In practice that can't coincide
	// with 5 minutes disconnected — handleTurnTimeout's 45s/3-hand disconnect
	// grace already forces them to SittingOut long before this fires. If it
	// ever races anyway, skip silently; nothing to retry from here.
	stackCh := make(chan int64, 1)
	holdIDCh := make(chan string, 1)
	if err := a.handleLeave(ctx, LeaveCmd{PlayerID: c.PlayerID, Stack: stackCh, HoldID: holdIDCh}); err != nil {
		return err
	}
	if a.onPlayerRemoved != nil {
		a.onPlayerRemoved(c.PlayerID, "disconnected", <-stackCh, <-holdIDCh)
	}
	return nil
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
	for _, p := range a.cached.PlayersForActor() {
		if p.ID == playerID {
			return true
		}
	}
	return false
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
		_ = a.Dispatch(afkSweepCmd{Reply: reply})
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
		if err := a.handleLeave(ctx, LeaveCmd{PlayerID: id, Stack: stackCh, HoldID: holdIDCh}); err != nil {
			continue // still dealt into a hand in progress; next sweep retries
		}
		if a.onPlayerRemoved != nil {
			a.onPlayerRemoved(id, "idle", <-stackCh, <-holdIDCh)
		}
	}
	return nil
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
			a.onHandUpdated(a.handID, *outcome, names)
		}
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
	if since, disconnected := a.disconnectedSince[c.PlayerID]; disconnected {
		a.consecutiveDisconnectedHands[c.PlayerID]++ // safe: runs in Run goroutine
		if timeNowFunc().Sub(since) >= a.disconnectGrace || a.consecutiveDisconnectedHands[c.PlayerID] >= 3 {
			// SitOutForActor folds the player out of the live round itself
			// (not just a bare state flip), so the round can actually
			// complete and, if this was the last decision pending, advance
			// the hand to Complete — broadcastAll's notifyHandComplete call
			// picks that up same as a normal Act would.
			a.consumeTimeBank(c.PlayerID)
			a.cached.SitOutForActor(c.PlayerID)
			if err := a.commit(ctx, "", &tablestore.ActionLogEntry{PlayerID: c.PlayerID, Action: "disconnect_sit_out"}); err != nil {
				if !errors.Is(err, tablestore.ErrVersionConflict) {
					return err
				}
				// a.cached now holds an uncommitted SitOutForActor mutation
				// layered on stale state -- discard it by reloading fresh,
				// authoritative state instead of leaving this fabricated,
				// never-persisted table in memory for whatever this actor
				// does next (e.g. a later kick-timeout removal computing
				// handInProgress/dealtIntoCurrentHand off of it, which could
				// wrongly allow removing a player still dealt into the REAL
				// hand and leave a stale handOrder entry for a since-removed
				// player — runShowdown's playerByID lookup on that entry
				// would then panic).
				if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
					return reloadErr
				}
			}
			a.broadcastAll()
			return nil
		}
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
	if c.MaxSeats > 0 && len(a.cached.PlayersForActor()) >= c.MaxSeats {
		return errors.New("table: no seats available")
	}
	p := &hand.Player{ID: c.PlayerID, Stack: c.Stack, HoldID: c.HoldID, LastActionAt: timeNowFunc().UnixMilli()}
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
	return a.commit(ctx, "", &tablestore.ActionLogEntry{PlayerID: c.PlayerID, Action: "join"})
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
	delete(a.consecutiveDisconnectedHands, c.PlayerID)
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
	stack, holdID, err := a.cached.RemovePlayerForActor(c.PlayerID)
	if err != nil {
		return 0, "", err
	}
	if err := a.commit(ctx, "", &tablestore.ActionLogEntry{PlayerID: c.PlayerID, Action: "leave"}); err != nil {
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
	if a.pendingPersistedDeadline > 0 {
		deadline = time.UnixMilli(a.pendingPersistedDeadline)
		a.turnBaseDeadline = deadline.Add(-bank)
	}
	a.pendingPersistedDeadline = 0
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
		_ = a.Dispatch(turnTimeoutCmd{PlayerID: current, Reply: reply})
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
		return
	}
	if a.handID == a.nextHandArmedFor {
		return
	}
	if a.nextHandTimer != nil {
		a.nextHandTimer.Stop()
	}
	a.nextHandArmedFor = a.handID
	a.nextHandDeadline = timeNowFunc().Add(a.nextHandDelay)
	a.nextHandTimer = time.AfterFunc(a.nextHandDelay, func() {
		reply := make(chan error, 1)
		_ = a.Dispatch(nextHandCmd{Reply: reply})
	})
}

// handleNextHand attempts to start the next hand once the post-hand
// countdown expires. A stale timer (the table isn't Complete anymore) is a
// silent no-op. StartHand's "fewer than 2 ready
// players" — StartHand falls the table back to WaitingForPlayers in that
// case, so it doesn't stay stuck on Complete; a ReadyCmd(true) later starts
// the next hand normally.
func (a *Actor) handleNextHand(ctx context.Context, c nextHandCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	if a.cached.Stage() != hand.Complete {
		return nil
	}
	a.saveHandHistorySnapshot(ctx)
	if err := a.cached.StartHand(); err == nil {
		a.handID = newHandID()
	}
	if err := a.commit(ctx, "", &tablestore.ActionLogEntry{Action: "next_hand"}); err != nil {
		if !errors.Is(err, tablestore.ErrVersionConflict) {
			return err
		}
		// a.cached now holds an uncommitted tryStartHand mutation (possibly a
		// whole fabricated next hand, dealt from a stale player roster) layered
		// on stale state -- discard it by reloading fresh, authoritative state
		// instead of leaving it in memory for this actor's next command to
		// trust (see handleTurnTimeout's identical fix for the full story).
		if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
			return reloadErr
		}
	}
	a.broadcastAll()
	return nil
}

// armRunoutTimer (re-)arms the paced all-in-runout timer while
// IsAwaitingRunoutForActor is true. Idempotent per (handID, stage) pair —
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
	if a.handID == a.runoutTimerHandID && stage == a.runoutTimerStage {
		return
	}
	if a.runoutTimer != nil {
		a.runoutTimer.Stop()
	}
	a.runoutTimerHandID = a.handID
	a.runoutTimerStage = stage
	a.runoutTimer = time.AfterFunc(a.runoutStreetDelay, func() {
		reply := make(chan error, 1)
		_ = a.Dispatch(runoutStepCmd{Reply: reply})
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
		if !errors.Is(err, tablestore.ErrVersionConflict) {
			return err
		}
		// Same discard-and-reload fix as handleTurnTimeout/handleNextHand:
		// a.cached currently holds an uncommitted AdvanceRunoutStreetForActor
		// mutation layered on stale state.
		if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
			return reloadErr
		}
	}
	if err := a.commitOutcomeLogEntries(ctx); err != nil {
		return err
	}
	a.broadcastAll()
	return nil
}

func (a *Actor) processInlinePreselections(ctx context.Context) {
	for a.activity.Preselections != nil && len(a.activity.Preselections) > 0 &&
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
			_ = a.ensureLoaded(ctx, true)
			return
		}
		_ = a.commitOutcomeLogEntries(ctx)
	}
}

func (a *Actor) broadcastAll() {
	if a.broadcast == nil || a.cached == nil {
		return
	}
	a.processInlinePreselections(context.Background())
	stage := a.cached.Stage()
	current := a.cached.CurrentPlayerIDForActor()
	grace := time.Duration(0)
	if stage != a.lastBroadcastStage && isRevealStreet(stage) {
		grace = RevealGrace
	}
	a.armTurnTimer(current, stage, grace)
	a.armRunoutTimer(a.cached.IsAwaitingRunoutForActor(), stage)
	a.armNextHandTimer(stage == hand.Complete)
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
					// Offload equity from the Run goroutine: compute in a
					// goroutine over captured values and push a follow-up
					// state update when ready.
					go a.computeAndSendEquity(p.ID, snapshot.SnapshotVersion, hole, board, opponents)
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
func (a *Actor) applyPresence(seats []hand.SeatView) {
	for i := range seats {
		seats[i].ConnectionState = "connected"
		if _, disconnected := a.disconnectedSince[seats[i].PlayerID]; disconnected {
			seats[i].ConnectionState = "disconnected"
		}
	}
}

func equityStage(stage hand.Stage) bool {
	return stage == hand.PreFlop || stage == hand.Flop || stage == hand.Turn || stage == hand.River
}

// computeAndSendEquity runs off the Run goroutine. It never touches a.cached;
// it works on a copy of the captured snapshot so there is no race with Run or
// with the synchronous broadcast that already sent the same Snapshot. When
// ready it pushes a follow-up state update carrying the equity.
func (a *Actor) computeAndSendEquity(
	viewerID string,
	snapshotVersion uint64,
	hole [2]deck.Card,
	board []deck.Card,
	opponents int,
) {
	estimate, err := equity.Estimate(hole, board, nil, opponents, 200)
	if err != nil {
		return
	}
	a.broadcast(viewerID, hand.Snapshot{
		SnapshotVersion: snapshotVersion,
		EquityOnly:      true,
		EquityPlayerID:  viewerID,
		EquityValue:     &estimate,
	})
}

func (a *Actor) SetEquityEnabledForActor(enabled bool) { a.equityEnabled.Store(enabled) }

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

// SetDisconnectGraceForActor overrides how long a disconnected player is
// given before their turn-timeout auto-fold escalates to auto-sit-out
// (handleTurnTimeout). Test-only knob — no room config exposes this today.
func (a *Actor) SetDisconnectGraceForActor(d time.Duration) {
	if d > 0 {
		a.disconnectGrace = d
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
