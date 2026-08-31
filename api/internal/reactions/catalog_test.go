package reactions

import "testing"

func TestIsKnownAndIsPremium(t *testing.T) {
	if !IsKnown("clap") || IsPremium("clap") {
		t.Fatal("clap must be known and free")
	}
	if !IsKnown("cold") || !IsPremium("cold") {
		t.Fatal("cold must be known and premium")
	}
	if IsKnown("not-a-real-reaction") {
		t.Fatal("unknown id must not be known")
	}
}

func TestSKUForPremiumMatchesPricingTable(t *testing.T) {
	cases := map[string]int64{
		"cold": 100_000, "fire": 100_000,
		"poop": 500_000, "rofl": 500_000, "knife": 500_000, "turtle": 500_000,
	}
	for id, wantFichas := range cases {
		sku, priceFichas, ok := SKUFor(id)
		if !ok || priceFichas != wantFichas || sku != "poker_reaction_"+id {
			t.Fatalf("SKUFor(%q) = %q, %d, %v; want poker_reaction_%s, %d, true", id, sku, priceFichas, ok, id, wantFichas)
		}
	}
}

func TestSKUForFreeReactionNotOK(t *testing.T) {
	if _, _, ok := SKUFor("clap"); ok {
		t.Fatal("SKUFor on a free reaction must return ok=false")
	}
}

func TestReactionForSKU(t *testing.T) {
	id, priceFichas, ok := ReactionForSKU("poker_reaction_cold")
	if !ok || id != "cold" || priceFichas != 100_000 {
		t.Fatalf("ReactionForSKU = %q, %d, %v", id, priceFichas, ok)
	}
	if _, _, ok := ReactionForSKU("not-a-product"); ok {
		t.Fatal("unknown SKU must not resolve to a reaction")
	}
}

func TestIsTargetedMatchesFrontend(t *testing.T) {
	// Mirrors ui/src/lib/reactions.ts's TABLE_REACTIONS `targeted` flags. Regression
	// for the tablews.go WS gateway hand-maintaining its own tableReactions /
	// targetedTableReactions maps: "knife" and "flowers" were added here and to
	// TABLE_REACTIONS but never to those maps, so every WS reaction send for either
	// id was rejected as invalid_reaction even when owned/free. The gate now calls
	// IsKnown/IsTargeted directly, so this test is the only place the two lists can
	// drift.
	targeted := map[string]bool{
		"clap": false, "laugh": false, "wow": false, "angry": false, "cry": false,
		"nervous": false, "cold": false, "fire": false, "respect": false, "sleepy": false,
		"heartbeat": false, "shark": false, "pokerface": false,
		"chip": true, "coffee": true, "clover": true, "horseshoe": true, "tear": true,
		"tomato": true, "poop": true, "rofl": true, "duck": true, "turtle": true,
		"knife": true, "flowers": true, "spotlight": true, "crown": true, "bandage": true,
		"cucumber": true, "boomerang": true,
	}
	for id, want := range targeted {
		if got := IsTargeted(id); got != want {
			t.Errorf("IsTargeted(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestEveryFreeTableReactionIsKnown(t *testing.T) {
	// Mirrors ui/src/lib/reactions.ts's TABLE_REACTIONS keys — keep this list in
	// sync by hand on any change to that file (see docs/specs/2026-08-12-
	// premium-reactions.md's Catalog section for why this can't be build-time
	// coupled across languages).
	all := []string{
		"clap", "laugh", "wow", "angry", "cry", "nervous", "cold", "fire", "respect", "sleepy",
		"heartbeat", "shark", "pokerface", "chip", "coffee", "clover", "horseshoe", "tear",
		"tomato", "poop", "rofl", "duck", "turtle", "knife", "flowers", "spotlight", "crown", "bandage",
		"cucumber", "boomerang",
	}
	for _, id := range all {
		if !IsKnown(id) {
			t.Fatalf("catalog is missing frontend reaction id %q", id)
		}
	}
}
