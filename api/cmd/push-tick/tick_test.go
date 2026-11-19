package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"gopkg.aoctech.app/api-commons/pushsvc"
	"gopkg.aoctech.app/poker/api/internal/pokerpush"
)

// 20:00 in São Paulo on 2026-11-18 is slot 80.
var now = time.Date(2026, 11, 18, 23, 0, 0, 0, time.UTC)

type fakeReminders struct {
	mu     sync.Mutex
	rows   map[string]pushsvc.Reminder // sub/kind
	marked map[string]string           // sub/kind -> day
	puts   []pushsvc.Reminder
	marks  int
}

func newReminders(rs ...pushsvc.Reminder) *fakeReminders {
	f := &fakeReminders{rows: map[string]pushsvc.Reminder{}, marked: map[string]string{}}
	for _, r := range rs {
		f.rows[r.Sub+"/"+r.Kind] = r
	}
	return f
}

func (f *fakeReminders) ListSlot(_ context.Context, slot, _ int, _ map[string]types.AttributeValue) ([]pushsvc.Reminder, map[string]types.AttributeValue, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []pushsvc.Reminder
	for _, r := range f.rows {
		if r.Enabled && r.Slot == slot {
			out = append(out, r)
		}
	}
	return out, nil, nil
}

func (f *fakeReminders) MarkSent(_ context.Context, sub, kind, day string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.marks++
	if f.marked[sub+"/"+kind] == day {
		return false, nil
	}
	f.marked[sub+"/"+kind] = day
	return true, nil
}

// MoveSlot is conditional like the real one: only an enabled reminder still in `from` moves.
func (f *fakeReminders) MoveSlot(_ context.Context, sub, kind string, from, to int) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[sub+"/"+kind]
	if !ok || !r.Enabled || r.Slot != from {
		return false, nil
	}
	r.Slot = to
	f.rows[sub+"/"+kind] = r
	f.puts = append(f.puts, r)
	return true, nil
}

type fakeDevices map[string]int

func (f fakeDevices) List(_ context.Context, sub string) ([]pushsvc.Subscription, error) {
	return make([]pushsvc.Subscription, f[sub]), nil
}

type fakeRules struct {
	fail map[string]bool
	skip map[string]bool
}

func (f fakeRules) Decide(_ context.Context, _ time.Time, r pushsvc.Reminder) (*pushsvc.Job, error) {
	if f.fail[r.Sub] {
		return nil, errors.New("boom")
	}
	if f.skip[r.Sub] {
		return nil, nil
	}
	return &pushsvc.Job{EventID: r.Kind + ":" + r.Sub, Recipients: []string{r.Sub}}, nil
}

type fakeSender struct {
	mu   sync.Mutex
	jobs []pushsvc.Job
	err  error
}

func (f *fakeSender) Send(_ context.Context, j pushsvc.Job) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.jobs = append(f.jobs, j)
	return f.err
}

type fakeHabits struct{ starts []time.Time }

func (f fakeHabits) RecentStarts(context.Context, string, int) ([]time.Time, error) {
	return f.starts, nil
}

func streak(sub string) pushsvc.Reminder {
	return pushsvc.Reminder{Sub: sub, Kind: pokerpush.KindStreak, Enabled: true, Slot: 80}
}

func tick(rem *fakeReminders, dev fakeDevices, rules fakeRules, s *fakeSender, habits fakeHabits) *Tick {
	return NewTick(rem, dev, rules, s, habits)
}

func TestRunSendsOncePerDayWhenTickedTwice(t *testing.T) {
	rem, s := newReminders(streak("u1")), &fakeSender{}
	tk := tick(rem, fakeDevices{"u1": 1}, fakeRules{}, s, fakeHabits{})
	for i := 0; i < 2; i++ {
		if err := tk.Run(context.Background(), now); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.jobs) != 1 {
		t.Errorf("sent %d, want 1", len(s.jobs))
	}
}

func TestRunProcessesPreviousSlot(t *testing.T) {
	late := streak("u1")
	late.Slot = 79 // 19:45, the slot before 20:00
	rem, s := newReminders(late), &fakeSender{}
	if err := tick(rem, fakeDevices{"u1": 1}, fakeRules{}, s, fakeHabits{}).Run(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if len(s.jobs) != 1 {
		t.Errorf("a reminder of the previous slot must be recovered, sent %d", len(s.jobs))
	}
}

func TestRunSkipsUserWithoutDeviceAndDoesNotConsumeTheDay(t *testing.T) {
	rem, s := newReminders(streak("u1")), &fakeSender{}
	if err := tick(rem, fakeDevices{}, fakeRules{}, s, fakeHabits{}).Run(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if len(s.jobs) != 0 || rem.marks != 0 {
		t.Errorf("sent=%d marks=%d, want none", len(s.jobs), rem.marks)
	}
}

func TestRunDoesNotMarkWhenTheRuleSaysNo(t *testing.T) {
	rem, s := newReminders(streak("u1")), &fakeSender{}
	if err := tick(rem, fakeDevices{"u1": 1}, fakeRules{skip: map[string]bool{"u1": true}}, s, fakeHabits{}).Run(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if len(s.jobs) != 0 || rem.marks != 0 {
		t.Errorf("sent=%d marks=%d: a day the rule declined must stay open for the catch-up", len(s.jobs), rem.marks)
	}
}

func TestRunMarksBeforeInvoking(t *testing.T) {
	rem, s := newReminders(streak("u1")), &fakeSender{err: errors.New("lambda unavailable")}
	tk := tick(rem, fakeDevices{"u1": 1}, fakeRules{}, s, fakeHabits{})
	if err := tk.Run(context.Background(), now); err != nil {
		t.Fatalf("a failed hand-off to the sender is logged, not a failed run: %v", err)
	}
	s.err = nil
	_ = tk.Run(context.Background(), now)
	if len(s.jobs) != 1 {
		t.Errorf("sent %d: a marked day is never sent twice, even after a failed invoke", len(s.jobs))
	}
}

func TestRunMovesPlaySlotAfterSend(t *testing.T) {
	play := pushsvc.Reminder{Sub: "u1", Kind: pokerpush.KindPlay, Enabled: true, Slot: 80}
	rem, s := newReminders(play), &fakeSender{}
	var starts []time.Time
	for _, hm := range [][2]int{{20, 10}, {20, 40}, {21, 5}, {20, 20}, {19, 55}} {
		starts = append(starts, time.Date(2026, 11, 10, hm[0]+3, hm[1], 0, 0, time.UTC))
	}
	if err := tick(rem, fakeDevices{"u1": 1}, fakeRules{}, s, fakeHabits{starts}).Run(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if len(rem.puts) != 1 || rem.puts[0].Slot != 81 || !rem.puts[0].Enabled || rem.puts[0].Kind != pokerpush.KindPlay {
		t.Errorf("puts = %+v, want the play reminder moved to slot 81", rem.puts)
	}
}

func TestRunKeepsPlaySlotWhenItDidNotMove(t *testing.T) {
	play := pushsvc.Reminder{Sub: "u1", Kind: pokerpush.KindPlay, Enabled: true, Slot: pokerpush.DefaultPlaySlot}
	play.Slot = 80
	rem, s := newReminders(play), &fakeSender{}
	tk := tick(rem, fakeDevices{"u1": 1}, fakeRules{}, s, fakeHabits{}) // no sessions: default slot 76, differs from 80
	_ = tk.Run(context.Background(), now)
	if len(rem.puts) != 1 || rem.puts[0].Slot != pokerpush.DefaultPlaySlot {
		t.Errorf("puts = %+v", rem.puts)
	}
	rem.puts = nil
	same := newReminders(pushsvc.Reminder{Sub: "u1", Kind: pokerpush.KindPlay, Enabled: true, Slot: 80})
	same.rows["u1/play"] = pushsvc.Reminder{Sub: "u1", Kind: pokerpush.KindPlay, Enabled: true, Slot: 80}
	starts := make([]time.Time, 5)
	for i := range starts {
		starts[i] = time.Date(2026, 11, 10, 23, 0, 0, 0, time.UTC) // 20:00 -> slot 80
	}
	_ = tick(same, fakeDevices{"u1": 1}, fakeRules{}, &fakeSender{}, fakeHabits{starts}).Run(context.Background(), now)
	if len(same.puts) != 0 {
		t.Errorf("puts = %+v: an unchanged slot is not rewritten", same.puts)
	}
}

func TestRunKeepsGoingAfterOneUserFails(t *testing.T) {
	rem, s := newReminders(streak("u1"), streak("u2")), &fakeSender{}
	if err := tick(rem, fakeDevices{"u1": 1, "u2": 1}, fakeRules{fail: map[string]bool{"u1": true}}, s, fakeHabits{}).Run(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if len(s.jobs) != 1 || s.jobs[0].Recipients[0] != "u2" {
		t.Errorf("jobs = %+v", s.jobs)
	}
}

// The player turns "come play" off while the tick is deciding: the learned-hour move must not switch it back on.
func TestRunNeverResurrectsAReminderThePlayerSwitchedOff(t *testing.T) {
	play := pushsvc.Reminder{Sub: "u1", Kind: pokerpush.KindPlay, Enabled: true, Slot: 80}
	rem := newReminders(play)
	s := &switchOffOnSend{fakeSender: &fakeSender{}, rem: rem}
	if err := NewTick(rem, fakeDevices{"u1": 1}, fakeRules{}, s, fakeHabits{}).Run(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if got := rem.rows["u1/play"]; got.Enabled {
		t.Errorf("reminder = %+v: the background move re-enabled a reminder the player switched off", got)
	}
}

// switchOffOnSend disables the reminder as the push is handed over, the way a PUT /preferences would land mid-tick.
type switchOffOnSend struct {
	*fakeSender
	rem *fakeReminders
}

func (s *switchOffOnSend) Send(ctx context.Context, j pushsvc.Job) error {
	s.rem.mu.Lock()
	r := s.rem.rows["u1/play"]
	r.Enabled = false
	s.rem.rows["u1/play"] = r
	s.rem.mu.Unlock()
	return s.fakeSender.Send(ctx, j)
}
