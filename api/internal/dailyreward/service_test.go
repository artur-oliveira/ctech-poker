package dailyreward

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"
)

type fakeSpinStore struct {
	records       map[string]DailyRewardRecord
	streaks       map[string]StreakRecord
	completeCalls int
	failComplete  bool
}

func (f *fakeSpinStore) Claim(_ context.Context, playerID, day string, amount int64, streak StreakRecord, _ time.Time) (DailyRewardRecord, error) {
	if f.records == nil {
		f.records = map[string]DailyRewardRecord{}
	}
	key := playerID + "#" + day
	// The day row is create-only and guards the whole transaction, so a
	// duplicate claim must leave the streak row untouched too.
	if record, ok := f.records[key]; ok {
		return record, nil
	}
	record := DailyRewardRecord{Amount: amount, Status: StatusPending}
	f.records[key] = record
	if f.streaks == nil {
		f.streaks = map[string]StreakRecord{}
	}
	f.streaks[playerID] = streak
	return record, nil
}

func (f *fakeSpinStore) Get(_ context.Context, playerID, day string) (DailyRewardRecord, error) {
	return f.records[playerID+"#"+day], nil
}

func (f *fakeSpinStore) LoadStreak(_ context.Context, playerID string) (StreakRecord, error) {
	return f.streaks[playerID], nil
}

func (f *fakeSpinStore) Complete(_ context.Context, playerID, day string, _ time.Time) error {
	f.completeCalls++
	if f.failComplete {
		return errors.New("dynamo unavailable")
	}
	key := playerID + "#" + day
	record := f.records[key]
	record.Status = StatusCompleted
	f.records[key] = record
	return nil
}

type fakeCredit struct {
	amounts []int64
	keys    []string
	fail    bool
}

func (f *fakeCredit) Credit(_ context.Context, _ string, amount int64, key, _ string) error {
	f.amounts = append(f.amounts, amount)
	f.keys = append(f.keys, key)
	if f.fail {
		return errors.New("wallet unavailable")
	}
	return nil
}

var baseDay = time.Date(2026, 7, 19, 12, 0, 0, 0, brt)

// veteran is a player who has already claimed before, so the one-off welcome
// award no longer masks the trail value under test.
func veteran(streak int, lastClaim string) StreakRecord {
	return StreakRecord{CurrentStreak: streak, BestStreak: streak, LastClaimDay: lastClaim, TotalClaims: streak}
}

func fixedService(wallet *fakeCredit, store *fakeSpinStore) *Service {
	s := NewService(wallet, store)
	s.now = func() time.Time { return baseDay }
	return s
}

func atDay(s *Service, offset int) { s.now = func() time.Time { return baseDay.AddDate(0, 0, offset) } }

func TestSpinPersistsThenCreditsAndCompletes(t *testing.T) {
	store := &fakeSpinStore{streaks: map[string]StreakRecord{"p1": veteran(2, "2026-07-18")}}
	wallet := &fakeCredit{}
	amount, _, err := fixedService(wallet, store).Spin(context.Background(), "p1")
	if err != nil || amount != RewardFor(3) {
		t.Fatalf("amount=%d want=%d err=%v", amount, RewardFor(3), err)
	}
	if len(wallet.amounts) != 1 || store.completeCalls != 1 {
		t.Fatalf("credits=%v completes=%d", wallet.amounts, store.completeCalls)
	}
	if got := store.streaks["p1"].CurrentStreak; got != 3 {
		t.Fatalf("streak did not advance: %d", got)
	}
}

func TestPendingSpinRetriesSamePrizeAndIdempotencyKey(t *testing.T) {
	store := &fakeSpinStore{streaks: map[string]StreakRecord{"p1": veteran(2, "2026-07-18")}}
	wallet := &fakeCredit{fail: true}
	svc := fixedService(wallet, store)
	if _, _, err := svc.Spin(context.Background(), "p1"); err == nil {
		t.Fatal("expected first wallet failure")
	}
	wallet.fail = false
	amount, _, err := svc.Spin(context.Background(), "p1")
	if err != nil || amount != RewardFor(3) {
		t.Fatalf("retry amount=%d err=%v", amount, err)
	}
	if len(wallet.amounts) != 2 || wallet.amounts[0] != wallet.amounts[1] || wallet.keys[0] != wallet.keys[1] {
		t.Fatalf("retry changed award/idempotency: amounts=%v keys=%v", wallet.amounts, wallet.keys)
	}
	if got := store.streaks["p1"].CurrentStreak; got != 3 {
		t.Fatalf("retry advanced the streak twice: %d", got)
	}
}

// A second claim on an already-paid day pays nothing: amount 0 is the
// handler's "already claimed today" contract, so Spin can be called
// unconditionally and still reach the retry path of a pending claim.
func TestCompletedSpinPaysNothingAndDoesNotCreditAgain(t *testing.T) {
	store := &fakeSpinStore{streaks: map[string]StreakRecord{"p1": veteran(2, "2026-07-18")}}
	wallet := &fakeCredit{}
	svc := fixedService(wallet, store)
	if _, _, err := svc.Spin(context.Background(), "p1"); err != nil {
		t.Fatal(err)
	}
	amount, remaining, err := svc.Spin(context.Background(), "p1")
	if err != nil || amount != 0 || remaining <= 0 || len(wallet.amounts) != 1 {
		t.Fatalf("amount=%d remaining=%d credits=%d err=%v", amount, remaining, len(wallet.amounts), err)
	}
}

func TestFirstSpinEverAwardsFirstAward(t *testing.T) {
	store, wallet := &fakeSpinStore{}, &fakeCredit{}
	amount, _, err := fixedService(wallet, store).Spin(context.Background(), "new-player")
	if err != nil || amount != FirstAward {
		t.Fatalf("amount=%d err=%v", amount, err)
	}
	if got := store.streaks["new-player"]; got.CurrentStreak != 1 || got.TotalClaims != 1 {
		t.Fatalf("first claim streak: %+v", got)
	}
}

func TestCompletionFailureRetriesWalletSafely(t *testing.T) {
	store := &fakeSpinStore{failComplete: true, streaks: map[string]StreakRecord{"p1": veteran(2, "2026-07-18")}}
	wallet := &fakeCredit{}
	svc := fixedService(wallet, store)
	if _, _, err := svc.Spin(context.Background(), "p1"); err == nil {
		t.Fatal("expected completion failure")
	}
	store.failComplete = false
	if _, _, err := svc.Spin(context.Background(), "p1"); err != nil {
		t.Fatal(err)
	}
	if len(wallet.keys) != 2 || wallet.keys[0] != wallet.keys[1] {
		t.Fatalf("expected same wallet idempotency key, got %v", wallet.keys)
	}
}

// The three streak outcomes the calendar exists to make visible: consecutive
// days advance it, one missed day is absorbed by a protection, and anything
// longer resets it to day 1.
func TestStreakAdvancesProtectsAndResets(t *testing.T) {
	for _, tc := range []struct {
		name       string
		start      StreakRecord
		offsetDays int
		wantStreak int
		wantShield bool
	}{
		{"consecutive day advances", veteran(3, "2026-07-18"), 0, 4, false},
		{"one missed day with protection holds", StreakRecord{CurrentStreak: 8, TotalClaims: 8, LastClaimDay: "2026-07-17", ProtectionAvailable: true}, 0, 9, false},
		{"one missed day without protection resets", veteran(8, "2026-07-17"), 0, 1, false},
		{"two missed days reset even with protection", StreakRecord{CurrentStreak: 8, TotalClaims: 8, LastClaimDay: "2026-07-16", ProtectionAvailable: true}, 0, 1, true},
		{"seventh day grants a protection", veteran(6, "2026-07-18"), 0, 7, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeSpinStore{streaks: map[string]StreakRecord{"p1": tc.start}}
			svc := fixedService(&fakeCredit{}, store)
			atDay(svc, tc.offsetDays)
			amount, _, err := svc.Spin(context.Background(), "p1")
			if err != nil {
				t.Fatal(err)
			}
			got := store.streaks["p1"]
			if got.CurrentStreak != tc.wantStreak {
				t.Fatalf("streak=%d want=%d", got.CurrentStreak, tc.wantStreak)
			}
			if got.ProtectionAvailable != tc.wantShield {
				t.Fatalf("protection=%v want=%v", got.ProtectionAvailable, tc.wantShield)
			}
			if amount != RewardFor(tc.wantStreak) {
				t.Fatalf("amount=%d want=%d", amount, RewardFor(tc.wantStreak))
			}
		})
	}
}

// The final day of the trail is the 1,000,000-chip prize, and the trail
// restarts rather than ending there.
func TestTrailEndsInMillionAndRestarts(t *testing.T) {
	if RewardFor(CycleLength) != 1_000_000 {
		t.Fatalf("day %d pays %d", CycleLength, RewardFor(CycleLength))
	}
	if CycleDayFor(CycleLength+1) != 1 || RewardFor(CycleLength+1) != RewardFor(1) {
		t.Fatalf("trail did not restart: cycleDay=%d", CycleDayFor(CycleLength+1))
	}
}

func TestStatusRendersPendingTodayAndKeepsCooldownField(t *testing.T) {
	store := &fakeSpinStore{streaks: map[string]StreakRecord{"p1": veteran(3, "2026-07-18")}}
	svc := fixedService(&fakeCredit{}, store)

	before, err := svc.Status(context.Background(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	if before.ClaimedToday || before.RemainingTimeSeconds != 0 || !before.StreakAtRisk {
		t.Fatalf("unclaimed status: %+v", before)
	}
	if len(before.Days) != CycleLength || before.CycleDay != 4 {
		t.Fatalf("cycleDay=%d days=%d", before.CycleDay, len(before.Days))
	}
	if before.Days[3].Claimed || !before.Days[3].Today || before.Days[2].Claimed != true {
		t.Fatalf("pending slot wrong: %+v %+v", before.Days[2], before.Days[3])
	}

	if _, _, err := svc.Spin(context.Background(), "p1"); err != nil {
		t.Fatal(err)
	}
	after, err := svc.Status(context.Background(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	if !after.ClaimedToday || after.RemainingTimeSeconds <= 0 || after.StreakAtRisk {
		t.Fatalf("claimed status: %+v", after)
	}
	if !after.Days[3].Claimed || !after.Days[3].Today {
		t.Fatalf("claimed slot wrong: %+v", after.Days[3])
	}
}

// A claim whose wallet credit failed advanced the streak but paid nothing, so
// Status must keep offering it (the client hides the claim button on
// claimed_today) on the same trail slot — not advance it a second time.
func TestStatusOffersAnUnpaidClaimAgain(t *testing.T) {
	store := &fakeSpinStore{streaks: map[string]StreakRecord{"p1": veteran(3, "2026-07-18")}}
	wallet := &fakeCredit{fail: true}
	svc := fixedService(wallet, store)
	if _, _, err := svc.Spin(context.Background(), "p1"); err == nil {
		t.Fatal("expected wallet failure")
	}
	got, err := svc.Status(context.Background(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ClaimedToday || got.RemainingTimeSeconds != 0 || got.CycleDay != 4 || got.Days[3].Claimed || !got.Days[3].Today {
		t.Fatalf("unpaid claim status: claimed=%v remaining=%d cycleDay=%d slot=%+v", got.ClaimedToday, got.RemainingTimeSeconds, got.CycleDay, got.Days[3])
	}
	if got.StreakAtRisk || got.StreakLost {
		t.Fatalf("streak already advanced today cannot be at risk/lost: %+v", got)
	}
}

// Status must tell an at-risk streak (still saveable today) apart from one the
// next claim will reset: showing a dead streak as "at risk" is how a player
// believes a missed day was covered when it was not.
func TestStatusSeparatesAtRiskFromLostStreak(t *testing.T) {
	for _, tc := range []struct {
		name       string
		start      StreakRecord
		wantRisk   bool
		wantLost   bool
		wantCovers bool
	}{
		{"claimed yesterday is at risk", veteran(5, "2026-07-18"), true, false, false},
		{"one missed day with protection is covered", StreakRecord{CurrentStreak: 8, TotalClaims: 8, LastClaimDay: "2026-07-17", ProtectionAvailable: true}, true, false, true},
		{"one missed day without protection is lost", veteran(8, "2026-07-17"), false, true, false},
		{"two missed days are lost even with protection", StreakRecord{CurrentStreak: 8, TotalClaims: 8, LastClaimDay: "2026-07-16", ProtectionAvailable: true}, false, true, false},
		{"never claimed is neither", StreakRecord{}, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeSpinStore{streaks: map[string]StreakRecord{"p1": tc.start}}
			got, err := fixedService(&fakeCredit{}, store).Status(context.Background(), "p1")
			if err != nil {
				t.Fatal(err)
			}
			if got.StreakAtRisk != tc.wantRisk || got.StreakLost != tc.wantLost || got.ProtectionWillCover != tc.wantCovers {
				t.Fatalf("at_risk=%v lost=%v covers=%v, want %v %v %v", got.StreakAtRisk, got.StreakLost, got.ProtectionWillCover, tc.wantRisk, tc.wantLost, tc.wantCovers)
			}
		})
	}
}

// Every claim leaves exactly one structured line with the before/after streak
// state, so "I missed one day and lost everything" can be answered from logs.
func TestSpinLogsOneLinePerClaim(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	store := &fakeSpinStore{streaks: map[string]StreakRecord{"p1": veteran(8, "2026-07-17")}}
	if _, _, err := fixedService(&fakeCredit{}, store).Spin(context.Background(), "p1"); err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) != 1 {
		t.Fatalf("want 1 log line, got %d: %s", len(lines), buf.String())
	}
	var entry map[string]any
	if err := json.Unmarshal(lines[0], &entry); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"msg": "daily reward claimed", "level": "INFO", "player": "p1", "day": "2026-07-19",
		"prev_streak": 8.0, "last_claim_day": "2026-07-17", "gap": 2.0, "protection_available": false,
		"protection_used": false, "new_streak": 1.0, "reset": true, "amount": float64(RewardFor(1)), "retry": false,
	}
	for k, v := range want {
		if entry[k] != v {
			t.Errorf("%s=%v want %v (line %s)", k, entry[k], v, lines[0])
		}
	}
}
