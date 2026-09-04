package table

import (
	"context"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

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
// once per hand for the whole fleet in the common case, but it fails OPEN on
// a Valkey error (see claimHandHooks/internal/handhook), so it cannot be the
// only guard for a truly non-idempotent counter: achievements.RecordHand and
// leaderboard's stats writers additionally claim
// achievements.Service.ClaimHandCounters (issue #66) before touching a
// counter, since a Valkey blip can still let two instances both reach here.
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
func (a *Actor) SetOnPlayerRemovedForActor(fn func(playerID, reason, settlementNonce string, stack int64, holdID string)) {
	a.onPlayerRemoved = fn
}

func (a *Actor) SetSystemSettlementIntentForActor(fn func(ctx context.Context, playerID, reason, settlementNonce string, stack int64, holdID string) (types.TransactWriteItem, error)) {
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
