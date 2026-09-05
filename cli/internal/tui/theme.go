package tui

import "github.com/charmbracelet/lipgloss"

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

// logo is the small header shown once at the top of the home screen.
const logo = "CTech Poker CLI"

func renderLogo() string { return titleStyle.Render(logo) }
