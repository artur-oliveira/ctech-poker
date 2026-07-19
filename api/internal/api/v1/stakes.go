package v1

type publicStake struct {
	SmallBlind int64 `json:"small_blind"`
	BigBlind   int64 `json:"big_blind"`
}

// Values are stored in the smallest integer unit. Real mode interprets 10/25
// as R$0.10/R$0.25; sandbox displays virtual chips without a currency symbol.
var realPublicStakes = []publicStake{
	{10, 25}, {50, 100}, {250, 500}, {500, 1000},
	{1000, 2500}, {2500, 5000}, {5000, 10000},
}

var sandboxPublicStakes = append(append([]publicStake{}, realPublicStakes...),
	publicStake{10000, 25000}, publicStake{25000, 50000}, publicStake{50000, 100000},
)

func isAllowedPublicStake(mode string, smallBlind, bigBlind int64) bool {
	stakes := sandboxPublicStakes
	if mode == "real" {
		stakes = realPublicStakes
	}
	for _, stake := range stakes {
		if stake.SmallBlind == smallBlind && stake.BigBlind == bigBlind {
			return true
		}
	}
	return false
}

func sandboxStakeCatalog() map[string]any {
	return map[string]any{
		"currency_mode": "sandbox",
		"unit":          "virtual_chip",
		"stakes":        sandboxPublicStakes,
	}
}
