// Package pokerpush holds Poker's scheduled Web Push rules: a streak reminder at a time the player picks and a "come play
// today" push at the hour they usually play. Subscriptions, slots, reminders and the sender are go-common's pushsvc; this
// package decides who gets what and when (docs/specs/2026-11-18-scheduled-web-push.md).
package pokerpush

import (
	"context"

	"gopkg.aoctech.app/api-commons/problem"
	"gopkg.aoctech.app/api-commons/pushsvc"
)

const (
	// KindStreak and KindPlay are the reminder kinds of a player.
	KindStreak = "streak"
	KindPlay   = "play"
	// DefaultPlaySlot is 19:00, where "come play" goes until there are enough sessions to learn the player's hour.
	DefaultPlaySlot = 76
)

// StreakPreference is the streak reminder: on or off, and the "HH:MM" (São Paulo) the player chose.
type StreakPreference struct {
	Enabled bool   `json:"enabled"`
	Time    string `json:"time"`
}

// PlayPreference is the "come play today" push; its hour is learned, not chosen.
type PlayPreference struct {
	Enabled bool `json:"enabled"`
}

// Preferences are a player's two reminders.
type Preferences struct {
	StreakReminder StreakPreference `json:"streak_reminder"`
	ComePlay       PlayPreference   `json:"come_play"`
}

// Reminders is what Service needs of pushsvc.ReminderStore.
type Reminders interface {
	Put(ctx context.Context, r pushsvc.Reminder) error
	Get(ctx context.Context, sub, kind string) (pushsvc.Reminder, bool, error)
}

// Service reads and writes a player's reminder preferences.
type Service struct{ reminders Reminders }

// NewService returns a Service over the reminder store.
func NewService(r Reminders) *Service { return &Service{reminders: r} }

// Get returns the player's preferences; a reminder never set reads as off.
func (s *Service) Get(ctx context.Context, sub string) (Preferences, error) {
	var p Preferences
	streak, ok, err := s.reminders.Get(ctx, sub, KindStreak)
	if err != nil {
		return p, err
	}
	if ok {
		p.StreakReminder = StreakPreference{Enabled: streak.Enabled, Time: pushsvc.FormatSlot(streak.Slot)}
	}
	play, ok, err := s.reminders.Get(ctx, sub, KindPlay)
	if err != nil {
		return p, err
	}
	p.ComePlay.Enabled = ok && play.Enabled
	return p, nil
}

// Set stores the preferences. Turning the streak reminder on needs a valid time; turning it off keeps the time, and
// toggling "come play" keeps the slot learned from the player's sessions.
func (s *Service) Set(ctx context.Context, sub string, p Preferences) (Preferences, error) {
	streak, hadStreak, err := s.reminders.Get(ctx, sub, KindStreak)
	if err != nil {
		return Preferences{}, err
	}
	streakSlot := streak.Slot
	if p.StreakReminder.Time != "" || p.StreakReminder.Enabled {
		slot, perr := pushsvc.ParseHHMM(p.StreakReminder.Time)
		if perr != nil {
			return Preferences{}, problem.BadRequest("informe o horário do lembrete no formato HH:MM")
		}
		streakSlot = slot
	}
	if p.StreakReminder.Enabled || hadStreak {
		if err := s.reminders.Put(ctx, pushsvc.Reminder{Sub: sub, Kind: KindStreak, Enabled: p.StreakReminder.Enabled, Slot: streakSlot}); err != nil {
			return Preferences{}, err
		}
	}
	play, hadPlay, err := s.reminders.Get(ctx, sub, KindPlay)
	if err != nil {
		return Preferences{}, err
	}
	playSlot := DefaultPlaySlot
	if hadPlay {
		playSlot = play.Slot
	}
	if p.ComePlay.Enabled || hadPlay {
		if err := s.reminders.Put(ctx, pushsvc.Reminder{Sub: sub, Kind: KindPlay, Enabled: p.ComePlay.Enabled, Slot: playSlot}); err != nil {
			return Preferences{}, err
		}
	}
	return s.Get(ctx, sub)
}

// SeedOnSubscribe turns "come play" on, at the default hour, the first time a device is registered. It keeps whatever the
// player already chose (a deliberate off stays off) and leaves the streak reminder unset until they pick a time. It is
// safe to run on every device registration.
func (s *Service) SeedOnSubscribe(ctx context.Context, sub string) error {
	if _, ok, err := s.reminders.Get(ctx, sub, KindPlay); err != nil || ok {
		return err
	}
	return s.reminders.Put(ctx, pushsvc.Reminder{Sub: sub, Kind: KindPlay, Enabled: true, Slot: DefaultPlaySlot})
}

// TablePrefix is the prefix Poker gives pushsvc so its table is `{env}_poker_push`, like every other Poker table
// (`{env}_poker_*`); a bare `{env}_push` could collide with another service's table in the same account.
func TablePrefix(env string) string { return env + "_poker" }
