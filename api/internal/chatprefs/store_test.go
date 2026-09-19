package chatprefs

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestNormalizeExtraWordsTrimsLowersAndDedupes(t *testing.T) {
	words, err := NormalizeExtraWords([]string{" Chato ", "chato", "", "LENTO"})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if len(words) != 2 || words[0] != "chato" || words[1] != "lento" {
		t.Fatalf("words=%v", words)
	}
}

func TestNormalizeExtraWordsRejectsOversizedInput(t *testing.T) {
	tooMany := make([]string, MaxExtraWords+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("w%d", i)
	}
	if _, err := NormalizeExtraWords(tooMany); !errors.Is(err, ErrTooManyWords) {
		t.Fatalf("expected ErrTooManyWords, got %v", err)
	}
	if _, err := NormalizeExtraWords([]string{strings.Repeat("x", MaxWordLength+1)}); !errors.Is(err, ErrWordTooLong) {
		t.Fatalf("expected ErrWordTooLong, got %v", err)
	}
}

func TestNormalizeExtraWordsWithNoUsableWordsIsEmpty(t *testing.T) {
	words, err := NormalizeExtraWords([]string{"", "   "})
	if err != nil || words != nil {
		t.Fatalf("words=%v err=%v", words, err)
	}
}
