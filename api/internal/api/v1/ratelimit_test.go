package v1

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// sharedCounter stands in for the fleet's Redis: several RateLimiter values
// (one per simulated API instance) INCR the same keyspace.
type sharedCounter struct {
	mu     sync.Mutex
	counts map[string]int64
	err    error
}

func newSharedCounter() *sharedCounter { return &sharedCounter{counts: map[string]int64{}} }

func (s *sharedCounter) incr(_ context.Context, key string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return 0, s.err
	}
	s.counts[key]++
	return s.counts[key], nil
}

// instance builds a limiter wired to this shared counter, i.e. one more
// process in the fleet.
func (s *sharedCounter) instance(limit int) *RateLimiter {
	return &RateLimiter{incr: s.incr, limit: limit, window: time.Second, mem: map[string]*rateWindow{}}
}

func TestSeatLimiterBlocksBurstAboveLimit(t *testing.T) {
	l := NewRateLimiter(nil, 3, time.Second)
	for i := 0; i < 3; i++ {
		if !l.AllowFailOpen(context.Background(), wsActionKey("p1")) {
			t.Fatalf("expected request %d within limit to be allowed", i)
		}
	}
	if l.AllowFailOpen(context.Background(), wsActionKey("p1")) {
		t.Fatal("expected 4th request in the same window to be blocked")
	}
}

func TestSeatLimiterTracksPlayersIndependently(t *testing.T) {
	l := NewRateLimiter(nil, 1, time.Second)
	if !l.AllowFailOpen(context.Background(), wsActionKey("p1")) {
		t.Fatal("expected p1's first request allowed")
	}
	if !l.AllowFailOpen(context.Background(), wsActionKey("p2")) {
		t.Fatal("expected p2's first request allowed independently of p1's count")
	}
}

// The #43 regression: two instances, one player, one budget.
func TestWSLimiterBudgetIsSharedAcrossInstances(t *testing.T) {
	fleet := newSharedCounter()
	instanceA, instanceB := fleet.instance(3), fleet.instance(3)
	ctx := context.Background()

	if !instanceA.AllowFailOpen(ctx, wsActionKey("p1")) || !instanceA.AllowFailOpen(ctx, wsActionKey("p1")) {
		t.Fatal("first two actions on instance A must be allowed")
	}
	if !instanceB.AllowFailOpen(ctx, wsActionKey("p1")) {
		t.Fatal("the third action must still be within the shared budget")
	}
	if instanceB.AllowFailOpen(ctx, wsActionKey("p1")) {
		t.Fatal("reconnecting to another instance must not refill the player's budget")
	}
	if !instanceB.AllowFailOpen(ctx, wsActionKey("p2")) {
		t.Fatal("one player's exhausted budget must not block another player")
	}
}

func TestWSLimiterFailsOpenOnBackendError(t *testing.T) {
	fleet := newSharedCounter()
	fleet.err = errors.New("valkey down")
	if !fleet.instance(1).AllowFailOpen(context.Background(), wsActionKey("p1")) {
		t.Fatal("a backend outage must never block legitimate play")
	}
}

func TestNilLimiterAllows(t *testing.T) {
	var l *RateLimiter
	if !l.AllowFailOpen(context.Background(), wsActionKey("p1")) {
		t.Fatal("an unwired limiter must allow (dev/test path)")
	}
}
