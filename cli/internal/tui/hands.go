package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"gopkg.aoctech.app/poker/cli/internal/game"
	"gopkg.aoctech.app/poker/cli/internal/rest"
)

type handsView int

const (
	handsListView handsView = iota
	handsDetailView
)

type handsPageMsg struct {
	page      rest.Page[rest.Hand]
	cursor    string
	pageIndex int
	seq       int
	err       error
}

type handDetailMsg struct {
	hand       rest.Hand
	history    rest.HandHistory
	historyErr error
	seq        int
	err        error
}

type handsExitMsg struct{}

// HandsModel is the dedicated keyboard-native archive opened by /hands.
// It owns cursor history, selection, and the scrollable hand-detail timeline.
type HandsModel struct {
	rc       *rest.Client
	cardMode game.CardMode
	width    int
	height   int
	view     handsView

	page       rest.Page[rest.Hand]
	pageIndex  int
	cursors    []string
	selected   int
	loading    bool
	err        error
	requestSeq int

	detail         rest.Hand
	history        rest.HandHistory
	historyErr     error
	detailLoading  bool
	detailErr      error
	detailViewport viewport.Model
	detailVPReady  bool
}

func NewHandsModel(rc *rest.Client, mode game.CardMode, width, height int) (*HandsModel, tea.Cmd) {
	m := &HandsModel{rc: rc, cardMode: mode, width: width, height: height, cursors: []string{""}}
	return m, m.loadPage("", 0)
}

func (m *HandsModel) loadPage(cursor string, pageIndex int) tea.Cmd {
	m.loading = true
	m.err = nil
	m.requestSeq++
	seq := m.requestSeq
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		page, err := m.rc.Hands(ctx, cursor)
		return handsPageMsg{page: page, cursor: cursor, pageIndex: pageIndex, seq: seq, err: err}
	}
}

func (m *HandsModel) loadDetail(hand rest.Hand) tea.Cmd {
	m.view = handsDetailView
	m.detailLoading = true
	m.detailErr = nil
	m.historyErr = nil
	m.detail = hand
	m.requestSeq++
	seq := m.requestSeq
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		detail, err := m.rc.Hand(ctx, hand.HandID)
		if err != nil {
			return handDetailMsg{seq: seq, err: err}
		}
		history, historyErr := m.rc.HandHistory(ctx, detail.TableID, detail.HandID)
		return handDetailMsg{hand: detail, history: history, historyErr: historyErr, seq: seq}
	}
}

func exitHands() tea.Cmd { return func() tea.Msg { return handsExitMsg{} } }

func (m *HandsModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.syncDetailViewport()
		return nil
	case handsPageMsg:
		if msg.seq != m.requestSeq {
			return nil
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return nil
		}
		m.page = msg.page
		m.pageIndex = msg.pageIndex
		m.selected = 0
		m.err = nil
		if msg.pageIndex >= len(m.cursors) {
			m.cursors = append(m.cursors, msg.cursor)
		} else {
			m.cursors[msg.pageIndex] = msg.cursor
			m.cursors = m.cursors[:msg.pageIndex+1]
		}
		return nil
	case handDetailMsg:
		if msg.seq != m.requestSeq {
			return nil
		}
		m.detailLoading = false
		m.detailErr = msg.err
		if msg.err == nil {
			m.detail = msg.hand
			m.history = msg.history
			m.historyErr = msg.historyErr
		}
		m.syncDetailViewport()
		return nil
	case tea.KeyMsg:
		if m.view == handsDetailView {
			return m.updateDetailKey(msg)
		}
		return m.updateListKey(msg)
	}
	return nil
}

func (m *HandsModel) updateListKey(key tea.KeyMsg) tea.Cmd {
	if m.loading {
		if key.String() == "esc" || key.String() == "q" {
			return exitHands()
		}
		return nil
	}
	switch key.String() {
	case "esc", "q":
		return exitHands()
	case "up", "k":
		if m.selected > 0 {
			m.selected--
		}
	case "down", "j":
		if m.selected+1 < len(m.page.Data) {
			m.selected++
		}
	case "home", "g":
		m.selected = 0
	case "end", "G":
		if len(m.page.Data) > 0 {
			m.selected = len(m.page.Data) - 1
		}
	case "pgup":
		m.selected -= m.listPageStep()
		if m.selected < 0 {
			m.selected = 0
		}
	case "pgdown":
		m.selected += m.listPageStep()
		if m.selected >= len(m.page.Data) {
			m.selected = len(m.page.Data) - 1
		}
	case "enter":
		if len(m.page.Data) > 0 {
			return m.loadDetail(m.page.Data[m.selected])
		}
	case "n", "right":
		if m.page.HasNext && m.page.NextCursor != "" {
			return m.loadPage(m.page.NextCursor, m.pageIndex+1)
		}
	case "p", "left":
		if m.pageIndex > 0 && m.pageIndex-1 < len(m.cursors) {
			return m.loadPage(m.cursors[m.pageIndex-1], m.pageIndex-1)
		}
	case "r":
		cursor := ""
		if m.pageIndex < len(m.cursors) {
			cursor = m.cursors[m.pageIndex]
		}
		return m.loadPage(cursor, m.pageIndex)
	}
	return nil
}

func (m *HandsModel) updateDetailKey(key tea.KeyMsg) tea.Cmd {
	if m.detailLoading {
		if key.String() == "esc" || key.String() == "backspace" {
			m.view = handsListView
		}
		return nil
	}
	switch key.String() {
	case "esc", "backspace":
		m.view = handsListView
		return nil
	case "q":
		return exitHands()
	case "r":
		return m.loadDetail(m.detail)
	}
	if m.detailVPReady {
		var cmd tea.Cmd
		m.detailViewport, cmd = m.detailViewport.Update(key)
		return cmd
	}
	return nil
}

func (m *HandsModel) listPageStep() int {
	step := (m.height - 6) / 3
	if step < 1 {
		return 1
	}
	return step
}

func (m *HandsModel) syncDetailViewport() {
	if m.view != handsDetailView || m.detailLoading || m.detailErr != nil {
		return
	}
	width := terminalLineWidth(m.width)
	height := m.height - 3
	if height < 0 {
		height = 0
	}
	if !m.detailVPReady {
		m.detailViewport = viewport.New(width, height)
		m.detailVPReady = true
	} else {
		m.detailViewport.Width = width
		m.detailViewport.Height = height
	}
	m.detailViewport.SetContent(renderHandDetail(m.detail, m.history, m.historyErr, width, m.cardMode))
}

func (m *HandsModel) View() string {
	if m.view == handsDetailView {
		return m.detailView()
	}
	return m.listView()
}

func (m *HandsModel) listView() string {
	width := terminalLineWidth(m.width)
	header := []string{
		titleStyle.Render(truncateVisible("♠ Histórico de mãos", width)),
	}
	if m.loading {
		return handsScreen(header, []string{accentStyle.Render(truncateVisible("carregando arquivo…", width))},
			dimStyle.Render(truncateVisible("Esc volta ao início", width)), m.height)
	}
	if m.err != nil {
		return handsScreen(header, []string{errorStyle.Render(truncateVisible("erro: "+m.err.Error(), width))},
			mutedStyle.Render(truncateVisible("R tenta novamente · Esc volta", width)), m.height)
	}

	header = append(header,
		truncateVisible(handPageSummary(m.page.Data), width),
		dimStyle.Render(strings.Repeat("─", width)),
	)
	footer := dimStyle.Render(truncateVisible(m.listFooter(width), width))
	bodyBudget := m.height - len(header) - 1
	if m.height <= 0 {
		bodyBudget = 9999
	}
	if bodyBudget < 1 {
		bodyBudget = 1
	}
	body := m.renderListBody(width, bodyBudget)
	return handsScreen(header, body, footer, m.height)
}

func (m *HandsModel) renderListBody(width, budget int) []string {
	if len(m.page.Data) == 0 {
		return []string{
			"",
			mutedStyle.Render(truncateVisible("Nenhuma mão registrada ainda.", width)),
			mutedStyle.Render(truncateVisible("Quando uma mão terminar, ela aparecerá aqui.", width)),
		}
	}
	count := budget / 3
	if count < 1 {
		count = 1
	}
	start := m.selected - count/2
	if start < 0 {
		start = 0
	}
	if start+count > len(m.page.Data) {
		start = len(m.page.Data) - count
		if start < 0 {
			start = 0
		}
	}
	end := start + count
	if end > len(m.page.Data) {
		end = len(m.page.Data)
	}

	dayCounts := map[string]int{}
	for _, hand := range m.page.Data {
		dayCounts[handDayKey(hand.EndedAt)]++
	}
	lines := make([]string, 0, budget)
	lastDay := ""
	for i := start; i < end; i++ {
		hand := m.page.Data[i]
		day := handDayKey(hand.EndedAt)
		if day != lastDay && len(lines) < budget {
			label := fmt.Sprintf("%s · %d %s", handDayLabel(hand.EndedAt, time.Now()), dayCounts[day], plural(dayCounts[day], "mão", "mãos"))
			lines = append(lines, titleStyle.Render(truncateVisible(label, width)))
			lastDay = day
		}
		row := renderHandRow(hand, i == m.selected, width, m.cardMode)
		if len(lines)+len(row) > budget {
			break
		}
		lines = append(lines, row...)
	}
	return lines
}

func (m *HandsModel) listFooter(width int) string {
	if width < 60 {
		keys := fmt.Sprintf("p%d · ↑↓ · Enter", m.pageIndex+1)
		if m.pageIndex > 0 {
			keys += " · P ant."
		}
		if m.page.HasNext {
			keys += " · N próx."
		}
		return keys + " · Esc"
	}
	page := fmt.Sprintf("página %d", m.pageIndex+1)
	keys := "↑↓ escolhe · Enter detalha"
	if m.pageIndex > 0 {
		keys += " · P anterior"
	}
	if m.page.HasNext {
		keys += " · N próxima"
	}
	return page + " · " + keys + " · Esc volta"
}

func (m *HandsModel) detailView() string {
	width := terminalLineWidth(m.width)
	header := titleStyle.Render(truncateVisible("♠ Mão "+shortID(m.detail.HandID), width))
	footer := dimStyle.Render(truncateVisible("↑↓/PgUp/PgDn rolam · Esc volta à lista · Q sai", width))
	if m.detailLoading {
		return handsScreen([]string{header}, []string{accentStyle.Render("carregando detalhe…")}, footer, m.height)
	}
	if m.detailErr != nil {
		return handsScreen([]string{header}, []string{errorStyle.Render(truncateVisible("erro: "+m.detailErr.Error(), width))},
			mutedStyle.Render(truncateVisible("R tenta novamente · Esc volta à lista", width)), m.height)
	}
	m.syncDetailViewport()
	view := ""
	if m.detailVPReady {
		view = m.detailViewport.View()
	}
	return handsScreen([]string{header}, strings.Split(view, "\n"), footer, m.height)
}

func handsScreen(header, body []string, footer string, height int) string {
	if height <= 0 {
		lines := append(append([]string{}, header...), body...)
		if footer != "" {
			lines = append(lines, footer)
		}
		return strings.Join(lines, "\n")
	}
	if len(header) >= height {
		return strings.Join(header[:height], "\n")
	}
	bodyBudget := height - len(header)
	if footer != "" {
		bodyBudget--
	}
	if bodyBudget < 0 {
		bodyBudget = 0
	}
	if len(body) > bodyBudget {
		body = body[:bodyBudget]
	}
	lines := append(append([]string{}, header...), body...)
	if footer != "" && len(lines) < height {
		lines = append(lines, footer)
	}
	return padViewHeight(strings.Join(lines, "\n"), height)
}

func handPageSummary(hands []rest.Hand) string {
	if len(hands) == 0 {
		return "0 mãos nesta página"
	}
	var net int64
	wins, ties := 0, 0
	for _, hand := range hands {
		net += hand.NetChange
		switch hand.Outcome {
		case "won":
			wins++
		case "tied":
			ties++
		}
	}
	losses := len(hands) - wins - ties
	return fmt.Sprintf("%d %s · %s fichas · %dV %dE %dD",
		len(hands), plural(len(hands), "mão", "mãos"), signedNumber(net), wins, ties, losses)
}

func renderHandRow(hand rest.Hand, selected bool, width int, mode game.CardMode) []string {
	marker := "  "
	if selected {
		marker = "› "
	}
	outcome := handOutcomeLabel(hand.Outcome)
	when := formatHandClock(hand.EndedAt)
	line1 := fmt.Sprintf("%s%-8s %s fichas · %s", marker, outcome, signedNumber(hand.NetChange), when)
	if selected {
		line1 = selectedStyle.Render(truncateVisible(line1, width))
	} else if hand.Outcome == "won" {
		line1 = successStyle.Render(truncateVisible(line1, width))
	} else {
		line1 = truncateVisible(line1, width)
	}
	hole := game.FormatCards(hand.HoleCards, mode)
	if hole == "" {
		hole = "—"
	}
	board := game.FormatCards(hand.Board, mode)
	if board == "" {
		board = "—"
	}
	meta := "  " + hole + " → " + board
	if width >= 52 {
		if strength := game.HandStrength(hand.HoleCards, hand.Board); strength != "" {
			meta += " · " + strength
		}
		meta += " · mesa " + shortID(hand.TableID)
	}
	if hand.BigBlind > 0 && width >= 64 {
		meta += fmt.Sprintf(" · %s/%s", pokerNumber(hand.SmallBlind), pokerNumber(hand.BigBlind))
	}
	return []string{line1, mutedStyle.Render(truncateVisible(meta, width))}
}

func renderHandDetail(hand rest.Hand, history rest.HandHistory, historyErr error, width int, mode game.CardMode) string {
	lines := []string{outcomeStyle(hand.Outcome).Render(truncateVisible(handOutcomeLabel(hand.Outcome)+" · "+signedNumber(hand.NetChange)+" fichas", width))}
	dateLine := formatHandDate(hand.EndedAt)
	tableLabel := shortID(hand.TableID)
	if visibleWidth(dateLine)+visibleWidth(tableLabel)+8 <= width {
		lines = append(lines, mutedStyle.Render(truncateVisible(dateLine+" · mesa "+tableLabel, width)))
	} else {
		lines = append(lines,
			mutedStyle.Render(truncateVisible(dateLine, width)),
			mutedStyle.Render(truncateVisible("Mesa: "+tableLabel, width)),
		)
	}
	if hand.BigBlind > 0 {
		lines = append(lines, fmt.Sprintf("Blinds: %s/%s", pokerNumber(hand.SmallBlind), pokerNumber(hand.BigBlind)))
	}
	hole := game.FormatCards(hand.HoleCards, mode)
	board := game.FormatCards(hand.Board, mode)
	if hole == "" {
		hole = "—"
	}
	if board == "" {
		board = "—"
	}
	holeLine := "Sua mão: " + hole
	if strength := game.HandStrength(hand.HoleCards, hand.Board); strength != "" {
		holeLine += " · " + strength
	}
	lines = append(lines, "", truncateVisible(holeLine, width), truncateVisible("Board: "+board, width))
	if len(hand.BoardTwo) > 0 {
		lines = append(lines, truncateVisible("Board 2: "+game.FormatCards(hand.BoardTwo, mode), width))
	}
	if len(hand.Opponents) > 0 {
		lines = append(lines, "", titleStyle.Render("Adversários"))
		for _, opponent := range hand.Opponents {
			name := opponent.Name
			if name == "" {
				name = shortID(opponent.PlayerID)
			}
			row := "○ " + name
			if cards := game.FormatCards(opponent.HoleCards, mode); cards != "" {
				row += " · " + cards
			}
			if opponent.Won {
				row += " · venceu"
			}
			lines = append(lines, truncateVisible(row, width))
		}
	}

	lines = append(lines, "", titleStyle.Render("Linha da mão"))
	if historyErr != nil {
		lines = append(lines, errorStyle.Render(truncateVisible("Ações indisponíveis: "+historyErr.Error(), width)))
	} else if len(history.Actions) == 0 {
		lines = append(lines, mutedStyle.Render("Nenhuma ação registrada para esta mão."))
	} else {
		lines = append(lines, renderActionTimeline(hand, history.Actions, width, mode)...)
	}

	lines = append(lines, "", titleStyle.Render("Integridade"))
	switch {
	case hand.ServerSeed != "":
		lines = append(lines, successStyle.Render("Prova completa disponível"))
		lines = appendWrappedField(lines, "Server seed", hand.ServerSeed, width)
		lines = appendWrappedField(lines, "Commit", hand.CommitHash, width)
	case hand.RootCommitHash != "":
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("Prova parcial · %d cartas reveladas · %d posições protegidas",
			len(hand.RevealedCardSalts), len(hand.UnrevealedCardHashes))))
		lines = appendWrappedField(lines, "Root commit", hand.RootCommitHash, width)
	default:
		lines = append(lines, mutedStyle.Render("Prova de embaralhamento indisponível para este registro."))
	}
	return strings.Join(lines, "\n")
}

func renderActionTimeline(hand rest.Hand, actions []rest.HandHistoryAction, width int, mode game.CardMode) []string {
	actions = append([]rest.HandHistoryAction(nil), actions...)
	sort.SliceStable(actions, func(i, j int) bool {
		if actions[i].Seq != actions[j].Seq {
			return actions[i].Seq < actions[j].Seq
		}
		return actions[i].Timestamp < actions[j].Timestamp
	})
	names := map[string]string{hand.PK: "Você"}
	for _, opponent := range hand.Opponents {
		name := opponent.Name
		if name == "" {
			name = shortID(opponent.PlayerID)
		}
		names[opponent.PlayerID] = name
	}
	for _, action := range actions {
		if action.Frame == nil {
			continue
		}
		for _, seat := range action.Frame.Seats {
			if _, ok := names[seat.PlayerID]; !ok && seat.Name != "" {
				names[seat.PlayerID] = seat.Name
			}
		}
	}
	resolve := func(id string) string {
		if id == "" {
			return "Mesa"
		}
		if name := names[id]; name != "" {
			return name
		}
		return shortID(id)
	}

	lines := []string{}
	street := ""
	for _, action := range actions {
		nextStreet := actionStreet(action, street)
		if nextStreet != street {
			street = nextStreet
			label := streetLabel(street)
			if action.Frame != nil {
				if cards := game.FormatCards(action.Frame.Board, mode); cards != "" {
					label += " · " + cards
				}
				if action.Frame.Pot > 0 {
					label += " · pote " + pokerNumber(action.Frame.Pot)
				}
			}
			lines = append(lines, "", accentStyle.Bold(true).Render(truncateVisible(label, width)))
		}
		who := resolve(action.PlayerID)
		what := actionLabel(action, resolve)
		when := formatActionClock(action.Timestamp)
		line := fmt.Sprintf("%-8s %-14s %s", when, who, what)
		lines = append(lines, truncateVisible(line, width))
	}
	return lines
}

func actionStreet(action rest.HandHistoryAction, previous string) string {
	if action.Action == "won" || action.Action == "lost" || action.Action == "tie" || action.Action == "show_cards" {
		return "showdown"
	}
	if action.Frame != nil && action.Frame.Stage != "" {
		stage := action.Frame.Stage
		if stage == "complete" {
			return "showdown"
		}
		return stage
	}
	if previous == "" {
		return "preflop"
	}
	return previous
}

func streetLabel(stage string) string {
	switch stage {
	case "pre_flop", "preflop":
		return "Pré-flop"
	case "flop":
		return "Flop"
	case "turn":
		return "Turn"
	case "river":
		return "River"
	case "showdown", "complete":
		return "Showdown"
	default:
		return strings.ReplaceAll(stage, "_", " ")
	}
}

func actionLabel(action rest.HandHistoryAction, resolve func(string) string) string {
	labels := map[string]string{
		"join": "entrou na mesa", "leave": "saiu da mesa", "ready": "ficou pronto",
		"not_ready": "não está pronto", "sit_out": "ficou fora", "disconnect_sit_out": "desconectou",
		"keep_seat": "confirmou presença", "set_run_it_twice": "ajustou rodar duas vezes",
		"next_hand": "nova mão", "runout_step": "board avançou", "request_exit": "pediu para sair",
		"escalate_blinds": "blinds aumentaram", "post_big_blind": "postou o big blind",
		"check": "passou (check)", "fold": "desistiu (fold)", "call": "pagou (call)",
		"bet": "apostou", "raise": "aumentou", "all_in": "foi all-in", "show_cards": "mostrou as cartas",
		"peek_cards": "espiou as cartas", "chat": "falou no chat", "reaction": "reagiu",
		"set_identity": "atualizou o perfil", "won": "venceu", "tie": "empatou", "lost": "perdeu",
	}
	label := labels[action.Action]
	if label == "" {
		label = strings.ReplaceAll(action.Action, "_", " ")
	}
	if action.Action == "reaction" && action.ReactionID != "" {
		label += " " + action.ReactionID
		if action.TargetPlayerID != "" {
			label += " para " + resolve(action.TargetPlayerID)
		}
	}
	if action.Amount > 0 {
		label += " " + pokerNumber(action.Amount)
	}
	return label
}

func appendWrappedField(lines []string, label, value string, width int) []string {
	if value == "" {
		return lines
	}
	prefix := label + ": "
	available := width - visibleWidth(prefix)
	if available < 8 {
		lines = append(lines, truncateVisible(label+":", width))
		prefix = "  "
		available = width - 2
	}
	chunks := fixedChunks(value, available)
	for i, chunk := range chunks {
		if i == 0 {
			lines = append(lines, truncateVisible(prefix+chunk, width))
		} else {
			lines = append(lines, truncateVisible(strings.Repeat(" ", visibleWidth(prefix))+chunk, width))
		}
	}
	return lines
}

func fixedChunks(value string, width int) []string {
	if width < 1 {
		return nil
	}
	runes := []rune(value)
	out := make([]string, 0, (len(runes)+width-1)/width)
	for len(runes) > 0 {
		n := width
		if n > len(runes) {
			n = len(runes)
		}
		out = append(out, string(runes[:n]))
		runes = runes[n:]
	}
	return out
}

func handOutcomeLabel(outcome string) string {
	switch outcome {
	case "won":
		return "Vitória"
	case "tied":
		return "Empate"
	case "lost":
		return "Derrota"
	default:
		return "Resultado"
	}
}

func outcomeStyle(outcome string) lipgloss.Style {
	if outcome == "won" {
		return successStyle.Bold(true)
	}
	if outcome == "tied" {
		return accentStyle.Bold(true)
	}
	return mutedStyle.Bold(true)
}

var ptPrinter = message.NewPrinter(language.BrazilianPortuguese)

func pokerNumber(value int64) string { return ptPrinter.Sprintf("%d", value) }

func signedNumber(value int64) string {
	if value > 0 {
		return "+" + pokerNumber(value)
	}
	return pokerNumber(value)
}

func plural(count int, singular, pluralForm string) string {
	if count == 1 {
		return singular
	}
	return pluralForm
}

func shortID(id string) string {
	if len(id) <= 11 {
		return id
	}
	return id[:5] + "…" + id[len(id)-5:]
}

func handDayKey(ms int64) string {
	return time.UnixMilli(ms).Local().Format("2006-01-02")
}

func handDayLabel(ms int64, now time.Time) string {
	date := time.UnixMilli(ms).Local()
	today := now.Local()
	if date.Format("2006-01-02") == today.Format("2006-01-02") {
		return "Hoje"
	}
	if date.Format("2006-01-02") == today.AddDate(0, 0, -1).Format("2006-01-02") {
		return "Ontem"
	}
	months := [...]string{"jan", "fev", "mar", "abr", "mai", "jun", "jul", "ago", "set", "out", "nov", "dez"}
	return fmt.Sprintf("%02d %s %d", date.Day(), months[int(date.Month())-1], date.Year())
}

func formatHandClock(ms int64) string {
	if ms <= 0 {
		return "—"
	}
	return time.UnixMilli(ms).Local().Format("15:04")
}

func formatActionClock(ms int64) string {
	if ms <= 0 {
		return "—"
	}
	return time.UnixMilli(ms).Local().Format("15:04:05")
}

func formatHandDate(ms int64) string {
	if ms <= 0 {
		return "data desconhecida"
	}
	date := time.UnixMilli(ms).Local()
	months := [...]string{"janeiro", "fevereiro", "março", "abril", "maio", "junho", "julho", "agosto", "setembro", "outubro", "novembro", "dezembro"}
	return fmt.Sprintf("%02d de %s de %d · %s", date.Day(), months[int(date.Month())-1], date.Year(), date.Format("15:04"))
}
