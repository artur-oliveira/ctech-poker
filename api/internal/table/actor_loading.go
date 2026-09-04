package table

import (
	"context"
	"fmt"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

// SnapshotReloadInterval bounds how stale a cache-served snapshot may be. A
// snapshot is a read: it never advances the table, and every real action
// carries a version/hand precondition that forces its own reload when this
// answer turns out to be behind. Sibling commits also reach this instance
// through ChangeNotifier, which forces a reload of its own — so the window
// below is a backstop, not the primary freshness mechanism.
const SnapshotReloadInterval = 2 * time.Second

func (a *Actor) handleSnapshot(ctx context.Context, c SnapshotCmd) error {
	// A snapshot used to always read the authoritative item, on the grounds
	// that another fleet actor may commit without owning this actor's
	// cache-affinity lease. That made every sync_state frame a DynamoDB
	// GetItem — at up to the connection's 10/s limit, so one reconnect loop
	// or misbehaving client turned into a read storm (#218). Serve the cache
	// while it is fresh instead; anything that genuinely needs the
	// authoritative item (a stale action precondition, a version conflict, a
	// handoff onto a cold actor) still forces its own reload.
	if c.AllowCached && a.cached != nil && timeNowFunc().Sub(a.lastLoadedAt) < SnapshotReloadInterval {
		// Still the paced caller for the fleet connection set — see
		// syncFleetConns — which a cache-served snapshot would otherwise skip
		// on a table whose only traffic is sync_state.
		a.syncFleetConns(false)
	} else if err := a.ensureLoaded(ctx, true); err != nil {
		return err
	}
	snapshot := a.cached.ViewFor(c.PlayerID)
	snapshot.SnapshotVersion = uint64(a.version)
	snapshot.HandID = a.handID
	current := a.cached.CurrentPlayerIDForActor()
	snapshot.ActionDeadlineUnixMs, snapshot.ActionBaseDeadlineUnixMs, snapshot.NextHandUnixMs =
		a.deadlinesForBroadcast(current, a.cached.Stage())
	for _, p := range a.cached.PlayersForActor() {
		if p.ID == c.PlayerID && p.LastActionAt > 0 {
			snapshot.IdleRemovalUnixMs = p.LastActionAt + a.kickGrace.Milliseconds()
			break
		}
	}
	a.applyPresence(snapshot.Seats)
	a.applyStreaks(snapshot.Seats)
	chat, reactions := a.activityViews()
	a.applyActivity(c.PlayerID, &snapshot, chat, reactions)
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
		return a.mutate(func() error {
			if !a.cached.SetPlayerIdentityForActor(c.PlayerID, c.Name, c.AvatarURL, c.PlaystyleBadge) {
				return nil
			}
			return a.commit(ctx, "", &tablestore.ActionLogEntry{
				PlayerID: c.PlayerID, Action: "set_identity",
			})
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
		return a.mutate(func() error {
			a.cached.EscalateBlindsForActor(a.escalationCfg.Multiplier, a.escalationCfg.Max)
			return a.commit(ctx, "", &tablestore.ActionLogEntry{Action: "escalate_blinds"})
		})
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
	a.lastLoadedAt = timeNowFunc()
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
