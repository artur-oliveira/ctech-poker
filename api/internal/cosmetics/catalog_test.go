package cosmetics

import (
	"strings"
	"testing"
	"time"
)

func TestCatalogRows(t *testing.T) {
	cases := []struct {
		kind        Kind
		id          string
		premium     bool
		priceFichas int64
		sku         string
	}{
		{KindDeck, "four-color", false, 0, ""},
		{KindDeck, "two-color", false, 0, ""},
		{KindDeck, "colorblind", false, 0, ""},
		{KindDeck, "high-constrast", false, 0, ""},
		{KindDeck, "casino", true, 200_000, "poker_deck_casino"},
		{KindDeck, "bicycle", true, 200_000, "poker_deck_bicycle"},
		{KindDeck, "vintage", true, 200_000, "poker_deck_vintage"},
		{KindDeck, "golden", true, 500_000, "poker_deck_golden"},
		{KindDeck, "pink", true, 500_000, "poker_deck_pink"},
		{KindDeck, "alt", true, 500_000, "poker_deck_alt"},
		{KindFelt, "classic", false, 0, ""},
		{KindFelt, "midnight", true, 1_000_000, "poker_felt_midnight"},
		{KindFelt, "burgundy", true, 1_000_000, "poker_felt_burgundy"},
		{KindFelt, "ocean", true, 1_000_000, "poker_felt_ocean"},
	}
	for _, tc := range cases {
		if !IsKnown(tc.kind, tc.id) {
			t.Fatalf("IsKnown(%v, %q) = false, want true", tc.kind, tc.id)
		}
		if got := IsPremium(tc.kind, tc.id); got != tc.premium {
			t.Fatalf("IsPremium(%v, %q) = %v, want %v", tc.kind, tc.id, got, tc.premium)
		}
		sku, priceFichas, ok := SKUFor(tc.kind, tc.id)
		if !tc.premium {
			if ok {
				t.Fatalf("SKUFor(%v, %q) ok = true, want false for a free item", tc.kind, tc.id)
			}
			continue
		}
		if !ok || sku != tc.sku || priceFichas != tc.priceFichas {
			t.Fatalf("SKUFor(%v, %q) = %q, %d, %v; want %q, %d, true", tc.kind, tc.id, sku, priceFichas, ok, tc.sku, tc.priceFichas)
		}
	}
}

func TestIsKnownRejectsUnknownAndCrossKindIDs(t *testing.T) {
	if IsKnown(KindDeck, "not-a-real-deck") {
		t.Fatal("unknown deck id must not be known")
	}
	if IsKnown(KindFelt, "casino") {
		t.Fatal("a deck-only id must not be known as a felt id")
	}
}

func TestItemForSKU(t *testing.T) {
	kind, id, priceFichas, ok := ItemForSKU("poker_deck_golden")
	if !ok || kind != KindDeck || id != "golden" || priceFichas != 500_000 {
		t.Fatalf("ItemForSKU(poker_deck_golden) = %v, %q, %d, %v", kind, id, priceFichas, ok)
	}
	kind, id, priceFichas, ok = ItemForSKU("poker_felt_ocean")
	if !ok || kind != KindFelt || id != "ocean" || priceFichas != 1_000_000 {
		t.Fatalf("ItemForSKU(poker_felt_ocean) = %v, %q, %d, %v", kind, id, priceFichas, ok)
	}
	if _, _, _, ok := ItemForSKU("not-a-product"); ok {
		t.Fatal("unknown SKU must not resolve to a cosmetic")
	}
}

func TestAllReturnsOnlyRequestedKind(t *testing.T) {
	decks := All(KindDeck)
	if len(decks) != 10 {
		t.Fatalf("All(KindDeck) len = %d, want 10", len(decks))
	}
	for _, e := range decks {
		if e.Kind != KindDeck {
			t.Fatalf("All(KindDeck) returned a %v entry", e.Kind)
		}
	}
	felt := All(KindFelt)
	if len(felt) != 4 {
		t.Fatalf("All(KindFelt) len = %d, want 4", len(felt))
	}
}

func TestAvatarCatalogInvariants(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range catalog {
		if seen[catalogKey(e.Kind, e.ID)] {
			t.Fatalf("duplicate %v/%s", e.Kind, e.ID)
		}
		seen[catalogKey(e.Kind, e.ID)] = true
		if !IsAvatarKind(e.Kind) {
			continue
		}
		switch e.Source {
		case SourcePurchase:
			if e.SKU == "" || e.PriceFichas <= 0 {
				t.Fatalf("%v/%s purchase needs SKU and price", e.Kind, e.ID)
			}
		case SourceAchievement, SourceSeason:
			if e.SKU != "" || e.PriceFichas != 0 {
				t.Fatalf("%v/%s non-purchase must have no SKU/price", e.Kind, e.ID)
			}
		default:
			t.Fatalf("%v/%s has no Source", e.Kind, e.ID)
		}
		if !e.From.IsZero() && !e.Until.IsZero() && !e.From.Before(e.Until) {
			t.Fatalf("%v/%s From must be before Until", e.Kind, e.ID)
		}
	}
}

// ListCatalog no longer fails loud (ErrCatalogMismatch) for a premium entry
// that is not purchasable, so a premium deck/felt entry missing its SKU or
// price would silently show no price; pin it here instead.
func TestPremiumDeckFeltHaveSKUAndPrice(t *testing.T) {
	for _, e := range catalog {
		if e.Premium && e.Source == "" && (e.SKU == "" || e.PriceFichas <= 0) {
			t.Fatalf("%v/%s premium without Source needs SKU and price", e.Kind, e.ID)
		}
	}
}

func TestAvailable(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	until := from.Add(24 * time.Hour)
	restore := WithCatalogForTest([]CatalogEntry{
		{Kind: KindHat, ID: "windowed", Source: SourceSeason, From: from, Until: until},
		{Kind: KindHat, ID: "open", Source: SourceSeason},
	})
	defer restore()
	cases := []struct {
		id   string
		now  time.Time
		want bool
	}{
		{"windowed", from.Add(-time.Second), false},
		{"windowed", from.Add(time.Hour), true},
		{"windowed", until.Add(time.Second), false},
		{"open", from.Add(-1000 * time.Hour), true},
		{"nope", from, false},
	}
	for _, c := range cases {
		if got := Available(KindHat, c.id, c.now); got != c.want {
			t.Fatalf("Available(%s, %v) = %v, want %v", c.id, c.now, got, c.want)
		}
	}
}

func TestWithCatalogForTestSwapsAndRestores(t *testing.T) {
	restore := WithCatalogForTest([]CatalogEntry{{Kind: KindHat, ID: "x", Source: SourcePurchase, SKU: "s", PriceFichas: 1}})
	if !IsKnown(KindHat, "x") || IsKnown(KindDeck, "casino") {
		t.Fatal("catalog not swapped")
	}
	if _, _, _, ok := ItemForSKU("s"); !ok {
		t.Fatal("bySKU not rebuilt")
	}
	restore()
	if IsKnown(KindHat, "x") || !IsKnown(KindDeck, "casino") {
		t.Fatal("catalog not restored")
	}
	if _, _, _, ok := ItemForSKU("s"); ok {
		t.Fatal("bySKU not restored")
	}
}

func TestDecorEmptyEqual(t *testing.T) {
	if !(Decor{}).Empty() || !(Decor{Badges: []string{}}).Empty() || (Decor{Hat: "h"}).Empty() {
		t.Fatal("Empty wrong")
	}
	if !(Decor{}).Equal(Decor{Badges: []string{}}) {
		t.Fatal("nil badges must equal empty")
	}
	a := Decor{Frame: "f", Badges: []string{"a", "b"}}
	if !a.Equal(Decor{Frame: "f", Badges: []string{"a", "b"}}) || a.Equal(Decor{Frame: "f", Badges: []string{"b", "a"}}) {
		t.Fatal("badge order must matter")
	}
}

func TestAvatarCatalogLaunchIDs(t *testing.T) {
	want := map[Kind][]string{
		KindFrame:     {"chip-ring"},
		KindHat:       {"dealer-visor", "crown", "laurel"},
		KindBadge:     {"all-in-hero", "iron-nerve", "rainmaker", "veteran", "social-butterfly"},
		KindAccessory: {"devil-horns", "angel-wings"},
	}
	for kind, ids := range want {
		for _, id := range ids {
			e, ok := byID[catalogKey(kind, id)]
			if !ok || !e.Premium || e.Source != SourceAchievement {
				t.Errorf("%s/%s: want premium achievement entry, got %+v (found=%v)", kind, id, e, ok)
			}
		}
	}
}

// The accessory slot holds the ornaments (wings and horns): five sold, two earned.
func TestAccessoryOrnaments(t *testing.T) {
	sold, earned := 0, 0
	for _, e := range All(KindAccessory) {
		switch e.Source {
		case SourcePurchase:
			sold++
		case SourceAchievement:
			earned++
		}
	}
	if sold != 5 || earned != 2 || !IsAvatarKind(KindAccessory) {
		t.Fatalf("accessory catalog: %d sold, %d earned, want 5 and 2", sold, earned)
	}
}

func TestAvatarPurchasableCatalog(t *testing.T) {
	want := []struct {
		kind  Kind
		id    string
		cents int64
	}{
		{KindFrame, "felt-ring", 600}, {KindFrame, "walnut-rail", 800}, {KindFrame, "oxblood-edge", 1000},
		{KindFrame, "silver-filigree", 800}, {KindFrame, "pearl-beads", 900}, {KindFrame, "rose-gold", 1000}, {KindFrame, "obsidian-edge", 1000}, {KindFrame, "emerald-cut", 1200}, {KindFrame, "sapphire-deco", 1200}, {KindFrame, "olive-branch", 1400}, {KindFrame, "suit-ring", 1500},
		{KindFrame, "inferno", 2500}, {KindFrame, "arcane-aura", 2200}, {KindFrame, "storm-ring", 2200}, {KindFrame, "gold-shimmer", 1800}, {KindFrame, "baroque-gilt", 1800}, {KindFrame, "laurel-wreath", 1600}, {KindFrame, "art-nouveau", 1600}, {KindFrame, "celtic-knot", 1400},
		{KindAccessory, "bat-wings", 2000}, {KindAccessory, "gold-wings", 2500}, {KindAccessory, "dragon-wings", 2500}, {KindAccessory, "ram-horns", 1800}, {KindAccessory, "bull-horns", 1500},
		{KindHat, "top-hat", 300}, {KindHat, "cowboy", 400}, {KindHat, "beanie", 200},
		{KindHat, "fedora", 500}, {KindHat, "beret", 400}, {KindHat, "bowler", 500}, {KindHat, "captain-cap", 600}, {KindHat, "wizard", 700}, {KindHat, "sun-visor", 300},
	}
	for _, w := range want {
		var got *CatalogEntry
		for _, e := range All(w.kind) {
			if e.ID == w.id {
				got = &e
			}
		}
		sku := "poker_" + string(w.kind) + "_" + strings.ReplaceAll(w.id, "-", "_")
		if got == nil || !got.Premium || got.Source != SourcePurchase || got.SKU != sku || got.PriceFichas != w.cents*3000 || !got.From.IsZero() || !got.Until.IsZero() {
			t.Errorf("%s/%s = %+v, want purchase %s %d fichas", w.kind, w.id, got, sku, w.cents*3000)
		}
	}
}
