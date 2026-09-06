// Package tui is the CTech Poker CLI's interactive shell — a single
// bubbletea program covering login, the `/command` home REPL, and (once
// wired) the lobby and table views.
package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"gopkg.aoctech.app/poker/cli/internal/game"
	"gopkg.aoctech.app/poker/cli/internal/rest"
)

// FormatProfile renders a player profile for the /profile command.
func FormatProfile(p rest.Profile) string {
	return FormatProfileWidth(p, fallbackLineWidth)
}

// FormatProfileWidth renders the profile as a compact, width-safe player
// ledger. The terminal owns the canvas; rules describe the identity region
// without pretending that it is a GUI card.
func FormatProfileWidth(p rest.Profile, maxWidth int) string {
	maxWidth = terminalLineWidth(maxWidth)
	if maxWidth > 58 {
		maxWidth = 58
	}
	if maxWidth < 1 {
		return ""
	}

	name := p.Name
	if name == "" {
		name = "(sem nome)"
	}
	mode := p.WalletMode
	if mode == "" || mode == "sandbox" {
		mode = "sandbox"
	}
	code := p.FriendCode
	if code == "" {
		code = "ainda não disponível"
	}
	pGer := message.NewPrinter(language.BrazilianPortuguese)

	heading := "♠ " + name
	if maxWidth >= 38 {
		badge := mode
		room := maxWidth - visibleWidth(heading) - visibleWidth(badge)
		if room >= 2 {
			heading = titleStyle.Render(heading) + strings.Repeat(" ", room) + successStyle.Bold(true).Render(badge)
		} else {
			heading = titleStyle.Render(truncateVisible(heading+" · "+mode, maxWidth))
		}
	} else {
		heading = titleStyle.Render(truncateVisible(heading+" · "+mode, maxWidth))
	}

	rule := dimStyle.Render(strings.Repeat("─", maxWidth))
	lines := []string{
		heading,
		rule,
		profileField("Fichas", pGer.Sprintf("%d", p.SandboxBalance), maxWidth),
	}
	if mode != "sandbox" {
		lines = append(lines, profileField("Dinheiro", pGer.Sprintf("R$ %d", p.GameBalance), maxWidth))
	}
	lines = append(lines,
		profileField("Convite", code, maxWidth),
		mutedStyle.Render(truncateVisible("Compartilhe o código com quem vai sentar à mesa.", maxWidth)),
		rule,
		accentStyle.Render(truncateVisible("→ /achievements  Veja sua jornada e os próximos marcos", maxWidth)),
	)
	return strings.Join(lines, "\n")
}

// FormatAchievements renders the achievement summary for /achievements.
func FormatAchievements(s rest.AchievementSummary) string {
	return FormatAchievementsWidth(s, fallbackLineWidth)
}

// FormatAchievementsWidth renders a responsive career ledger: progress and
// the closest useful milestone come first, followed by scannable state groups.
// Secret achievements stay secret until the server marks them unlocked.
func FormatAchievementsWidth(s rest.AchievementSummary, maxWidth int) string {
	maxWidth = terminalLineWidth(maxWidth)
	if maxWidth < 1 {
		return ""
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render(truncateVisible("Conquistas", maxWidth)))
	b.WriteByte('\n')
	starsLabel := fmt.Sprintf("★ %d/%d estrelas", s.Totals.Stars, s.Totals.MaxStars)
	starsPercent := percent(s.Totals.Stars, s.Totals.MaxStars)
	barWidth := maxWidth - visibleWidth(starsLabel) - 8
	if barWidth > 18 {
		barWidth = 18
	}
	if barWidth >= 6 {
		topLine := fmt.Sprintf("%s  %s %3d%%", accentStyle.Bold(true).Render(starsLabel), progressBar(starsPercent, barWidth), starsPercent)
		b.WriteString(truncateVisible(topLine, maxWidth))
	} else {
		topLine := fmt.Sprintf("%s · %d%%", accentStyle.Bold(true).Render(starsLabel), starsPercent)
		b.WriteString(truncateVisible(topLine, maxWidth))
	}
	b.WriteByte('\n')
	summaryLine := fmt.Sprintf("%d desbloqueadas · %d reveladas · %s",
		s.Totals.Unlocked, s.Totals.Revealed, completedCountLabel(s.Totals.Completed))
	if maxWidth < 42 {
		summaryLine = fmt.Sprintf("%d/%d desbloq. · %s",
			s.Totals.Unlocked, s.Totals.Revealed, completedCountLabel(s.Totals.Completed))
	}
	b.WriteString(truncateVisible(summaryLine, maxWidth))

	var active, available, completed []rest.Achievement
	secretCount := 0
	for _, a := range s.Achievements {
		switch {
		case a.Secret && !a.Unlocked:
			secretCount++
		case a.Completed:
			completed = append(completed, a)
		case a.Progress > 0 || a.Unlocked:
			active = append(active, a)
		default:
			available = append(available, a)
		}
	}
	sort.SliceStable(active, func(i, j int) bool {
		pi, pj := achievementPercent(active[i]), achievementPercent(active[j])
		if pi != pj {
			return pi > pj
		}
		return game.AchievementLabel(active[i].Key) < game.AchievementLabel(active[j].Key)
	})
	sortAchievementsByLabel(available)
	sortAchievementsByLabel(completed)

	nextCandidates := active
	if len(nextCandidates) == 0 {
		nextCandidates = available
	}
	if next, ok := closestAchievement(nextCandidates); ok {
		b.WriteByte('\n')
		nextPrefix := "Próximo marco: "
		if maxWidth < 42 {
			nextPrefix = "Próximo: "
		}
		b.WriteString(accentStyle.Render(truncateVisible(nextPrefix+game.AchievementLabel(next.Key)+" · "+remainingLabel(next), maxWidth)))
	}

	if len(active)+len(available)+len(completed) == 0 && secretCount == 0 {
		b.WriteString("\n\n")
		b.WriteString(mutedStyle.Render(truncateVisible("Sua jornada começa na primeira mão.", maxWidth)))
		return b.String()
	}

	writeAchievementGroup(&b, "Em andamento", active, maxWidth, true)
	writeAchievementGroup(&b, "Disponíveis", available, maxWidth, false)
	writeAchievementGroup(&b, "Completas", completed, maxWidth, false)
	if secretCount > 0 {
		b.WriteString("\n\n")
		b.WriteString(titleStyle.Render(truncateVisible(fmt.Sprintf("Por descobrir · %d", secretCount), maxWidth)))
		b.WriteByte('\n')
		for i, line := range wrapPlainText("Continue jogando: algumas histórias só aparecem na mesa.", maxWidth) {
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(mutedStyle.Render(line))
		}
	}
	return b.String()
}

func completedCountLabel(count int) string {
	if count == 1 {
		return "1 completa"
	}
	return fmt.Sprintf("%d completas", count)
}

func profileField(label, value string, maxWidth int) string {
	return truncateVisible(label+": "+value, maxWidth)
}

func percent(current, target int) int {
	if target <= 0 || current <= 0 {
		return 0
	}
	if current >= target {
		return 100
	}
	return int(float64(current) / float64(target) * 100)
}

func achievementTarget(a rest.Achievement) int {
	if a.NextTarget != nil && *a.NextTarget > 0 {
		return *a.NextTarget
	}
	if a.MaxTarget > 0 {
		return a.MaxTarget
	}
	if a.Progress > 0 {
		return a.Progress
	}
	return 1
}

func achievementPercent(a rest.Achievement) int {
	return percent(a.Progress, achievementTarget(a))
}

func remainingLabel(a rest.Achievement) string {
	remaining := achievementTarget(a) - a.Progress
	if remaining < 1 {
		return "marco alcançado"
	}
	return fmt.Sprintf("faltam %d", remaining)
}

func progressBar(pct, width int) string {
	if width < 1 {
		return ""
	}
	filled := (pct*width + 50) / 100
	if filled > width {
		filled = width
	}
	return successStyle.Render(strings.Repeat("█", filled)) + dimStyle.Render(strings.Repeat("░", width-filled))
}

func sortAchievementsByLabel(items []rest.Achievement) {
	sort.SliceStable(items, func(i, j int) bool {
		return game.AchievementLabel(items[i].Key) < game.AchievementLabel(items[j].Key)
	})
}

func closestAchievement(items []rest.Achievement) (rest.Achievement, bool) {
	if len(items) == 0 {
		return rest.Achievement{}, false
	}
	return items[0], true
}

func writeAchievementGroup(b *strings.Builder, title string, items []rest.Achievement, maxWidth int, showProgress bool) {
	if len(items) == 0 {
		return
	}
	b.WriteString("\n\n")
	b.WriteString(titleStyle.Render(truncateVisible(fmt.Sprintf("%s · %d", title, len(items)), maxWidth)))
	for _, a := range items {
		b.WriteByte('\n')
		mark := "○"
		style := accentStyle
		if a.Completed {
			mark = "✓"
			style = successStyle
		}
		label := game.AchievementLabel(a.Key)
		meta := fmt.Sprintf("%d★", a.Stars)
		if !showProgress && !a.Completed {
			meta = fmt.Sprintf("objetivo %d", achievementTarget(a))
		}
		line := fmt.Sprintf("%s %s · %s", mark, label, meta)
		b.WriteString(style.Render(truncateVisible(line, maxWidth)))
		if showProgress {
			p := achievementPercent(a)
			barWidth := maxWidth - 22
			if barWidth > 18 {
				barWidth = 18
			}
			if barWidth >= 6 {
				progressLine := fmt.Sprintf("  %s %d/%d · %s", progressBar(p, barWidth), a.Progress, achievementTarget(a), remainingLabel(a))
				b.WriteByte('\n')
				b.WriteString(truncateVisible(progressLine, maxWidth))
			} else {
				progressLine := fmt.Sprintf("  %d/%d · %s", a.Progress, achievementTarget(a), remainingLabel(a))
				b.WriteByte('\n')
				b.WriteString(truncateVisible(progressLine, maxWidth))
			}
		}
		if desc := game.AchievementDescription(a.Key); desc != "" {
			for _, line := range wrapPlainText(desc, maxWidth-2) {
				b.WriteString("\n  ")
				b.WriteString(dimStyle.Render(line))
			}
		}
	}
}

func wrapPlainText(text string, width int) []string {
	if width < 1 {
		return nil
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	lines := []string{truncateVisible(words[0], width)}
	for _, word := range words[1:] {
		last := len(lines) - 1
		if visibleWidth(lines[last])+1+visibleWidth(word) <= width {
			lines[last] += " " + word
		} else {
			lines = append(lines, truncateVisible(word, width))
		}
	}
	return lines
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
	return FormatSocialPlayersPage("social", title, rest.Page[rest.SocialPlayer]{Data: players}, 1, fallbackLineWidth, "")
}

// FormatSocialPlayersPage renders one cursor page as a compact people ledger.
// kind supplies color-independent state marks and contextual empty guidance.
func FormatSocialPlayersPage(kind, title string, page rest.Page[rest.SocialPlayer], pageNumber, maxWidth int, command string) string {
	maxWidth = terminalLineWidth(maxWidth)
	if maxWidth < 1 {
		return ""
	}
	players := page.Data
	if len(players) == 0 {
		hint := "Quando houver alguém aqui, a lista aparecerá nesta tela."
		switch kind {
		case "friends":
			hint = "Compartilhe seu código de convite para encontrar companhia."
		case "incoming":
			hint = "Quando alguém adicionar você, o pedido aparecerá aqui."
		case "outgoing":
			hint = "Nenhuma solicitação enviada está pendente."
		case "blocked":
			hint = "Você não bloqueou ninguém."
		case "recent":
			hint = "Depois de uma mão, os adversários aparecem aqui por 90 dias."
		}
		lines := []string{
			titleStyle.Render(truncateVisible(title, maxWidth)),
			mutedStyle.Render(truncateVisible("Nenhum jogador nesta lista.", maxWidth)),
		}
		for _, line := range wrapPlainText(hint, maxWidth) {
			lines = append(lines, mutedStyle.Render(line))
		}
		if command != "" && pageNumber > 1 {
			lines = append(lines, dimStyle.Render(truncateVisible("Página anterior: "+command+" prev", maxWidth)))
		}
		return strings.Join(lines, "\n")
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render(truncateVisible(fmt.Sprintf("%s · página %d", title, pageNumber), maxWidth)))
	_, _ = fmt.Fprintf(&b, "\n%d %s nesta página", len(players), plural(len(players), "pessoa", "pessoas"))
	for _, p := range players {
		name := p.Name
		if name == "" {
			name = shortID(p.PlayerID)
		}
		mark := "○"
		style := mutedStyle
		switch kind {
		case "blocked":
			mark = "×"
		case "incoming":
			mark, style = "?", accentStyle
		case "outgoing":
			mark = "→"
		default:
			switch p.Presence {
			case "in_table":
				mark, style = "▶", accentStyle
			case "online":
				mark, style = "●", successStyle
			}
		}
		line := mark + " " + name
		if p.Presence != "" {
			line += " · " + presenceLabel(p.Presence)
		}
		if p.Muted {
			line += " · silenciado"
		}
		if p.Blocked {
			line += " · bloqueado"
		}
		b.WriteByte('\n')
		b.WriteString(style.Render(truncateVisible(line, maxWidth)))
		details := []string{}
		if p.HandsTogether > 0 {
			details = append(details, fmt.Sprintf("%d %s juntos", p.HandsTogether, plural(int(p.HandsTogether), "mão", "mãos")))
		}
		if p.LastPlayedAt > 0 {
			details = append(details, "última mesa "+socialDateLabel(p.LastPlayedAt))
		}
		if p.RoomID != "" {
			details = append(details, "jogando agora · /enter "+p.RoomID)
		}
		if len(details) > 0 {
			b.WriteString("\n")
			b.WriteString(dimStyle.Render(truncateVisible("  "+strings.Join(details, " · "), maxWidth)))
		}
	}
	if command != "" && (page.HasNext || pageNumber > 1) {
		b.WriteString("\n")
		parts := []string{}
		if pageNumber > 1 {
			parts = append(parts, command+" prev")
		}
		if page.HasNext {
			parts = append(parts, command+" next")
		}
		b.WriteString(dimStyle.Render(truncateVisible("Páginas: "+strings.Join(parts, " · "), maxWidth)))
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
	return FormatInboxPage(rest.Page[rest.SocialInboxEvent]{Data: events}, 1, fallbackLineWidth, "")
}

func FormatInboxPage(page rest.Page[rest.SocialInboxEvent], pageNumber, maxWidth int, command string) string {
	maxWidth = terminalLineWidth(maxWidth)
	events := page.Data
	if len(events) == 0 {
		lines := []string{
			titleStyle.Render(truncateVisible("Atividades", maxWidth)),
			mutedStyle.Render(truncateVisible("Nenhuma atividade social por aqui.", maxWidth)),
		}
		if command != "" && pageNumber > 1 {
			lines = append(lines, dimStyle.Render(truncateVisible("Página anterior: "+command+" prev", maxWidth)))
		}
		return strings.Join(lines, "\n")
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render(truncateVisible(fmt.Sprintf("Atividades · página %d", pageNumber), maxWidth)))
	for _, e := range events {
		who := e.ActorName
		if who == "" {
			who = e.ActorPlayerID
		}
		mark := "○"
		if e.Unread {
			mark = "●"
		}
		line := fmt.Sprintf("%s %s · %s", mark, who, inboxEventLabel(e.Type))
		b.WriteByte('\n')
		if e.Unread {
			b.WriteString(accentStyle.Render(truncateVisible(line, maxWidth)))
		} else {
			b.WriteString(truncateVisible(line, maxWidth))
		}
		detail := "  " + socialDateLabel(e.CreatedAt)
		if e.RoomID != "" {
			detail += " · /enter " + e.RoomID
		}
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(truncateVisible(detail, maxWidth)))
	}
	if command != "" && (page.HasNext || pageNumber > 1) {
		b.WriteString("\n")
		parts := []string{}
		if pageNumber > 1 {
			parts = append(parts, command+" prev")
		}
		if page.HasNext {
			parts = append(parts, command+" next")
		}
		b.WriteString(dimStyle.Render(truncateVisible("Páginas: "+strings.Join(parts, " · "), maxWidth)))
	}
	return b.String()
}

func socialDateLabel(ms int64) string {
	if ms <= 0 {
		return "data desconhecida"
	}
	date := time.UnixMilli(ms).Local()
	now := time.Now().Local()
	days := int(now.Sub(date).Hours() / 24)
	switch {
	case date.Format("2006-01-02") == now.Format("2006-01-02"):
		return "hoje às " + date.Format("15:04")
	case days <= 1:
		return "ontem às " + date.Format("15:04")
	default:
		return date.Format("02/01/2006")
	}
}
