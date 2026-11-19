package pokerpush

import (
	"context"
	"strings"
	"sync"
	"testing"

	"gopkg.aoctech.app/api-commons/pushsvc"
)

type memReminders struct {
	mu   sync.Mutex
	rows map[string]pushsvc.Reminder
}

func newMem() *memReminders { return &memReminders{rows: map[string]pushsvc.Reminder{}} }

func (m *memReminders) Put(_ context.Context, r pushsvc.Reminder) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if old, ok := m.rows[r.Sub+"/"+r.Kind]; ok {
		r.LastSentDay = old.LastSentDay // the real store keeps it
	}
	m.rows[r.Sub+"/"+r.Kind] = r
	return nil
}

func (m *memReminders) Get(_ context.Context, sub, kind string) (pushsvc.Reminder, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[sub+"/"+kind]
	return r, ok, nil
}

func prefs(streakOn bool, streakTime string, playOn bool) Preferences {
	var p Preferences
	p.StreakReminder.Enabled, p.StreakReminder.Time, p.ComePlay.Enabled = streakOn, streakTime, playOn
	return p
}

func TestSetStreakRequiresTime(t *testing.T) {
	svc := NewService(newMem())
	_, err := svc.Set(context.Background(), "u1", prefs(true, "", true))
	if err == nil || !strings.Contains(err.Error(), "horário") {
		t.Fatalf("err = %v, want a bad-request about the time", err)
	}
}

func TestSetRejectsBadTime(t *testing.T) {
	svc := NewService(newMem())
	for _, bad := range []string{"24:00", "7:5", "noon"} {
		if _, err := svc.Set(context.Background(), "u1", prefs(true, bad, false)); err == nil {
			t.Errorf("time %q accepted", bad)
		}
	}
}

func TestSetThenGetRoundTrips(t *testing.T) {
	svc := NewService(newMem())
	got, err := svc.Set(context.Background(), "u1", prefs(true, "20:30", true))
	if err != nil || !got.StreakReminder.Enabled || got.StreakReminder.Time != "20:30" || !got.ComePlay.Enabled {
		t.Fatalf("Set = %+v, %v", got, err)
	}
	back, err := svc.Get(context.Background(), "u1")
	if err != nil || back != got {
		t.Errorf("Get = %+v, %v; want %+v", back, err, got)
	}
}

func TestDisablingStreakKeepsItsTime(t *testing.T) {
	svc := NewService(newMem())
	_, _ = svc.Set(context.Background(), "u1", prefs(true, "20:30", true))
	got, _ := svc.Set(context.Background(), "u1", prefs(false, "", true))
	if got.StreakReminder.Enabled || got.StreakReminder.Time != "20:30" {
		t.Errorf("got %+v; turning it off must keep the chosen time for when it is turned on again", got)
	}
}

func TestTogglingComePlayKeepsTheLearnedSlot(t *testing.T) {
	mem := newMem()
	svc := NewService(mem)
	_ = mem.Put(context.Background(), pushsvc.Reminder{Sub: "u1", Kind: KindPlay, Enabled: true, Slot: 81})
	_, _ = svc.Set(context.Background(), "u1", prefs(false, "", false))
	_, _ = svc.Set(context.Background(), "u1", prefs(false, "", true))
	if r, _, _ := mem.Get(context.Background(), "u1", KindPlay); r.Slot != 81 || !r.Enabled {
		t.Errorf("play reminder = %+v, want slot 81 enabled", r)
	}
}

func TestSeedOnSubscribeEnablesPlayOnly(t *testing.T) {
	mem := newMem()
	svc := NewService(mem)
	if err := svc.SeedOnSubscribe(context.Background(), "u1"); err != nil {
		t.Fatal(err)
	}
	play, ok, _ := mem.Get(context.Background(), "u1", KindPlay)
	if !ok || !play.Enabled || play.Slot != DefaultPlaySlot {
		t.Errorf("play = %+v, %v", play, ok)
	}
	if _, ok, _ := mem.Get(context.Background(), "u1", KindStreak); ok {
		t.Error("streak reminder must stay unset until the person picks a time")
	}
}

func TestSeedOnSubscribeKeepsExistingChoice(t *testing.T) {
	mem := newMem()
	svc := NewService(mem)
	_ = mem.Put(context.Background(), pushsvc.Reminder{Sub: "u1", Kind: KindPlay, Enabled: false, Slot: 81}) // the person turned it off
	_ = svc.SeedOnSubscribe(context.Background(), "u1")
	if r, _, _ := mem.Get(context.Background(), "u1", KindPlay); r.Enabled || r.Slot != 81 {
		t.Errorf("play = %+v; a new device must not switch a deliberate off back on", r)
	}
}
