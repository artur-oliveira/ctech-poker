package leaderboard

import (
	"context"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/achievements"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

type memStats struct{ rows map[string]*Entry }

func (m *memStats) IncrementStats(_ context.Context, id string, p, w int) error {
	if m.rows[id] == nil {
		m.rows[id] = &Entry{PlayerID: id}
	}
	m.rows[id].HandsPlayed += p
	m.rows[id].HandsWon += w
	return nil
}
func (m *memStats) IncrementAchievementPoints(_ context.Context, id string, points int) error {
	if m.rows[id] == nil {
		m.rows[id] = &Entry{PlayerID: id}
	}
	m.rows[id].AchievementPoints += points
	return nil
}
func (m *memStats) Top(_ context.Context, _ string, _ int) ([]Entry, error) {
	out := []Entry{}
	for _, e := range m.rows {
		out = append(out, *e)
	}
	return out, nil
}
func TestRecordHandAndTop(t *testing.T) {
	m := &memStats{rows: map[string]*Entry{}}
	s := NewServiceWithStore(m)
	if err := s.RecordHand(context.Background(), hand.HandOutcome{Winners: []string{"p1"}, Participants: []string{"p1", "p2"}}); err != nil {
		t.Fatal(err)
	}
	if m.rows["p1"].HandsWon != 1 || m.rows["p2"].HandsPlayed != 1 {
		t.Fatalf("rows=%+v", m.rows)
	}
	top, err := s.Top(context.Background(), "win_rate", 10)
	if err != nil || top[0].PlayerID != "p1" {
		t.Fatalf("top=%+v err=%v", top, err)
	}
	if err := s.RecordUnlocks(context.Background(), []achievements.TierUnlock{{PlayerID: "p1", Stars: 2}}); err != nil {
		t.Fatal(err)
	}
	if m.rows["p1"].AchievementPoints != 2 {
		t.Fatalf("achievement points=%d", m.rows["p1"].AchievementPoints)
	}
}
