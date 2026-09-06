package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// commandSpec is one entry in a slash-command menu: its name (with the
// leading "/"), an optional argument placeholder shown after the name, and a
// short description.
type commandSpec struct {
	Name   string
	Args   string
	Desc   string
	Hotkey string
}

// commandMenu is a Claude-Code-style `/command` suggestion list: it narrows
// as the user types, Up/Down move the selection, Tab/Enter accept it.
type commandMenu struct {
	specs []commandSpec
	// argFn, when set, supplies argument-level suggestions once the user has
	// typed past the command token (a space). It returns the choices plus the
	// input prefix to keep verbatim ahead of the token being completed; a nil
	// or empty result hides the menu.
	argFn    func(value string) (choices []commandSpec, prefix string)
	items    []commandSpec
	prefix   string // text before the completed token (arg mode); "" in command mode
	selected int
	visible  bool
}

func newCommandMenu(specs []commandSpec) *commandMenu {
	return &commandMenu{specs: specs}
}

// UpdateInput recomputes the visible suggestion list from the current input
// value. While the user is still typing the command token it narrows the
// command list; once they type a space it hands off to argFn (if set) for
// argument-level suggestions, and otherwise hides.
func (m *commandMenu) UpdateInput(value string) {
	previous := ""
	if m.selected >= 0 && m.selected < len(m.items) {
		previous = m.items[m.selected].Name
	}
	m.prefix = ""
	if !strings.HasPrefix(value, "/") || strings.Contains(value, " ") {
		if m.argFn != nil {
			if choices, prefix := m.argFn(value); len(choices) > 0 {
				// Collapse once the token is fully typed and nothing follows.
				if len(choices) == 1 && choices[0].Args == "" &&
					value == prefix+choices[0].Name {
					m.visible, m.items = false, nil
					return
				}
				m.items, m.prefix, m.visible = choices, prefix, true
				m.selected = 0
				for i, item := range m.items {
					if item.Name == previous {
						m.selected = i
						break
					}
				}
				return
			}
		}
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
	m.selected = 0
	for i, item := range m.items {
		if item.Name == previous {
			m.selected = i
			break
		}
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
		return m.prefix + item.Name, true
	}
	return m.prefix + item.Name + " ", false
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
		return m.prefix + item.Name
	}
	return m.prefix + item.Name + " "
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

// terminalLineWidth leaves the last terminal column unused. Some terminals
// auto-wrap as soon as that column is painted, which is enough to throw an
// inline renderer's cursor accounting off by one physical row.
func terminalLineWidth(maxWidth int) int {
	if maxWidth <= 0 {
		maxWidth = fallbackLineWidth
	}
	if maxWidth > 1 {
		return maxWidth - 1
	}
	return maxWidth
}

// truncateVisible clamps s to at most max terminal cells, preserving ANSI
// styling and grapheme clusters, and appends "…" if it had to cut.
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
	if ansi.StringWidth(s) <= max {
		return s
	}
	return ansi.Truncate(s, max, "…")
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
	maxWidth = terminalLineWidth(maxWidth)

	itemRows := maxRows
	if itemRows > maxMenuRows {
		itemRows = maxMenuRows
	}
	showSummary := len(m.items) > itemRows && itemRows > 1
	if showSummary {
		itemRows--
	}
	if itemRows > len(m.items) {
		itemRows = len(m.items)
	}

	// Keep the selected command inside the rendered window. This matters in
	// short terminals and after navigating a long list: the marker must never
	// disappear merely because the list is clipped.
	start := 0
	if m.selected >= itemRows {
		start = m.selected - itemRows + 1
	}
	if start+itemRows > len(m.items) {
		start = len(m.items) - itemRows
	}
	items := m.items[start : start+itemRows]

	var b strings.Builder
	for i, it := range items {
		absoluteIndex := start + i
		marker, style := "  ", mutedStyle
		if absoluteIndex == m.selected {
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
	if showSummary {
		before := start
		after := len(m.items) - (start + itemRows)
		summary := ""
		switch {
		case before > 0 && after > 0:
			summary = fmt.Sprintf("  ↑ %d anteriores · ↓ %d próximos", before, after)
		case before > 0:
			summary = fmt.Sprintf("  ↑ %d anteriores", before)
		default:
			summary = fmt.Sprintf("  ↓ %d próximos", after)
		}
		b.WriteString(dimStyle.Render(truncateVisible(summary, maxWidth)))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// formatCommandList renders every spec as "name args  description", one per
// line, for /help — the full reference, unlike the narrowing suggestion
// menu. maxWidth is applied the same way View's is (0 or negative falls back
// to fallbackLineWidth) — see truncateVisible's doc comment for why this
// matters even here.
func formatCommandList(specs []commandSpec, maxWidth int) string {
	maxWidth = terminalLineWidth(maxWidth)
	var b strings.Builder
	b.WriteString(titleStyle.Render("Comandos disponíveis"))
	for _, it := range specs {
		name := it.Name
		if it.Args != "" {
			name += " " + it.Args
		}
		line := truncateVisible(fmt.Sprintf("  %-28s %s", name, it.Desc), maxWidth)
		b.WriteString("\n")
		b.WriteString(line)
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(truncateVisible("  ↑↓ navegar · Tab completar · Esc fechar", maxWidth)))
	return b.String()
}

// prioritizeCommandSpecs keeps every command discoverable while moving the
// commands relevant to the current table state to the top of the `/` menu.
func prioritizeCommandSpecs(specs []commandSpec, names ...string) []commandSpec {
	byName := make(map[string]commandSpec, len(specs))
	for _, spec := range specs {
		byName[spec.Name] = spec
	}
	out := make([]commandSpec, 0, len(specs))
	seen := make(map[string]bool, len(specs))
	for _, name := range names {
		if spec, ok := byName[name]; ok && !seen[name] {
			out = append(out, spec)
			seen[name] = true
		}
	}
	for _, spec := range specs {
		if !seen[spec.Name] {
			out = append(out, spec)
		}
	}
	return out
}

// formatHotkeyHelp derives the table shortcut reference from the same command
// metadata the suggestion menu uses, so help cannot drift independently.
func formatHotkeyHelp(specs []commandSpec) string {
	groups := make([]string, 0, 5)
	seen := map[string]int{}
	for _, spec := range specs {
		if spec.Hotkey == "" {
			continue
		}
		if i, ok := seen[spec.Hotkey]; ok {
			groups[i] += " ou " + spec.Name
			continue
		}
		seen[spec.Hotkey] = len(groups)
		groups = append(groups, spec.Hotkey+" "+spec.Name)
	}
	return "atalhos: " + strings.Join(groups, " · ")
}

var homeCommandSpecs = []commandSpec{
	{Name: "/achievements", Desc: "Veja progresso, estrelas e conquistas"},
	{Name: "/profile", Desc: "Veja seu perfil, código e saldos"},
	{Name: "/play", Desc: "Escolha mesa, blinds e buy-in"},
	{Name: "/enter", Args: "<room-id>", Desc: "Entre em uma mesa pelo ID"},
	{Name: "/help", Desc: "Veja todos os comandos e atalhos"},
	{Name: "/clear", Desc: "Limpe o histórico desta tela"},
	{Name: "/exit", Desc: "Encerre o CTech Poker"},
	{Name: "/logout", Desc: "Esqueça as credenciais salvas"},
}

var tableCommandSpecs = []commandSpec{
	{Name: "/check", Desc: "Passa a vez sem apostar", Hotkey: "c"},
	{Name: "/call", Desc: "Paga a aposta atual", Hotkey: "c"},
	{Name: "/raise", Args: "<valor>", Desc: "Aumenta para o valor", Hotkey: "r"},
	{Name: "/pot", Desc: "Aumenta para o tamanho do pote", Hotkey: "p"},
	{Name: "/allin", Desc: "Aposta todas as fichas"},
	{Name: "/fold", Desc: "Desiste da mão", Hotkey: "f"},
	{Name: "/talk", Args: "<mensagem>", Desc: "Envia uma mensagem no chat"},
	{Name: "/react", Args: "<código> [jogador]", Desc: "Envia uma reação (Tab lista códigos e alvos)"},
	{Name: "/peek", Args: "[all|1|2]", Desc: "Mostra/esconde suas cartas", Hotkey: "k"},
	{Name: "/showcards", Args: "[all|1|2]", Desc: "Mostra suas cartas no showdown"},
	{Name: "/rit", Args: "<on|off>", Desc: "Liga/desliga run it twice"},
	{Name: "/rabbit", Desc: "Pede rabbit hunt após todos correrem"},
	{Name: "/reqcards", Desc: "Paga pra ver a mão vencedora muckada"},
	{Name: "/accept", Desc: "Aceita mostrar sua mão vencedora"},
	{Name: "/decline", Desc: "Recusa mostrar sua mão vencedora"},
	{Name: "/keep", Desc: "Mantém o assento após aviso de remoção"},
	{Name: "/postbb", Desc: "Paga o big blind pra voltar fora de vez"},
	{Name: "/preselect", Args: "<modo|off>", Desc: "Pré-seleciona sua ação antes da vez"},
	{Name: "/sitout", Desc: "Senta fora nas próximas mãos"},
	{Name: "/ready", Desc: "Volta a jogar"},
	{Name: "/summary", Desc: "Resumo da sessão atual"},
	{Name: "/last-winners", Desc: "Últimos vencedores"},
	{Name: "/share", Desc: "Copia o link de convite da mesa"},
	{Name: "/clear", Desc: "Limpa o log da mesa"},
	{Name: "/exit", Desc: "Pede pra sair; você sai ao fim da mão"},
	{Name: "/exit!", Desc: "Sai da mesa agora, sem avisar o servidor"},
	{Name: "/help", Desc: "Lista os comandos"},
}
