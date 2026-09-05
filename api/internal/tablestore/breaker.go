package tablestore

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"gopkg.aoctech.app/poker/api/internal/metrics"
)

// ErrCommitThrottled means this table's commit circuit is open — the write
// never reached DynamoDB, so nothing about the caller's action was decided. It
// wraps ErrUnavailable on purpose: that is already the "the table is
// unavailable, retry" signal the WS gateway (internal/api/v1/tablews.go)
// surfaces and every actor handler treats as "abort this command", whereas
// ErrVersionConflict would be answered with an immediate reload-and-retry —
// exactly the loop this guard exists to stop. Callers that want to tell a
// throttle apart from a genuine store outage can errors.Is against this value.
var ErrCommitThrottled = fmt.Errorf("%w: table commit circuit open", ErrUnavailable)

// Why a per-table commit circuit exists, and where these numbers come from
// (issue #207, incident docs/specs/2026-09-03-next-hand-rearm-storm.md):
//
// That incident burned ~607k WCU on ONE table because a re-armed next-hand
// timer kept dispatching a transaction DynamoDB kept rejecting — ~8 rejected
// TransactWriteItems per second for 11.5 minutes, each still billed at 2x WCU
// per item. The fix capped re-arms inside internal/table
// (MaxNextHandArmsPerHand), which is the right fix for that loop; this is the
// defence in depth CommitAction itself owes every *other* caller, because it
// is the one shared write sink for every command, timer and sweep in the
// process.
//
// The trip condition is the storm's shape, not its rate: a run of REJECTED
// commits with no accepted commit in between. That is the pattern no
// legitimate traffic produces, and it is provably hopeless — a rejected
// conditional write means the table did not advance, so replaying the same
// mutation cannot succeed either. Real play never produces a long run of
// them: an actor's conflict retry reloads fresh state between attempts and
// gives up after one retry (Actor.retryOnConflict), and any accepted commit
// resets the run to zero.
//
// A per-table token bucket on commit *attempts* was tried first and
// deliberately dropped, because it cannot be both safe and useful. Commit rate
// is not a signal: legitimate play is paced by people, but nothing else in the
// system is, and a rate cap low enough to bound the incident's cost throttles
// any table driven faster than a human. Measured, not assumed —
// internal/table's own nine-handed integration test
// (TestNineHandedTableGrowsPlaysPausesAndLeaves) sustains ~115 commits/s on
// one table, ~14x the incident's ~8/s, so any ceiling that would have caught
// the incident breaks legitimate traffic, and any ceiling that leaves that
// traffic alone is far too high to bound a bill. The per-command ceilings that
// *are* meaningful stay where the pacing is known: internal/table's own timer
// and retry caps.
const (
	// maxConsecutiveRejections is the run of version-conflict/duplicate
	// rejections — with zero accepted commits in between — that opens a
	// table's circuit. Deliberately well above what genuine cross-instance
	// contention produces (any node may serve any table, so a conflict or two
	// per commit is routine; a run of 32 with nothing ever committing is not a
	// race, it is a loop). At the incident's ~8 dispatches/s this trips about
	// four seconds in, long before any material cost.
	maxConsecutiveRejections = 32

	// commitCooldownBase / commitCooldownMax bound how long an open circuit
	// stays open. Short to begin with, because the plausible false positive is
	// a burst of contention that resolves itself, and a 2s cooldown costs a
	// player one resync rather than a broken table; doubled on every failed
	// probe so a genuinely wedged table — which only cmd/tablecleanup, an
	// operator or the fixed request_exit can clear, never a transaction that
	// keeps being rejected — settles at one probe a minute.
	commitCooldownBase = 2 * time.Second
	commitCooldownMax  = 60 * time.Second

	// commitBudgetSoftCap / commitBudgetIdleTTL bound the per-table state. The
	// live set is the tables this process is committing to, but the map would
	// otherwise keep an entry for every table it ever touched, so entries idle
	// past the TTL are swept once the map grows past the cap.
	commitBudgetSoftCap = 512
	commitBudgetIdleTTL = 5 * time.Minute
)

// commitBudget is one table's rejection circuit.
type commitBudget struct {
	// rejections counts consecutive rejected commits; any accepted commit
	// resets it.
	rejections int
	openUntil  time.Time
	cooldown   time.Duration
	// halfOpen means the circuit has tripped and has not yet seen a commit
	// succeed. Once the cooldown expires exactly one probe commit is admitted
	// (probing); if that is rejected too the circuit re-opens with a doubled
	// cooldown, instead of letting the caller back into a full-rate loop.
	halfOpen bool
	probing  bool
	lastSeen time.Time
}

// commitBreaker is the per-process, per-table write guard in front of
// CommitAction. Per-process is the whole scope on purpose: a runaway loop
// lives in one process and every instance carries the same rule, so no shared
// state is needed — and a Valkey round trip on the write hot path would cost
// more than it saves.
type commitBreaker struct {
	mu      sync.Mutex
	tables  map[string]*commitBudget
	nowFunc func() time.Time
}

// metricCircuit counts circuit state changes, dimensioned by direction. An
// alarm on Transition=open is the "a table is in a rejection storm right now"
// signal #207 could not emit before #279.
const metricCircuit = "TableCommitCircuit"

func newCommitBreaker() *commitBreaker {
	return &commitBreaker{tables: make(map[string]*commitBudget)}
}

func (c *commitBreaker) now() time.Time {
	if c.nowFunc != nil {
		return c.nowFunc()
	}
	return timeNowFunc()
}

// allow decides whether tableID may attempt one commit right now, returning
// ErrCommitThrottled without touching DynamoDB while its circuit is open.
// action is used only for the transition log line, never as a map key, so log
// cardinality stays bounded by the number of state changes.
func (c *commitBreaker) allow(tableID, action string) error {
	if c == nil {
		return nil
	}
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()

	budget := c.tables[tableID]
	if budget == nil {
		c.evictIdleLocked(now)
		budget = &commitBudget{}
		c.tables[tableID] = budget
	}
	budget.lastSeen = now

	if now.Before(budget.openUntil) {
		return ErrCommitThrottled
	}
	if budget.halfOpen {
		if budget.probing {
			return ErrCommitThrottled
		}
		// One probe commit at a time: it is the only way to learn whether the
		// table can make progress again.
		budget.probing = true
	}
	return nil
}

// record feeds one commit's verdict back into tableID's circuit. Only a
// conditional rejection (version conflict or duplicate action) counts: a store
// outage (ErrUnavailable) is not a rejected write, and tripping on it would
// turn a DynamoDB blip into a self-inflicted table outage on top of it.
func (c *commitBreaker) record(tableID, action string, err error) {
	if c == nil {
		return
	}
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	budget := c.tables[tableID]
	if budget == nil {
		return
	}
	budget.lastSeen = now

	rejected := errors.Is(err, ErrVersionConflict) || errors.Is(err, ErrDuplicateAction)
	if budget.probing {
		budget.probing = false
		if rejected {
			c.openLocked(tableID, action, budget, now, "probe commit still rejected")
		} else if err == nil {
			c.closeLocked(tableID, budget)
		}
		return
	}
	switch {
	case err == nil:
		c.closeLocked(tableID, budget)
	case rejected:
		budget.rejections++
		if budget.rejections >= maxConsecutiveRejections {
			c.openLocked(tableID, action, budget, now, "consecutive rejected commits with no accepted commit")
		}
	}
}

func (c *commitBreaker) openLocked(tableID, action string, budget *commitBudget, now time.Time, cause string) {
	if budget.cooldown == 0 {
		budget.cooldown = commitCooldownBase
	} else if budget.cooldown < commitCooldownMax {
		budget.cooldown *= 2
		if budget.cooldown > commitCooldownMax {
			budget.cooldown = commitCooldownMax
		}
	}
	budget.openUntil = now.Add(budget.cooldown)
	budget.halfOpen = true
	budget.probing = false
	budget.rejections = 0
	// One line per state change, carrying the table, the action that tripped
	// it and the cause — enough to find the loop, and bounded by transitions
	// rather than by attempts (the incident's own symptom was 5,779 WARN lines
	// for one table; a guard that logged per attempt would reproduce it).
	slog.Error("tablestore: commit circuit open",
		"table", tableID, "action", action, "cause", cause,
		"cooldown_ms", budget.cooldown.Milliseconds())
	// Same rule as the log line: transitions, never attempts. A metric on a
	// per-attempt event is the 5,779-line symptom again, only on the bill.
	metrics.Record(metricCircuit, metrics.Count, metrics.Dims{"Transition": "open"}, 1)
}

func (c *commitBreaker) closeLocked(tableID string, budget *commitBudget) {
	budget.rejections = 0
	if budget.halfOpen {
		slog.Info("tablestore: commit circuit closed", "table", tableID)
		metrics.Record(metricCircuit, metrics.Count, metrics.Dims{"Transition": "closed"}, 1)
	}
	budget.halfOpen = false
	budget.cooldown = 0
	budget.openUntil = time.Time{}
}

// evictIdleLocked drops per-table state nothing has touched for
// commitBudgetIdleTTL, but only once the map has grown past its soft cap, so
// the common case pays nothing. Dropping an entry resets that table to a
// closed circuit, which is correct: idle for five minutes is not a storm. An
// entry whose circuit is still open is never dropped, so a wedged table cannot
// evict its own guard.
func (c *commitBreaker) evictIdleLocked(now time.Time) {
	if len(c.tables) <= commitBudgetSoftCap {
		return
	}
	for id, budget := range c.tables {
		if now.Sub(budget.lastSeen) > commitBudgetIdleTTL && !now.Before(budget.openUntil) {
			delete(c.tables, id)
		}
	}
}
