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

// accept resolves the highlighted suggestion: value is what the input field
// should become, and submit is true when the command takes no arguments (so
// the caller can dispatch it immediately instead of waiting for another
// Enter).
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

func (m *commandMenu) hide() { m.visible = false }

// View renders the suggestion list, or "" when hidden.
func (m *commandMenu) View() string {
	if !m.visible {
		return ""
	}
	var b strings.Builder
	for i, it := range m.items {
		marker, style := "  ", mutedStyle
		if i == m.selected {
			marker, style = "› ", selectedStyle
		}
		name := it.Name
		if it.Args != "" {
			name += " " + it.Args
		}
		line := fmt.Sprintf("%s%-28s %s", marker, name, it.Desc)
		b.WriteString(style.Render(line))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// formatCommandList renders every spec as "name args  description", one per
// line, for /help — the full reference, unlike the narrowing suggestion menu.
func formatCommandList(specs []commandSpec) string {
	var b strings.Builder
	b.WriteString("comandos:")
	for _, it := range specs {
		name := it.Name
		if it.Args != "" {
			name += " " + it.Args
		}
		fmt.Fprintf(&b, "\n  %-28s %s", name, it.Desc)
	}
	return b.String()
}

var homeCommandSpecs = []commandSpec{
	{Name: "/profile", Desc: "Mostra seu perfil"},
	{Name: "/achievements", Desc: "Mostra suas conquistas"},
	{Name: "/play", Desc: "Entra numa mesa (escolhe tamanho/stake)"},
	{Name: "/enter", Args: "<room-id>", Desc: "Entra numa mesa por id"},
	{Name: "/clear", Desc: "Limpa a tela"},
	{Name: "/logout", Desc: "Esquece as credenciais salvas"},
	{Name: "/help", Desc: "Lista os comandos"},
	{Name: "/exit", Desc: "Sai do programa"},
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
