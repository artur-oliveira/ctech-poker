package table

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

func (a *Actor) applyActAndCommit(ctx context.Context, c ActCmd) (bool, error) {
	var applied bool
	err := a.mutate(func() error {
		bettingAction := a.cached.NormalizedActionForActor(c.PlayerID, c.Action)
		ok, err := a.cached.ActIdempotent(c.ActionID, c.PlayerID, c.Action, c.Amount)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		applied = true
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
		return a.commit(ctx, c.ActionID, &entry)
	})
	if err != nil {
		return false, err
	}
	return applied && a.cached.Stage() == hand.Complete, nil
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
		a.notifyChange()
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
	a.notifyChange()
	return nil
}

// notifyChange announces a just-completed commit to every sibling process
// serving this table (see ChangeNotifier's doc comment). Detached: Notify has
// its own short timeout and never returns an error the caller could act on —
// blocking every commit on a Valkey round trip would cost every player at
// this table real turn-decision latency for a signal that only ever speeds
// up a SIBLING process's convergence.
func (a *Actor) notifyChange() {
	if a.changeNotifier == nil {
		return
	}
	go a.changeNotifier.Notify(context.Background(), a.id)
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
// the previous hand's expiry behind).
//
// The fresh branch stashes its computed value in pendingNextHandDeadline
// (normally the field ensureLoaded fills from a *reload*) so armNextHandTimer
// — called from broadcastAll right after every commit — reuses this exact
// timestamp instead of computing its own via a second, independent
// timeNowFunc() call a few instructions later. Two separate "now"s here
// disagreed by tens of milliseconds in production (2026-09-04, see the
// incident spec's second follow-up): the client's useTableOutcome.ts latches
// onto whichever next_hand_unix_ms value it sees first and only accepts a
// later broadcast's value if it matches *exactly*, so any drift between the
// persisted and the broadcast deadline permanently froze the hand-outcome
// ring's countdown at 0. commitOutcomeLogEntries commits several more times
// for the same completed hand before broadcastAll ever runs; the
// pendingNextHandDeadline > 0 check below makes every one of those later
// calls reuse the same stashed value too, rather than drifting further with
// each one.
func (a *Actor) nextHandDeadlineForPersist() int64 {
	if a.cached == nil || a.cached.Stage() != hand.Complete {
		return 0
	}
	if a.handID != "" && a.handID == a.nextHandArmedFor {
		return a.nextHandDeadline.UnixMilli()
	}
	if a.pendingNextHandDeadline > 0 {
		return a.pendingNextHandDeadline
	}
	fresh := timeNowFunc().Add(a.nextHandDelay).UnixMilli()
	a.pendingNextHandDeadline = fresh
	return fresh
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
