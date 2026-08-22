// Package cosmetics is the poker-owned source of truth for which deck and
// felt cosmetic ids exist and which are premium — a game-design fact, not a
// money fact, same rationale as internal/reactions (see
// docs/specs/2026-08-21-premium-cosmetics-overhaul.md).
package cosmetics

// Kind distinguishes the two cosmetic catalogs. Deck and felt ids don't
// collide today, but byID/bySKU key on the (Kind, ID) pair regardless so the
// map never has to assume that forever.
type Kind string

const (
	KindDeck Kind = "deck"
	KindFelt Kind = "felt"
)

// CatalogEntry mirrors reactions.ReactionCatalogEntry's shape. PriceFichas is
// fixed here (never client-supplied); PriceCents for the same item is fixed
// in ctech-wallet's own productSKUCatalog and fetched at request time — never
// hardcoded locally.
type CatalogEntry struct {
	Kind        Kind
	ID          string // DeckVariantId or TableThemeId string
	Premium     bool
	PriceFichas int64  // 0 if !Premium
	SKU         string // wallet ProductSKU ID. "" if !Premium.
}

var catalog = []CatalogEntry{
	{Kind: KindDeck, ID: "four-color"},
	{Kind: KindDeck, ID: "two-color"},
	{Kind: KindDeck, ID: "colorblind"},
	{Kind: KindDeck, ID: "high-constrast"},
	{Kind: KindDeck, ID: "casino", Premium: true, PriceFichas: 200_000, SKU: "poker_deck_casino"},
	{Kind: KindDeck, ID: "bicycle", Premium: true, PriceFichas: 200_000, SKU: "poker_deck_bicycle"},
	{Kind: KindDeck, ID: "vintage", Premium: true, PriceFichas: 200_000, SKU: "poker_deck_vintage"},
	{Kind: KindDeck, ID: "golden", Premium: true, PriceFichas: 500_000, SKU: "poker_deck_golden"},
	{Kind: KindDeck, ID: "pink", Premium: true, PriceFichas: 500_000, SKU: "poker_deck_pink"},
	{Kind: KindDeck, ID: "alt", Premium: true, PriceFichas: 500_000, SKU: "poker_deck_alt"},

	{Kind: KindFelt, ID: "classic"},
	{Kind: KindFelt, ID: "midnight", Premium: true, PriceFichas: 1_000_000, SKU: "poker_felt_midnight"},
	{Kind: KindFelt, ID: "burgundy", Premium: true, PriceFichas: 1_000_000, SKU: "poker_felt_burgundy"},
	{Kind: KindFelt, ID: "ocean", Premium: true, PriceFichas: 1_000_000, SKU: "poker_felt_ocean"},
}

func catalogKey(kind Kind, id string) string { return string(kind) + "#" + id }

var byID = func() map[string]CatalogEntry {
	m := make(map[string]CatalogEntry, len(catalog))
	for _, e := range catalog {
		m[catalogKey(e.Kind, e.ID)] = e
	}
	return m
}()

var bySKU = func() map[string]CatalogEntry {
	m := make(map[string]CatalogEntry)
	for _, e := range catalog {
		if e.SKU != "" {
			m[e.SKU] = e
		}
	}
	return m
}()

func IsKnown(kind Kind, id string) bool {
	_, ok := byID[catalogKey(kind, id)]
	return ok
}

func IsPremium(kind Kind, id string) bool {
	e, ok := byID[catalogKey(kind, id)]
	return ok && e.Premium
}

// SKUFor returns the wallet SKU and fichas price for a premium item, or
// ok=false for an unknown or free one.
func SKUFor(kind Kind, id string) (sku string, priceFichas int64, ok bool) {
	e, found := byID[catalogKey(kind, id)]
	if !found || !e.Premium {
		return "", 0, false
	}
	return e.SKU, e.PriceFichas, true
}

// ItemForSKU maps a wallet-owned product purchase back to the game-owned
// cosmetic. Webhook recovery uses this instead of depending on a local
// history row that may not have been persisted before the callback arrived.
func ItemForSKU(sku string) (kind Kind, id string, priceFichas int64, ok bool) {
	e, found := bySKU[sku]
	if !found {
		return "", "", 0, false
	}
	return e.Kind, e.ID, e.PriceFichas, true
}

// All returns every catalog entry for kind — used by ListCatalog to build the
// merged premium-flag + dual-price response.
func All(kind Kind) []CatalogEntry {
	out := make([]CatalogEntry, 0, len(catalog))
	for _, e := range catalog {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}
