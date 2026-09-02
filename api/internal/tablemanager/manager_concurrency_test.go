package tablemanager

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

// TestGetOrCreateActorDoesNotSerializeAcrossDifferentTables proves #31 is
// fixed: two concurrent GetOrCreateActor calls for two different tables no
// longer contend on one process-global mutex across their (simulated)
// network round trips. preRegisterHook stands in for the real
// LoadTable/Acquire/roomLoader calls — it fires once per actual actor
// creation, still inside the create path, so an artificial delay there
// exercises exactly the section #31 said was serializing the whole
// instance. If the old global-mutex behavior were still in place, the two
// hook invocations below could never overlap and this call would take
// roughly 2*delay instead of ~1*delay.
func TestGetOrCreateActorDoesNotSerializeAcrossDifferentTables(t *testing.T) {
	const delay = 100 * time.Millisecond

	m := NewManager(nil, nil, nil, nil)

	var mu sync.Mutex
	var windows []struct{ start, end time.Time }
	m.preRegisterHook = func(tableID string) {
		start := time.Now()
		time.Sleep(delay)
		end := time.Now()
		mu.Lock()
		windows = append(windows, struct{ start, end time.Time }{start, end})
		mu.Unlock()
	}

	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }

	var wg sync.WaitGroup
	overall := time.Now()
	for _, id := range []string{"table-A", "table-B"} {
		wg.Add(1)
		go func(tableID string) {
			defer wg.Done()
			if _, err := m.GetOrCreateActor(context.Background(), tableID, seed); err != nil {
				t.Errorf("GetOrCreateActor(%s): %v", tableID, err)
			}
		}(id)
	}
	wg.Wait()
	elapsed := time.Since(overall)

	if elapsed >= 2*delay {
		t.Fatalf("two different-table creates took %v (>= 2x delay of %v) — they serialized on a shared lock", elapsed, delay)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(windows) != 2 {
		t.Fatalf("expected 2 create windows, got %d", len(windows))
	}
	// Overlap check: window[0] must start before window[1] ends AND
	// window[1] must start before window[0] ends.
	a, b := windows[0], windows[1]
	overlap := a.start.Before(b.end) && b.start.Before(a.end)
	if !overlap {
		t.Fatalf("create windows for the two tables did not overlap: %+v vs %+v", a, b)
	}
}

// TestGetOrCreateActorDedupesSameTableUnderConcurrency stresses T7 under
// real concurrency (run with -race -count=5): many goroutines racing to
// create the actor for the same tableID must still produce exactly one
// actor and run the create path exactly once, even though the create path
// now runs outside the global mutex.
func TestGetOrCreateActorDedupesSameTableUnderConcurrency(t *testing.T) {
	m := NewManager(nil, nil, nil, nil)

	var created int32
	m.preRegisterHook = func(tableID string) {
		atomic.AddInt32(&created, 1)
		time.Sleep(5 * time.Millisecond)
	}

	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }

	const goroutines = 20
	actors := make([]*Actor, goroutines)
	errs := make([]error, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			actors[i], errs[i] = m.GetOrCreateActor(context.Background(), "table-dup", seed)
		}(i)
	}
	wg.Wait()

	if got := atomic.LoadInt32(&created); got != 1 {
		t.Fatalf("expected exactly 1 actor creation across %d concurrent callers, got %d", goroutines, got)
	}
	first := actors[0]
	if first == nil {
		t.Fatal("first actor is nil")
	}
	for i, a := range actors {
		if errs[i] != nil {
			t.Fatalf("goroutine %d: %v", i, errs[i])
		}
		if a != first {
			t.Fatalf("goroutine %d got a different actor pointer than goroutine 0", i)
		}
	}
}
