package cosmeticpurchase

import (
	"reflect"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/achievements"
	"gopkg.aoctech.app/poker/api/internal/cosmetics"
)

func TestEveryGrantTargetsAnAchievementItem(t *testing.T) {
	seen := map[string]int{}
	for _, g := range cosmetics.AchievementGrants {
		seen[string(g.Kind)+"#"+g.ID]++
		found := false
		for _, e := range cosmetics.All(g.Kind) {
			if e.ID == g.ID {
				found = true
				if e.Source != cosmetics.SourceAchievement {
					t.Errorf("%s/%s source = %q", g.Kind, g.ID, e.Source)
				}
			}
		}
		if !found {
			t.Errorf("%s/%s not in catalog", g.Kind, g.ID)
		}
	}
	for _, k := range []cosmetics.Kind{cosmetics.KindFrame, cosmetics.KindBadge, cosmetics.KindHat, cosmetics.KindAccessory} {
		for _, e := range cosmetics.All(k) {
			if e.Source == cosmetics.SourceAchievement && seen[string(k)+"#"+e.ID] != 1 {
				t.Errorf("%s/%s appears in %d grant rows, want 1", k, e.ID, seen[string(k)+"#"+e.ID])
			}
		}
	}
}

func TestGrantRowsSortedByStarsWithinKey(t *testing.T) {
	last := map[string]int{}
	for _, g := range cosmetics.AchievementGrants {
		if g.Stars < last[g.Key] {
			t.Errorf("%s: %s at %d stars after a %d-star row", g.Key, g.ID, g.Stars, last[g.Key])
		}
		last[g.Key] = g.Stars
	}
}

func TestEveryGrantKeyAndStarsExist(t *testing.T) {
	for _, g := range cosmetics.AchievementGrants {
		ok := false
		for _, a := range achievements.Catalog {
			if a.Key != g.Key {
				continue
			}
			for _, tr := range a.Tiers {
				if tr.Stars == g.Stars {
					ok = true
				}
			}
		}
		if !ok {
			t.Errorf("%s/%s: no achievement %q with %d-star tier", g.Kind, g.ID, g.Key, g.Stars)
		}
	}
}

func TestGrantsForSkippedTier(t *testing.T) {
	var ids []string
	for _, g := range cosmetics.GrantsFor("hands_played", 3) {
		ids = append(ids, g.ID)
	}
	if want := []string{"dealer-visor", "veteran", "chip-ring"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("got %v want %v (tier order)", ids, want)
	}
}

func TestGrantsForUnmappedAndLowStars(t *testing.T) {
	if g := cosmetics.GrantsFor("nope", 5); g != nil {
		t.Errorf("unknown key: %v", g)
	}
	if g := cosmetics.GrantsFor("wins", 2); g != nil {
		t.Errorf("wins@2: %v", g)
	}
}

// The two earned ornaments: devil horns for a 3-star bluff, angel wings for a 5-star wins
// (after the laurel and the crown, tier order).
func TestGrantsForOrnaments(t *testing.T) {
	ids := func(key string, stars int) (out []string) {
		for _, g := range cosmetics.GrantsFor(key, stars) {
			out = append(out, string(g.Kind)+"/"+g.ID)
		}
		return out
	}
	if got, want := ids("bluff", 3), []string{"accessory/devil-horns"}; !reflect.DeepEqual(got, want) {
		t.Errorf("bluff@3 = %v, want %v", got, want)
	}
	if got := ids("bluff", 2); got != nil {
		t.Errorf("bluff@2 = %v, want none", got)
	}
	if got, want := ids("wins", 5), []string{"hat/laurel", "hat/crown", "accessory/angel-wings"}; !reflect.DeepEqual(got, want) {
		t.Errorf("wins@5 = %v, want %v", got, want)
	}
}
