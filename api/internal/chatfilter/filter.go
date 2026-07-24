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
