package game

import (
	"strings"
	"testing"
)

func TestFormatCardASCIIIsPlain(t *testing.T) {
	if got := FormatCard("As", CardASCII); got != "As" {
		t.Errorf("got %q", got)
	}
}

func TestFormatCardColorRendersSuitGlyphAndColor(t *testing.T) {
	tests := []struct {
		code, glyph string
	}{
		{"Ah", "♥"}, {"Kd", "♦"}, {"7c", "♣"}, {"Qs", "♠"},
	}
	for _, tt := range tests {
		got := FormatCard(tt.code, CardColor)
		if !strings.Contains(got, tt.glyph) {
			t.Errorf("FormatCard(%q) = %q, missing glyph %q", tt.code, got, tt.glyph)
		}
		if !strings.HasPrefix(got, string(tt.code[0])) {
			t.Errorf("FormatCard(%q) = %q, should start with the rank", tt.code, got)
		}
		// Color emission itself depends on the terminal's detected profile
		// (a test process has no TTY), so we assert rank+glyph structure
		// here; StyledInColorTerminal covers the ANSI path.
	}
}

func TestFormatCardStyledWhenColorProfileForced(t *testing.T) {
	restore := forceColorProfile()
	defer restore()
	if got := FormatCard("Ah", CardColor); !strings.Contains(got, "\x1b[") {
		t.Errorf("with a color profile forced, FormatCard should emit ANSI: %q", got)
	}
}

func TestFormatCardsJoinsWithSpaces(t *testing.T) {
	got := FormatCards([]string{"As", "Kd", "7c"}, CardASCII)
	if got != "As Kd 7c" {
		t.Errorf("got %q", got)
	}
}

func TestHandStrengthPair(t *testing.T) {
	if got := HandStrength([]string{"As", "Ad"}, nil); got != "par de ases" {
		t.Errorf("got %q", got)
	}
}

func TestHandStrengthTwoPairOnBoard(t *testing.T) {
	got := HandStrength([]string{"Ks", "Kd"}, []string{"7c", "7d", "2h"})
	if got != "dois pares" {
		t.Errorf("got %q", got)
	}
}

func TestHandStrengthTrinca(t *testing.T) {
	got := HandStrength([]string{"9s", "9d"}, []string{"9c", "2d", "4h"})
	if got != "trinca" {
		t.Errorf("got %q", got)
	}
}

func TestHandStrengthStraight(t *testing.T) {
	got := HandStrength([]string{"9s", "8d"}, []string{"7c", "6d", "5h", "2s", "Ah"})
	if got != "sequência" {
		t.Errorf("got %q", got)
	}
}

func TestHandStrengthWheelStraight(t *testing.T) {
	got := HandStrength([]string{"As", "2d"}, []string{"3c", "4d", "5h", "9s", "Kh"})
	if got != "sequência" {
		t.Errorf("got %q (wheel A-2-3-4-5 must count)", got)
	}
}

func TestHandStrengthFlush(t *testing.T) {
	got := HandStrength([]string{"Ah", "Jh"}, []string{"8h", "5h", "2h", "9c", "Kd"})
	if got != "flush" {
		t.Errorf("got %q", got)
	}
}

func TestHandStrengthFullHouse(t *testing.T) {
	got := HandStrength([]string{"Ks", "Kd"}, []string{"Kc", "5s", "5d"})
	if got != "full house" {
		t.Errorf("got %q", got)
	}
}

func TestHandStrengthQuads(t *testing.T) {
	got := HandStrength([]string{"9s", "9d"}, []string{"9c", "9h", "2d"})
	if got != "quadra" {
		t.Errorf("got %q", got)
	}
}

func TestHandStrengthStraightFlush(t *testing.T) {
	got := HandStrength([]string{"9s", "8s"}, []string{"7s", "6s", "5s"})
	if got != "straight flush" {
		t.Errorf("got %q", got)
	}
}

func TestHandStrengthHighCard(t *testing.T) {
	got := HandStrength([]string{"As", "Jd"}, []string{"8c", "5d", "2h"})
	if got != "carta alta: A" {
		t.Errorf("got %q", got)
	}
}

func TestHandStrengthEmptyWithoutTwoHoleCards(t *testing.T) {
	if got := HandStrength([]string{"As"}, nil); got != "" {
		t.Errorf("got %q, want empty", got)
	}
	if got := HandStrength(nil, []string{"As", "Kd"}); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
