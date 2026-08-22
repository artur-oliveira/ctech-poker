package v1

// Tier values for publicStake.Tier / roomstore.Room.Tier. A rebind (buyin's
// entitlement flow) only ever moves a reservation to another table of the
// same tier — see docs/plans/2026-08-21-entry-fee-entitlement.md.
const (
	TierMicro = "micro"
	TierLow   = "low"
	TierMid   = "mid"
	TierHigh  = "high"
)

type publicStake struct {
	SmallBlind int64 `json:"small_blind"`
	BigBlind   int64 `json:"big_blind"`
	// Tier groups stakes for the entry-fee entitlement's rebind eligibility
	// (buyin.Service) — a reservation only ever moves to another table of the
	// same tier. Empty for sandbox stakes, which have no entitlement.
	Tier string `json:"tier,omitempty"`
	// FeeCents is the fixed real-money table-entry fee for this stake tier, in
	// BRL cents. Zero for sandbox stakes (sandbox has no entry fee, only rake
	// — see hand.Table.ConfigureRake). Never derived from SmallBlind/BigBlind
	// at charge time — always a stored lookup (see Global Constraints).
	FeeCents int64 `json:"fee_cents,omitempty"`
}

// Values are stored in the smallest integer unit. Real mode interprets 5 as
// R$0,05; sandbox displays virtual chips without a currency symbol. Grouped
// by risk/compliance tier (see docs/plans/2026-07-25-realmoney-fixed-fee-and-sandbox-rake.md).
var realPublicStakes = []publicStake{
	// Micro — R$1,00 fee
	{SmallBlind: 5, BigBlind: 10, Tier: TierMicro, FeeCents: 100},
	{SmallBlind: 10, BigBlind: 20, Tier: TierMicro, FeeCents: 100},
	{SmallBlind: 25, BigBlind: 50, Tier: TierMicro, FeeCents: 100},
	{SmallBlind: 50, BigBlind: 100, Tier: TierMicro, FeeCents: 100},
	// Low — R$2,00 fee
	{SmallBlind: 100, BigBlind: 200, Tier: TierLow, FeeCents: 200},
	{SmallBlind: 200, BigBlind: 500, Tier: TierLow, FeeCents: 200},
	// Mid — R$4,00 fee
	{SmallBlind: 500, BigBlind: 1000, Tier: TierMid, FeeCents: 400},
	{SmallBlind: 1000, BigBlind: 2000, Tier: TierMid, FeeCents: 400},
	// High — R$8,00 fee
	{SmallBlind: 2500, BigBlind: 5000, Tier: TierHigh, FeeCents: 800},
	{SmallBlind: 5000, BigBlind: 10000, Tier: TierHigh, FeeCents: 800},
}

var sandboxPublicStakes = []publicStake{
	{SmallBlind: 10, BigBlind: 20},
	{SmallBlind: 25, BigBlind: 50},
	{SmallBlind: 50, BigBlind: 100},
	// Low — R$2,00 fee
	{SmallBlind: 100, BigBlind: 200},
	{SmallBlind: 200, BigBlind: 500},
	// Mid — R$4,00 fee
	{SmallBlind: 500, BigBlind: 1000},
	{SmallBlind: 1000, BigBlind: 2000},
	// High — R$8,00 fee
	{SmallBlind: 2500, BigBlind: 5000},
	{SmallBlind: 5000, BigBlind: 10000},
	{SmallBlind: 10000, BigBlind: 25000},
	{SmallBlind: 25000, BigBlind: 50000},
	{SmallBlind: 50000, BigBlind: 100000},
}

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

// realStakeLookup returns the fixed table-entry fee and tier for a real-money
// stake pair in one lookup. ok is false only if the pair matches no catalog
// tier — callers must have already validated the stake via
// isAllowedPublicStake("real", …) before relying on this to be true.
func realStakeLookup(smallBlind, bigBlind int64) (feeCents int64, tier string, ok bool) {
	for _, stake := range realPublicStakes {
		if stake.SmallBlind == smallBlind && stake.BigBlind == bigBlind {
			return stake.FeeCents, stake.Tier, true
		}
	}
	return 0, "", false
}

func sandboxStakeCatalog() map[string]any {
	return map[string]any{
		"currency_mode": "sandbox",
		"unit":          "virtual_chip",
		"stakes":        sandboxPublicStakes,
	}
}

func realStakeCatalog() map[string]any {
	return map[string]any{
		"currency_mode": "real",
		"unit":          "brl_cent",
		"stakes":        realPublicStakes,
	}
}
