// Package table drives one table's hand.Table from exactly one goroutine per
// instance — not because that instance owns write authority (it doesn't;
// ARCHITECTURE.md §2 makes DynamoDB's conditional writes the sole
// correctness mechanism), but because hand.Table has no internal lock, so
// two of this instance's own goroutines must still be serialized.
package table

import (
	"context"
	"errors"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

var timeNowFunc = time.Now

// Actor is the local serialization point for one table's hand.Table.
type Actor struct {
	id         string
	store      *tablestore.Store
	trustCache bool // set once at construction — see New's doc comment
	broadcast  func(viewerID string, snap hand.Snapshot)

	cmds chan Command

	cached  *hand.Table // nil until first loaded; never trusted when !trustCache
	version int
	handID  string

	actionDeadline               time.Duration
	disconnectGrace              time.Duration
	disconnectedSince            map[string]time.Time
	consecutiveDisconnectedHands map[string]int
	deadlineTimer                *time.Timer
}

// New returns an Actor for tableID. trustCache should be true only when the
// caller currently holds tableID's tablelease — it is read once here and
// never re-consulted; losing the lease later does not retroactively
// invalidate an in-flight Actor (ARCHITECTURE.md §2: the lease bounds
// latency, not correctness — a stale cache is always caught by
// CommitAction's version check regardless of trustCache).
func New(id string, store *tablestore.Store, trustCache bool, broadcast func(string, hand.Snapshot)) *Actor {
	return &Actor{
		id: id, store: store, trustCache: trustCache, broadcast: broadcast, cmds: make(chan Command, 64),
		actionDeadline:               30 * time.Second,
		disconnectGrace:              45 * time.Second,
		disconnectedSince:            make(map[string]time.Time),
		consecutiveDisconnectedHands: make(map[string]int),
	}
}

func (a *Actor) Dispatch(cmd Command) error {
	a.cmds <- cmd
	return <-cmd.reply()
}

func (a *Actor) Run(ctx context.Context) {
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
	case DisconnectCmd:
		return a.handleDisconnect(c)
	case ReconnectCmd:
		return a.handleReconnect(c)
	case SitOutCmd:
		return a.handleSitOut(ctx, c)
	default:
		return nil
	}
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
	return nil
}

func (a *Actor) handleReady(ctx context.Context, c ReadyCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	if err := a.applyReadyAndCommit(ctx, c); err != nil {
		if errors.Is(err, tablestore.ErrVersionConflict) {
			// ARCHITECTURE.md §2: "the handler retries against the freshly-read
			// state" — exactly once; a second conflict inside the same dispatch
			// would mean real, sustained contention, not ordinary human-paced play.
			if err := a.ensureLoaded(ctx, true); err != nil {
				return err
			}
			if err := a.applyReadyAndCommit(ctx, c); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	a.broadcastAll()
	return nil
}

func (a *Actor) applyReadyAndCommit(ctx context.Context, c ReadyCmd) error {
	for _, p := range a.cached.PlayersForActor() {
		if p.ID == c.PlayerID {
			p.Ready = c.Ready
		}
	}
	if a.cached.Stage() == hand.WaitingForPlayers || a.cached.Stage() == hand.Complete {
		if err := a.cached.StartHand(); err == nil {
			a.handID = newHandID()
		}
		// "need at least 2 ready players" is not a caller error — the table
		// just keeps waiting; swallow it here.
	}
	return a.commit(ctx, "", nil)
}

func (a *Actor) handleAct(ctx context.Context, c ActCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	err := a.applyActAndCommit(ctx, c)
	if errors.Is(err, tablestore.ErrVersionConflict) {
		// See handleReady's identical rationale — retry exactly once.
		if err := a.ensureLoaded(ctx, true); err != nil {
			return err
		}
		err = a.applyActAndCommit(ctx, c)
	}
	if err != nil && !errors.Is(err, tablestore.ErrDuplicateAction) {
		return err
	}
	a.armActionDeadlineForCurrentTurn()
	a.broadcastAll()
	return nil
}

// applyActAndCommit reports success (nil error) both when the action applied
// and committed, and when it was already applied elsewhere (not applied
// locally, or ErrDuplicateAction from the store) — handleAct treats both as
// "nothing left to do but broadcast current state" and calls broadcastAll
// exactly once itself, so this method never calls it.
func (a *Actor) applyActAndCommit(ctx context.Context, c ActCmd) error {
	applied, err := a.cached.ActIdempotent(c.ActionID, c.PlayerID, c.Action, c.Amount)
	if err != nil {
		return err
	}
	if !applied {
		return nil
	}
	entry := tablestore.ActionLogEntry{PlayerID: c.PlayerID, ActionID: c.ActionID, Action: string(c.Action), Amount: c.Amount}
	return a.commit(ctx, c.ActionID, &entry)
}

func (a *Actor) commit(ctx context.Context, actionID string, entry *tablestore.ActionLogEntry) error {
	newState := a.cached.ExportState()
	if entry == nil {
		entry = &tablestore.ActionLogEntry{}
	}
	entry.TableID, entry.HandID, entry.Version = a.id, a.handID, a.version+1
	if err := a.store.CommitAction(ctx, a.id, a.handID, actionID, a.version, newState, *entry); err != nil {
		return err
	}
	a.version++
	return nil
}

func (a *Actor) handleDisconnect(c DisconnectCmd) error {
	a.disconnectedSince[c.PlayerID] = timeNowFunc()
	a.armActionDeadlineIfTheirTurn(c.PlayerID)
	a.broadcastAll()
	return nil
}

func (a *Actor) handleReconnect(c ReconnectCmd) error {
	delete(a.disconnectedSince, c.PlayerID)
	if a.deadlineTimer != nil {
		a.deadlineTimer.Stop()
	}
	a.broadcastAll()
	return nil
}

func (a *Actor) handleSitOut(ctx context.Context, c SitOutCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	a.cached.SitOutForActor(c.PlayerID)
	if err := a.commit(ctx, "", nil); err != nil && !errors.Is(err, tablestore.ErrVersionConflict) {
		return err
	}
	a.broadcastAll()
	return nil
}

// armActionDeadlineIfTheirTurn starts (or restarts) the auto-fold timer when
// the just-disconnected player is the one currently on the clock. A
// disconnect that happens on someone else's turn arms nothing yet —
// armActionDeadlineForCurrentTurn (called from handleAct) picks it up once
// it actually becomes their turn.
func (a *Actor) armActionDeadlineIfTheirTurn(playerID string) {
	if a.cached == nil || !a.cached.CurrentPlayerCanActForActor(playerID) {
		return
	}
	if a.deadlineTimer != nil {
		a.deadlineTimer.Stop()
	}
	a.deadlineTimer = time.AfterFunc(a.actionDeadline, func() {
		reply := make(chan error, 1)
		_ = a.Dispatch(ActCmd{PlayerID: playerID, ActionID: "auto-fold-" + playerID, Action: "fold", Reply: reply})
		a.consecutiveDisconnectedHands[playerID]++
		if timeNowFunc().Sub(a.disconnectedSince[playerID]) >= a.disconnectGrace || a.consecutiveDisconnectedHands[playerID] >= 3 {
			reply2 := make(chan error, 1)
			_ = a.Dispatch(SitOutCmd{PlayerID: playerID, Reply: reply2})
		}
	})
}

// armActionDeadlineForCurrentTurn re-arms the auto-fold timer for whichever
// disconnected player's turn it now is, called after every committed action
// so a turn change picks up the right player's deadline.
func (a *Actor) armActionDeadlineForCurrentTurn() {
	if a.deadlineTimer != nil {
		a.deadlineTimer.Stop()
	}
	for id := range a.disconnectedSince {
		if a.cached.CurrentPlayerCanActForActor(id) {
			a.armActionDeadlineIfTheirTurn(id)
			return
		}
	}
}

func (a *Actor) broadcastAll() {
	if a.broadcast == nil || a.cached == nil {
		return
	}
	for _, p := range a.cached.PlayersForActor() {
		a.broadcast(p.ID, a.cached.ViewFor(p.ID))
	}
}

func newHandID() string {
	return timeNowFunc().Format("20060102T150405.000000000")
}

// TableForTest exposes the cached hand.Table for integration-test assertions.
func (a *Actor) TableForTest() *hand.Table { return a.cached }
