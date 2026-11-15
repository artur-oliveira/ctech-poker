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
		"poker_frame_felt_ring": 600, "poker_frame_walnut_rail": 800, "poker_frame_oxblood_edge": 1000,
		"poker_frame_silver_filigree": 800, "poker_frame_pearl_beads": 900, "poker_frame_rose_gold": 1000, "poker_frame_obsidian_edge": 1000, "poker_frame_emerald_cut": 1200, "poker_frame_sapphire_deco": 1200, "poker_frame_olive_branch": 1400, "poker_frame_suit_ring": 1500,
		"poker_hat_top_hat": 300, "poker_hat_cowboy": 400, "poker_hat_beanie": 200,
		"poker_accessory_shades": 300, "poker_accessory_monocle": 500, "poker_accessory_mustache": 200, "poker_accessory_headset": 400,
		"poker_hat_fedora": 500, "poker_hat_beret": 400, "poker_hat_bowler": 500, "poker_hat_captain_cap": 600, "poker_hat_wizard": 700, "poker_hat_sun_visor": 300, "poker_accessory_round_glasses": 400, "poker_accessory_eyepatch": 300, "poker_accessory_cigar": 500, "poker_accessory_handlebar": 400, "poker_accessory_earring": 300,
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
