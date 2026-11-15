package cosmetics

// AchievementGrant maps an achievement tier to the avatar item it unlocks.
// Key is the achievements.Key* string value; this package is a leaf and must
// not import achievements (a test in cosmeticpurchase pins the keys/stars).
type AchievementGrant struct {
	Key   string
	Stars int
	Kind  Kind
	ID    string
}

// AchievementGrants is the binding grant table, rows ordered by Stars within
// each key so a tier-skipping unlock toasts its items in tier order. Season-
// source items have no achievement trigger and are intentionally absent.
//
// social-butterfly: no social achievement exists, so it maps to
// won_full_table (the closest table-presence metric) until a hosting metric
// exists.
var AchievementGrants = []AchievementGrant{
	{"hands_played", 1, KindHat, "dealer-visor"},
	{"hands_played", 2, KindBadge, "veteran"},
	{"hands_played", 3, KindFrame, "chip-ring"},
	{"wins", 3, KindHat, "laurel"},
	{"wins", 4, KindHat, "crown"},
	{"wins", 5, KindAccessory, "angel-wings"},
	{"bluff", 3, KindAccessory, "devil-horns"},
	{"all_in", 3, KindBadge, "all-in-hero"},
	{"folded_streak", 1, KindBadge, "iron-nerve"},
	{"sandbox_chips_earned", 3, KindBadge, "rainmaker"},
	{"won_full_table", 1, KindBadge, "social-butterfly"},
}

// GrantsFor returns, in table order, every row for key reached at stars
// (row.Stars <= stars, so a skipped tier still grants). nil when none.
func GrantsFor(key string, stars int) []AchievementGrant {
	var out []AchievementGrant
	for _, g := range AchievementGrants {
		if g.Key == key && g.Stars <= stars {
			out = append(out, g)
		}
	}
	return out
}
