package leaderboard

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

// The month a hand belongs to is the player's month, not UTC's: in UTC the
// last three hours of every month already read as the next one.
func TestMonthKeyTurnsAtBRTMidnight(t *testing.T) {
	for _, tc := range []struct {
		name string
		at   time.Time
		want string
	}{
		{"23:59 BRT on the last day is still that month", time.Date(2026, 9, 30, 23, 59, 0, 0, brt), "2026-09"},
		{"00:00 BRT on the first is the new month", time.Date(2026, 10, 1, 0, 0, 0, 0, brt), "2026-10"},
		{"02:00 UTC on the first is still the previous month in BRT", time.Date(2026, 10, 1, 2, 0, 0, 0, time.UTC), "2026-09"},
	} {
		if got := MonthKey(tc.at); got != tc.want {
			t.Errorf("%s: MonthKey=%q want %q", tc.name, got, tc.want)
		}
	}
}

func TestNormalizePeriodDefaultsToLifetime(t *testing.T) {
	// An empty period must stay the lifetime board: the CLI and the mobile
	// client never send one and must keep seeing what they see today.
	if got, err := normalizePeriod(""); err != nil || got != PeriodAll {
		t.Fatalf("normalizePeriod(\"\")=%q err=%v", got, err)
	}
	if _, err := normalizePeriod("week"); err == nil {
		t.Fatal("expected an unsupported period to be rejected, not silently served as another board")
	}
}

// The lifetime board's keys must be byte-identical to what rows were written
// with before periods existed — that is what makes this change need no
// backfill.
func TestLifetimeKeysAreUnchanged(t *testing.T) {
	if got := statsSKFor("sandbox", ""); got != "stats#sandbox" {
		t.Errorf("lifetime sort key=%q", got)
	}
	if got := boardKey("sandbox", ""); got != "sandbox" {
		t.Errorf("lifetime GSI partition=%q", got)
	}
	if got := statsSKFor("sandbox", "2026-09"); got != "stats#sandbox#2026-09" {
		t.Errorf("monthly sort key=%q", got)
	}
	if got := boardKey("sandbox", "2026-09"); got != "sandbox#2026-09" {
		t.Errorf("monthly GSI partition=%q", got)
	}
}

// A hand counts on both boards, and a new month starts the monthly board from
// zero while the lifetime one keeps accumulating.
func TestMonthlyBoardResetsAndLifetimeDoesNot(t *testing.T) {
	m := &memStats{rows: map[string]*Entry{}}
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, brt)
	s := NewServiceWithStore(m).WithClock(func() time.Time { return now })
	outcome := hand.HandOutcome{Winners: []string{"p1"}, Participants: []string{"p1", "p2"}}

	for i := 0; i < 3; i++ {
		if err := s.RecordHand(context.Background(), "sandbox", outcome, nil); err != nil {
			t.Fatal(err)
		}
	}
	now = now.AddDate(0, 0, 1) // 2026-10-01 BRT
	if err := s.RecordHand(context.Background(), "sandbox", outcome, nil); err != nil {
		t.Fatal(err)
	}

	lifetime, _, err := s.Top(context.Background(), Board{Mode: "sandbox", Period: PeriodAll}, 10, nil)
	if err != nil || len(lifetime) != 2 || lifetime[0].HandsWon != 4 {
		t.Fatalf("lifetime board=%+v err=%v", lifetime, err)
	}
	month, _, err := s.Top(context.Background(), Board{Mode: "sandbox", Period: PeriodMonth}, 10, nil)
	if err != nil || len(month) != 2 || month[0].HandsWon != 1 {
		t.Fatalf("october board=%+v err=%v; a new month must not inherit September's counters", month, err)
	}
	if september := m.rows[rowKey("sandbox", "2026-09", "p1")]; september == nil || september.HandsWon != 3 {
		t.Fatalf("september row=%+v; a closed month must stay queryable", september)
	}
}

func TestMyRankIsScopedToItsPeriod(t *testing.T) {
	m := &memStats{rows: map[string]*Entry{}}
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, brt)
	s := NewServiceWithStore(m).WithClock(func() time.Time { return now })

	// p2 has a long lifetime lead; p1 played more this month.
	for i := 0; i < 5; i++ {
		if err := s.RecordHand(context.Background(), "sandbox", hand.HandOutcome{Winners: []string{"p2"}, Participants: []string{"p2"}}, nil); err != nil {
			t.Fatal(err)
		}
	}
	now = now.AddDate(0, 1, 0) // October: only p1 plays
	if err := s.RecordHand(context.Background(), "sandbox", hand.HandOutcome{Winners: []string{"p1"}, Participants: []string{"p1"}}, nil); err != nil {
		t.Fatal(err)
	}

	monthly, err := s.MyRank(context.Background(), Board{Mode: "sandbox", Period: PeriodMonth}, "p1")
	if err != nil || monthly == nil || monthly.Rank != 1 {
		t.Fatalf("monthly rank=%+v err=%v; October's board must not carry September's leader", monthly, err)
	}
	lifetime, err := s.MyRank(context.Background(), Board{Mode: "sandbox", Period: PeriodAll}, "p1")
	if err != nil || lifetime == nil || lifetime.Rank != 2 {
		t.Fatalf("lifetime rank=%+v err=%v", lifetime, err)
	}
	if _, err := s.MyRank(context.Background(), Board{Mode: "sandbox", Period: "week"}, "p1"); err == nil {
		t.Fatal("expected an unsupported period to be rejected")
	}
}
