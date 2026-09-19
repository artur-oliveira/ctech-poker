package chatfilter

import "strings"

type Filter struct{ words []string }

func New(bannedWords []string) *Filter {
	words := make([]string, 0, len(bannedWords))
	for _, word := range bannedWords {
		if word = strings.TrimSpace(strings.ToLower(word)); word != "" {
			words = append(words, word)
		}
	}
	return &Filter{words: words}
}

// Effective returns a filter that masks every word in global plus the
// caller's personal extraWords (#327). It only ever adds: extraWords can
// never remove or override a global (floor) word, so a personal preference
// cannot weaken table-wide moderation, only extend it. Returning global
// itself when extraWords is empty avoids allocating a filter identical to
// the one the caller already has.
func Effective(global *Filter, extraWords []string) *Filter {
	if global == nil {
		return New(extraWords)
	}
	if len(extraWords) == 0 {
		return global
	}
	words := make([]string, 0, len(global.words)+len(extraWords))
	words = append(words, global.words...)
	words = append(words, extraWords...)
	return New(words)
}

func (f *Filter) Clean(message string) string {
	lower, out := strings.ToLower(message), message
	for _, word := range f.words {
		mask := strings.Repeat("*", len(word))
		for {
			idx := strings.Index(lower, word)
			if idx < 0 {
				break
			}
			out = out[:idx] + mask + out[idx+len(word):]
			lower = lower[:idx] + mask + lower[idx+len(word):]
		}
	}
	return out
}
