package dailyreward

import "time"

// The daily reward is a 30-day streak trail, not a random spin: day N of the
// current cycle always pays trail[N-1], so the calendar the client renders is
// the contract, not a preview of a weighted draw (#293). The trail restarts
// at day 1 once a cycle is completed, keeping a long streak rewarding
// forever instead of ending at a terminal prize.
const (
	// CycleLength is how many consecutive days one full trail takes.
	CycleLength = 30
	// ProtectionGrantEvery grants one streak protection every N consecutive
	// days. A protection absorbs exactly one missed day; without one, a gap
	// of any size resets the streak to day 1.
	ProtectionGrantEvery = 7
	// FirstAward is the one-off welcome award, paid instead of day 1's trail
	// value the very first time a player ever claims.
	FirstAward int64 = 100_000
)

// trail is the reward for each day of a cycle, in sandbox chips. The three
// weekly checkpoints (7, 14, 21) and the cycle's final day are milestones —
// the client renders them as chests, and day 30 is the 1,000,000-chip prize.
var trail = [CycleLength]int64{
	5_000, 7_500, 10_000, 12_500, 15_000, 20_000, 50_000, // 1-7
	25_000, 27_500, 30_000, 32_500, 35_000, 40_000, 100_000, // 8-14
	45_000, 50_000, 55_000, 60_000, 65_000, 70_000, 250_000, // 15-21
	80_000, 90_000, 100_000, 110_000, 120_000, 130_000, 140_000, 150_000, // 22-29
	1_000_000, // 30
}

// milestoneDays are the trail positions worth calling out visually. Derived
// from trail rather than hard-coded twice: a milestone is a day that pays
// more than the day after it.
func isMilestone(cycleDay int) bool {
	switch cycleDay {
	case 7, 14, 21, CycleLength:
		return true
	}
	return false
}

// CycleDayFor maps an absolute streak length to its position on the trail.
func CycleDayFor(streak int) int {
	if streak <= 0 {
		return 1
	}
	return ((streak - 1) % CycleLength) + 1
}

// RewardFor is the chip award for the streak's Nth consecutive day.
func RewardFor(streak int) int64 { return trail[CycleDayFor(streak)-1] }

// StreakRecord is the single per-player streak item (sk = streakSK). It is
// written inside the same transaction as the day's claim, so it can never
// advance without the day being claimed, nor the reverse.
type StreakRecord struct {
	CurrentStreak       int    `dynamodbav:"current_streak" json:"current_streak"`
	BestStreak          int    `dynamodbav:"best_streak" json:"best_streak"`
	LastClaimDay        string `dynamodbav:"last_claim_day,omitempty" json:"last_claim_day,omitempty"`
	ProtectionAvailable bool   `dynamodbav:"protection_available" json:"protection_available"`
	// ProtectionUsedDay records the day a protection covered, so the client
	// can render that slot as "saved by a shield" instead of a gap.
	ProtectionUsedDay string `dynamodbav:"protection_used_day,omitempty" json:"protection_used_day,omitempty"`
	TotalClaims       int    `dynamodbav:"total_claims" json:"total_claims"`
}

// advance returns what the streak becomes once day is claimed. Pure — the
// caller decides whether to persist it, which is what makes a retry of an
// already-claimed day safe (the streak is simply not recomputed).
func advance(record StreakRecord, day string) StreakRecord {
	gap := dayGap(record.LastClaimDay, day)
	switch {
	case record.LastClaimDay == "" || gap <= 0:
		record.CurrentStreak = 1
	case gap == 1:
		record.CurrentStreak++
	case gap == 2 && record.ProtectionAvailable:
		record.CurrentStreak++
		record.ProtectionAvailable = false
		record.ProtectionUsedDay = shiftDay(day, -1)
	default:
		record.CurrentStreak = 1
	}
	record.LastClaimDay = day
	record.TotalClaims++
	if record.CurrentStreak > record.BestStreak {
		record.BestStreak = record.CurrentStreak
	}
	if record.CurrentStreak%ProtectionGrantEvery == 0 {
		record.ProtectionAvailable = true
	}
	return record
}

// dayGap is how many calendar days separate two cooldown keys. 0 when either
// side is unparseable, which advance treats as "no usable history".
func dayGap(from, to string) int {
	if from == "" {
		return 0
	}
	fromTime, err := time.ParseInLocation("2006-01-02", from, brt)
	if err != nil {
		return 0
	}
	toTime, err := time.ParseInLocation("2006-01-02", to, brt)
	if err != nil {
		return 0
	}
	return int(toTime.Sub(fromTime).Hours() / 24)
}

func shiftDay(day string, days int) string {
	parsed, err := time.ParseInLocation("2006-01-02", day, brt)
	if err != nil {
		return ""
	}
	return parsed.AddDate(0, 0, days).Format("2006-01-02")
}
