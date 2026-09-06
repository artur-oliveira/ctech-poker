package tui

import (
	"strings"
	"testing"

	"gopkg.aoctech.app/poker/cli/internal/rest"
)

func TestFormatProfileIncludesNameAndBalances(t *testing.T) {
	p := rest.Profile{Name: "Ana", FriendCode: "PKR-AAAA-BBBB-CCCC", WalletMode: "sandbox", SandboxBalance: 5000}
	out := FormatProfile(p)
	if !strings.Contains(out, "Ana") || !strings.Contains(out, "PKR-AAAA-BBBB-CCCC") || !strings.Contains(out, "Fichas: 5.000") {
		t.Fatalf("missing expected fields: %q", out)
	}
	for _, want := range []string{"sandbox", "Fichas", "/achievements", "próximos marcos"} {
		if !strings.Contains(out, want) {
			t.Errorf("profile ledger missing %q: %q", want, out)
		}
	}
}

func TestFormatProfileFallsBackWhenNoName(t *testing.T) {
	out := FormatProfile(rest.Profile{})
	if !strings.Contains(out, "sem nome") {
		t.Fatalf("want a no-name fallback, got %q", out)
	}
}

func TestFormatAchievementsIncludesTotalsLabelAndDescription(t *testing.T) {
	s := rest.AchievementSummary{
		Achievements: []rest.Achievement{{Key: "wins", Stars: 1, Progress: 1, Completed: true, MaxTarget: 1}},
	}
	s.Totals.Unlocked = 3
	s.Totals.Revealed = 10
	s.Totals.Stars = 7
	s.Totals.MaxStars = 40
	out := FormatAchievements(s)
	if !strings.Contains(out, "3 desbloqueadas") || !strings.Contains(out, "10 reveladas") || !strings.Contains(out, "7/40") {
		t.Fatalf("missing totals: %q", out)
	}
	if !strings.Contains(out, "Vitórias") {
		t.Fatalf("missing translated label: %q", out)
	}
	if !strings.Contains(out, "Toda mão vencida conta um ponto.") {
		t.Fatalf("missing description: %q", out)
	}
}

func TestFormatAchievementsPrioritizesClosestMilestone(t *testing.T) {
	nextTen, nextFive := 10, 5
	s := rest.AchievementSummary{Achievements: []rest.Achievement{
		{Key: "hands_played", Progress: 2, NextTarget: &nextTen},
		{Key: "wins", Progress: 4, NextTarget: &nextFive},
	}}
	out := FormatAchievementsWidth(s, 60)
	if !strings.Contains(out, "Próximo marco: Vitórias · faltam 1") {
		t.Fatalf("closest milestone was not prioritized: %q", out)
	}
	if !strings.Contains(out, "Em andamento · 2") || !strings.Contains(out, "4/5") {
		t.Fatalf("active progress ledger missing: %q", out)
	}
}

func TestFormatAchievementsKeepsUnrevealedSecretsPrivate(t *testing.T) {
	s := rest.AchievementSummary{Achievements: []rest.Achievement{
		{Key: "all_in_blind", Secret: true},
	}}
	out := FormatAchievements(s)
	if strings.Contains(out, "All-in às Cegas") || strings.Contains(out, "sem ver nenhuma") {
		t.Fatalf("secret achievement leaked: %q", out)
	}
	if !strings.Contains(out, "Por descobrir · 1") {
		t.Fatalf("secret count missing: %q", out)
	}
}

func TestCareerLedgerLinesFitNarrowTerminal(t *testing.T) {
	next := 100
	s := rest.AchievementSummary{Achievements: []rest.Achievement{{
		Key: "sandbox_chips_earned", Progress: 73, NextTarget: &next,
	}}}
	for _, width := range []int{12, 28} {
		for _, out := range []string{
			FormatProfileWidth(rest.Profile{Name: "Uma jogadora com nome muito longo", FriendCode: "PKR-LONGO-LONGO-LONGO"}, width),
			FormatAchievementsWidth(s, width),
		} {
			for _, line := range strings.Split(out, "\n") {
				if visibleWidth(line) > terminalLineWidth(width) {
					t.Fatalf("line width %d exceeds terminal width %d: %q", visibleWidth(line), width, line)
				}
			}
		}
	}
}

func TestFormatAchievementsWinCategoryDerivesLabelAndDescription(t *testing.T) {
	s := rest.AchievementSummary{
		Achievements: []rest.Achievement{{Key: "win_category_flush", Stars: 2}},
	}
	out := FormatAchievements(s)
	if !strings.Contains(out, "Flush") {
		t.Fatalf("missing derived win_category label: %q", out)
	}
	if !strings.Contains(out, "showdown com flush") {
		t.Fatalf("missing derived win_category description: %q", out)
	}
}

func TestFormatSocialLedgerShowsStateContextAndPagination(t *testing.T) {
	page := rest.Page[rest.SocialPlayer]{
		Data:    []rest.SocialPlayer{{Name: "Caio", Presence: "in_table", HandsTogether: 12, RoomID: "room-1"}},
		HasNext: true,
	}
	out := FormatSocialPlayersPage("friends", "Amigos", page, 2, 80, "/friends")
	for _, want := range []string{"Amigos · página 2", "▶ Caio · na mesa", "12 mãos juntos", "/enter room-1", "/friends prev", "/friends next"} {
		if !strings.Contains(out, want) {
			t.Errorf("social ledger missing %q: %q", want, out)
		}
	}
}

func TestFormatSocialLedgerHasUsefulEmptyState(t *testing.T) {
	out := FormatSocialPlayersPage("recent", "Jogadores recentes", rest.Page[rest.SocialPlayer]{}, 1, 50, "/recent")
	if !strings.Contains(out, "Nenhum jogador") || !strings.Contains(out, "90 dias") {
		t.Fatalf("empty state does not teach the surface: %q", out)
	}
}

func TestFormatSocialLedgerFitsNarrowWidth(t *testing.T) {
	page := rest.Page[rest.SocialPlayer]{Data: []rest.SocialPlayer{{
		Name: "Uma pessoa com nome muito longo", Presence: "in_table", HandsTogether: 999, RoomID: "room-with-a-long-id",
	}}, HasNext: true}
	out := FormatSocialPlayersPage("friends", "Amigos", page, 1, 24, "/friends")
	for _, line := range strings.Split(out, "\n") {
		if visibleWidth(line) > terminalLineWidth(24) {
			t.Fatalf("line overflow (%d): %q", visibleWidth(line), line)
		}
	}
}
