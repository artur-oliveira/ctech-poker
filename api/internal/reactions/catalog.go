package reactions

// ReactionCatalogEntry is the poker-owned source of truth for which reaction
// IDs exist and which are premium — a game-design fact, not a money fact
// (docs/specs/2026-08-12-premium-reactions.md). PriceFichas is fixed here
// (never client-supplied); PriceCents for the same reaction is fixed in
// ctech-wallet's own productSKUCatalog and fetched at request time via
// walletclient.ListProductSKUs — never hardcoded locally.
type ReactionCatalogEntry struct {
	ID          string
	Premium     bool
	Targeted    bool   // whether the client must supply TargetPlayerId (docs/specs/2026-08-12-premium-reactions.md)
	PriceFichas int64  // 0 if !Premium
	SKU         string // wallet ProductSKU ID. "" if !Premium.
}

var catalog = []ReactionCatalogEntry{
	{ID: "clap"}, {ID: "laugh"}, {ID: "wow"}, {ID: "angry"}, {ID: "cry"},
	{ID: "nervous"}, {ID: "respect"}, {ID: "sleepy"}, {ID: "heartbeat"},
	{ID: "shark"}, {ID: "pokerface"},
	{ID: "chip", Targeted: true}, {ID: "coffee", Targeted: true}, {ID: "clover", Targeted: true},
	{ID: "horseshoe", Targeted: true}, {ID: "tear", Targeted: true}, {ID: "tomato", Targeted: true},
	{ID: "duck", Targeted: true}, {ID: "flowers", Targeted: true},
	{ID: "spotlight", Targeted: true}, {ID: "crown", Targeted: true}, {ID: "bandage", Targeted: true},
	{ID: "cucumber", Targeted: true}, {ID: "boomerang", Targeted: true},

	{ID: "cold", Premium: true, PriceFichas: 100_000, SKU: "poker_reaction_cold"},
	{ID: "fire", Premium: true, PriceFichas: 100_000, SKU: "poker_reaction_fire"},
	{ID: "poop", Premium: true, Targeted: true, PriceFichas: 500_000, SKU: "poker_reaction_poop"},
	{ID: "rofl", Premium: true, Targeted: true, PriceFichas: 500_000, SKU: "poker_reaction_rofl"},
	{ID: "knife", Premium: true, Targeted: true, PriceFichas: 500_000, SKU: "poker_reaction_knife"},
	{ID: "turtle", Premium: true, Targeted: true, PriceFichas: 500_000, SKU: "poker_reaction_turtle"},
}

var byID = func() map[string]ReactionCatalogEntry {
	m := make(map[string]ReactionCatalogEntry, len(catalog))
	for _, e := range catalog {
		m[e.ID] = e
	}
	return m
}()

var bySKU = func() map[string]ReactionCatalogEntry {
	m := make(map[string]ReactionCatalogEntry)
	for _, e := range catalog {
		if e.SKU != "" {
			m[e.SKU] = e
		}
	}
	return m
}()

func IsKnown(id string) bool {
	_, ok := byID[id]
	return ok
}

func IsPremium(id string) bool {
	e, ok := byID[id]
	return ok && e.Premium
}

// IsTargeted reports whether id requires a TargetPlayerId, per its shape in
// ui/src/lib/reactions.ts's TABLE_REACTIONS. Unknown ids return false.
func IsTargeted(id string) bool {
	e, ok := byID[id]
	return ok && e.Targeted
}

// SKUFor returns the wallet SKU and fichas price for a premium reaction, or
// ok=false for an unknown or free one.
func SKUFor(id string) (sku string, priceFichas int64, ok bool) {
	e, found := byID[id]
	if !found || !e.Premium {
		return "", 0, false
	}
	return e.SKU, e.PriceFichas, true
}

// ReactionForSKU maps a wallet-owned product purchase back to the game-owned
// reaction. Webhook recovery uses this instead of depending on a local history
// row that may not have been persisted before the callback arrived.
func ReactionForSKU(sku string) (id string, priceFichas int64, ok bool) {
	e, found := bySKU[sku]
	if !found {
		return "", 0, false
	}
	return e.ID, e.PriceFichas, true
}

// All returns every catalog entry — used by ListCatalog (Task 4) to build the
// merged premium-flag + dual-price response.
func All() []ReactionCatalogEntry {
	return catalog
}
