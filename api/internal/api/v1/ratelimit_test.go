package v1

import "testing"

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
