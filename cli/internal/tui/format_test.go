package tui

import (
	"strings"
	"testing"

	"gopkg.aoctech.app/poker/cli/internal/rest"
)

func TestFormatProfileIncludesNameAndBalances(t *testing.T) {
	p := rest.Profile{Name: "Ana", FriendCode: "PKR-AAAA-BBBB-CCCC", WalletMode: "sandbox", SandboxBalance: 5000}
	out := FormatProfile(p)
	if !strings.Contains(out, "Ana") || !strings.Contains(out, "PKR-AAAA-BBBB-CCCC") || !strings.Contains(out, "5000") {
		t.Fatalf("missing expected fields: %q", out)
	}
}

func TestFormatProfileFallsBackWhenNoName(t *testing.T) {
	out := FormatProfile(rest.Profile{})
	if !strings.Contains(out, "sem nome") {
		t.Fatalf("want a no-name fallback, got %q", out)
	}
}

func TestFormatAchievementsIncludesTotalsAndEntries(t *testing.T) {
	s := rest.AchievementSummary{
		Achievements: []rest.Achievement{{Key: "first_win", Stars: 1, Progress: 1, Completed: true}},
	}
	s.Totals.Unlocked = 3
	s.Totals.Revealed = 10
	s.Totals.Stars = 7
	s.Totals.MaxStars = 40
	out := FormatAchievements(s)
	if !strings.Contains(out, "3/10") || !strings.Contains(out, "first_win") || !strings.Contains(out, "7/40") {
		t.Fatalf("missing expected fields: %q", out)
	}
}
