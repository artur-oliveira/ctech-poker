package table

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/oklog/ulid/v2"
	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

func (a *Actor) handleTurnTimeout(ctx context.Context, c turnTimeoutCmd) error {
	// Force a fresh reload here, same reasoning as handleJoin: this command
	// only ever arrives from a time.AfterFunc armed up to a full turn timeout
	// plus time bank ago, on an Actor whose trustCache affinity is a
	// latency-only optimization (internal/tablelease), not a fleet-wide
	// exclusive lock — another instance is free to have processed this exact
	// player's decision (or several more hands) in the meantime and this
	// instance never observed it. The trustCache fast path would let the stale
	// CurrentPlayerIDForActor check below pass anyway, folding/time-banking a
	// decision that already resolved elsewhere — reproduced live on
	// 2026-09-04 (docs/specs/2026-09-04-cross-instance-stale-turn-timer.md):
	// two instances both charged time bank for the same player's turn on a
	// hand the other had already carried to Complete.
	if err := a.ensureLoaded(ctx, true); err != nil {
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
		// a.mutate is what guarantees a commit failure here can't leave an
		// uncommitted SitOutForActor mutation layered on stale state trusted
		// in a.cached for whatever this actor does next (e.g. a later
		// kick-timeout removal computing handInProgress/dealtIntoCurrentHand
		// off of it, which could wrongly allow removing a player still dealt
		// into the REAL hand and leave a stale handOrder entry for a
		// since-removed player — runShowdown's playerByID lookup on that
		// entry would then panic).
		err := a.mutate(func() error {
			timeBankMs := a.consumeTimeBank(c.PlayerID)
			a.cached.SitOutForActor(c.PlayerID)
			return a.commit(ctx, "", &tablestore.ActionLogEntry{
				PlayerID: c.PlayerID, Action: "disconnect_sit_out", TimeBankMs: timeBankMs,
			})
		})
		if err != nil {
			// An ErrVersionConflict genuinely means someone else already
			// advanced, so reload to reconcile before broadcasting; mutate has
			// already restored a.cached for any other error, so nothing further
			// needs discarding there — just propagate it.
			if errors.Is(err, tablestore.ErrVersionConflict) {
				if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
					return reloadErr
				}
			} else {
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
	// mutate is what guarantees a commit failure below (a transient store
	// error, not just a version conflict retryOnConflict already reloads on)
	// can't leave a phantom seated player — already carrying their debited
	// buy-in stack — trusted in this actor's cache with no matching
	// poker_action_log entry. Without this, the next unrelated successful
	// commit persists the ghost seat for real the first time any other
	// player's action commits — the 2026-09-01 incident.
	return a.mutate(func() error {
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
				return err
			}
			extra = append(extra, intent)
		}
		return a.commit(ctx, "", &tablestore.ActionLogEntry{PlayerID: c.PlayerID, Action: "join"}, extra...)
	})
}

// newSettlementNonce returns a fresh token to make one system removal's
// settlement key unique (see the onPlayerRemoved field comment). Generated
// once per removal attempt and forwarded verbatim to both the settlement
// intent builder and the onPlayerRemoved hook so the co-committed
// poker_pending_cashouts row and the follow-up SettleSystemRemoval credit
// resolve the same key.
func newSettlementNonce() string {
	return ulid.MustNew(ulid.Timestamp(timeNowFunc()), rand.Reader).String()
}

func (a *Actor) systemLeaveCmd(ctx context.Context, playerID, reason, settlementNonce string, stack chan int64, holdID chan string) LeaveCmd {
	cmd := LeaveCmd{PlayerID: playerID, Stack: stack, HoldID: holdID}
	if a.systemSettlementIntent != nil {
		cmd.SettlementIntent = func(amount int64, hold string) (types.TransactWriteItem, error) {
			return a.systemSettlementIntent(ctx, playerID, reason, settlementNonce, amount, hold)
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
	var stack int64
	var holdID string
	err := a.mutate(func() error {
		var err error
		stack, holdID, err = a.cached.RemovePlayerForActor(c.PlayerID)
		if err != nil {
			return err
		}
		var extra []types.TransactWriteItem
		if c.SettlementIntent != nil {
			intent, err := c.SettlementIntent(stack, holdID)
			if err != nil {
				return fmt.Errorf("table: build settlement intent: %w", err)
			}
			extra = append(extra, intent)
		}
		return a.commit(ctx, "", &tablestore.ActionLogEntry{PlayerID: c.PlayerID, Action: "leave"}, extra...)
	})
	if err != nil {
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
