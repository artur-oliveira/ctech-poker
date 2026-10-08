package dailyreward

import (
	"context"
	"fmt"
	"log/slog"
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

// walletDescription is the statement text of the daily reward credit.
const walletDescription = "Recompensa diária"

type credit interface {
	Credit(context.Context, string, int64, string, string, string) error
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
	// Get reads one day's claim; the zero record when there is none.
	Get(context.Context, string, string) (DailyRewardRecord, error)
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
	// StreakAtRisk is true when the player holds a streak that today's claim
	// still continues — the state the client warns about. CurrentStreak is the
	// last claimed value either way, so read it together with StreakLost.
	StreakAtRisk bool `json:"streak_at_risk"`
	// StreakLost is true when the next claim restarts at day 1: one missed day
	// with no protection left, or two or more missed days.
	StreakLost bool `json:"streak_lost"`
	// ProtectionWillCover is true when exactly one day was missed and the
	// stored protection will absorb it on today's claim.
	ProtectionWillCover bool          `json:"protection_will_cover"`
	Days                []CalendarDay `json:"days"`
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
	retry := stored.LastClaimDay == day
	next := stored
	if !retry {
		next = advance(stored, day)
	}
	proposed := awardFor(next)

	record, err := s.store.Claim(ctx, playerID, day, proposed, next, now)
	if err != nil {
		return 0, 0, fmt.Errorf("dailyreward: claim spin: %w", err)
	}
	// Already paid today: nothing new is credited, and 0 is the caller's
	// "already claimed" signal. Not logged — it is not a claim.
	if record.Status == StatusCompleted {
		return 0, s.remTime(), nil
	}
	gap := dayGap(stored.LastClaimDay, day)
	slog.Info("daily reward claimed",
		"player", playerID, "day", day,
		"prev_streak", stored.CurrentStreak, "last_claim_day", stored.LastClaimDay, "gap", gap,
		"protection_available", stored.ProtectionAvailable,
		"protection_used", !retry && gap == 2 && stored.ProtectionAvailable,
		"new_streak", next.CurrentStreak,
		"reset", !retry && stored.CurrentStreak > 0 && next.CurrentStreak == 1,
		"amount", record.Amount, "retry", retry)

	idemKey := fmt.Sprintf("%s#daily_reward#%s", playerID, day)
	if err := s.wallet.Credit(ctx, playerID, record.Amount, idemKey, "daily_reward", walletDescription); err != nil {
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

	// advancedToday means the streak row already counts today; claimedToday
	// additionally needs the credit to have landed. An advanced-but-pending
	// claim (wallet failure) stays claimable so the client can retry it.
	advancedToday := stored.LastClaimDay == day
	claimedToday := advancedToday
	if advancedToday {
		record, err := s.store.Get(ctx, playerID, day)
		if err != nil {
			return Status{}, fmt.Errorf("dailyreward: load claim: %w", err)
		}
		claimedToday = record.Status == StatusCompleted
	}
	// The trail always shows the cycle the NEXT claim lands on, so an unclaimed
	// day is rendered as the pending slot rather than as yesterday's position.
	shown := stored
	if !advancedToday {
		shown = advance(stored, day)
	}
	cycleDay := CycleDayFor(shown.CurrentStreak)
	// A held streak that advance would restart is already gone, not at risk.
	lost := !advancedToday && stored.CurrentStreak > 0 && shown.CurrentStreak == 1

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
		StreakAtRisk:         !advancedToday && stored.CurrentStreak > 0 && !lost,
		StreakLost:           lost,
		ProtectionWillCover:  !advancedToday && stored.ProtectionAvailable && dayGap(stored.LastClaimDay, day) == 2,
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
