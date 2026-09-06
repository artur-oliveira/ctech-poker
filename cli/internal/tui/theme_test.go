package tui

import (
	"strings"
	"testing"
)

func TestHomeHeaderIsDescriptiveAndWidthSafe(t *testing.T) {
	out := renderHomeHeader(80)
	for _, want := range []string{"CTech Poker", "SANDBOX", "fichas virtuais"} {
		if !strings.Contains(out, want) {
			t.Fatalf("header missing %q:\n%s", want, out)
		}
	}
	if lines := strings.Count(out, "\n") + 1; lines != 3 {
		t.Fatalf("wide header has %d lines, want 3", lines)
	}
	for _, line := range strings.Split(out, "\n") {
		if width := len([]rune(stripANSI(line))); width > 79 {
			t.Fatalf("header line width = %d, want <= 79: %q", width, line)
		}
	}
}

func TestHomeHeaderCollapsesOnNarrowTerminals(t *testing.T) {
	out := renderHomeHeader(30)
	if strings.Contains(out, "\n") {
		t.Fatalf("narrow header should use one row:\n%s", out)
	}
	if width := len([]rune(stripANSI(out))); width > 29 {
		t.Fatalf("narrow header width = %d, want <= 29", width)
	}
}

func TestPadViewHeightKeepsAStableFullScreenFrame(t *testing.T) {
	out := padViewHeight("one\ntwo", 5)
	if lines := strings.Count(out, "\n") + 1; lines != 5 {
		t.Fatalf("padded view has %d lines, want 5", lines)
	}
	if got := padViewHeight("one\ntwo", 1); got != "one\ntwo" {
		t.Fatalf("must not clip a taller view, got %q", got)
	}
}
