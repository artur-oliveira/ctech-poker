package playernotes

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	t.Run("trims and normalizes", func(t *testing.T) {
		got, err := Normalize(" viewer ", " opponent ", " BLUE ", "  blefa muito  ")
		if err != nil {
			t.Fatal(err)
		}
		if got.ViewerID != "viewer" || got.OpponentID != "opponent" || got.Tag != "blue" || got.Text != "blefa muito" {
			t.Fatalf("unexpected normalized note: %#v", got)
		}
	})
	t.Run("rejects self note", func(t *testing.T) {
		_, err := Normalize("same", "same", "", "")
		if !errors.Is(err, ErrInvalidOpponent) {
			t.Fatalf("expected ErrInvalidOpponent, got %v", err)
		}
	})
	t.Run("rejects unknown tag", func(t *testing.T) {
		_, err := Normalize("a", "b", "black", "")
		if !errors.Is(err, ErrInvalidTag) {
			t.Fatalf("expected ErrInvalidTag, got %v", err)
		}
	})
	t.Run("counts unicode characters", func(t *testing.T) {
		_, err := Normalize("a", "b", "", strings.Repeat("á", MaxNoteLength+1))
		if !errors.Is(err, ErrNoteTooLong) {
			t.Fatalf("expected ErrNoteTooLong, got %v", err)
		}
	})
}
