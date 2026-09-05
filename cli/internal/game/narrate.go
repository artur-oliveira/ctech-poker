package game

import (
	"fmt"
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

	total := map[string]int64{}
	for _, pr := range s.PotResults {
		for _, w := range pr.WinnerPlayerIds {
			total[w] += pr.PayoutAmount
		}
	}
	winnersThisHand := s.Winners
	if len(winnersThisHand) == 0 {
		for w := range total {
			winnersThisHand = append(winnersThisHand, w)
		}
	}
	for _, w := range winnersThisHand {
		if amt := total[w]; amt > 0 {
			out = append(out, fmt.Sprintf("  %s vence %d", n.label(w), amt))
		} else {
			out = append(out, fmt.Sprintf("  %s vence", n.label(w)))
		}
		n.winners = append(n.winners, fmt.Sprintf("%s (+%d)", n.label(w), total[w]))
	}
	return out
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
