package achievements

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

type memStore struct {
	progress map[string]map[string]int
	// claimed simulates the fleet-wide DynamoDB hand-counter guard: a
	// (table_id, hand_id) key present here has already been claimed by some
	// caller, so a later ClaimHandCounters call for the same key returns
	// false, exactly like a duplicate onHandComplete invocation racing a
	// Valkey blip (issue #66).
	claimed map[string]bool
	// stamps counts StampTierUnlock calls per "mode#playerID#key" (issue
	// #72), so a test can assert a tier crossing stamps exactly once and a
	// replayed hand stamps not at all.
	stamps map[string]int
}

func (m *memStore) StampTierUnlock(_ context.Context, playerID, mode, key string) error {
	if m.stamps == nil {
		m.stamps = map[string]int{}
	}
	m.stamps[mode+"#"+playerID+"#"+key]++
	return nil
}

func (m *memStore) ClaimHandCounters(_ context.Context, tableID, handID string) (bool, error) {
	if m.claimed == nil {
		m.claimed = map[string]bool{}
	}
	key := tableID + "#" + handID
	if m.claimed[key] {
		return false, nil
	}
	m.claimed[key] = true
	return true, nil
}

func (m *memStore) Increment(_ context.Context, playerID, mode, key string, by int) (int, int, error) {
	row := mode + "#" + playerID
	if m.progress[row] == nil {
		m.progress[row] = map[string]int{}
	}
	previous := m.progress[row][key]
	m.progress[row][key] += by
	return previous, m.progress[row][key], nil
}

func (m *memStore) IncrementStreak(_ context.Context, playerID, mode, key string, reset bool, resetTo int) (int, error) {
	row := mode + "#" + playerID
	if m.progress[row] == nil {
		m.progress[row] = map[string]int{}
	}
	if reset {
		m.progress[row][key] = resetTo
	} else {
		m.progress[row][key]++
	}
	return m.progress[row][key], nil
}

func (m *memStore) UpdateTableStreak(_ context.Context, playerID, mode, tableID string, won bool) (int, error) {
	row := mode + "#" + playerID
	if m.progress[row] == nil {
		m.progress[row] = map[string]int{}
	}
	key := streakKeyPrefix + tableID
	current := m.progress[row][key]
	switch {
	case won && current >= 0:
		current++
	case won:
		current = 1
	case !won && current <= 0:
		current--
	default:
		current = -1
	}
	m.progress[row][key] = current
	return current, nil
}

func (m *memStore) ListAchievements(_ context.Context, playerID, mode string, _ int, _ map[string]types.AttributeValue) ([]PlayerAchievementProgress, map[string]types.AttributeValue, error) {
	row := mode + "#" + playerID
	out := make([]PlayerAchievementProgress, 0, len(m.progress[row]))
	for key, count := range m.progress[row] {
		out = append(out, PlayerAchievementProgress{Key: key, Count: count})
	}
	return out, nil, nil
}

// TestClaimHandCountersOnlyFirstCallerWins covers the guard itself (issue
// #66): a second claim for the same (table_id, hand_id) must lose, no matter
// how many times it is retried, while a different hand_id is unaffected.
func TestClaimHandCountersOnlyFirstCallerWins(t *testing.T) {
	store := &memStore{progress: map[string]map[string]int{}}
	service := NewServiceWithStore(store)
	first, err := service.ClaimHandCounters(context.Background(), "table-1", "hand-1")
	if err != nil || !first {
		t.Fatalf("first claim: got (%v, %v), want (true, nil)", first, err)
	}
	second, err := service.ClaimHandCounters(context.Background(), "table-1", "hand-1")
	if err != nil || second {
		t.Fatalf("duplicate claim: got (%v, %v), want (false, nil)", second, err)
	}
	third, err := service.ClaimHandCounters(context.Background(), "table-1", "hand-1")
	if err != nil || third {
		t.Fatalf("re-retried claim: got (%v, %v), want (false, nil)", third, err)
	}
	otherHand, err := service.ClaimHandCounters(context.Background(), "table-1", "hand-2")
	if err != nil || !otherHand {
		t.Fatalf("different hand claim: got (%v, %v), want (true, nil)", otherHand, err)
	}
}

// TestRecordHandGuardPreventsDoubleCountOnDuplicateInvocation simulates the
// exact failure this issue describes: a Valkey blip during hand completion
// lets two instances both pass internal/handhook's fail-open claim and both
// reach the gamification pipeline for the same hand. The caller (mirroring
// app.go's onHandComplete) must gate RecordHand behind ClaimHandCounters, so
// only the winning call ever increments progress.
func TestRecordHandGuardPreventsDoubleCountOnDuplicateInvocation(t *testing.T) {
	store := &memStore{progress: map[string]map[string]int{}}
	service := NewServiceWithStore(store)
	outcome := hand.HandOutcome{Winners: []string{"p1"}, Participants: []string{"p1", "p2"}}

	runOnce := func() {
		claimed, err := service.ClaimHandCounters(context.Background(), "table-1", "hand-1")
		if err != nil {
			t.Fatal(err)
		}
		if !claimed {
			return
		}
		if _, err := service.RecordHand(context.Background(), "table-1", "sandbox", outcome); err != nil {
			t.Fatal(err)
		}
	}
	// Two "instances" both reach the pipeline for the same hand.
	runOnce()
	runOnce()

	if got := store.progress["sandbox#p1"][KeyHandsPlayed]; got != 1 {
		t.Fatalf("p1 hands played = %d, want 1 (double-counted)", got)
	}
	if got := store.progress["sandbox#p1"][KeyWins]; got != 1 {
		t.Fatalf("p1 wins = %d, want 1 (double-counted)", got)
	}
	if got := store.progress["sandbox#p2"][KeyHandsPlayed]; got != 1 {
		t.Fatalf("p2 hands played = %d, want 1 (double-counted)", got)
	}
}

func TestRecordHandUpdatesProgressAndUnlocks(t *testing.T) {
	store := &memStore{progress: map[string]map[string]int{}}
	service := NewServiceWithStore(store)
	outcome := hand.HandOutcome{Winners: []string{"p1"}, WinningCategory: "flush", ComebackWinners: []string{"p1"}, Participants: []string{"p1", "p2"}}
	unlocks, err := service.RecordHand(context.Background(), "table-1", "sandbox", outcome)
	if err != nil {
		t.Fatal(err)
	}
	if store.progress["sandbox#p1"][KeyWins] != 1 || store.progress["sandbox#p1"][KeyWinByCategory("flush")] != 1 || store.progress["sandbox#p1"][KeyComeback] != 1 {
		t.Fatalf("winner progress: %+v", store.progress["sandbox#p1"])
	}
	if store.progress["sandbox#p1"][KeyHandsPlayed] != 1 || store.progress["sandbox#p2"][KeyHandsPlayed] != 1 {
		t.Fatal("participants not counted")
	}
	if len(unlocks) != 3 {
		t.Fatalf("got %d first-tier unlocks, want 3", len(unlocks))
	}
}

// TestRecordHandStampsEachTierCrossingExactlyOnce covers issue #72: every
// unlock RecordHand reports gets its "recently unlocked" timestamp stamped
// once, and a hand that crosses nothing new (the second hand below moves the
// same counters but no threshold) stamps nothing at all — the "no timestamp
// rewrite on replayed hand hooks" acceptance criterion, which holds even past
// ClaimHandCounters because a counter that does not cross a tier never
// reports an unlock.
func TestRecordHandStampsEachTierCrossingExactlyOnce(t *testing.T) {
	store := &memStore{progress: map[string]map[string]int{}}
	service := NewServiceWithStore(store)
	outcome := hand.HandOutcome{Winners: []string{"p1"}, WinningCategory: "flush", Participants: []string{"p1", "p2"}}

	unlocks, err := service.RecordHand(context.Background(), "table-1", "sandbox", outcome)
	if err != nil {
		t.Fatal(err)
	}
	if len(unlocks) == 0 {
		t.Fatal("expected first-tier unlocks on the very first hand")
	}
	for _, unlock := range unlocks {
		if got := store.stamps["sandbox#"+unlock.PlayerID+"#"+unlock.Key]; got != 1 {
			t.Fatalf("unlock %s/%s stamped %d times, want exactly 1", unlock.PlayerID, unlock.Key, got)
		}
	}
	if len(store.stamps) != len(unlocks) {
		t.Fatalf("stamped %d keys for %d unlocks", len(store.stamps), len(unlocks))
	}
	before := len(store.stamps)

	// Same shape of hand again: counters move, no new threshold is crossed.
	if _, err := service.RecordHand(context.Background(), "table-1", "sandbox", outcome); err != nil {
		t.Fatal(err)
	}
	for key, count := range store.stamps {
		if count != 1 {
			t.Fatalf("key %s was re-stamped (%d) by a hand that crossed no new tier", key, count)
		}
	}
	if len(store.stamps) != before {
		t.Fatalf("stamped keys grew from %d to %d without a new tier", before, len(store.stamps))
	}
}

func TestRecordHandSeparatesCurrencyModes(t *testing.T) {
	store := &memStore{progress: map[string]map[string]int{}}
	service := NewServiceWithStore(store)
	outcome := hand.HandOutcome{Participants: []string{"p1"}}
	if _, err := service.RecordHand(context.Background(), "sandbox-table", "sandbox", outcome); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordHand(context.Background(), "real-table", "real", outcome); err != nil {
		t.Fatal(err)
	}
	if store.progress["sandbox#p1"][KeyHandsPlayed] != 1 || store.progress["real#p1"][KeyHandsPlayed] != 1 {
		t.Fatalf("progress was blended: %+v", store.progress)
	}
}

func TestRecordHandGrantsBlindAchievementsOnlyWhenReportedUnpeeked(t *testing.T) {
	store := &memStore{progress: map[string]map[string]int{}}
	service := NewServiceWithStore(store)
	outcome := hand.HandOutcome{
		Winners: []string{"p1"}, AllInPlayers: []string{"p1", "p2"},
		Participants: []string{"p1", "p2"},
	}
	metrics := []HandMetric{{PlayerID: "p1", Peeked: false}, {PlayerID: "p2", Peeked: true}}
	if _, err := service.RecordHand(context.Background(), "table-1", "sandbox", outcome, metrics); err != nil {
		t.Fatal(err)
	}
	if store.progress["sandbox#p1"][KeyBlindMagic] != 1 {
		t.Fatalf("p1 won blind and should have KeyBlindMagic: %+v", store.progress["sandbox#p1"])
	}
	if store.progress["sandbox#p1"][KeyAllInBlind] != 1 {
		t.Fatalf("p1 went all-in blind and should have KeyAllInBlind: %+v", store.progress["sandbox#p1"])
	}
	if store.progress["sandbox#p2"][KeyAllInBlind] != 0 {
		t.Fatalf("p2 peeked before going all-in, should not have KeyAllInBlind: %+v", store.progress["sandbox#p2"])
	}
}

func TestRecordHandSkipsBlindAchievementsWithoutPeekData(t *testing.T) {
	store := &memStore{progress: map[string]map[string]int{}}
	service := NewServiceWithStore(store)
	outcome := hand.HandOutcome{Winners: []string{"p1"}, AllInPlayers: []string{"p1"}, Participants: []string{"p1"}}
	// No metricSets at all: the action log couldn't be analyzed. Missing
	// peek data must never be treated as "definitely didn't peek".
	if _, err := service.RecordHand(context.Background(), "table-1", "sandbox", outcome); err != nil {
		t.Fatal(err)
	}
	if store.progress["sandbox#p1"][KeyBlindMagic] != 0 || store.progress["sandbox#p1"][KeyAllInBlind] != 0 {
		t.Fatalf("blind achievements must not be granted on missing peek data: %+v", store.progress["sandbox#p1"])
	}
}

func TestRecordTableStreakFlipsSignOnBreak(t *testing.T) {
	store := &memStore{progress: map[string]map[string]int{}}
	service := NewServiceWithStore(store)
	win := hand.HandOutcome{Winners: []string{"p1"}, Participants: []string{"p1", "p2"}}
	lose := hand.HandOutcome{Winners: []string{"p2"}, Participants: []string{"p1", "p2"}}

	streaks, err := service.RecordTableStreak(context.Background(), "table-1", "sandbox", win)
	if err != nil {
		t.Fatal(err)
	}
	if streaks["p1"] != 1 || streaks["p2"] != -1 {
		t.Fatalf("first hand streaks: %+v", streaks)
	}
	streaks, err = service.RecordTableStreak(context.Background(), "table-1", "sandbox", win)
	if err != nil {
		t.Fatal(err)
	}
	if streaks["p1"] != 2 || streaks["p2"] != -2 {
		t.Fatalf("continuing streaks: %+v", streaks)
	}
	streaks, err = service.RecordTableStreak(context.Background(), "table-1", "sandbox", lose)
	if err != nil {
		t.Fatal(err)
	}
	if streaks["p1"] != -1 || streaks["p2"] != 1 {
		t.Fatalf("streak should flip sign on break, not just reset to zero: %+v", streaks)
	}
}

func TestRecordHandTracksShowdownLossesAndGiantSlayer(t *testing.T) {
	store := &memStore{progress: map[string]map[string]int{}}
	service := NewServiceWithStore(store)
	outcome := hand.HandOutcome{
		Winners:         []string{"p1", "p2"},
		WinningCategory: "flush",
		ComebackWinners: []string{"p1"},
		Participants:    []string{"p1", "p2", "p3", "p4"},
		Contributions:   map[string]int64{"p1": 100, "p2": 500, "p3": 300, "p4": 300},
		PlayerHands: map[string]hand.PlayerHandInfo{
			"p3": {HoleCards: [2]string{"Ah", "Ac"}},
			"p4": {HoleCards: [2]string{"Kh", "Kc"}},
		},
		ShowdownResults: map[string]hand.ShowdownResult{
			"p1": {Category: "flush", Won: true, Tied: true},
			"p2": {Category: "flush", Won: true, Tied: true},
			"p3": {Category: "flush", Won: false},      // almost_winner: same category, lost
			"p4": {Category: "full_house", Won: false}, // outranked flush despite full_house: bad_beat + cooler
		},
	}
	if _, err := service.RecordHand(context.Background(), "table-1", "sandbox", outcome); err != nil {
		t.Fatal(err)
	}
	if store.progress["sandbox#p1"][KeyTied] != 1 || store.progress["sandbox#p2"][KeyTied] != 1 {
		t.Fatalf("tied winners not counted: p1=%+v p2=%+v", store.progress["sandbox#p1"], store.progress["sandbox#p2"])
	}
	if store.progress["sandbox#p1"][KeyGiantSlayer] != 1 {
		t.Fatalf("p1 beat bigger stacks (p2/p3/p4) while all-in, want giant_slayer: %+v", store.progress["sandbox#p1"])
	}
	if store.progress["sandbox#p3"][KeyLooser] != 1 || store.progress["sandbox#p3"][KeyAlmostWinner] != 1 || store.progress["sandbox#p3"][KeyCrackedAces] != 1 {
		t.Fatalf("p3 (lost flush vs flush with pocket aces) progress: %+v", store.progress["sandbox#p3"])
	}
	if store.progress["sandbox#p3"][KeyBadBeat] != 1 {
		t.Fatalf("p3 lost with a flush (>= three_of_a_kind), want bad_beat=1: %+v", store.progress["sandbox#p3"])
	}
	if store.progress["sandbox#p3"][KeyCooler] != 0 {
		t.Fatalf("p3's flush is below full_house's floor, must not count as cooler: %+v", store.progress["sandbox#p3"])
	}
	if store.progress["sandbox#p4"][KeyBadBeat] != 1 || store.progress["sandbox#p4"][KeyCooler] != 1 || store.progress["sandbox#p4"][KeyAlmostWinner] != 0 {
		t.Fatalf("p4 (lost with full_house, different category) progress: %+v", store.progress["sandbox#p4"])
	}
	for _, id := range []string{"p1", "p2", "p3", "p4"} {
		if store.progress["sandbox#"+id][KeyShowdownWarrior] != 1 {
			t.Fatalf("%s reached showdown, want showdown_warrior=1, got %d", id, store.progress["sandbox#"+id][KeyShowdownWarrior])
		}
	}
}

func TestRecordHandUsesEachSidePotWinnersOwnCategory(t *testing.T) {
	store := &memStore{progress: map[string]map[string]int{}}
	service := NewServiceWithStore(store)
	outcome := hand.HandOutcome{
		Winners:         []string{"main", "side"},
		WinningCategory: "full_house",
		Participants:    []string{"main", "side"},
		ShowdownResults: map[string]hand.ShowdownResult{
			"main": {Category: "full_house", Won: true},
			"side": {Category: "three_of_a_kind", Won: true},
		},
	}
	if _, err := service.RecordHand(context.Background(), "table-side-pot", "sandbox", outcome); err != nil {
		t.Fatal(err)
	}
	if store.progress["sandbox#main"][KeyWinByCategory("full_house")] != 1 {
		t.Fatalf("main-pot winner category missing: %+v", store.progress["sandbox#main"])
	}
	if store.progress["sandbox#side"][KeyWinByCategory("three_of_a_kind")] != 1 ||
		store.progress["sandbox#side"][KeyWinByCategory("full_house")] != 0 {
		t.Fatalf("side-pot winner inherited the global category: %+v", store.progress["sandbox#side"])
	}
	if store.progress["sandbox#main"][KeyTied] != 0 || store.progress["sandbox#side"][KeyTied] != 0 {
		t.Fatal("distinct pot winners must not earn the tied achievement")
	}
}

func TestTierCrossedReturnsHighestTierAcrossLargeIncrement(t *testing.T) {
	stars, ok := TierCrossed(KeyWins, 0, 100)
	if !ok || stars != 3 {
		t.Fatalf("got (%d,%v), want (3,true)", stars, ok)
	}
}

// pocketPairWin is one hand where p1 wins holding the given pocket pair. The
// full board is required: the pocket-pair branch sits behind
// hasCompleteCards, which needs five community cards to parse.
func pocketPairWin(rank string) hand.HandOutcome {
	return hand.HandOutcome{
		Winners:      []string{"p1"},
		Participants: []string{"p1"},
		Board:        []string{"2c", "5d", "8h", "9s", "Tc"},
		PlayerHands:  map[string]hand.PlayerHandInfo{"p1": {HoleCards: [2]string{rank + "h", rank + "s"}}},
	}
}

// "Which pocket pair did this player last win with" decided whether
// KeySamePocketPairStreak continued or reset, and it lived in one process's
// map. Any instance can serve the hand that completes, so the streak advanced
// or reset depending purely on which one did — the same class of bug as the
// per-table streak badge.
func TestSamePocketPairStreakIsSharedBetweenInstances(t *testing.T) {
	ctx := context.Background()
	store := &memStore{progress: map[string]map[string]int{}}
	backend := cache.NewMemoryBackend(16)
	// Two API instances over one progress store and one shared cache.
	instanceA, instanceB := NewServiceWithStore(store), NewServiceWithStore(store)
	instanceA.SetCache(backend)
	instanceB.SetCache(backend)

	if _, err := instanceA.RecordHand(ctx, "table-1", "sandbox", pocketPairWin("K")); err != nil {
		t.Fatal(err)
	}
	// The next hand of the same pair completes on the OTHER instance.
	if _, err := instanceB.RecordHand(ctx, "table-1", "sandbox", pocketPairWin("K")); err != nil {
		t.Fatal(err)
	}
	if got := store.progress["sandbox#p1"][KeySamePocketPairStreak]; got != 2 {
		t.Fatalf("streak = %d, want 2 — instance B must see A's pair", got)
	}

	// A different pair restarts the streak, from either instance.
	if _, err := instanceA.RecordHand(ctx, "table-1", "sandbox", pocketPairWin("Q")); err != nil {
		t.Fatal(err)
	}
	if got := store.progress["sandbox#p1"][KeySamePocketPairStreak]; got != 1 {
		t.Fatalf("streak = %d, want 1 after a different pair", got)
	}
}

// A hand that does not qualify clears the shared memory, so the next win with
// that same pair starts a fresh streak rather than continuing the old one.
func TestSamePocketPairStreakClearsOnANonQualifyingHand(t *testing.T) {
	ctx := context.Background()
	store := &memStore{progress: map[string]map[string]int{}}
	svc := NewServiceWithStore(store)
	svc.SetCache(cache.NewMemoryBackend(16))

	if _, err := svc.RecordHand(ctx, "table-1", "sandbox", pocketPairWin("K")); err != nil {
		t.Fatal(err)
	}
	// Same pair, but p1 loses: not a qualifying hand.
	lost := pocketPairWin("K")
	lost.Winners = nil
	if _, err := svc.RecordHand(ctx, "table-1", "sandbox", lost); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RecordHand(ctx, "table-1", "sandbox", pocketPairWin("K")); err != nil {
		t.Fatal(err)
	}
	if got := store.progress["sandbox#p1"][KeySamePocketPairStreak]; got != 1 {
		t.Fatalf("streak = %d, want 1 — the loss cleared the remembered pair", got)
	}
}

// Without a cache the service keeps its own map, so dev and single-instance
// deployments behave exactly as before.
func TestSamePocketPairStreakFallsBackToLocalMemory(t *testing.T) {
	ctx := context.Background()
	store := &memStore{progress: map[string]map[string]int{}}
	svc := NewServiceWithStore(store)

	for range 2 {
		if _, err := svc.RecordHand(ctx, "table-1", "sandbox", pocketPairWin("K")); err != nil {
			t.Fatal(err)
		}
	}
	if got := store.progress["sandbox#p1"][KeySamePocketPairStreak]; got != 2 {
		t.Fatalf("streak = %d, want 2", got)
	}
}

func TestNoRushAwardsOnFirstMinute(t *testing.T) {
	store := &memStore{progress: map[string]map[string]int{}}
	service := NewServiceWithStore(store)
	outcome := hand.HandOutcome{Participants: []string{"p1"}}
	unlocks, err := service.RecordHand(context.Background(), "table-1", "sandbox", outcome,
		[]HandMetric{{PlayerID: "p1", TimeBankMs: 60_000}})
	if err != nil {
		t.Fatal(err)
	}
	stars := 0
	for _, unlock := range unlocks {
		if unlock.Key == KeyNoRush {
			stars = unlock.Stars
		}
	}
	if stars != 1 {
		t.Fatalf("want one star for no_rush, got %d", stars)
	}
	if got := store.progress["sandbox#p1"][KeyNoRush]; got != 60_000 {
		t.Fatalf("progress=%d, want 60000", got)
	}
}

func TestNoRushIgnoresZero(t *testing.T) {
	store := &memStore{progress: map[string]map[string]int{}}
	service := NewServiceWithStore(store)
	outcome := hand.HandOutcome{Participants: []string{"p1"}}
	unlocks, err := service.RecordHand(context.Background(), "table-1", "sandbox", outcome,
		[]HandMetric{{PlayerID: "p1", TimeBankMs: 0}})
	if err != nil {
		t.Fatal(err)
	}
	for _, unlock := range unlocks {
		if unlock.Key == KeyNoRush {
			t.Fatal("no_rush must not unlock without consumed time")
		}
	}
	if _, ok := store.progress["sandbox#p1"][KeyNoRush]; ok {
		t.Fatal("no_rush progress written for a zero charge")
	}
}
