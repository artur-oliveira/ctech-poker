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
}

const (
	KeyWins            = "wins"
	KeyHandsPlayed     = "hands_played"
	KeyComeback        = "comeback"
	KeyBluff           = "bluff"
	KeySurvivor        = "survivor"
	KeyLooser          = "looser"
	KeyFallenKing      = "fallen_king"
	KeyAlmostWinner    = "almost_winner"
	KeyTied            = "tied"
	KeyBadBeat         = "bad_beat"
	KeyCooler          = "cooler"
	KeyCrackedAces     = "cracked_aces"
	KeyGiantSlayer     = "giant_slayer"
	KeyShowdownWarrior = "showdown_warrior"
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
		Tiers:  []Tier{{1, 100}, {2, 1000}, {3, 10000}, {4, 50000}, {5, 100000}}},
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

func init() {
	for _, category := range categoryOrder {
		tiers := commonTiers
		if category == "royal_flush" {
			tiers = []Tier{{1, 1}, {2, 5}, {3, 10}, {4, 25}, {5, 50}}
		}
		Catalog = append(Catalog, Achievement{
			Key:    KeyWinByCategory(category),
			Metric: "hand_won_with_category",
			Tiers:  tiers},
		)
	}
}
