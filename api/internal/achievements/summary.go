package achievements

// AchievementState is one achievement's complete per-player state: the catalog
// definition plus where this player stands against every tier. It is the unit
// the summary endpoint returns so the client never has to page progress rows to
// compute stars, completion %, the next milestone, or the secret-unlock gate.
type AchievementState struct {
	Key        string `json:"key"`
	Metric     string `json:"metric"`
	Secret     bool   `json:"secret,omitempty"`
	Tiers      []Tier `json:"tiers"`
	Progress   int    `json:"progress"`
	Stars      int    `json:"stars"`
	Unlocked   bool   `json:"unlocked"`
	Completed  bool   `json:"completed"`
	NextTarget *int   `json:"next_target"`
	MaxTarget  int    `json:"max_target"`
}

// SummaryTotals are the roll-ups the achievements page header needs, computed
// server-side over the whole catalog so they cannot be understated by a
// truncated client page.
type SummaryTotals struct {
	Revealed  int `json:"revealed"`
	Unlocked  int `json:"unlocked"`
	Completed int `json:"completed"`
	Stars     int `json:"stars"`
	MaxStars  int `json:"max_stars"`
}

// Summary is the full-state achievements response for one player and mode.
type Summary struct {
	Mode         string             `json:"mode"`
	Totals       SummaryTotals      `json:"totals"`
	Achievements []AchievementState `json:"achievements"`
}

// BuildSummary folds a player's progress rows over the whole catalog. A secret
// achievement whose progress has not reached its first tier is omitted
// entirely — the same gate Store.ListAchievements applies — so this never
// reveals a secret the player has not started; every other achievement is
// always present, unlocked or not.
func BuildSummary(mode string, progress []PlayerAchievementProgress) Summary {
	counts := make(map[string]int, len(progress))
	for _, row := range progress {
		counts[row.Key] = row.Count
	}
	out := Summary{Mode: mode, Achievements: make([]AchievementState, 0, len(Catalog))}
	for _, achievement := range Catalog {
		count := counts[achievement.Key]
		if achievement.Secret && count < minimumThreshold(achievement.Tiers) {
			continue
		}
		state := stateFor(achievement, count)
		out.Totals.Revealed++
		out.Totals.MaxStars += maxStars(achievement.Tiers)
		out.Totals.Stars += state.Stars
		if state.Unlocked {
			out.Totals.Unlocked++
		}
		if state.Completed {
			out.Totals.Completed++
		}
		out.Achievements = append(out.Achievements, state)
	}
	return out
}

func stateFor(achievement Achievement, count int) AchievementState {
	state := AchievementState{
		Key:      achievement.Key,
		Metric:   achievement.Metric,
		Secret:   achievement.Secret,
		Tiers:    achievement.Tiers,
		Progress: count,
	}
	for _, tier := range achievement.Tiers {
		if tier.Threshold > state.MaxTarget {
			state.MaxTarget = tier.Threshold
		}
		if count >= tier.Threshold {
			if tier.Stars > state.Stars {
				state.Stars = tier.Stars
			}
			continue
		}
		if state.NextTarget == nil || tier.Threshold < *state.NextTarget {
			threshold := tier.Threshold
			state.NextTarget = &threshold
		}
	}
	state.Unlocked = state.Stars > 0
	state.Completed = state.NextTarget == nil
	return state
}

func maxStars(tiers []Tier) int {
	most := 0
	for _, tier := range tiers {
		if tier.Stars > most {
			most = tier.Stars
		}
	}
	return most
}
