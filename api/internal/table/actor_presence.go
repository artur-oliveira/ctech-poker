package table

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

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
	nonce := newSettlementNonce()
	if err := a.handleLeave(ctx, a.systemLeaveCmd(ctx, c.PlayerID, "disconnected", nonce, stackCh, holdIDCh)); err != nil {
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
		a.onPlayerRemoved(c.PlayerID, "disconnected", nonce, <-stackCh, <-holdIDCh)
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
		nonce := newSettlementNonce()
		if err := a.handleLeave(ctx, a.systemLeaveCmd(ctx, id, "idle", nonce, stackCh, holdIDCh)); err != nil {
			continue // still dealt into a hand in progress; next sweep retries
		}
		if a.onPlayerRemoved != nil {
			a.onPlayerRemoved(id, "idle", nonce, <-stackCh, <-holdIDCh)
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
		nonce := newSettlementNonce()
		if err := a.handleLeave(ctx, a.systemLeaveCmd(ctx, id, "idle", nonce, stackCh, holdIDCh)); err != nil {
			continue
		}
		if a.onPlayerRemoved != nil {
			a.onPlayerRemoved(id, "idle", nonce, <-stackCh, <-holdIDCh)
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
		nonce := newSettlementNonce()
		if err := a.handleLeave(ctx, a.systemLeaveCmd(ctx, id, "exit_requested", nonce, stackCh, holdIDCh)); err != nil {
			continue
		}
		if a.onPlayerRemoved != nil {
			a.onPlayerRemoved(id, "exit_requested", nonce, <-stackCh, <-holdIDCh)
		}
	}
}
