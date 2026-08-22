package cosmetics

import "testing"

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
		{KindFelt, "midnight", true, 200_000, "poker_felt_midnight"},
		{KindFelt, "burgundy", true, 200_000, "poker_felt_burgundy"},
		{KindFelt, "ocean", true, 200_000, "poker_felt_ocean"},
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
	if !ok || kind != KindFelt || id != "ocean" || priceFichas != 200_000 {
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
