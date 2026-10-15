package catalog

import "testing"

// The literal list: the prices and credits ctech-wallet held when they moved here. Editing
// one is a pricing decision, and this test makes it a deliberate one.
func TestSandboxPacksAreTheOnesTheWalletSold(t *testing.T) {
	want := []struct {
		id                 string
		price, totalCredit int64
	}{
		{"pack_100", 100, 10000},
		{"pack_500", 500, 55000},
		{"pack_1000", 1000, 120000},
		{"pack_2000", 2000, 270000},
		{"pack_5000", 5000, 750000},
		{"pack_10000", 10000, 2000000},
	}
	got := Sandboxes()
	if len(got) != len(want) {
		t.Fatalf("%d packs, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].ID != w.id || got[i].PriceCents != w.price || got[i].TotalCredits() != w.totalCredit {
			t.Errorf("pack %d = %+v (%d credits), want %s %d -> %d", i, got[i], got[i].TotalCredits(), w.id, w.price, w.totalCredit)
		}
		if s, ok := SandboxByID(w.id); !ok || s != got[i] {
			t.Errorf("SandboxByID(%s) = %+v %v", w.id, s, ok)
		}
	}
	if _, ok := SandboxByID("pack_free"); ok {
		t.Error("an unknown pack resolved")
	}
}

func TestProductsAreTheOnesTheWalletSold(t *testing.T) {
	want := map[string]int64{
		"poker_reaction_cold": 100, "poker_reaction_fire": 100, "poker_reaction_poop": 500, "poker_reaction_rofl": 500,
		"poker_reaction_knife": 500, "poker_reaction_turtle": 500,
		"poker_deck_casino": 200, "poker_deck_bicycle": 200, "poker_deck_vintage": 200,
		"poker_deck_golden": 500, "poker_deck_pink": 500, "poker_deck_alt": 500,
		"poker_felt_midnight": 1000, "poker_felt_burgundy": 1000, "poker_felt_ocean": 1000,
	}
	got := Products()
	if len(got) != len(want) {
		t.Fatalf("%d products, want %d", len(got), len(want))
	}
	for _, p := range got {
		if want[p.ID] != p.PriceCents {
			t.Errorf("%s = %d, want %d", p.ID, p.PriceCents, want[p.ID])
		}
	}
	if _, ok := ProductByID("poker_deck_free"); ok {
		t.Error("an unknown product resolved")
	}
}
