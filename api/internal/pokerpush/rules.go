package pokerpush

import (
	"context"
	"fmt"
	"slices"
	"time"

	"gopkg.aoctech.app/api-commons/pushsvc"
	"gopkg.aoctech.app/poker/api/internal/dailyreward"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
)

const (
	// minSessionsToLearn is how many sessions it takes before "come play" moves off the default hour.
	minSessionsToLearn = 5
	// sessionsToLearnFrom is how many of the newest sessions the typical hour is taken from.
	sessionsToLearnFrom = 14
)

// StatusReader is the slice of dailyreward.Service the rules use.
type StatusReader interface {
	Status(ctx context.Context, playerID string) (dailyreward.Status, error)
}

// SessionReader is what the rules need of the session log.
type SessionReader interface {
	RecentStarts(ctx context.Context, playerID string, n int) ([]time.Time, error)
	PlayedSince(ctx context.Context, playerID string, since time.Time) (bool, error)
}

// SessionLog adapts *sessionlog.Store to SessionReader (PlayedSince is the store's own).
type SessionLog struct{ *sessionlog.Store }

// RecentStarts returns the start times of the player's n newest sessions, newest first.
func (s SessionLog) RecentStarts(ctx context.Context, playerID string, n int) ([]time.Time, error) {
	items, _, err := s.ListSessions(ctx, playerID, n, nil)
	if err != nil {
		return nil, err
	}
	out := make([]time.Time, 0, len(items))
	for _, it := range items {
		out = append(out, time.UnixMilli(it.JoinedAt))
	}
	return out, nil
}

// Rules decides, for one due reminder, whether a push goes out and what it says.
type Rules struct {
	status   StatusReader
	sessions SessionReader
	prefs    Reminders
}

// NewRules returns the Poker reminder rules.
func NewRules(status StatusReader, sessions SessionReader, prefs Reminders) *Rules {
	return &Rules{status: status, sessions: sessions, prefs: prefs}
}

// Decide returns the push for a due reminder, or nil when nothing should go out. Texts never carry chip amounts: a lock
// screen shows them.
//
//   - streak: the player holds a streak that today's claim still continues (not lost, not yet claimed).
//   - play: the player has not played today, and the streak reminder is not going to speak for today (it is off, or there
//     is nothing at risk). The streak reminder takes priority regardless of which hour comes first.
func (r *Rules) Decide(ctx context.Context, now time.Time, rem pushsvc.Reminder) (*pushsvc.Job, error) {
	switch rem.Kind {
	case KindStreak:
		st, err := r.status.Status(ctx, rem.Sub)
		if err != nil || !st.StreakAtRisk {
			return nil, err
		}
		unit := "dias"
		if st.CurrentStreak == 1 {
			unit = "dia"
		}
		return r.job(now, rem.Sub, KindStreak, fmt.Sprintf("Sua ofensiva de %d %s termina hoje", st.CurrentStreak, unit),
			"Resgate o bônus diário para mantê-la."), nil
	case KindPlay:
		if streak, ok, err := r.prefs.Get(ctx, rem.Sub, KindStreak); err != nil {
			return nil, err
		} else if ok && streak.Enabled {
			st, err := r.status.Status(ctx, rem.Sub)
			if err != nil || st.StreakAtRisk {
				return nil, err
			}
		}
		played, err := r.sessions.PlayedSince(ctx, rem.Sub, pushsvc.StartOfDay(now))
		if err != nil || played {
			return nil, err
		}
		return r.job(now, rem.Sub, KindPlay, "A mesa está esperando", "Venha jogar hoje."), nil
	}
	return nil, nil
}

func (r *Rules) job(now time.Time, sub, kind, title, body string) *pushsvc.Job {
	day := pushsvc.Today(now)
	return &pushsvc.Job{
		EventID: kind + ":" + sub + ":" + day, Type: kind, Title: title, Body: body, Navigate: "/", Tag: kind + "-" + day,
		Topic: kind, TTLSeconds: max(60, int(pushsvc.UntilMidnight(now).Seconds())), Urgency: "normal", Recipients: []string{sub},
	}
}

// TypicalSlot is the 15-minute slot of the median start time of a player's sessions (newest first, at most 14 are
// read), or DefaultPlaySlot when there are too few to say.
//
// ponytail: a plain median of the minute of the day; a player whose sessions straddle midnight gets a meaningless hour,
// use a circular mean if that shows up.
func TypicalSlot(starts []time.Time) int {
	if len(starts) < minSessionsToLearn {
		return DefaultPlaySlot
	}
	starts = starts[:min(len(starts), sessionsToLearnFrom)]
	minutes := make([]int, len(starts))
	for i, s := range starts {
		minutes[i] = int(s.Sub(pushsvc.StartOfDay(s)).Minutes())
	}
	slices.Sort(minutes)
	return minutes[len(minutes)/2] / 15
}
