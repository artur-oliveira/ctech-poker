package dailyreward

import (
	"context"
	"fmt"
	"time"
)

const (
	StatusPending   = "pending"
	StatusCompleted = "completed"
)

type DailyRewardRecord struct {
	Amount int64
	Status string
}

type credit interface {
	Credit(context.Context, string, int64, string, string) error
}

// spinStore persists the selected prize before the external wallet call. A
// retry therefore always uses the same amount and idempotency key.
type spinStore interface {
	// Claim writes the day's claim (create-only) and the player's streak
	// record in one transaction, so the streak can never advance without the
	// day being claimed. On a duplicate day it returns the stored claim.
	Claim(ctx context.Context, playerID, day string, proposed int64, streak StreakRecord, now time.Time) (DailyRewardRecord, error)
	Complete(context.Context, string, string, time.Time) error
	LoadStreak(context.Context, string) (StreakRecord, error)
}

type Service struct {
	wallet credit
	store  spinStore
	now    func() time.Time
}

func NewService(wallet credit, store spinStore) *Service {
	return &Service{wallet: wallet, store: store, now: time.Now}
}

// CalendarDay is one slot of the 30-day trail as the client renders it.
type CalendarDay struct {
	Day       int   `json:"day"`
	Amount    int64 `json:"amount"`
	Milestone bool  `json:"milestone"`
	Claimed   bool  `json:"claimed"`
	Today     bool  `json:"today"`
}

// Status is everything the daily-reward surface needs in one read: the
// cooldown clients already consumed, plus the streak and its calendar.
type Status struct {
	RemainingTimeSeconds int64  `json:"remaining_time_seconds"`
	CurrentStreak        int    `json:"current_streak"`
	BestStreak           int    `json:"best_streak"`
	TotalClaims          int    `json:"total_claims"`
	CycleDay             int    `json:"cycle_day"`
	CycleLength          int    `json:"cycle_length"`
	ProtectionAvailable  bool   `json:"protection_available"`
	ProtectionUsedDay    string `json:"protection_used_day,omitempty"`
	ClaimedToday         bool   `json:"claimed_today"`
	// StreakAtRisk is true when the player still holds a streak but has not
	// claimed today — the state the client warns about.
	StreakAtRisk bool          `json:"streak_at_risk"`
	Days         []CalendarDay `json:"days"`
}

func (s *Service) Spin(ctx context.Context, playerID string) (int64, int64, error) {
	if playerID == "" {
		return 0, 0, fmt.Errorf("dailyreward: empty player id")
	}
	now := s.now()
	day := cooldownKey(now)

	stored, err := s.store.LoadStreak(ctx, playerID)
	if err != nil {
		return 0, 0, fmt.Errorf("dailyreward: load streak: %w", err)
	}
	// Today already claimed → this is a retry of a claim whose wallet credit
	// or completion failed. Recomputing the streak would advance it twice, so
	// the stored record is passed through untouched; Claim's create-only
	// condition aborts the whole transaction anyway.
	next := stored
	if stored.LastClaimDay != day {
		next = advance(stored, day)
	}
	proposed := awardFor(next)

	record, err := s.store.Claim(ctx, playerID, day, proposed, next, now)
	if err != nil {
		return 0, 0, fmt.Errorf("dailyreward: claim spin: %w", err)
	}
	if record.Status == StatusCompleted {
		return record.Amount, 0, nil
	}

	idemKey := fmt.Sprintf("%s#daily_reward#%s", playerID, day)
	if err := s.wallet.Credit(ctx, playerID, record.Amount, idemKey, "daily_reward"); err != nil {
		return 0, 0, err
	}
	if err := s.store.Complete(ctx, playerID, day, now); err != nil {
		return 0, 0, fmt.Errorf("dailyreward: mark completed: %w", err)
	}
	return record.Amount, s.remTime(), nil
}

// awardFor is the trail value for the streak day being claimed, except for a
// player's very first claim ever, which pays the flat welcome award.
func awardFor(next StreakRecord) int64 {
	if next.TotalClaims <= 1 {
		return FirstAward
	}
	return RewardFor(next.CurrentStreak)
}

func (s *Service) RemainingTime(ctx context.Context, playerID string) (int64, error) {
	status, err := s.Status(ctx, playerID)
	if err != nil {
		return 0, err
	}
	return status.RemainingTimeSeconds, nil
}

// Status reads the streak item once and derives the whole calendar from it —
// the day claims themselves are TTL'd 48h rows and are never scanned.
func (s *Service) Status(ctx context.Context, playerID string) (Status, error) {
	if playerID == "" {
		return Status{}, fmt.Errorf("dailyreward: empty player id")
	}
	now := s.now()
	day := cooldownKey(now)
	stored, err := s.store.LoadStreak(ctx, playerID)
	if err != nil {
		return Status{}, fmt.Errorf("dailyreward: load streak: %w", err)
	}

	claimedToday := stored.LastClaimDay == day
	// The trail always shows the cycle the NEXT claim lands on, so an unclaimed
	// day is rendered as the pending slot rather than as yesterday's position.
	shown := stored
	if !claimedToday {
		shown = advance(stored, day)
	}
	cycleDay := CycleDayFor(shown.CurrentStreak)

	status := Status{
		RemainingTimeSeconds: 0,
		CurrentStreak:        stored.CurrentStreak,
		BestStreak:           stored.BestStreak,
		TotalClaims:          stored.TotalClaims,
		CycleDay:             cycleDay,
		CycleLength:          CycleLength,
		ProtectionAvailable:  stored.ProtectionAvailable,
		ProtectionUsedDay:    stored.ProtectionUsedDay,
		ClaimedToday:         claimedToday,
		StreakAtRisk:         !claimedToday && stored.CurrentStreak > 0,
		Days:                 make([]CalendarDay, 0, CycleLength),
	}
	if claimedToday {
		status.RemainingTimeSeconds = s.remTime()
	}
	for i := 1; i <= CycleLength; i++ {
		amount := trail[i-1]
		if i == 1 && shown.TotalClaims <= 1 {
			amount = FirstAward
		}
		status.Days = append(status.Days, CalendarDay{
			Day:       i,
			Amount:    amount,
			Milestone: isMilestone(i),
			Claimed:   i < cycleDay || (claimedToday && i == cycleDay),
			Today:     i == cycleDay,
		})
	}
	return status, nil
}

func (s *Service) remTime() int64 {
	now := s.now()
	nowBRT := now.In(brt)
	tomorrow := time.Date(nowBRT.Year(), nowBRT.Month(), nowBRT.Day()+1, 0, 0, 0, 0, brt)
	return int64(tomorrow.Sub(nowBRT).Seconds())
}
