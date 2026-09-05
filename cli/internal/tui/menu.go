package tui

import (
	"fmt"
	"strings"
)

// commandSpec is one entry in a slash-command menu: its name (with the
// leading "/"), an optional argument placeholder shown after the name, and a
// short description.
type commandSpec struct {
	Name string
	Args string
	Desc string
}

// commandMenu is a Claude-Code-style `/command` suggestion list: it narrows
// as the user types, Up/Down move the selection, Tab/Enter accept it.
type commandMenu struct {
	specs    []commandSpec
	items    []commandSpec
	selected int
	visible  bool
}

func newCommandMenu(specs []commandSpec) *commandMenu {
	return &commandMenu{specs: specs}
}

// UpdateInput recomputes the visible suggestion list from the current input
// value. The menu shows only while the user is still typing the command
// token itself — a space (moving on to arguments) or an exact single match
// hides it.
func (m *commandMenu) UpdateInput(value string) {
	if !strings.HasPrefix(value, "/") || strings.Contains(value, " ") {
		m.visible, m.items = false, nil
		return
	}
	var matches []commandSpec
	for _, s := range m.specs {
		if strings.HasPrefix(s.Name, value) {
			matches = append(matches, s)
		}
	}
	if len(matches) == 0 || (len(matches) == 1 && matches[0].Name == value) {
		m.visible, m.items = false, nil
		return
	}
	m.items = matches
	m.visible = true
	if m.selected >= len(m.items) {
		m.selected = 0
	}
}

func (m *commandMenu) movePrev() {
	if len(m.items) == 0 {
		return
	}
	m.selected = (m.selected - 1 + len(m.items)) % len(m.items)
}

func (m *commandMenu) moveNext() {
	if len(m.items) == 0 {
		return
	}
	m.selected = (m.selected + 1) % len(m.items)
}

// accept resolves the highlighted suggestion for Enter: value is what the
// input field should become, and submit is true when the command takes no
// arguments (so the caller can dispatch it immediately instead of waiting
// for another Enter).
func (m *commandMenu) accept() (value string, submit bool) {
	if len(m.items) == 0 {
		return "", false
	}
	item := m.items[m.selected]
	m.visible = false
	if item.Args == "" {
		return item.Name, true
	}
	return item.Name + " ", false
}

// fill resolves the highlighted suggestion for Tab: pure autocomplete, never
// submits — even a zero-argument command is only filled in, left for the
// user to confirm with Enter.
func (m *commandMenu) fill() string {
	if len(m.items) == 0 {
		return ""
	}
	item := m.items[m.selected]
	if item.Args == "" {
		return item.Name
	}
	return item.Name + " "
}

func (m *commandMenu) hide() { m.visible = false }

// maxMenuRows caps how many suggestions are shown at once even when the
// terminal has plenty of room, so a broad prefix (or the table's larger
// command set) never grows the menu unreasonably large.
const maxMenuRows = 8

// DesiredRows is how many lines View would render given unlimited room (but
// still capped at maxMenuRows, +1 for a "... and N more" line past that).
// The caller compares this against its own actual available space and, when
// there isn't enough, passes a smaller cap into View — never rendering more
// than the terminal has room for is the caller's job, not the menu's.
func (m *commandMenu) DesiredRows() int {
	if !m.visible || len(m.items) == 0 {
		return 0
	}
	if len(m.items) > maxMenuRows {
		return maxMenuRows + 1 // +1 for the "... and N more" line
	}
	return len(m.items)
}

// fallbackLineWidth is used when the caller doesn't know the real terminal
// width yet (no WindowSizeMsg has arrived). It's conservative — narrower
// than nearly any real terminal — specifically so a too-long line is never
// handed to the terminal before its true width is known.
const fallbackLineWidth = 76

// truncateVisible clamps s to at most max visible characters (runes, not
// bytes — this text is Portuguese with accented characters that are
// multi-byte in UTF-8 but one column wide), appending "…" if it had to cut.
// This is the actual fix for a real, reproduced bug: a menu line longer than
// the terminal's column count wraps onto a second physical row, but
// bubbletea's renderer counts rows by '\n' alone — it has no idea the
// terminal just wrapped one logical line into two, so its cursor-repositioning
// math for the *next* frame is off by that many rows. The visible result
// (reproduced live) was exactly what a misaligned repaint looks like: menu
// entries disappearing, the selection marker missing, both recoverable only
// by scrolling the terminal's own history — not a logic bug in matching or
// selection (both were already verified correct), a rendering desync caused
// by one specific line (a verbose /logout description, ~110 columns) being
// far longer than every other entry and than most real terminal widths.
func truncateVisible(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max == 1 {
		return string(r[:1])
	}
	return string(r[:max-1]) + "…"
}

// View renders the suggestion list within maxRows total lines (0 or a
// negative cap renders nothing) and maxWidth visible columns per line (0 or
// negative falls back to fallbackLineWidth) — the caller has already worked
// out how much room is actually available and must never be handed back
// more than that, on any terminal size.
func (m *commandMenu) View(maxRows, maxWidth int) string {
	if !m.visible || len(m.items) == 0 || maxRows <= 0 {
		return ""
	}
	if maxWidth <= 0 {
		maxWidth = fallbackLineWidth
	}
	limit := maxRows
	if limit > maxMenuRows {
		limit = maxMenuRows
	}
	items := m.items
	showCount := limit
	truncated := 0
	if len(items) > limit {
		showCount = limit - 1
		if showCount < 0 {
			showCount = 0
		}
		truncated = len(items) - showCount
	}
	items = items[:showCount]

	var b strings.Builder
	for i, it := range items {
		marker, style := "  ", mutedStyle
		if i == m.selected {
			marker, style = "› ", selectedStyle
		}
		name := it.Name
		if it.Args != "" {
			name += " " + it.Args
		}
		line := truncateVisible(fmt.Sprintf("%s%-28s %s", marker, name, it.Desc), maxWidth)
		b.WriteString(style.Render(line))
		b.WriteString("\n")
	}
	if truncated > 0 {
		_, _ = fmt.Fprintf(&b, "  … e mais %d\n", truncated)
	}
	return strings.TrimRight(b.String(), "\n")
}

// formatCommandList renders every spec as "name args  description", one per
// line, for /help — the full reference, unlike the narrowing suggestion
// menu. maxWidth is applied the same way View's is (0 or negative falls back
// to fallbackLineWidth) — see truncateVisible's doc comment for why this
// matters even here.
func formatCommandList(specs []commandSpec, maxWidth int) string {
	if maxWidth <= 0 {
		maxWidth = fallbackLineWidth
	}
	var b strings.Builder
	b.WriteString("comandos:")
	for _, it := range specs {
		name := it.Name
		if it.Args != "" {
			name += " " + it.Args
		}
		line := truncateVisible(fmt.Sprintf("  %-28s %s", name, it.Desc), maxWidth)
		b.WriteString("\n")
		b.WriteString(line)
	}
	return b.String()
}

var homeCommandSpecs = []commandSpec{
	{Name: "/achievements", Desc: "Mostra suas conquistas"},
	{Name: "/profile", Desc: "Mostra os dados do perfil"},
	{Name: "/play", Desc: "Entra numa mesa (escolhe tamanho/stake)"},
	{Name: "/enter", Args: "<room-id>", Desc: "Entra numa mesa por ID"},
	{Name: "/help", Desc: "Lista os comandos disponíveis"},
	{Name: "/clear", Desc: "Limpar comandos (CTRL + L)"},
	{Name: "/exit", Desc: "Sair"},
	{Name: "/logout", Desc: "Esquece as credenciais salvas"},
}

var tableCommandSpecs = []commandSpec{
	{Name: "/check", Desc: "Passa a vez sem apostar"},
	{Name: "/call", Desc: "Paga a aposta atual"},
	{Name: "/raise", Args: "<valor>", Desc: "Aumenta para o valor"},
	{Name: "/pot", Desc: "Aumenta o valor do pote"},
	{Name: "/allin", Desc: "Aposta todas as fichas"},
	{Name: "/fold", Desc: "Desiste da mão"},
	{Name: "/talk", Args: "<mensagem>", Desc: "Envia uma mensagem no chat"},
	{Name: "/react", Args: "<código> [jogador]", Desc: "Envia uma reação"},
	{Name: "/peek", Args: "[all|1|2]", Desc: "Espia suas cartas"},
	{Name: "/sitout", Desc: "Senta fora nas próximas mãos"},
	{Name: "/ready", Desc: "Volta a jogar"},
	{Name: "/summary", Desc: "Resumo da sessão atual"},
	{Name: "/last-winners", Desc: "Últimos vencedores"},
	{Name: "/share", Desc: "Copia o link de convite da mesa"},
	{Name: "/clear", Desc: "Limpa o log da mesa"},
	{Name: "/exit", Desc: "Sai da mesa"},
	{Name: "/exit!", Desc: "Sai da mesa sem avisar o servidor"},
	{Name: "/help", Desc: "Lista os comandos"},
}
