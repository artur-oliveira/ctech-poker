package pokerpush

import (
	"context"
	"strings"
	"testing"
	"time"

	"gopkg.aoctech.app/api-commons/pushsvc"
	"gopkg.aoctech.app/poker/api/internal/dailyreward"
)

type fakeStatus struct{ st dailyreward.Status }

func (f fakeStatus) Status(context.Context, string) (dailyreward.Status, error) { return f.st, nil }

type fakeSessions struct {
	starts []time.Time
	played bool
}

func (f fakeSessions) RecentStarts(context.Context, string, int) ([]time.Time, error) {
	return f.starts, nil
}
func (f fakeSessions) PlayedSince(context.Context, string, time.Time) (bool, error) {
	return f.played, nil
}

// 20:00 in São Paulo on 2026-11-18.
var evening = time.Date(2026, 11, 18, 23, 0, 0, 0, time.UTC)

func atRisk(n int) dailyreward.Status {
	return dailyreward.Status{CurrentStreak: n, StreakAtRisk: true}
}

func rules(t *testing.T, st dailyreward.Status, s fakeSessions, streakOn bool) *Rules {
	t.Helper()
	mem := newMem()
	if streakOn {
		_ = mem.Put(context.Background(), pushsvc.Reminder{Sub: "u1", Kind: KindStreak, Enabled: true, Slot: 80})
	}
	return NewRules(fakeStatus{st}, s, mem)
}

func decide(t *testing.T, r *Rules, kind string) *pushsvc.Job {
	t.Helper()
	job, err := r.Decide(context.Background(), evening, pushsvc.Reminder{Sub: "u1", Kind: kind, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func TestStreakSendsWhenAtRisk(t *testing.T) {
	job := decide(t, rules(t, atRisk(5), fakeSessions{}, true), KindStreak)
	if job == nil || !strings.Contains(job.Title, "5 dias") || len(job.Recipients) != 1 || job.Recipients[0] != "u1" {
		t.Fatalf("job = %+v", job)
	}
	if job.Navigate != "/" || job.EventID != "streak:u1:2026-11-18" || job.TTLSeconds <= 0 || job.TTLSeconds > 4*3600 {
		t.Errorf("job = %+v (ttl is what is left of the day: 4h at 20:00)", job)
	}
}

func TestStreakSingularDay(t *testing.T) {
	if job := decide(t, rules(t, atRisk(1), fakeSessions{}, true), KindStreak); job == nil || !strings.Contains(job.Title, "1 dia ") {
		t.Errorf("job = %+v", job)
	}
}

func TestStreakSkipsWhenClaimedToday(t *testing.T) {
	st := dailyreward.Status{CurrentStreak: 5, ClaimedToday: true}
	if job := decide(t, rules(t, st, fakeSessions{}, true), KindStreak); job != nil {
		t.Errorf("job = %+v", job)
	}
}

func TestStreakSkipsWhenNoStreak(t *testing.T) {
	if job := decide(t, rules(t, dailyreward.Status{}, fakeSessions{}, true), KindStreak); job != nil {
		t.Errorf("job = %+v", job)
	}
}

func TestStreakSkipsWhenStreakLost(t *testing.T) {
	st := dailyreward.Status{CurrentStreak: 9, StreakLost: true}
	if job := decide(t, rules(t, st, fakeSessions{}, true), KindStreak); job != nil {
		t.Errorf("a lost streak must not be told it ends today: %+v", job)
	}
}

func TestPlaySkipsWhenPlayedToday(t *testing.T) {
	if job := decide(t, rules(t, dailyreward.Status{}, fakeSessions{played: true}, false), KindPlay); job != nil {
		t.Errorf("job = %+v", job)
	}
}

func TestPlayYieldsToStreakWhenStreakDue(t *testing.T) {
	if job := decide(t, rules(t, atRisk(5), fakeSessions{}, true), KindPlay); job != nil {
		t.Errorf("the streak reminder goes instead: %+v", job)
	}
}

func TestPlaySendsWhenStreakReminderIsOff(t *testing.T) {
	job := decide(t, rules(t, atRisk(5), fakeSessions{}, false), KindPlay)
	if job == nil || job.Tag != "play-2026-11-18" || job.EventID != "play:u1:2026-11-18" {
		t.Errorf("job = %+v", job)
	}
}

func TestPlaySendsWhenStreakIsAlreadyClaimedEvenIfReminderIsOn(t *testing.T) {
	st := dailyreward.Status{CurrentStreak: 5, ClaimedToday: true}
	if job := decide(t, rules(t, st, fakeSessions{}, true), KindPlay); job == nil {
		t.Error("nothing is due for the streak, so come-play goes")
	}
}

func TestUnknownKindSendsNothing(t *testing.T) {
	if job := decide(t, rules(t, atRisk(5), fakeSessions{}, true), "other"); job != nil {
		t.Errorf("job = %+v", job)
	}
}

func TestTypicalSlotMedian(t *testing.T) {
	var starts []time.Time
	for _, hm := range [][2]int{{20, 10}, {20, 40}, {21, 5}, {20, 20}, {19, 55}} {
		starts = append(starts, time.Date(2026, 11, 10, hm[0]+3, hm[1], 0, 0, time.UTC)) // BRT = UTC-3
	}
	if got := TypicalSlot(starts); got != 81 { // median 20:20 -> slot 81
		t.Errorf("TypicalSlot = %d, want 81", got)
	}
}

func TestTypicalSlotDefaultsBelowFiveSessions(t *testing.T) {
	if got := TypicalSlot(nil); got != DefaultPlaySlot {
		t.Errorf("empty = %d", got)
	}
	four := make([]time.Time, 4)
	for i := range four {
		four[i] = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	}
	if got := TypicalSlot(four); got != DefaultPlaySlot {
		t.Errorf("four = %d", got)
	}
}

func TestTextsCarryNoChipAmounts(t *testing.T) {
	jobs := map[string]*pushsvc.Job{
		KindStreak: decide(t, rules(t, atRisk(30), fakeSessions{}, true), KindStreak),
		KindPlay:   decide(t, rules(t, atRisk(30), fakeSessions{}, false), KindPlay),
	}
	for kind, job := range jobs {
		if job == nil {
			t.Fatalf("%s: no job", kind)
		}
		for _, text := range []string{job.Title, job.Body} {
			if strings.Contains(strings.ToLower(text), "ficha") || strings.Contains(text, "1.000.000") {
				t.Errorf("%s text names an amount: %q", kind, text)
			}
		}
	}
	if play := jobs[KindPlay]; strings.ContainsAny(play.Title+play.Body, "0123456789") {
		t.Errorf("the come-play text has no numbers: %q %q", play.Title, play.Body)
	}
}
