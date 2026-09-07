package player

import "time"

// Profile milestones are longevity/volume marks, deliberately distinct from
// the `achievements` catalog, which is entirely skill- and hand-event-based
// (#330). Every one of them is DERIVED at read time from counters that
// already exist — account age, the `hands_played` aggregate `achievements`
// has materialized since #198, and the caller's current leaderboard rank —
// so nothing here adds a write to the per-hand pipeline (#204's budget) and
// no new DynamoDB table or GSI exists to back them.
const (
	MilestoneVeteran1y = "veteran_1y"
	MilestoneVeteran3y = "veteran_3y"
	MilestoneHands1k   = "hands_1k"
	MilestoneHands10k  = "hands_10k"
	MilestoneHands100k = "hands_100k"
	MilestoneTop100    = "top100"
	MilestoneTop10     = "top10"
)

// Milestone categories, so a client can group the marks without hard-coding
// which key belongs where.
const (
	MilestoneCategoryTenure  = "tenure"
	MilestoneCategoryVolume  = "volume"
	MilestoneCategoryRanking = "ranking"
)

// Milestone is one earned mark. Value carries the figure the mark was earned
// with — days for tenure, hands for volume, the rank itself for ranking — so
// the client can render "1 ano de conta" next to the real number without a
// second lookup.
type Milestone struct {
	Key      string `json:"key"`
	Category string `json:"category"`
	Value    int64  `json:"value"`
}

// MilestoneInput is everything the marks are computed from. Rank is 0 when
// the player is unranked (never played that mode), which earns no ranking
// mark — a missing rank is not rank 1.
type MilestoneInput struct {
	CreatedAt   time.Time
	Now         time.Time
	HandsPlayed int
	Rank        int64
}

// Milestones returns the marks this player has earned, best-first within each
// category, and never returns nil (an empty slice serialises as `[]`, which
// is what a client renders as "no marks yet").
//
// The ranking marks are the player's CURRENT rank, not their peak. A peak
// would need a durable high-water mark written whenever a rank improves, and
// there is no cheap place to put that write: the leaderboard row is updated
// by the per-hand pipeline, which #198/#217 spent real effort taking writes
// *out* of. Current rank reuses `leaderboard.Service.MyRank`, which answers
// from the Valkey rank mirror (#202) and falls back to the existing
// `gsi_hands_won` COUNT — no new index either way.
func Milestones(in MilestoneInput) []Milestone {
	marks := make([]Milestone, 0, 4)

	if !in.CreatedAt.IsZero() && !in.Now.Before(in.CreatedAt) {
		days := int64(in.Now.Sub(in.CreatedAt).Hours() / 24)
		switch {
		case days >= 3*365:
			marks = append(marks, Milestone{MilestoneVeteran3y, MilestoneCategoryTenure, days})
		case days >= 365:
			marks = append(marks, Milestone{MilestoneVeteran1y, MilestoneCategoryTenure, days})
		}
	}

	hands := int64(in.HandsPlayed)
	switch {
	case hands >= 100_000:
		marks = append(marks, Milestone{MilestoneHands100k, MilestoneCategoryVolume, hands})
	case hands >= 10_000:
		marks = append(marks, Milestone{MilestoneHands10k, MilestoneCategoryVolume, hands})
	case hands >= 1_000:
		marks = append(marks, Milestone{MilestoneHands1k, MilestoneCategoryVolume, hands})
	}

	switch {
	case in.Rank >= 1 && in.Rank <= 10:
		marks = append(marks, Milestone{MilestoneTop10, MilestoneCategoryRanking, in.Rank})
	case in.Rank >= 1 && in.Rank <= 100:
		marks = append(marks, Milestone{MilestoneTop100, MilestoneCategoryRanking, in.Rank})
	}

	return marks
}
