package handmeta

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeMeta(t *testing.T) {
	t.Run("trims and drops empty street notes", func(t *testing.T) {
		got, err := NormalizeMeta(" p1 ", " h1 ", map[string]string{
			"PREFLOP": "  3-bet bluff  ",
			"flop":    "   ",
		}, true, []string{" Estudar depois ", "Estudar depois", ""})
		if err != nil {
			t.Fatal(err)
		}
		if got.PlayerID != "p1" || got.HandID != "h1" || got.SK != "hand#h1" {
			t.Fatalf("unexpected identity: %#v", got)
		}
		if got.StreetNotes["preflop"] != "3-bet bluff" {
			t.Fatalf("expected trimmed lowercase-keyed note, got %#v", got.StreetNotes)
		}
		if _, ok := got.StreetNotes["flop"]; ok {
			t.Fatalf("blank note must be dropped, got %#v", got.StreetNotes)
		}
		if !got.ReviewMarked {
			t.Fatalf("expected review marked")
		}
		if len(got.Collections) != 1 || got.Collections[0] != "Estudar depois" {
			t.Fatalf("expected one deduped collection, got %#v", got.Collections)
		}
	})

	t.Run("rejects unknown street", func(t *testing.T) {
		_, err := NormalizeMeta("p1", "h1", map[string]string{"turnturn": "x"}, false, nil)
		if !errors.Is(err, ErrInvalidStreet) {
			t.Fatalf("expected ErrInvalidStreet, got %v", err)
		}
	})

	t.Run("rejects overlong note", func(t *testing.T) {
		_, err := NormalizeMeta("p1", "h1", map[string]string{"river": strings.Repeat("a", MaxStreetNoteLength+1)}, false, nil)
		if !errors.Is(err, ErrNoteTooLong) {
			t.Fatalf("expected ErrNoteTooLong, got %v", err)
		}
	})

	t.Run("rejects missing hand", func(t *testing.T) {
		_, err := NormalizeMeta("p1", "  ", nil, false, nil)
		if !errors.Is(err, ErrInvalidHand) {
			t.Fatalf("expected ErrInvalidHand, got %v", err)
		}
	})

	t.Run("rejects too many collections", func(t *testing.T) {
		many := make([]string, MaxCollectionsPerHand+1)
		for i := range many {
			many[i] = strings.Repeat("x", i%5+1) + string(rune('a'+i))
		}
		_, err := NormalizeMeta("p1", "h1", nil, false, many)
		if !errors.Is(err, ErrTooManyCollections) {
			t.Fatalf("expected ErrTooManyCollections, got %v", err)
		}
	})

	t.Run("rejects overlong collection name", func(t *testing.T) {
		_, err := NormalizeMeta("p1", "h1", nil, false, []string{strings.Repeat("a", MaxCollectionNameLength+1)})
		if !errors.Is(err, ErrCollectionNameInvalid) {
			t.Fatalf("expected ErrCollectionNameInvalid, got %v", err)
		}
	})
}
