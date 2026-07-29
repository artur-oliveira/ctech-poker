package pokerstats

// Badge is a public, qualitative label derived from private aggregate stats.
// It deliberately exposes no exact rates: the label is comparable to what an
// attentive opponent can infer at the table, while the underlying numbers are
// not.
type Badge struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Reason string `json:"reason"`
}

const (
	// MinHandsPublic is the sample floor before a badge may be shown to other
	// players. Smaller samples are too noisy to present as opponent information.
	MinHandsPublic int64 = 200
	// MinHandsSelf lets the owner see an explicitly provisional reading sooner.
	MinHandsSelf int64 = 30
)

var styles = []struct {
	Badge Badge
	Match func(Stats) bool
}{
	{
		Badge: Badge{Key: "selective", Label: "Seletivo", Reason: "VPIP de até 22%"},
		Match: func(s Stats) bool { return s.VPIPRate <= .22 },
	},
	{
		Badge: Badge{Key: "explorer", Label: "Explorador", Reason: "VPIP a partir de 38%"},
		Match: func(s Stats) bool { return s.VPIPRate >= .38 },
	},
	{
		Badge: Badge{Key: "initiative", Label: "Iniciativa", Reason: "PFR representa pelo menos 70% do VPIP"},
		Match: func(s Stats) bool { return s.VPIPRate > 0 && s.PFRRate/s.VPIPRate >= .7 },
	},
	{
		Badge: Badge{Key: "counter", Label: "Contra-ataque", Reason: "3-bet de pelo menos 10%"},
		Match: func(s Stats) bool { return s.ThreeBetChances >= 10 && s.ThreeBetRate >= .1 },
	},
}

var balancedBadge = Badge{
	Key: "balanced", Label: "Equilibrado", Reason: "Sem tendência dominante nesta amostra",
}

// StyleFor returns at most three ordered badges, or nil below minHands.
func StyleFor(s Stats, minHands int64) []Badge {
	if s.Hands < minHands {
		return nil
	}
	badges := make([]Badge, 0, 3)
	for _, style := range styles {
		if style.Match(s) {
			badges = append(badges, style.Badge)
			if len(badges) == 3 {
				break
			}
		}
	}
	if len(badges) == 0 {
		return []Badge{balancedBadge}
	}
	return badges
}
