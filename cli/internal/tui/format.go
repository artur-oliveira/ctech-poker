// Package tui is the CTech Poker CLI's interactive shell — a single
// bubbletea program covering login, the `/command` home REPL, and (once
// wired) the lobby and table views.
package tui

import (
	"fmt"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"gopkg.aoctech.app/poker/cli/internal/game"
	"gopkg.aoctech.app/poker/cli/internal/rest"
)

// FormatProfile renders a player profile for the /profile command.
func FormatProfile(p rest.Profile) string {
	name := p.Name
	if name == "" {
		name = "(sem nome)"
	}
	if p.WalletMode == "sandbox" {
		pGer := message.NewPrinter(language.BrazilianPortuguese)
		return fmt.Sprintf(
			"%s\nCódigo de convite: %s\nModo de jogo: %s\nFichas: %s",
			name,
			p.FriendCode,
			p.WalletMode,
			pGer.Sprintf("%d", p.SandboxBalance),
		)
	}
	return fmt.Sprintf(
		"%s\nCódigo de convite: %s\nModo de jogo: %s\nFichas: %d\nDinheiro: R$ %d",
		name,
		p.FriendCode,
		p.WalletMode,
		p.SandboxBalance,
		p.GameBalance,
	)
}

// FormatAchievements renders the achievement summary for /achievements.
func FormatAchievements(s rest.AchievementSummary) string {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "%d/%d desbloqueadas, %d completas, %d/%d estrelas",
		s.Totals.Unlocked, s.Totals.Revealed, s.Totals.Completed, s.Totals.Stars, s.Totals.MaxStars)
	for _, a := range s.Achievements {
		mark := " "
		if a.Completed {
			mark = "✓"
		} else if a.Unlocked {
			mark = "•"
		}
		label := game.AchievementLabel(a.Key)
		_, _ = fmt.Fprintf(&b, "\n[%s] %-26s %d★  progresso %d", mark, label, a.Stars, a.Progress)
		if desc := game.AchievementDescription(a.Key); desc != "" {
			_, _ = fmt.Fprintf(&b, "\n      %s", dimStyle.Render(desc))
		}
	}
	return b.String()
}

// presenceLabel renders a rest.SocialPlayer.Presence value in Portuguese;
// empty (not resolved for this list) and unrecognized values fall back to
// "offline" so a stale/older server field never surfaces as raw JSON.
func presenceLabel(presence string) string {
	switch presence {
	case "online":
		return "online"
	case "in_table":
		return "na mesa"
	default:
		return "offline"
	}
}

// FormatSocialPlayers renders one friends/friend-requests/blocked/recent list
// for its command (/friends, /requests, /blocked, /recent).
func FormatSocialPlayers(title string, players []rest.SocialPlayer) string {
	if len(players) == 0 {
		return title + ": nenhum jogador"
	}
	var b strings.Builder
	b.WriteString(title)
	for _, p := range players {
		name := p.Name
		if name == "" {
			name = p.PlayerID
		}
		_, _ = fmt.Fprintf(&b, "\n  %-20s", name)
		if p.Presence != "" {
			b.WriteString(" · " + presenceLabel(p.Presence))
		}
		if p.HandsTogether > 0 {
			_, _ = fmt.Fprintf(&b, " · %d mão(s) juntos", p.HandsTogether)
		}
		if p.RoomID != "" {
			b.WriteString(" · jogando agora (/enter " + p.RoomID + ")")
		}
	}
	return b.String()
}

// inboxEventLabel renders a SocialInboxEvent.Type in Portuguese.
func inboxEventLabel(eventType string) string {
	switch eventType {
	case "friend_request":
		return "pedido de amizade"
	case "friend_accepted":
		return "aceitou seu pedido de amizade"
	case "table_invite":
		return "convite de mesa"
	default:
		return eventType
	}
}

// FormatInbox renders the social inbox for /inbox.
func FormatInbox(events []rest.SocialInboxEvent) string {
	if len(events) == 0 {
		return "Caixa de entrada: nenhum evento"
	}
	var b strings.Builder
	b.WriteString("Caixa de entrada")
	for _, e := range events {
		who := e.ActorName
		if who == "" {
			who = e.ActorPlayerID
		}
		mark := " "
		if e.Unread {
			mark = "•"
		}
		_, _ = fmt.Fprintf(&b, "\n[%s] %s — %s", mark, who, inboxEventLabel(e.Type))
		if e.RoomID != "" {
			b.WriteString(" (/enter " + e.RoomID + ")")
		}
	}
	return b.String()
}
