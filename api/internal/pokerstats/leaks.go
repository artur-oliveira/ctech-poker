package pokerstats

// Leak is a short, actionable tip for one metric currently outside a healthy
// range (#331). It reuses style.go's own thresholds — a metric only ever
// gets a leak tip on the same side StyleFor would badge it "explorer"
// (too loose) or the mirror-image "too passive"/"too passive facing raises"
// reading it doesn't currently badge at all.
type Leak struct {
	Metric string `json:"metric"`
	Tip    string `json:"tip"`
}

// Leaks derives at most one tip per metric, or nil below minHands — same
// sample floor as StyleFor, so a leak is never surfaced from noise.
func Leaks(s Stats, minHands int64) []Leak {
	if s.Hands < minHands {
		return nil
	}
	var leaks []Leak
	if s.VPIPRate >= .38 {
		leaks = append(leaks, Leak{Metric: "vpip_rate", Tip: "Você entra em mãos demais pré-flop — tente ser mais seletivo."})
	}
	if s.VPIPRate > 0 && s.PFRRate/s.VPIPRate < .5 {
		leaks = append(leaks, Leak{Metric: "pfr_rate", Tip: "Você entra em muitas mãos só pagando — prefira aumentar ao decidir jogar."})
	}
	if s.ThreeBetChances >= 10 && s.ThreeBetRate < .05 {
		leaks = append(leaks, Leak{Metric: "three_bet_rate", Tip: "Você quase nunca 3-beta — considere ser mais agressivo diante de um aumento."})
	}
	return leaks
}
