// Package achievements implements data-driven achievement progress and tiers.
package achievements

import "fmt"

type Tier struct {
	Stars     int `json:"stars"`
	Threshold int `json:"threshold"`
}

type Achievement struct {
	Key    string `json:"key"`
	Metric string `json:"metric"`
	Tiers  []Tier `json:"tiers"`
	Secret bool   `json:"secret,omitempty"`
}

const (
	KeyWins                      = "wins"
	KeyHandsPlayed               = "hands_played"
	KeyComeback                  = "comeback"
	KeyBluff                     = "bluff"
	KeySurvivor                  = "survivor"
	KeyLooser                    = "looser"
	KeyFallenKing                = "fallen_king"
	KeyAlmostWinner              = "almost_winner"
	KeyTied                      = "tied"
	KeyBadBeat                   = "bad_beat"
	KeyCooler                    = "cooler"
	KeyCrackedAces               = "cracked_aces"
	KeyGiantSlayer               = "giant_slayer"
	KeyShowdownWarrior           = "showdown_warrior"
	KeyAllIn                     = "all_in"
	KeyRealMoneyEarned           = "real_money_earned"
	KeySandboxChipsEarned        = "sandbox_chips_earned"
	KeyWonWithPocketPair         = "won_with_pocket_pair"
	KeyWonFullTable              = "won_full_table"
	KeyWonHeadsUp                = "won_heads_up"
	KeyLostStraightFlushToRoyal  = "lost_straight_flush_to_royal"
	KeyFirstHandAllInWin         = "first_hand_allin_win"
	KeyBeatPocketAces            = "beat_pocket_aces"
	KeyBeatTripsOrBetter         = "beat_trips_or_better"
	KeyThreeBetWonNoShowdown     = "three_bet_won_no_showdown"
	KeyFoldedStreak              = "folded_streak"
	KeyFourToRoyalMissed         = "four_to_royal_missed"
	KeyFourToStraightFlushMissed = "four_to_straight_flush_missed"
	KeyPaidRiverDrawMissed       = "paid_river_draw_missed"
	KeyLostRiverAfterLeadingTurn = "lost_river_after_leading_turn"
	KeyWonRunnerRunner           = "won_runner_runner"
	KeyWonWithNuts               = "won_with_nuts"
	KeySamePocketPairStreak      = "same_pocket_pair_streak"
	KeyAllInBlind                = "all_in_blind"
	KeyBlindMagic                = "blind_magic"
	KeyNoRush                    = "no_rush"
)

func KeyWinByCategory(category string) string { return fmt.Sprintf("win_category_%s", category) }

var commonTiers = []Tier{{1, 1}, {2, 10}, {3, 100}, {4, 1000}, {5, 10000}}

var Catalog = []Achievement{
	{
		Key:    KeyWins,
		Metric: "hand_won",
		Tiers:  commonTiers,
	},
	{
		Key:    KeyHandsPlayed,
		Metric: "hand_played",
		Tiers:  []Tier{{1, 100}, {2, 1000}, {3, 5000}, {4, 10000}, {5, 100000}}},
	{
		Key:    KeyComeback,
		Metric: "won_after_all_in",
		Tiers:  []Tier{{1, 1}, {2, 5}, {3, 25}, {4, 100}, {5, 500}}},
	{
		Key:    KeyBluff,
		Metric: "won_without_showdown_weaker_hand",
		Tiers:  []Tier{{1, 1}, {2, 5}, {3, 25}, {4, 100}, {5, 500}}},
	{
		Key:    KeySurvivor,
		Metric: "hands_without_leaving",
		Tiers:  []Tier{{1, 50}, {2, 250}, {3, 1000}, {4, 5000}, {5, 25000}}},
	{
		Key:    KeyLooser,
		Metric: "hand_lost_at_showdown",
		Tiers:  commonTiers,
	},
	{
		Key:    KeyAlmostWinner,
		Metric: "hand_lost_same_category_as_winner",
		Tiers:  commonTiers,
	},
	{
		Key:    KeyTied,
		Metric: "hand_tied",
		Tiers:  commonTiers,
	},
	{
		Key:    KeyBadBeat,
		Metric: "hand_lost_with_trips_or_better",
		Tiers:  []Tier{{1, 1}, {2, 5}, {3, 25}, {4, 100}, {5, 500}},
	},
	{
		Key:    KeyCooler,
		Metric: "hand_lost_with_full_house_or_better",
		Tiers:  []Tier{{1, 1}, {2, 5}, {3, 25}, {4, 100}, {5, 500}},
	},
	{
		Key:    KeyCrackedAces,
		Metric: "pocket_aces_lost",
		Tiers:  []Tier{{1, 1}, {2, 5}, {3, 25}, {4, 100}, {5, 500}},
	},
	{
		Key:    KeyFallenKing,
		Metric: "pocket_kings_lost",
		Tiers:  []Tier{{1, 1}, {2, 5}, {3, 25}, {4, 100}, {5, 500}},
	},
	{
		Key:    KeyGiantSlayer,
		Metric: "won_allin_vs_bigger_stack",
		Tiers:  []Tier{{1, 1}, {2, 5}, {3, 25}, {4, 100}, {5, 500}},
	},
	{
		Key:    KeyShowdownWarrior,
		Metric: "reached_showdown",
		Tiers:  commonTiers,
	},
	{
		Key:    KeyAllIn,
		Metric: "went_all_in",
		Tiers:  commonTiers,
	},
	{Key: KeyRealMoneyEarned, Metric: "real_money_won", Tiers: []Tier{{1, 100}, {2, 1000}, {3, 10000}, {4, 50000}, {5, 100000}}},
	{Key: KeySandboxChipsEarned, Metric: "sandbox_chips_won", Tiers: []Tier{{1, 10000}, {2, 100000}, {3, 1000000}, {4, 500000000}, {5, 1000000000}}},
	{Key: KeyWonWithPocketPair, Metric: "hand_won_with_pocket_pair", Tiers: []Tier{{1, 1}, {2, 10}, {3, 50}, {4, 100}, {5, 500}}},
	{Key: KeyWonFullTable, Metric: "hand_won_full_table", Tiers: []Tier{{1, 1}, {2, 5}, {3, 10}, {4, 25}, {5, 50}}},
	{Key: KeyWonHeadsUp, Metric: "hand_won_heads_up", Tiers: []Tier{{1, 10}, {2, 50}, {3, 100}, {4, 500}}},
	{Key: KeyLostStraightFlushToRoyal, Metric: "hand_lost_straight_flush_to_royal", Tiers: []Tier{{1, 1}, {2, 2}, {3, 5}}, Secret: true},
	{Key: KeyFirstHandAllInWin, Metric: "first_hand_won_allin", Tiers: []Tier{{1, 1}}, Secret: true},
	{Key: KeyBeatPocketAces, Metric: "beat_opponent_pocket_aces", Tiers: []Tier{{1, 1}, {2, 5}, {3, 25}, {4, 100}, {5, 500}}},
	{Key: KeyBeatTripsOrBetter, Metric: "beat_opponent_trips_or_better", Tiers: []Tier{{1, 1}, {2, 5}, {3, 25}, {4, 100}, {5, 500}}},
	{Key: KeyThreeBetWonNoShowdown, Metric: "three_bet_win_no_showdown", Tiers: []Tier{{1, 5}, {2, 25}, {3, 100}, {4, 500}}},
	{Key: KeyFoldedStreak, Metric: "consecutive_hands_no_vpip", Tiers: []Tier{{1, 100}, {2, 500}, {3, 1000}}},
	{Key: KeyFourToRoyalMissed, Metric: "near_miss_royal_flush", Tiers: []Tier{{1, 5}, {2, 10}, {3, 25}, {4, 50}}},
	{Key: KeyFourToStraightFlushMissed, Metric: "near_miss_straight_flush", Tiers: []Tier{{1, 10}, {2, 50}, {3, 100}, {4, 500}}},
	{Key: KeyPaidRiverDrawMissed, Metric: "river_draw_missed", Tiers: []Tier{{1, 10}, {2, 50}, {3, 100}}},
	{Key: KeyLostRiverAfterLeadingTurn, Metric: "lost_river_after_leading_turn", Tiers: []Tier{{1, 5}, {2, 25}, {3, 100}, {4, 500}}},
	{Key: KeyWonRunnerRunner, Metric: "won_runner_runner", Tiers: []Tier{{1, 1}, {2, 5}, {3, 10}, {4, 25}}},
	{Key: KeyWonWithNuts, Metric: "hand_won_with_nuts", Tiers: []Tier{{1, 1}, {2, 5}, {3, 25}, {4, 100}, {5, 500}}},
	{Key: KeySamePocketPairStreak, Metric: "same_pocket_pair_win_streak", Tiers: []Tier{{1, 3}}, Secret: true},
	{Key: KeyAllInBlind, Metric: "went_all_in_without_peeking", Tiers: []Tier{{1, 1}, {2, 5}, {3, 25}, {4, 100}, {5, 500}}},
	// Thresholds are MILLISECONDS of consumed time bank: 1 minute, 1 hour,
	// 1 day, 1 week, 30 days. The frontend renders them as those durations.
	{Key: KeyNoRush, Metric: "time_bank_ms_consumed", Tiers: []Tier{
		{1, 60_000}, {2, 3_600_000}, {3, 86_400_000}, {4, 604_800_000}, {5, 2_592_000_000}}},
	{Key: KeyBlindMagic, Metric: "hand_won_without_peeking", Tiers: []Tier{{1, 1}, {2, 5}, {3, 25}, {4, 100}, {5, 500}}},
	{Key: KeyWinByCategory("royal_flush"), Metric: "hand_won_with_category", Tiers: []Tier{{1, 1}, {2, 2}, {3, 3}, {4, 5}, {5, 10}}},
	{Key: KeyWinByCategory("straight_flush"), Metric: "hand_won_with_category", Tiers: []Tier{{1, 1}, {2, 5}, {3, 10}, {4, 25}, {5, 50}}},
	{Key: KeyWinByCategory("four_of_a_kind"), Metric: "hand_won_with_category", Tiers: []Tier{{1, 1}, {2, 25}, {3, 50}, {4, 100}, {5, 500}}},
	{Key: KeyWinByCategory("full_house"), Metric: "hand_won_with_category", Tiers: commonTiers},
	{Key: KeyWinByCategory("flush"), Metric: "hand_won_with_category", Tiers: commonTiers},
	{Key: KeyWinByCategory("straight"), Metric: "hand_won_with_category", Tiers: commonTiers},
	{Key: KeyWinByCategory("three_of_a_kind"), Metric: "hand_won_with_category", Tiers: commonTiers},
	{Key: KeyWinByCategory("two_pair"), Metric: "hand_won_with_category", Tiers: commonTiers},
	{Key: KeyWinByCategory("pair"), Metric: "hand_won_with_category", Tiers: commonTiers},
	{Key: KeyWinByCategory("high_card"), Metric: "hand_won_with_category", Tiers: commonTiers},
}

// categoryOrder ranks hand categories weakest to strongest, used both to seed
// KeyWinByCategory achievements below and by categoryAtLeast (service.go) to
// gate bad_beat/cooler on the loser's own hand strength.
var categoryOrder = []string{
	"high_card",
	"pair",
	"two_pair",
	"three_of_a_kind",
	"straight",
	"flush",
	"full_house",
	"four_of_a_kind",
	"straight_flush",
	"royal_flush",
}
