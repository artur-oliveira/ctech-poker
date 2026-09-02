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
func (m *memStats) PlayerEntry(_ context.Context, id, mode string) (*Entry, error) {
	e, ok := m.rows[mode+"#"+id]
	if !ok {
		return nil, nil
	}
	return e, nil
}

// RankOf mirrors Store.RankOf's semantics (better-than count, then
// tied-before-by-player-id count, then +1) over the in-memory rows, so the
// fake tests the same rank contract the real GSI-backed store implements.
func (m *memStats) RankOf(_ context.Context, mode, metric string, entry Entry) (int64, int64, error) {
	score := func(e *Entry) float64 {
		switch metric {
		case "hands_played":
			return float64(e.HandsPlayed)
		case "win_rate":
			return e.WinRate
		default:
			return float64(e.HandsWon)
		}
	}
	mine := score(&entry)
	var better, tied, total int64
	for key, e := range m.rows {
		if len(key) <= len(mode) || key[:len(mode)+1] != mode+"#" {
			continue
		}
		total++
		s := score(e)
		if s > mine {
			better++
		} else if s == mine && e.PlayerID < entry.PlayerID {
			tied++
		}
	}
	return better + tied + 1, total, nil
}
func TestRecordHandAndTop(t *testing.T) {
	m := &memStats{rows: map[string]*Entry{}}
	s := NewServiceWithStore(m)
	names := map[string]string{"p1": "Player One"}
	// p1 must clear MinHandsForWinRateRank to be eligible for the win_rate board.
	for i := 0; i < MinHandsForWinRateRank; i++ {
		if err := s.RecordHand(context.Background(), "sandbox", hand.HandOutcome{Winners: []string{"p1"}, Participants: []string{"p1", "p2"}}, names); err != nil {
			t.Fatal(err)
		}
	}
	if m.rows["sandbox#p1"].HandsWon != MinHandsForWinRateRank || m.rows["sandbox#p2"].HandsPlayed != MinHandsForWinRateRank {
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

func TestMyRank(t *testing.T) {
	m := &memStats{rows: map[string]*Entry{}}
	s := NewServiceWithStore(m)
	// p1: 2/2, p2: 1/2, p3: 0/1 win_rate; ranking by hands_won: p1=2, p2=1, p3=0.
	if err := s.RecordHand(context.Background(), "sandbox", hand.HandOutcome{Winners: []string{"p1"}, Participants: []string{"p1", "p2"}}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordHand(context.Background(), "sandbox", hand.HandOutcome{Winners: []string{"p1"}, Participants: []string{"p1", "p3"}}, nil); err != nil {
		t.Fatal(err)
	}

	info, err := s.MyRank(context.Background(), "sandbox", "hands_won", "p1")
	if err != nil {
		t.Fatal(err)
	}
	if info == nil || info.Rank != 1 || info.Total != 3 {
		t.Fatalf("expected p1 rank 1 of 3, got %+v", info)
	}

	info, err = s.MyRank(context.Background(), "sandbox", "hands_won", "p3")
	if err != nil {
		t.Fatal(err)
	}
	if info == nil || info.Rank != 3 || info.Total != 3 {
		t.Fatalf("expected p3 rank 3 of 3, got %+v", info)
	}

	info, err = s.MyRank(context.Background(), "sandbox", "hands_won", "ghost")
	if err != nil {
		t.Fatal(err)
	}
	if info != nil {
		t.Fatalf("expected unranked (nil) for a player with no stats row, got %+v", info)
	}

	if _, err := s.MyRank(context.Background(), "sandbox", "not_a_metric", "p1"); err == nil {
		t.Fatal("expected error for unsupported metric")
	}
}

// TestWinRateMinHandsFloor: a 1-hand 100% player is excluded from the win_rate
// board and does not occupy a rank slot; a 150-hand grinder is ranked (#63).
func TestWinRateMinHandsFloor(t *testing.T) {
	m := &memStats{rows: map[string]*Entry{
		"sandbox#oneHand": {PlayerID: "oneHand", HandsPlayed: 1, HandsWon: 1},
		"sandbox#grinder": {PlayerID: "grinder", HandsPlayed: 150, HandsWon: 87},
	}}
	s := NewServiceWithStore(m)

	top, _, err := s.Top(context.Background(), "sandbox", "win_rate", 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 1 || top[0].PlayerID != "grinder" {
		t.Fatalf("expected only the 150-hand grinder ranked, got %+v", top)
	}
	for _, e := range top {
		if e.PlayerID == "oneHand" {
			t.Fatalf("1-hand 100%% player must not appear on the win_rate board: %+v", top)
		}
	}

	// Other metrics are unaffected — the low-hand player still ranks there.
	byPlayed, _, err := s.Top(context.Background(), "sandbox", "hands_won", 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(byPlayed) != 2 {
		t.Fatalf("hands_won board must not apply the win_rate floor, got %+v", byPlayed)
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
