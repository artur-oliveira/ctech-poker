package v1

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeRedisCounter is an in-memory stand-in for redisCounter that models
// Redis key/TTL semantics closely enough to exercise allowRedis: INCR
// creating/bumping a counter, and a key that (for whatever reason) carries no
// TTL getting one re-applied on its next hit — the exact recovery path #45
// depends on.
type fakeRedisCounter struct {
	counts map[string]int64
	hasTTL map[string]bool
	calls  int

	// failExpireOnce, if set, simulates the EXPIRE half of the very first
	// INCR failing to persist a TTL (e.g. the old code's EXPIRE call erroring
	// or the process dying between INCR and EXPIRE): the key is created by
	// INCR but never gets hasTTL set, exactly like a bare INCR would leave it.
	failExpireOnce bool
}

func newFakeRedisCounter() *fakeRedisCounter {
	return &fakeRedisCounter{counts: map[string]int64{}, hasTTL: map[string]bool{}}
}

func (f *fakeRedisCounter) incrAndBoundTTL(_ context.Context, key string, windowSeconds int64) (int64, error) {
	f.calls++
	f.counts[key]++
	n := f.counts[key]
	if f.failExpireOnce && n == 1 {
		f.failExpireOnce = false
		// Simulate the pre-fix bug: INCR created the key, but the TTL never
		// got applied, so hasTTL stays false.
		return n, nil
	}
	if !f.hasTTL[key] {
		f.hasTTL[key] = true
	}
	return n, nil
}

func TestRateLimiterAllowRedisIncrementsAndBlocksAboveLimit(t *testing.T) {
	fake := newFakeRedisCounter()
	rl := &RateLimiter{client: fake, limit: 2, window: time.Minute}

	for i := 0; i < 2; i++ {
		allow, err := rl.Allow(context.Background(), "k")
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i, err)
		}
		if !allow {
			t.Fatalf("request %d: expected within-limit request to be allowed", i)
		}
	}
	allow, err := rl.Allow(context.Background(), "k")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allow {
		t.Fatal("expected 3rd request over the limit to be blocked")
	}
	if !fake.hasTTL["k"] {
		t.Fatal("expected the key to carry a TTL after the first increment")
	}
}

func TestRateLimiterAllowRedisFirstKeyGetsTTL(t *testing.T) {
	fake := newFakeRedisCounter()
	rl := &RateLimiter{client: fake, limit: 5, window: time.Minute}

	if allow, err := rl.Allow(context.Background(), "fresh"); err != nil || !allow {
		t.Fatalf("expected first hit allowed, got allow=%v err=%v", allow, err)
	}
	if !fake.hasTTL["fresh"] {
		t.Fatal("expected a brand-new key to be given a TTL on its first increment")
	}
}

// TestRateLimiterAllowRedisRecoversMissingTTL is the regression test for #45:
// a key that ends up with a count but no TTL (the old EXPIRE-only-on-n==1
// code path failing) must not stay stuck forever — the very next hit against
// it must re-apply a TTL rather than leaving it TTL-less permanently.
func TestRateLimiterAllowRedisRecoversMissingTTL(t *testing.T) {
	fake := newFakeRedisCounter()
	fake.failExpireOnce = true
	rl := &RateLimiter{client: fake, limit: 5, window: time.Minute}

	if allow, err := rl.Allow(context.Background(), "k"); err != nil || !allow {
		t.Fatalf("expected first hit allowed, got allow=%v err=%v", allow, err)
	}
	if fake.hasTTL["k"] {
		t.Fatal("test setup: first hit should have simulated a missing TTL")
	}

	if allow, err := rl.Allow(context.Background(), "k"); err != nil || !allow {
		t.Fatalf("expected second hit allowed, got allow=%v err=%v", allow, err)
	}
	if !fake.hasTTL["k"] {
		t.Fatal("expected the TTL to be re-applied once the key is found without one")
	}
}

func TestRateLimiterAllowRedisPropagatesBackendError(t *testing.T) {
	wantErr := errors.New("boom")
	rl := &RateLimiter{
		client: redisCounterFunc(func(context.Context, string, int64) (int64, error) {
			return 0, wantErr
		}),
		limit:  5,
		window: time.Minute,
	}
	allow, err := rl.Allow(context.Background(), "k")
	if allow {
		t.Fatal("expected a backend error to not report an allowed request")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected the backend error to propagate, got %v", err)
	}
}

// redisCounterFunc adapts a plain function to redisCounter for tests that
// only need to control one call's return value.
type redisCounterFunc func(ctx context.Context, key string, windowSeconds int64) (int64, error)

func (f redisCounterFunc) incrAndBoundTTL(ctx context.Context, key string, windowSeconds int64) (int64, error) {
	return f(ctx, key, windowSeconds)
}

func TestSeatLimiterBlocksBurstAboveLimit(t *testing.T) {
	l := newSeatLimiter(3)
	for i := 0; i < 3; i++ {
		if !l.Allow("p1") {
			t.Fatalf("expected request %d within limit to be allowed", i)
		}
	}
	if l.Allow("p1") {
		t.Fatal("expected 4th request in the same window to be blocked")
	}
}

func TestSeatLimiterTracksPlayersIndependently(t *testing.T) {
	l := newSeatLimiter(1)
	if !l.Allow("p1") {
		t.Fatal("expected p1's first request allowed")
	}
	if !l.Allow("p2") {
		t.Fatal("expected p2's first request allowed independently of p1's count")
	}
}
