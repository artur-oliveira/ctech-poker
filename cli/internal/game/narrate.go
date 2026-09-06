package game

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.aoctech.app/poker/cli/internal/proto"
)

// Narrator turns a stream of table snapshots (and out-of-band server
// messages) into human narration lines, by diffing each snapshot against the
// last.
type Narrator struct {
	youID string
	mode  CardMode

	haveStage bool
	stage     string
	contrib   map[string]int64
	folded    map[string]bool
	names     map[string]string

	winners []string // newest last
}

// NewNarrator starts a narrator for viewer youID. Card rendering follows
// mode (CardColor unless the caller sets it otherwise).
func NewNarrator(youID string) *Narrator {
	return &Narrator{
		youID:   youID,
		mode:    CardColor,
		contrib: map[string]int64{},
		folded:  map[string]bool{},
		names:   map[string]string{},
	}
}

// WithCardMode sets how showdown / board cards render.
func (n *Narrator) WithCardMode(m CardMode) *Narrator { n.mode = m; return n }

func (n *Narrator) label(id string) string {
	if id == n.youID {
		return "você"
	}
	if name := n.names[id]; name != "" {
		return name
	}
	return id
}

// OnSnapshot diffs s against the previous snapshot and returns the narration
// it implies: street changes, per-player contributions and folds, and the
// showdown result when the hand completes.
func (n *Narrator) OnSnapshot(s *proto.TableSnapshot) []string {
	var out []string

	for _, seat := range s.Seats {
		if seat.Name != "" {
			n.names[seat.PlayerId] = seat.Name
		}
	}

	stage := strings.ToLower(s.Stage)

	// Player actions within the same street.
	if n.haveStage && stage == n.stage && stage != "complete" {
		for _, seat := range s.Seats {
			id := seat.PlayerId
			if seat.State == "folded" && !n.folded[id] {
				out = append(out, fmt.Sprintf("  %s desiste", n.label(id)))
			}
			if delta := seat.Contributed - n.contrib[id]; delta > 0 {
				out = append(out, fmt.Sprintf("  %s aposta %d", n.label(id), delta))
			}
		}
	}

	// Street transition.
	if !n.haveStage || stage != n.stage {
		switch stage {
		case "flop", "turn", "river":
			out = append(out, fmt.Sprintf("  — %s — %s", strings.ToUpper(stage), FormatCards(s.Board, n.mode)))
		case "complete":
			out = append(out, n.showdownLines(s)...)
		}
	}

	// Snapshot the new state.
	n.haveStage = true
	n.stage = stage
	n.contrib = map[string]int64{}
	n.folded = map[string]bool{}
	for _, seat := range s.Seats {
		n.contrib[seat.PlayerId] = seat.Contributed
		n.folded[seat.PlayerId] = seat.State == "folded"
	}
	return out
}

func (n *Narrator) showdownLines(s *proto.TableSnapshot) []string {
	var out []string
	if len(s.Board) > 0 {
		out = append(out, "  showdown: "+FormatCards(s.Board, n.mode))
	}
	for _, seat := range s.Seats {
		if len(seat.HoleCards) == 2 {
			out = append(out, fmt.Sprintf("  %s mostra %s (%s)",
				n.label(seat.PlayerId), FormatCards(seat.HoleCards, n.mode),
				HandStrength(seat.HoleCards, s.Board)))
		}
	}

	// Per-player winnings, contested pots only (refunds are not "winning").
	total := map[string]int64{}
	for _, pr := range s.PotResults {
		if pr.Refund {
			continue
		}
		if len(pr.Payouts) > 0 {
			for w, amt := range pr.Payouts {
				total[w] += amt
			}
		} else {
			for _, w := range pr.WinnerPlayerIds {
				total[w] += pr.PayoutAmount
			}
		}
	}

	if detail := n.potBreakdownLines(s.PotResults); len(detail) > 0 {
		out = append(out, detail...)
	} else {
		// Server sent no pot detail — collapsed winner lines.
		winnersThisHand := s.Winners
		if len(winnersThisHand) == 0 {
			for w := range total {
				winnersThisHand = append(winnersThisHand, w)
			}
			sort.Strings(winnersThisHand)
		}
		for _, w := range winnersThisHand {
			if amt := total[w]; amt > 0 {
				out = append(out, fmt.Sprintf("  %s vence %d", n.label(w), amt))
			} else {
				out = append(out, fmt.Sprintf("  %s vence", n.label(w)))
			}
		}
	}

	// History: one "Name (+total)" per winner, stable order.
	histWinners := s.Winners
	if len(histWinners) == 0 {
		for w := range total {
			histWinners = append(histWinners, w)
		}
		sort.Strings(histWinners)
	}
	for _, w := range histWinners {
		n.winners = append(n.winners, fmt.Sprintf("%s (+%d)", n.label(w), total[w]))
	}
	return out
}

// potBreakdownLines renders each pot layer from the server's PotResults. A
// single uncontested pot with no refund collapses to one "Name vence N" line;
// anything with side pots, refunds, or a run-it-twice runout gets a labeled
// breakdown ("pote principal", "pote lateral 1", "devolvido"). Returns nil
// when the server sent no PotResults at all.
func (n *Narrator) potBreakdownLines(prs []*proto.PotResult) []string {
	if len(prs) == 0 {
		return nil
	}
	contested, refunds := 0, 0
	runouts := map[int32]bool{}
	for _, pr := range prs {
		if pr.Refund {
			refunds++
		} else {
			contested++
		}
		runouts[pr.Runout] = true
	}
	multiRunout := len(runouts) > 1 || runouts[2]

	if contested <= 1 && refunds == 0 && !multiRunout {
		pr := prs[0]
		return []string{fmt.Sprintf("  %s vence %d", n.payoutParts(pr), pr.PayoutAmount)}
	}

	out := make([]string, 0, len(prs))
	layerByRunout := map[int32]int{}
	for _, pr := range prs {
		who := n.payoutParts(pr)
		if pr.Refund {
			out = append(out, fmt.Sprintf("  devolvido %d · %s", pr.PayoutAmount, who))
			continue
		}
		l := layerByRunout[pr.Runout]
		layerByRunout[pr.Runout]++
		label := "pote principal"
		if l > 0 {
			label = fmt.Sprintf("pote lateral %d", l)
		}
		if multiRunout && pr.Runout > 0 {
			label += fmt.Sprintf(" (corrida %d)", pr.Runout)
		}
		out = append(out, fmt.Sprintf("  %s %d · %s", label, pr.PayoutAmount, who))
	}
	return out
}

// payoutParts names who collected a pot layer and how much (the amount only
// when it splits between players), preferring the exact Payouts map and
// falling back to the winner/eligible id lists.
func (n *Narrator) payoutParts(pr *proto.PotResult) string {
	ids := make([]string, 0, len(pr.Payouts))
	for id := range pr.Payouts {
		ids = append(ids, id)
	}
	if len(ids) > 0 {
		sort.Slice(ids, func(i, j int) bool {
			if pr.Payouts[ids[i]] != pr.Payouts[ids[j]] {
				return pr.Payouts[ids[i]] > pr.Payouts[ids[j]]
			}
			return n.label(ids[i]) < n.label(ids[j])
		})
		parts := make([]string, len(ids))
		for i, id := range ids {
			if len(ids) == 1 {
				parts[i] = n.label(id)
			} else {
				parts[i] = fmt.Sprintf("%s %d", n.label(id), pr.Payouts[id])
			}
		}
		return strings.Join(parts, " e ")
	}
	fallback := pr.WinnerPlayerIds
	if len(fallback) == 0 {
		fallback = pr.EligiblePlayerIds
	}
	if len(fallback) == 0 {
		return "sem vencedor"
	}
	parts := make([]string, len(fallback))
	for i, w := range fallback {
		parts[i] = n.label(w)
	}
	return strings.Join(parts, " e ")
}

// OnMessage narrates one out-of-band server message (chat, reaction,
// achievement unlock, removal). Returns nil for message types it doesn't
// narrate.
func (n *Narrator) OnMessage(m *proto.ServerMessage) []string {
	switch m.Type {
	case "chat":
		return []string{fmt.Sprintf("%s: %s", n.label(m.PlayerId), m.Message)}
	case "reaction":
		target := ""
		if m.TargetPlayerId != "" {
			target = " → " + n.label(m.TargetPlayerId)
		}
		return []string{fmt.Sprintf("%s reagiu %s%s", n.label(m.PlayerId), m.ReactionId, target)}
	case "achievement_unlocked":
		return []string{fmt.Sprintf("conquista: %s (+%d★)", m.Key, m.Stars)}
	case "removed":
		return []string{fmt.Sprintf("você saiu da mesa (%s) — banca %d", m.Message, m.Amount)}
	case "table_migrating":
		note := m.Text
		if note == "" {
			note = "esta mesa está migrando de servidor — a conexão será restabelecida automaticamente"
		}
		return []string{note}
	default:
		return nil
	}
}

// LastWinners returns up to n recent hand winners, newest first.
func (n *Narrator) LastWinners(count int) []string {
	out := []string{}
	for i := len(n.winners) - 1; i >= 0 && len(out) < count; i-- {
		out = append(out, n.winners[i])
	}
	return out
}
