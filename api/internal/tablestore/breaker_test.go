package tablestore

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

// fakeClock drives the breaker without spending wall time, so an 11-minute
// storm is a unit test rather than a load test.
type fakeClock struct{ now time.Time }

func (c *fakeClock) advance(d time.Duration) { c.now = c.now.Add(d) }

func newTestBreaker() (*commitBreaker, *fakeClock) {
	clock := &fakeClock{now: time.Date(2026, 9, 3, 13, 0, 0, 0, time.UTC)}
	breaker := newCommitBreaker()
	breaker.nowFunc = func() time.Time { return clock.now }
	return breaker, clock
}

// TestCommitThrottledIsUnavailableNotAConflict pins the error contract the
// whole guard depends on: the actor must abort the command, not reload and
// retry. ErrVersionConflict is answered with an immediate retry
// (Actor.retryOnConflict), which would defeat the point.
func TestCommitThrottledIsUnavailableNotAConflict(t *testing.T) {
	if !errors.Is(ErrCommitThrottled, ErrUnavailable) {
		t.Fatal("a throttled commit must read as ErrUnavailable so callers abort the command")
	}
	if errors.Is(ErrCommitThrottled, ErrVersionConflict) || errors.Is(ErrCommitThrottled, ErrDuplicateAction) {
		t.Fatal("a throttled commit must never read as a conflict: that path retries immediately")
	}
}

// TestCommitBreakerPassesALegitimateHand is the false-positive guard: a whole
// nine-handed hand's worth of accepted commits must sail through, hand after
// hand.
func TestCommitBreakerPassesALegitimateHand(t *testing.T) {
	breaker, clock := newTestBreaker()
	const commitsPerHand = 60
	for hand := range 20 {
		for i := range commitsPerHand {
			if err := breaker.allow("t1", "act"); err != nil {
				t.Fatalf("hand %d commit %d of legitimate play was throttled: %v", hand, i, err)
			}
			breaker.record("t1", "act", nil)
			clock.advance(333 * time.Millisecond)
		}
	}
}

// TestCommitBreakerPassesMachineSpeedPlay is the other half of the
// false-positive guard, and the reason the rate-based half of this guard was
// dropped: nothing outside real play is paced by people. Accepted commits at
// the ~115/s one table sustains under internal/table's nine-handed
// integration test must never be throttled.
func TestCommitBreakerPassesMachineSpeedPlay(t *testing.T) {
	breaker, clock := newTestBreaker()
	for i := range 5000 {
		if err := breaker.allow("t1", "act"); err != nil {
			t.Fatalf("machine-speed commit %d was throttled: %v", i, err)
		}
		breaker.record("t1", "act", nil)
		clock.advance(8 * time.Millisecond) // ~125 commits/s
	}
}

// TestCommitBreakerToleratesContentionThatStillMakesProgress covers the third
// false positive: any node may serve any table, so version conflicts are
// routine. A conflict (or several) followed by an accepted commit must never
// open the circuit, however long that goes on.
func TestCommitBreakerToleratesContentionThatStillMakesProgress(t *testing.T) {
	breaker, clock := newTestBreaker()
	for round := range 200 {
		for range maxConsecutiveRejections - 1 {
			if err := breaker.allow("t1", "act"); err != nil {
				t.Fatalf("round %d: contention retry was throttled: %v", round, err)
			}
			breaker.record("t1", "act", ErrVersionConflict)
			clock.advance(200 * time.Millisecond)
		}
		if err := breaker.allow("t1", "act"); err != nil {
			t.Fatalf("round %d: the commit that finally lands was throttled: %v", round, err)
		}
		breaker.record("t1", "act", nil)
		clock.advance(time.Second)
	}
}

// TestCommitBreakerIgnoresStoreOutages: a DynamoDB blip is not a rejected
// write, and tripping on it would turn an outage into a self-inflicted table
// outage on top of it.
func TestCommitBreakerIgnoresStoreOutages(t *testing.T) {
	breaker, clock := newTestBreaker()
	outage := errors.New("boom")
	for i := range 500 {
		if err := breaker.allow("t1", "act"); err != nil {
			t.Fatalf("attempt %d: store outages must not open the circuit, got %v", i, err)
		}
		breaker.record("t1", "act", fmt.Errorf("%w: commit: %w", ErrUnavailable, outage))
		clock.advance(100 * time.Millisecond)
	}
}

// TestCommitBreakerStopsARejectionStorm replays the shape of the 2026-09-03
// incident (docs/specs/2026-09-03-next-hand-rearm-storm.md): one table, one
// wedged seat, a re-armed timer dispatching a transaction DynamoDB keeps
// rejecting at ~8/s for 11.5 minutes. That cost 5,779 rejected
// TransactWriteItems. The circuit must cut it to a trivial number without ever
// letting the caller loop, and must recover on its own afterwards.
func TestCommitBreakerStopsARejectionStorm(t *testing.T) {
	breaker, clock := newTestBreaker()
	const (
		stormInterval = 125 * time.Millisecond // ~8 dispatches/s
		stormDuration = 11*time.Minute + 30*time.Second
	)
	attempts, admitted := 0, 0
	for elapsed := time.Duration(0); elapsed < stormDuration; elapsed += stormInterval {
		attempts++
		if err := breaker.allow("t1", "next_hand"); err != nil {
			if !errors.Is(err, ErrUnavailable) {
				t.Fatalf("a throttled commit must be recoverable/abortable, got %v", err)
			}
			clock.advance(stormInterval)
			continue
		}
		admitted++
		breaker.record("t1", "next_hand", ErrVersionConflict)
		clock.advance(stormInterval)
	}
	if attempts < 5000 {
		t.Fatalf("test did not reproduce the storm's attempt volume: %d", attempts)
	}
	// maxConsecutiveRejections to open the circuit, then one probe per
	// (doubling, capped at commitCooldownMax) cooldown window for the rest of
	// the storm: a few dozen transactions instead of the incident's 5,779.
	if admitted > 60 {
		t.Fatalf("storm of %d attempts still reached DynamoDB %d times; expected the circuit to cap it",
			attempts, admitted)
	}
	if admitted < maxConsecutiveRejections {
		t.Fatalf("circuit opened before the documented rejection run: %d admitted", admitted)
	}
	// Self-healing: wait out the cooldown, let the probe through, confirm
	// normal service returns.
	clock.advance(commitCooldownMax + time.Second)
	if err := breaker.allow("t1", "next_hand"); err != nil {
		t.Fatalf("probe commit must be admitted after the cooldown: %v", err)
	}
	breaker.record("t1", "next_hand", nil)
	for i := range 100 {
		if err := breaker.allow("t1", "act"); err != nil {
			t.Fatalf("commit %d after recovery was throttled: %v", i, err)
		}
		breaker.record("t1", "act", nil)
	}
}

// TestCommitBreakerIsPerTable: one wedged table must not throttle any other
// table this process is serving.
func TestCommitBreakerIsPerTable(t *testing.T) {
	breaker, clock := newTestBreaker()
	for range maxConsecutiveRejections {
		_ = breaker.allow("wedged", "next_hand")
		breaker.record("wedged", "next_hand", ErrVersionConflict)
	}
	if err := breaker.allow("wedged", "next_hand"); err == nil {
		t.Fatal("the wedged table's circuit should be open")
	}
	clock.advance(time.Second)
	for i := range 50 {
		if err := breaker.allow("healthy", "act"); err != nil {
			t.Fatalf("healthy table commit %d was throttled by another table: %v", i, err)
		}
		breaker.record("healthy", "act", nil)
	}
}

// TestCommitBreakerEvictsIdleTables keeps the per-table state from growing
// with every table the process ever touched.
func TestCommitBreakerEvictsIdleTables(t *testing.T) {
	breaker, clock := newTestBreaker()
	for i := range commitBudgetSoftCap + 1 {
		_ = breaker.allow(fmt.Sprintf("table-%d", i), "act")
	}
	clock.advance(commitBudgetIdleTTL + time.Minute)
	// The next new table triggers the sweep of everything gone idle.
	_ = breaker.allow("fresh", "act")
	breaker.mu.Lock()
	remaining := len(breaker.tables)
	breaker.mu.Unlock()
	if remaining != 1 {
		t.Fatalf("expected only the fresh table to remain, got %d entries", remaining)
	}
}

// TestCommitBreakerKeepsAWedgedTablesGuardWhileSweeping: eviction must not be
// a way for a storming table to lose its own circuit and start over.
func TestCommitBreakerKeepsAWedgedTablesGuardWhileSweeping(t *testing.T) {
	breaker, clock := newTestBreaker()
	for range maxConsecutiveRejections {
		_ = breaker.allow("wedged", "next_hand")
		breaker.record("wedged", "next_hand", ErrVersionConflict)
	}
	for i := range commitBudgetSoftCap + 1 {
		_ = breaker.allow(fmt.Sprintf("table-%d", i), "act")
	}
	// Long enough for the idle TTL to pass but not the wedged table's cooldown.
	clock.advance(commitBudgetIdleTTL + time.Minute)
	breaker.mu.Lock()
	budget := breaker.tables["wedged"]
	breaker.mu.Unlock()
	if budget == nil {
		t.Fatal("a wedged table must keep its circuit across an idle sweep")
	}
}
