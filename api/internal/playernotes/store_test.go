package playernotes

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	t.Run("trims and normalizes", func(t *testing.T) {
		got, err := Normalize(" viewer ", " opponent ", " BLUE ", "  blefa muito  ", nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.ViewerID != "viewer" || got.OpponentID != "opponent" || got.Tag != "blue" || got.Text != "blefa muito" {
			t.Fatalf("unexpected normalized note: %#v", got)
		}
	})
	t.Run("rejects self note", func(t *testing.T) {
		_, err := Normalize("same", "same", "", "", nil)
		if !errors.Is(err, ErrInvalidOpponent) {
			t.Fatalf("expected ErrInvalidOpponent, got %v", err)
		}
	})
	t.Run("rejects unknown tag", func(t *testing.T) {
		_, err := Normalize("a", "b", "black", "", nil)
		if !errors.Is(err, ErrInvalidTag) {
			t.Fatalf("expected ErrInvalidTag, got %v", err)
		}
	})
	t.Run("counts unicode characters", func(t *testing.T) {
		_, err := Normalize("a", "b", "", strings.Repeat("á", MaxNoteLength+1), nil)
		if !errors.Is(err, ErrNoteTooLong) {
			t.Fatalf("expected ErrNoteTooLong, got %v", err)
		}
	})
}

func TestNormalizeLabelsAreTrimmedLoweredAndDeduped(t *testing.T) {
	note, err := Normalize("viewer", "opponent", "", "nota", []string{" Agressivo ", "agressivo", "", "3-BET"})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if len(note.Labels) != 2 || note.Labels[0] != "agressivo" || note.Labels[1] != "3-bet" {
		t.Fatalf("labels=%v", note.Labels)
	}
}

func TestNormalizeRejectsOversizedLabelSets(t *testing.T) {
	tooMany := make([]string, MaxLabels+1)
	for i := range tooMany {
		tooMany[i] = string(rune('a' + i))
	}
	if _, err := Normalize("a", "b", "", "", tooMany); !errors.Is(err, ErrInvalidLabel) {
		t.Fatalf("too many labels err=%v", err)
	}
	if _, err := Normalize("a", "b", "", "", []string{strings.Repeat("x", MaxLabelLength+1)}); !errors.Is(err, ErrInvalidLabel) {
		t.Fatalf("long label err=%v", err)
	}
}

// Search is the point of labels: a label filter is exact, while q is a
// substring over both the note text and its labels.
func TestFilterMatchesLabelsAndFreeText(t *testing.T) {
	notes := []Note{
		{OpponentID: "a", Text: "paga demais", Labels: []string{"calling-station"}},
		{OpponentID: "b", Text: "3-bet leve no botão", Labels: []string{"agressivo"}},
		{OpponentID: "c", Text: "sem leitura"},
	}
	if got := Filter(notes, "agressivo", ""); len(got) != 1 || got[0].OpponentID != "b" {
		t.Fatalf("label filter=%v", got)
	}
	if got := Filter(notes, "", "BOTÃO"); len(got) != 1 || got[0].OpponentID != "b" {
		t.Fatalf("text query=%v", got)
	}
	if got := Filter(notes, "", "station"); len(got) != 1 || got[0].OpponentID != "a" {
		t.Fatalf("label substring query=%v", got)
	}
	if got := Filter(notes, "agressivo", "paga"); len(got) != 0 {
		t.Fatalf("label and query must both hold: %v", got)
	}
	if got := Filter(notes, "", ""); len(got) != len(notes) {
		t.Fatalf("empty filter must not drop notes: %v", got)
	}
}
