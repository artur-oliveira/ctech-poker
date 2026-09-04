package table

import (
	"context"
	"errors"
	"fmt"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

func (a *Actor) handleSitOut(ctx context.Context, c SitOutCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	apply := func() error {
		return a.mutate(func() error {
			if !a.isSeated(c.PlayerID) {
				return fmt.Errorf("table: player %s is not seated", c.PlayerID)
			}
			a.markLastAction(c.PlayerID)
			a.cached.SitOutForActor(c.PlayerID)
			return a.commit(ctx, "", &tablestore.ActionLogEntry{PlayerID: c.PlayerID, Action: "sit_out"})
		})
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
		return a.mutate(func() error {
			if !a.isSeated(c.PlayerID) {
				return fmt.Errorf("table: player %s is not seated", c.PlayerID)
			}
			a.markLastAction(c.PlayerID)
			return a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
				PlayerID: c.PlayerID, ActionID: c.ActionID, Action: "keep_seat",
			})
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
		return a.mutate(func() error {
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
		return a.mutate(func() error {
			if _, err := a.cached.RequestRabbitHunt(c.PlayerID); err != nil {
				return err
			}
			changed = true
			return a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
				PlayerID: c.PlayerID, ActionID: c.ActionID, Action: "request_rabbit_hunt",
			})
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
		return a.mutate(func() error {
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
		return a.mutate(func() error {
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
		// RequestWinnerCards can expire (and refund) a stale pending request
		// before it validates and rejects the rest of this call — mutate covers
		// that partial mutation the same as a commit failure, restoring the
		// pre-call snapshot on any error so a rejected request never leaves that
		// refund sitting uncommitted in memory for the next command to trust.
		return a.mutate(func() error {
			if _, err := a.cached.RequestWinnerCards(c.PlayerID, timeNowFunc()); err != nil {
				return err
			}
			changed = true
			return a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
				PlayerID: c.PlayerID, ActionID: c.ActionID, Action: "request_winner_cards",
			})
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

func (a *Actor) applyWinnerCardsAnswer(ctx context.Context, playerID, actionID, action string, engineMutate func() error) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	changed := false
	apply := func() error {
		// AcceptWinnerCards expires a stale request and refunds the requester
		// before reporting that the window closed or the winner has left, so a
		// failed answer can still have moved chips in a.cached on the way to
		// that error. a.mutate covers exactly that: it restores the pre-call
		// snapshot on any error from engineMutate or from commit, so a failed
		// answer never leaves those chips sitting uncommitted in memory.
		return a.mutate(func() error {
			if err := engineMutate(); err != nil {
				return err
			}
			changed = true
			return a.commit(ctx, actionID, &tablestore.ActionLogEntry{
				PlayerID: playerID, ActionID: actionID, Action: action,
			})
		})
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		if !errors.Is(err, tablestore.ErrDuplicateAction) {
			return err
		}
		if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
			return reloadErr
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
		return a.mutate(func() error {
			if !a.cached.ExpireWinnerCards(timeNowFunc()) {
				return nil
			}
			changed = true
			return a.commit(ctx, "", &tablestore.ActionLogEntry{Action: "expire_winner_cards"})
		})
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		if !errors.Is(err, tablestore.ErrVersionConflict) {
			return err
		}
		if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
			return reloadErr
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
		return a.mutate(func() error {
			if err := a.cached.RefundRabbitHunt(c.PlayerID); err != nil {
				return err
			}
			changed = true
			return a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
				PlayerID: c.PlayerID, ActionID: c.ActionID, Action: "rabbit_hunt_verify_failed",
			})
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
		return a.mutate(func() error {
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
