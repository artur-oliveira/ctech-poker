package player

import (
	"testing"
	"time"
)

var milestoneNow = time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

func keysOf(marks []Milestone) []string {
	keys := make([]string, 0, len(marks))
	for _, mark := range marks {
		keys = append(keys, mark.Key)
	}
	return keys
}

func TestMilestonesCoverEveryCategory(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   MilestoneInput
		want []string
	}{
		{
			name: "a brand-new account with no hands earns nothing",
			in:   MilestoneInput{Now: milestoneNow, CreatedAt: milestoneNow.AddDate(0, 0, -3)},
			want: []string{},
		},
		{
			// Tenure.
			name: "one year of account earns veteran_1y, not veteran_3y",
			in:   MilestoneInput{Now: milestoneNow, CreatedAt: milestoneNow.AddDate(-1, 0, -1)},
			want: []string{MilestoneVeteran1y},
		},
		{
			name: "three years supersede the one-year mark",
			in:   MilestoneInput{Now: milestoneNow, CreatedAt: milestoneNow.AddDate(-4, 0, 0)},
			want: []string{MilestoneVeteran3y},
		},
		{
			// Volume.
			name: "hands cross exactly one volume tier at a time",
			in:   MilestoneInput{Now: milestoneNow, HandsPlayed: 10_000},
			want: []string{MilestoneHands10k},
		},
		{
			name: "one hand short of a tier stays on the tier below",
			in:   MilestoneInput{Now: milestoneNow, HandsPlayed: 9_999},
			want: []string{MilestoneHands1k},
		},
		{
			// Ranking.
			name: "rank 1 earns top10, never both ranking marks",
			in:   MilestoneInput{Now: milestoneNow, Rank: 1},
			want: []string{MilestoneTop10},
		},
		{
			name: "rank 100 is still top100",
			in:   MilestoneInput{Now: milestoneNow, Rank: 100},
			want: []string{MilestoneTop100},
		},
		{
			name: "rank 101 earns no ranking mark",
			in:   MilestoneInput{Now: milestoneNow, Rank: 101},
			want: []string{},
		},
		{
			// An unranked player has no rank at all, and 0 must never be read
			// as "rank zero, better than first".
			name: "unranked earns no ranking mark",
			in:   MilestoneInput{Now: milestoneNow, Rank: 0},
			want: []string{},
		},
		{
			name: "all three categories at once",
			in: MilestoneInput{
				Now: milestoneNow, CreatedAt: milestoneNow.AddDate(-3, 0, -10),
				HandsPlayed: 120_000, Rank: 7,
			},
			want: []string{MilestoneVeteran3y, MilestoneHands100k, MilestoneTop10},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := keysOf(Milestones(tc.in))
			if len(got) != len(tc.want) {
				t.Fatalf("marks = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("marks = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// A missing CreatedAt (a profile written before the field existed, or one that
// failed to parse) must not be read as the zero time, which would make every
// such player a three-year veteran.
func TestMilestonesIgnoreAMissingCreatedAt(t *testing.T) {
	if marks := Milestones(MilestoneInput{Now: milestoneNow}); len(marks) != 0 {
		t.Fatalf("zero CreatedAt earned %v", keysOf(marks))
	}
}
