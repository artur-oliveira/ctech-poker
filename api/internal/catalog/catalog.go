// Package catalog is Poker's price list: the Sandbox chip packs and the premium goods
// (reactions, decks, felts) sold for Pix. It moved here from ctech-wallet, which is a
// ledger and knows no SKU. Prices live only here: Billing receives a description and
// an amount, the wallet receives a number of credits.
//
// The numbers are the ones the wallet held (domain/wallet/sandbox_sku.go and
// product_sku.go); catalog_test.go lists them literally so changing one is deliberate.
package catalog

import "sort"

// SandboxCreditsPerCent is the fixed rate between a centavo of a pack's price and the
// base credits it buys (before bonus). The same constant the wallet used.
const SandboxCreditsPerCent int64 = 100

// Sandbox is one chip pack.
type Sandbox struct {
	ID           string
	PriceCents   int64
	BaseCredits  int64
	BonusPercent int64
}

// TotalCredits is what the pack grants: base plus the bonus on it.
func (s Sandbox) TotalCredits() int64 { return s.BaseCredits + s.BaseCredits*s.BonusPercent/100 }

var sandboxes = map[string]Sandbox{
	"pack_100":   {ID: "pack_100", PriceCents: 100, BaseCredits: 100 * SandboxCreditsPerCent, BonusPercent: 0},
	"pack_500":   {ID: "pack_500", PriceCents: 500, BaseCredits: 500 * SandboxCreditsPerCent, BonusPercent: 10},
	"pack_1000":  {ID: "pack_1000", PriceCents: 1000, BaseCredits: 1000 * SandboxCreditsPerCent, BonusPercent: 20},
	"pack_2000":  {ID: "pack_2000", PriceCents: 2000, BaseCredits: 2000 * SandboxCreditsPerCent, BonusPercent: 35},
	"pack_5000":  {ID: "pack_5000", PriceCents: 5000, BaseCredits: 5000 * SandboxCreditsPerCent, BonusPercent: 50},
	"pack_10000": {ID: "pack_10000", PriceCents: 10000, BaseCredits: 10000 * SandboxCreditsPerCent, BonusPercent: 100},
}

// Product is one fixed-price digital good.
type Product struct {
	ID         string
	PriceCents int64
}

var products = map[string]Product{
	"poker_reaction_cold":   {ID: "poker_reaction_cold", PriceCents: 100},
	"poker_reaction_fire":   {ID: "poker_reaction_fire", PriceCents: 100},
	"poker_reaction_poop":   {ID: "poker_reaction_poop", PriceCents: 500},
	"poker_reaction_rofl":   {ID: "poker_reaction_rofl", PriceCents: 500},
	"poker_reaction_knife":  {ID: "poker_reaction_knife", PriceCents: 500},
	"poker_reaction_turtle": {ID: "poker_reaction_turtle", PriceCents: 500},
	"poker_deck_casino":     {ID: "poker_deck_casino", PriceCents: 200},
	"poker_deck_bicycle":    {ID: "poker_deck_bicycle", PriceCents: 200},
	"poker_deck_vintage":    {ID: "poker_deck_vintage", PriceCents: 200},
	"poker_deck_golden":     {ID: "poker_deck_golden", PriceCents: 500},
	"poker_deck_pink":       {ID: "poker_deck_pink", PriceCents: 500},
	"poker_deck_alt":        {ID: "poker_deck_alt", PriceCents: 500},
	"poker_felt_midnight":   {ID: "poker_felt_midnight", PriceCents: 1000},
	"poker_felt_burgundy":   {ID: "poker_felt_burgundy", PriceCents: 1000},
	"poker_felt_ocean":      {ID: "poker_felt_ocean", PriceCents: 1000},
}

// Sandboxes lists the chip packs, cheapest first.
func Sandboxes() []Sandbox {
	out := make([]Sandbox, 0, len(sandboxes))
	for _, s := range sandboxes {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PriceCents < out[j].PriceCents })
	return out
}

// SandboxByID looks a pack up, or reports ok=false.
func SandboxByID(id string) (Sandbox, bool) { s, ok := sandboxes[id]; return s, ok }

// Products lists the goods, by id.
func Products() []Product {
	out := make([]Product, 0, len(products))
	for _, p := range products {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ProductByID looks a good up, or reports ok=false.
func ProductByID(id string) (Product, bool) { p, ok := products[id]; return p, ok }
