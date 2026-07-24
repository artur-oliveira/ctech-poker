// Package achievements implements data-driven achievement progress and tiers.
package achievements

import "fmt"

type Tier struct {
	Stars     int
	Threshold int
}

type Achievement struct {
	Key    string
	Metric string
	Tiers  []Tier
}

const (
	KeyWins        = "wins"
	KeyHandsPlayed = "hands_played"
	KeyComeback    = "comeback"
	KeyBluff       = "bluff"
	KeySurvivor    = "survivor"
)

func KeyWinByCategory(category string) string { return fmt.Sprintf("win_category_%s", category) }

var commonTiers = []Tier{{1, 1}, {2, 10}, {3, 100}, {4, 1000}, {5, 10000}}

var Catalog = []Achievement{
	{Key: KeyWins, Metric: "hand_won", Tiers: commonTiers},
	{Key: KeyHandsPlayed, Metric: "hand_played", Tiers: []Tier{{1, 100}, {2, 1000}, {3, 10000}, {4, 50000}, {5, 100000}}},
	{Key: KeyComeback, Metric: "won_after_all_in", Tiers: []Tier{{1, 1}, {2, 5}, {3, 25}, {4, 100}, {5, 500}}},
	{Key: KeyBluff, Metric: "won_without_showdown_weaker_hand", Tiers: []Tier{{1, 1}, {2, 5}, {3, 25}, {4, 100}, {5, 500}}},
	{Key: KeySurvivor, Metric: "hands_without_leaving", Tiers: []Tier{{1, 50}, {2, 250}, {3, 1000}, {4, 5000}, {5, 25000}}},
}

func init() {
	for _, category := range []string{"high_card", "pair", "two_pair", "three_of_a_kind", "straight", "flush", "full_house", "four_of_a_kind", "straight_flush", "royal_flush"} {
		tiers := commonTiers
		if category == "royal_flush" {
			tiers = []Tier{{1, 1}, {2, 5}, {3, 10}, {4, 25}, {5, 50}}
		}
		Catalog = append(Catalog, Achievement{Key: KeyWinByCategory(category), Metric: "hand_won_with_category", Tiers: tiers})
	}
}
