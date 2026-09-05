package table

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

// Deadlines and mailbox budget for the per-table actor (#223).
//
// One goroutine serves one table: every command for that table runs on it,
// one at a time, and Dispatch parks the caller's socket goroutine until the
// reply comes back. So unbounded I/O inside a handler does not stall one
// command, it stalls the whole table and every socket queued behind it. Until
// this existed, Run handed each handler the actor's *lifetime* context —
// cancelled only at process shutdown — and Dispatch waited forever for a
// mailbox slot, so a single hung DynamoDB or wallet call wedged the table for
// as long as the dependency stayed hung.
//
// The Valkey side-paths were already bounded individually (connStoreTimeout,
// streakStoreTimeout, handHookTimeout, and #222's pacing of the streak read);
// what was missing was a ceiling on the command as a whole, which is what
// covers the DynamoDB calls and anything added later.
//
// Two classes, because they fail differently:
//
//   - defaultCommandBudget covers everything a player is actively waiting on
//     (act, chat, snapshot, connect, ...). Losing one of these costs a resync:
//     the error wraps tablestore.ErrUnavailable, so tablews.go answers
//     "unavailable" and the client resubmits with the same action ID, which
//     CommitAction's idempotency guard collapses. Short on purpose — past ten
//     seconds the player has already given up on the frame.
//
//   - settlementCommandBudget covers the commands that can move money or
//     remove a seat (join, leave, the kick/AFK sweeps, next hand). Cutting one
//     of these short is not free — a removal whose settlement never ran is a
//     player without their chips — so the ceiling is deliberately far above
//     any healthy latency (the wallet client's own HTTP timeout is 6s) and
//     exists only to stop a dead dependency from pinning the table forever.
//     Everything on this path records its recovery intent to
//     poker_pending_cashouts before touching the wallet, so cmd/reconcile
//     still finishes a settlement this deadline interrupts.
const (
	defaultCommandBudget    = 10 * time.Second
	settlementCommandBudget = 30 * time.Second

	// defaultQueueBudget bounds how long Dispatch waits for a slot in a full
	// mailbox. The mailbox is 64 deep and every handler is now bounded, so
	// waiting longer than this means the table is genuinely wedged, and the
	// caller is better off being told to resync than parked on it.
	defaultQueueBudget = 2 * time.Second
)

// ErrQueueSaturated means the command never entered the actor's mailbox, so
// nothing about it was decided. It wraps tablestore.ErrUnavailable — the same
// reasoning as tablestore.ErrCommitThrottled: the gateway answers "unavailable"
// (resync and resubmit) instead of "invalid_action", which would blame the
// player for the server's backlog.
var ErrQueueSaturated = fmt.Errorf("%w: table command queue saturated", tablestore.ErrUnavailable)

// budgetFor classifies one command. Anything that can reach onPlayerRemoved /
// a settlement intent gets the safe-completion budget; everything else gets
// the interactive one.
func (a *Actor) budgetFor(cmd Command) time.Duration {
	switch cmd.(type) {
	case JoinCmd, LeaveCmd, kickTimeoutCmd, afkSweepCmd, nextHandCmd:
		return a.settlementBudget
	default:
		return a.commandBudget
	}
}

// budgetCounters is the only observability this has: there is no
// internal/metrics in this service (#279), so these are asserted in tests and
// logged at WARN when they move, rather than emitted anywhere.
type budgetCounters struct {
	// queueSaturations counts commands rejected because the mailbox stayed
	// full for the whole queue budget.
	queueSaturations atomic.Int64
	// queueWaitNanos sums the time Dispatch spent blocked on a full mailbox —
	// zero while the actor keeps up, which is what makes it a useful signal.
	queueWaitNanos atomic.Int64
	// handlerOverruns counts commands whose own deadline expired.
	handlerOverruns atomic.Int64
}

// BudgetSnapshot is a readable copy of those counters.
type BudgetSnapshot struct {
	QueueSaturations int64
	HandlerOverruns  int64
	QueueWait        time.Duration
}

// BudgetSnapshot reports what this actor's deadlines and mailbox budget have
// done so far. Safe to call from any goroutine.
func (a *Actor) BudgetSnapshot() BudgetSnapshot {
	return BudgetSnapshot{
		QueueSaturations: a.budget.queueSaturations.Load(),
		HandlerOverruns:  a.budget.handlerOverruns.Load(),
		QueueWait:        time.Duration(a.budget.queueWaitNanos.Load()),
	}
}

// handleWithBudget runs one command under its class deadline. The deadline is
// derived from Run's own context, so shutdown still cancels everything.
func (a *Actor) handleWithBudget(ctx context.Context, cmd Command) error {
	budget := a.budgetFor(cmd)
	cmdCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	started := time.Now()
	err := a.handleSafely(cmdCtx, cmd)
	if !errors.Is(cmdCtx.Err(), context.DeadlineExceeded) || ctx.Err() != nil {
		return err
	}
	a.budget.handlerOverruns.Add(1)
	slog.WarnContext(ctx, "table command exceeded its deadline",
		"table_id", a.id, "hand_id", a.handID, "command", fmt.Sprintf("%T", cmd),
		"budget", budget, "elapsed", time.Since(started))
	if err == nil || errors.Is(err, tablestore.ErrUnavailable) {
		return err
	}
	// The command ran out of time rather than reaching a verdict about the
	// player's action, so it must not surface as invalid_action.
	return fmt.Errorf("%w: table command deadline exceeded: %v", tablestore.ErrUnavailable, err)
}
