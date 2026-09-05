// Package tui is the CTech Poker CLI's interactive shell — a single
// bubbletea program covering login, the `/command` home REPL, and (once
// wired) the lobby and table views.
package tui

import (
	"fmt"
	"strings"

	"gopkg.aoctech.app/poker/cli/internal/game"
	"gopkg.aoctech.app/poker/cli/internal/rest"
)

// FormatProfile renders a player profile for the /profile command.
func FormatProfile(p rest.Profile) string {
	name := p.Name
	if name == "" {
		name = "(sem nome)"
	}
	return fmt.Sprintf("%s\nfriend code: %s\nwallet mode: %s\nsandbox balance: %d\ngame balance: %d",
		name, p.FriendCode, p.WalletMode, p.SandboxBalance, p.GameBalance)
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
