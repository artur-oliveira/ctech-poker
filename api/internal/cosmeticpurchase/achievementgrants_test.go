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
	if want := []string{"dealer-visor", "chip-ring", "veteran"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("got %v want %v (table order)", ids, want)
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
