package pokerpush

import (
	"context"
	"sync"
	"testing"
	"time"

	"gopkg.aoctech.app/api-commons/pushsvc"
)

type fakeMover struct {
	mu    sync.Mutex
	rows  map[string]pushsvc.Reminder
	moves []pushsvc.Reminder
}

// MoveSlot is conditional like the real one: only an enabled reminder still in `from` moves.
func (f *fakeMover) MoveSlot(_ context.Context, sub, kind string, from, to int) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[sub+"/"+kind]
	if !ok || !r.Enabled || r.Slot != from {
		return false, nil
	}
	r.Slot = to
	f.rows[sub+"/"+kind] = r
	f.moves = append(f.moves, r)
	return true, nil
}

type fakeStarts struct{ starts []time.Time }

func (f fakeStarts) RecentStarts(context.Context, string, int) ([]time.Time, error) {
	return f.starts, nil
}

func play(slot int) pushsvc.Reminder {
	return pushsvc.Reminder{Sub: "u1", Kind: KindPlay, Enabled: true, Slot: slot}
}

func startsAt(hm ...[2]int) []time.Time {
	var out []time.Time
	for _, t := range hm {
		out = append(out, time.Date(2026, 11, 10, t[0]+3, t[1], 0, 0, time.UTC)) // BRT = UTC-3
	}
	return out
}

func TestRelearnMovesPlayToTheTypicalSlot(t *testing.T) {
	m := &fakeMover{rows: map[string]pushsvc.Reminder{"u1/play": play(80)}}
	r := NewRelearner(m, fakeStarts{startsAt([2]int{20, 10}, [2]int{20, 40}, [2]int{21, 5}, [2]int{20, 20}, [2]int{19, 55})})
	r.OnSent(context.Background(), play(80))
	if len(m.moves) != 1 || m.moves[0].Slot != 81 {
		t.Errorf("moves = %+v, want the play reminder moved to slot 81 (20:20)", m.moves)
	}
}

func TestRelearnWithoutSessionsFallsBackToTheDefault(t *testing.T) {
	m := &fakeMover{rows: map[string]pushsvc.Reminder{"u1/play": play(80)}}
	NewRelearner(m, fakeStarts{}).OnSent(context.Background(), play(80))
	if len(m.moves) != 1 || m.moves[0].Slot != DefaultPlaySlot {
		t.Errorf("moves = %+v", m.moves)
	}
}

func TestRelearnDoesNotRewriteAnUnchangedSlot(t *testing.T) {
	m := &fakeMover{rows: map[string]pushsvc.Reminder{"u1/play": play(80)}}
	NewRelearner(m, fakeStarts{startsAt([2]int{20, 0}, [2]int{20, 0}, [2]int{20, 0}, [2]int{20, 0}, [2]int{20, 0})}).OnSent(context.Background(), play(80))
	if len(m.moves) != 0 {
		t.Errorf("moves = %+v", m.moves)
	}
}

func TestRelearnIgnoresTheStreakReminder(t *testing.T) {
	m := &fakeMover{rows: map[string]pushsvc.Reminder{"u1/streak": {Sub: "u1", Kind: KindStreak, Enabled: true, Slot: 80}}}
	NewRelearner(m, fakeStarts{}).OnSent(context.Background(), m.rows["u1/streak"])
	if len(m.moves) != 0 {
		t.Errorf("moves = %+v: the streak time is the player's own choice, never learned", m.moves)
	}
}

// The player turns "come play" off while the tick is working: the learned-hour move must not switch it back on.
func TestRelearnNeverResurrectsAReminderThePlayerSwitchedOff(t *testing.T) {
	off := play(80)
	off.Enabled = false
	m := &fakeMover{rows: map[string]pushsvc.Reminder{"u1/play": off}}
	NewRelearner(m, fakeStarts{}).OnSent(context.Background(), play(80)) // the tick still holds the enabled copy
	if got := m.rows["u1/play"]; got.Enabled || got.Slot != 80 || len(m.moves) != 0 {
		t.Errorf("reminder = %+v moves = %+v: a background move re-enabled or moved an opt-out", got, m.moves)
	}
}
