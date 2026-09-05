package game

import "github.com/charmbracelet/lipgloss"

// forceColorProfile makes lipgloss emit ANSI escapes regardless of whether
// the test process has a TTY, and returns a function that restores the
// previous profile.
func forceColorProfile() func() {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(2) // termenv.ANSI256
	return func() { lipgloss.SetColorProfile(prev) }
}
