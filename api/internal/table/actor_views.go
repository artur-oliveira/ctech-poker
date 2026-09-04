package table

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"maps"
	"time"

	"github.com/oklog/ulid/v2"
	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/deck"
	"gopkg.aoctech.app/poker/api/internal/engine/equity"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tableconn"
)

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
	// Chat and reactions are identical for every viewer, so build them once
	// per broadcast instead of once per seat (#37). Both slices are only ever
	// read downstream (ConvertSnapshot marshals them), never mutated.
	chat, reactions := a.activityViews()
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
		a.applyActivity(p.ID, &snapshot, chat, reactions)
		if doEquity {
			if hole, board, ok := a.cached.HoleAndBoardForActor(p.ID); ok {
				opponents := 0
				for _, seat := range snapshot.Seats {
					if seat.PlayerID != p.ID && (seat.State == "active" || seat.State == "all_in") {
						opponents++
					}
				}
				if opponents > 0 {
					if estimate, ok := a.equityFor(hole, board, opponents); ok {
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

// activityViews converts the table-wide activity (chat + unexpired
// reactions) into their snapshot form. Viewer-independent by construction,
// so broadcastAll builds them once and hands the same slices to every seat.
func (a *Actor) activityViews() ([]hand.ChatMessageView, []hand.ReactionView) {
	chat := make([]hand.ChatMessageView, 0, len(a.activity.Chat))
	for _, message := range a.activity.Chat {
		chat = append(chat, hand.ChatMessageView{
			ID: message.ID, PlayerID: message.PlayerID, Message: message.Message, Timestamp: message.Timestamp,
		})
	}
	var reactions []hand.ReactionView
	now := timeNowFunc().UnixMilli()
	for _, reaction := range a.activity.Reactions {
		if reaction.ExpiresAt <= now {
			continue
		}
		reactions = append(reactions, hand.ReactionView{
			ID: reaction.ID, PlayerID: reaction.PlayerID, ReactionID: reaction.ReactionID,
			TargetPlayerID: reaction.TargetPlayerID, Timestamp: reaction.Timestamp, ExpiresAt: reaction.ExpiresAt,
		})
	}
	return chat, reactions
}

// applyActivity attaches the shared activity views built by activityViews
// plus the one genuinely per-viewer piece: that viewer's own preselection.
func (a *Actor) applyActivity(viewerID string, snapshot *hand.Snapshot, chat []hand.ChatMessageView, reactions []hand.ReactionView) {
	snapshot.ChatMessages = chat
	snapshot.Reactions = reactions
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

// equityIterations is the Monte-Carlo sample count behind the seat's win-%
// hint. It runs on the actor goroutine, so it is deliberately small: enough
// for a stable one-decimal display, cheap enough not to stall the table.
const equityIterations = 200

// equityFor returns the cached win-probability estimate for one seat,
// computing it only on a miss. The estimate depends solely on (hole, board,
// opponent count), so the key below is exact — a fold changes the opponent
// count and a new street changes the board, both producing a fresh entry.
// Without this, every broadcast during a street (each chat message, act or
// reconnect signal) re-ran the simulation for every active seat (#37).
func (a *Actor) equityFor(hole [2]deck.Card, board []deck.Card, opponents int) (float64, bool) {
	if a.equityCacheHand != a.handID || a.equityCache == nil {
		a.equityCache = make(map[string]float64)
		a.equityCacheHand = a.handID
	}
	key := fmt.Sprintf("%v|%v|%d", hole, board, opponents)
	if estimate, hit := a.equityCache[key]; hit {
		return estimate, true
	}
	estimate, _, err := equity.EstimateForTableWithStats(a.id, hole, board, nil, opponents, equityIterations)
	if err != nil {
		return 0, false
	}
	a.equityCache[key] = estimate
	return estimate, true
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
