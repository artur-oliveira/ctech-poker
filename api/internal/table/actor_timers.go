package table

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

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
		a.nextHandArmGuardHand = ""
		a.nextHandArmsForHand = 0
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
	// Wedge guard (see the nextHandArm* field docs). handleNextHand clears
	// nextHandArmedFor on entry so the check above stops throttling the moment
	// the timer first fires; from then on every rearmTimersFromCache (a
	// reconnect, a keepalive ping, the AFK sweep) re-arms a fresh timer, and an
	// already-past persisted deadline makes it fire instantly. Cap the arms per
	// hand — past MaxNextHandArmsPerHand stop re-arming entirely; a table this
	// stuck recovers via tablecleanup or an operator, not by retrying.
	if a.nextHandArmGuardHand != a.handID {
		a.nextHandArmGuardHand = a.handID
		a.nextHandArmsForHand = 0
	}
	if a.nextHandArmsForHand >= MaxNextHandArmsPerHand {
		if a.nextHandArmsForHand == MaxNextHandArmsPerHand {
			a.nextHandArmsForHand++ // log exactly once per stuck hand
			slog.Error("table next hand permanently stalled; leaving timer un-armed",
				"table_id", a.id, "hand_id", a.handID, "arms", MaxNextHandArmsPerHand)
		}
		a.nextHandArmedFor = a.handID
		a.pendingNextHandDeadline = 0
		return
	}
	a.nextHandArmsForHand++
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
		return a.retryNextHand(err)
	}
	if a.cached.Stage() != hand.Complete {
		a.nextHandRetries = 0
		return nil
	}
	a.saveHandHistorySnapshot(ctx)
	a.removeIdlePlayersBetweenHands(ctx)
	// A concurrent actor may have advanced the table while an idle-player
	// removal retried/reloaded. Never start from a stage that is no longer the
	// completed hand this timer was responsible for.
	if a.cached.Stage() != hand.Complete {
		a.nextHandRetries = 0
		return nil
	}
	// a.mutate is what guarantees a commit failure here can't leave an
	// uncommitted StartHand mutation (possibly a whole fabricated next hand,
	// dealt from a stale player roster) trusted in a.cached for this actor's
	// next command (see handleTurnTimeout's identical guard for the full
	// story).
	err := a.mutate(func() error {
		if err := a.cached.StartHand(); err == nil {
			a.handID = newHandID()
			a.prunePreselections()
		}
		return a.commit(ctx, "", &tablestore.ActionLogEntry{Action: "next_hand"})
	})
	if err != nil {
		// An ErrVersionConflict genuinely means someone else already advanced
		// this table, so reload to reconcile before broadcasting; mutate has
		// already restored a.cached for any other error, so it just propagates.
		if errors.Is(err, tablestore.ErrVersionConflict) {
			if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
				return a.retryNextHand(reloadErr)
			}
		} else {
			return a.retryNextHand(err)
		}
	}
	a.nextHandRetries = 0
	a.broadcastAll()
	return nil
}

// retryNextHand re-arms the post-hand countdown after handleNextHand failed
// for an ordinary (non-panic) reason, and returns err unchanged so the caller
// still reports the failure. Clearing nextHandArmedFor alone only unblocks a
// LATER re-arm — on a quiet table between hands there is no later command to
// carry one, which is exactly how a hand stalled silently (#136). Bounded by
// MaxNextHandRetries so a permanently broken store degrades to the AFK
// sweep's watchdog instead of spinning a timer forever.
func (a *Actor) retryNextHand(err error) error {
	if a.nextHandRetries >= MaxNextHandRetries {
		slog.Error("table next hand retries exhausted", "table_id", a.id, "hand_id", a.handID, "err", err)
		a.nextHandRetries = 0
		return err
	}
	a.nextHandRetries++
	if a.nextHandTimer != nil {
		a.nextHandTimer.Stop()
	}
	slog.Warn("table next hand failed; re-arming",
		"table_id", a.id, "hand_id", a.handID, "attempt", a.nextHandRetries, "err", err)
	a.nextHandTimer = time.AfterFunc(time.Duration(a.nextHandRetries)*a.nextHandRetryDelay, func() {
		reply := make(chan error, 1)
		if dispatchErr := a.Dispatch(nextHandCmd{Reply: reply}); dispatchErr != nil {
			slog.Warn("table next hand retry dispatch failed", "table_id", a.id, "err", dispatchErr)
		}
	})
	return err
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
	// Same guard as handleTurnTimeout/handleNextHand: a.mutate restores
	// a.cached to its pre-call snapshot if AdvanceRunoutStreetForActor's
	// mutation never actually commits.
	err := a.mutate(func() error {
		a.cached.AdvanceRunoutStreetForActor()
		return a.commit(ctx, "", &tablestore.ActionLogEntry{Action: "runout_step"})
	})
	if err != nil {
		if errors.Is(err, tablestore.ErrVersionConflict) {
			if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
				return reloadErr
			}
		} else {
			return err
		}
	}
	if err := a.commitOutcomeLogEntries(ctx); err != nil {
		return err
	}
	a.broadcastAll()
	return nil
}
