package achievements

import "testing"

func TestBuildSummaryFoldsWholeCatalog(t *testing.T) {
	summary := BuildSummary("sandbox", []PlayerAchievementProgress{
		{Key: KeyWins, Count: 15, UnlockedAt: "2026-09-01T10:00:00Z"},
		{Key: KeyFirstHandAllInWin, Count: 1},        // secret, past first tier
		{Key: KeyLostStraightFlushToRoyal, Count: 1}, // secret, past first tier
		{Key: KeySamePocketPairStreak, Count: 2},     // secret, below first tier (3) -> hidden
	})

	if summary.Mode != "sandbox" {
		t.Fatalf("mode = %q", summary.Mode)
	}
	byKey := map[string]AchievementState{}
	for _, s := range summary.Achievements {
		byKey[s.Key] = s
	}

	if _, revealed := byKey[KeySamePocketPairStreak]; revealed {
		t.Fatal("still-locked secret must stay hidden")
	}
	if len(summary.Achievements) != len(Catalog)-1 {
		t.Fatalf("revealed %d, want %d", len(summary.Achievements), len(Catalog)-1)
	}

	wins := byKey[KeyWins]
	// The unlock timestamp rides through to the payload (issue #72) while a
	// row that never carried one — every legacy row — stays empty rather than
	// erroring or inventing a date.
	if wins.UnlockedAt != "2026-09-01T10:00:00Z" {
		t.Fatalf("wins.UnlockedAt = %q, want the stored stamp", wins.UnlockedAt)
	}
	if got := byKey[KeyFirstHandAllInWin].UnlockedAt; got != "" {
		t.Fatalf("legacy row without a stamp reported UnlockedAt %q", got)
	}
	if wins.Stars != 2 || wins.Progress != 15 || !wins.Unlocked || wins.Completed {
		t.Fatalf("wins = %+v", wins)
	}
	if wins.NextTarget == nil || *wins.NextTarget != 100 {
		t.Fatalf("wins next target = %v, want 100", wins.NextTarget)
	}
	if wins.MaxTarget != 10000 {
		t.Fatalf("wins max target = %d, want 10000", wins.MaxTarget)
	}

	secret := byKey[KeyFirstHandAllInWin]
	if !secret.Unlocked || !secret.Completed || secret.NextTarget != nil {
		t.Fatalf("revealed secret = %+v", secret)
	}

	untouched := byKey[KeyWins] // sanity: an untouched key is still listed at zero
	untouched = byKey["won_heads_up"]
	if untouched.Unlocked || untouched.Progress != 0 || untouched.NextTarget == nil {
		t.Fatalf("untouched = %+v", untouched)
	}

	if summary.Totals.Revealed != len(summary.Achievements) {
		t.Fatalf("totals.Revealed = %d", summary.Totals.Revealed)
	}
	if summary.Totals.Stars != 4 || summary.Totals.Unlocked != 3 || summary.Totals.Completed < 1 {
		t.Fatalf("totals = %+v", summary.Totals)
	}
	if summary.Totals.MaxStars == 0 {
		t.Fatal("totals.MaxStars unset")
	}
}
