package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Palette mirrors the product's own tokens (ui/src/app/base.css): gold is the
// accent everywhere outside the felt, the table-felt green is the secondary
// surface color, and the brand red is reserved for errors/danger — never
// reused for chrome, since red is also the hearts suit color at the table.
var (
	colorGold   = lipgloss.Color("#e6b85c")
	colorFelt   = lipgloss.Color("#18765b")
	colorDanger = lipgloss.Color("#d9464d")
	colorMuted  = lipgloss.Color("245")
	colorDim    = lipgloss.Color("240")
)

var (
	titleStyle    = lipgloss.NewStyle().Foreground(colorGold).Bold(true)
	accentStyle   = lipgloss.NewStyle().Foreground(colorGold)
	promptStyle   = lipgloss.NewStyle().Foreground(colorGold).Bold(true)
	mutedStyle    = lipgloss.NewStyle().Foreground(colorMuted)
	dimStyle      = lipgloss.NewStyle().Foreground(colorDim)
	successStyle  = lipgloss.NewStyle().Foreground(colorFelt)
	errorStyle    = lipgloss.NewStyle().Foreground(colorDanger)
	selectedStyle = lipgloss.NewStyle().Foreground(colorGold).Bold(true)
	cursorGlyph   = accentStyle.Render("›")
)

// logo is the compact identity used when there isn't room for the full home
// header (and as a fallback before Bubble Tea reports the terminal size).
const logo = "♠ CTech Poker"

func renderLogo() string { return titleStyle.Render(logo) }

// renderHomeHeader gives the shell a compact Codex-like identity block while
// keeping the terminal, not decoration, as the main surface. It deliberately
// stays at three rows and collapses to one row on narrow terminals.
func renderHomeHeader(maxWidth int) string {
	width := terminalLineWidth(maxWidth)
	if width > 72 {
		width = 72
	}
	if width < 38 {
		return titleStyle.Render(truncateVisible(logo+" · sandbox", width))
	}

	brand := logo
	badge := "SANDBOX"
	left, right := "╭─ ", " ─╮"
	fixed := len([]rune(left)) + len([]rune(brand)) + len([]rune(badge)) + len([]rune(right)) + 2
	filler := width - fixed
	if filler < 1 {
		filler = 1
	}

	top := dimStyle.Render(left) +
		titleStyle.Render(brand) +
		dimStyle.Render(" "+strings.Repeat("─", filler)+" ") +
		successStyle.Bold(true).Render(badge) +
		dimStyle.Render(right)

	innerWidth := width - 2
	description := truncateVisible(" Poker no terminal · fichas virtuais · sem dinheiro real", innerWidth)
	description += strings.Repeat(" ", innerWidth-len([]rune(description)))
	middle := dimStyle.Render("│") + mutedStyle.Render(description) + dimStyle.Render("│")
	bottom := dimStyle.Render("╰" + strings.Repeat("─", innerWidth) + "╯")
	return strings.Join([]string{top, middle, bottom}, "\n")
}
