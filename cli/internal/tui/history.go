package tui

import "fmt"

// history is one input-line history, navigated with ↑/↓ the way a shell (or
// Claude Code's own prompt) does. The home shell and the table each keep
// their own instance: the command vocabularies are disjoint, so mixing them
// would make ↑ unpredictable in both places.
//
// idx == len(items) means "not browsing" — the input holds whatever the user
// typed. Stepping back past the newest entry restores that half-typed draft
// instead of leaving the line clobbered.
type history struct {
	items []string
	idx   int
	draft string
}

// add records a submitted line. Empty lines and an immediate repeat of the
// previous command are dropped (spamming ↑ /peek should not bury the rest of
// the history), and the cursor returns to the live input.
func (h *history) add(line string) {
	if line != "" && (len(h.items) == 0 || h.items[len(h.items)-1] != line) {
		h.items = append(h.items, line)
	}
	h.reset()
}

func (h *history) browsing() bool { return h.idx < len(h.items) }

// prev steps one command further back, remembering current as the draft on
// the first step. At the oldest entry it stays put rather than wrapping.
func (h *history) prev(current string) (string, bool) {
	if len(h.items) == 0 {
		return "", false
	}
	if !h.browsing() {
		h.draft = current
		h.idx = len(h.items)
	}
	if h.idx > 0 {
		h.idx--
	}
	return h.items[h.idx], true
}

// next steps forward, returning the draft once it walks off the newest entry.
func (h *history) next() (string, bool) {
	if !h.browsing() {
		return "", false
	}
	h.idx++
	if !h.browsing() {
		draft := h.draft
		h.draft = ""
		return draft, true
	}
	return h.items[h.idx], true
}

// cancel abandons browsing and hands back the draft the user was typing.
func (h *history) cancel() (string, bool) {
	if !h.browsing() {
		return "", false
	}
	draft := h.draft
	h.reset()
	return draft, true
}

func (h *history) reset() {
	h.idx = len(h.items)
	h.draft = ""
}

// label is the "3/12" position badge shown while browsing: the recalled
// command's chronological place in the list, oldest = 1, newest = N. ↑ walks
// backwards in time, so the number counts down. Empty when not browsing.
func (h *history) label() string {
	if !h.browsing() {
		return ""
	}
	return fmt.Sprintf("%d/%d", h.idx+1, len(h.items))
}

// hint is the full line rendered above the input while browsing.
func (h *history) hint() string {
	if !h.browsing() {
		return ""
	}
	return dimStyle.Render("histórico " + h.label() + " · ↑↓ navega · Esc volta ao que você digitou")
}
