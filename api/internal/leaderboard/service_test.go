package leaderboard

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/poker/api/internal/achievements"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

type memStats struct{ rows map[string]*Entry }

func (m *memStats) IncrementStats(_ context.Context, id, name, mode string, p, w int) error {
	key := mode + "#" + id
	if m.rows[key] == nil {
		m.rows[key] = &Entry{PlayerID: id}
	}
	if name != "" {
		m.rows[key].PlayerName = name
	}
	m.rows[key].HandsPlayed += p
	m.rows[key].HandsWon += w
	return nil
}
func (m *memStats) IncrementAchievementPoints(_ context.Context, id, mode string, points int) error {
	key := mode + "#" + id
	if m.rows[key] == nil {
		m.rows[key] = &Entry{PlayerID: id}
	}
	m.rows[key].AchievementPoints += points
	return nil
}
func (m *memStats) Top(_ context.Context, mode, _ string, _ int, _ map[string]types.AttributeValue) ([]Entry, map[string]types.AttributeValue, error) {
	out := []Entry{}
	for key, e := range m.rows {
		if len(key) > len(mode) && key[:len(mode)+1] == mode+"#" {
			out = append(out, *e)
		}
	}
	return out, nil, nil
}
func TestRecordHandAndTop(t *testing.T) {
	m := &memStats{rows: map[string]*Entry{}}
	s := NewServiceWithStore(m)
	names := map[string]string{"p1": "Player One"}
	if err := s.RecordHand(context.Background(), "sandbox", hand.HandOutcome{Winners: []string{"p1"}, Participants: []string{"p1", "p2"}}, names); err != nil {
		t.Fatal(err)
	}
	if m.rows["sandbox#p1"].HandsWon != 1 || m.rows["sandbox#p2"].HandsPlayed != 1 {
		t.Fatalf("rows=%+v", m.rows)
	}
	if m.rows["sandbox#p1"].PlayerName != "Player One" {
		t.Fatalf("expected denormalized name carried through to the stats row, got %+v", m.rows["sandbox#p1"])
	}
	if m.rows["sandbox#p2"].PlayerName != "" {
		t.Fatalf("expected p2's unknown name to stay blank rather than overwrite with empty, got %+v", m.rows["sandbox#p2"])
	}
	top, _, err := s.Top(context.Background(), "sandbox", "win_rate", 10, nil)
	if err != nil || top[0].PlayerID != "p1" {
		t.Fatalf("top=%+v err=%v", top, err)
	}
	if err := s.RecordUnlocks(context.Background(), "sandbox", []achievements.TierUnlock{{PlayerID: "p1", Stars: 2}}); err != nil {
		t.Fatal(err)
	}
	if m.rows["sandbox#p1"].AchievementPoints != 2 {
		t.Fatalf("achievement points=%d", m.rows["sandbox#p1"].AchievementPoints)
	}
}

func TestRecordHandSeparatesCurrencyModes(t *testing.T) {
	m := &memStats{rows: map[string]*Entry{}}
	s := NewServiceWithStore(m)
	outcome := hand.HandOutcome{Winners: []string{"p1"}, Participants: []string{"p1"}}
	if err := s.RecordHand(context.Background(), "sandbox", outcome, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordHand(context.Background(), "real", outcome, nil); err != nil {
		t.Fatal(err)
	}
	if m.rows["sandbox#p1"].HandsPlayed != 1 || m.rows["real#p1"].HandsPlayed != 1 {
		t.Fatalf("rows were blended: %+v", m.rows)
	}
}
