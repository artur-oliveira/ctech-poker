package chatfilter

import (
	"strings"
	"testing"
)

func TestFilterMasksKnownWordsCaseInsensitively(t *testing.T) {
	if got := New([]string{"idiota"}).Clean("Você é um IDIOTA mesmo"); got == "Você é um IDIOTA mesmo" {
		t.Fatal("word was not masked")
	}
}
func TestFilterLeavesCleanMessagesUntouched(t *testing.T) {
	const message = "boa mão!"
	if got := New([]string{"idiota"}).Clean(message); got != message {
		t.Fatalf("got %q", got)
	}
}

// The three cases #327's acceptance criteria calls for: the global floor
// alone, the floor plus a personal addition, and an attempt to weaken the
// floor — which Effective structurally cannot do, since extraWords is only
// ever appended to the global word list, never substituted for it.
func TestEffectiveFloorAlone(t *testing.T) {
	global := New([]string{"idiota", "burro"})
	eff := Effective(global, nil)
	if got := eff.Clean("você é um idiota"); got == "você é um idiota" {
		t.Fatal("floor word must still be masked with no personal preference")
	}
}

func TestEffectiveFloorPlusPersonal(t *testing.T) {
	global := New([]string{"idiota", "burro"})
	eff := Effective(global, []string{"chato"})
	got := eff.Clean("que jogador chato e idiota")
	if strings.Contains(got, "chato") || strings.Contains(got, "idiota") {
		t.Fatalf("expected both floor and personal words masked, got %q", got)
	}
}

func TestEffectiveCannotWeakenTheFloor(t *testing.T) {
	global := New([]string{"idiota", "burro"})
	// A personal preference has no field or path that removes a floor word —
	// passing no extras, or repeating a floor word, both leave the floor
	// fully intact. There is no "disable" operation to reject because none
	// is exposed: Effective only ever unions onto global.
	for _, extra := range [][]string{nil, {}, {"idiota"}} {
		eff := Effective(global, extra)
		got := eff.Clean("idiota e burro")
		if strings.Contains(got, "idiota") || strings.Contains(got, "burro") {
			t.Fatalf("floor weakened with extra=%v: got %q", extra, got)
		}
	}
}
