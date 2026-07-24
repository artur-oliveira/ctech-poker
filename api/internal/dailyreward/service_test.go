package dailyreward

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeSpinStore struct {
	records       map[string]DailyRewardRecord
	completeCalls int
	failComplete  bool
	seenPlayers   map[string]bool
}

func (f *fakeSpinStore) Claim(_ context.Context, playerID, day string, amount int64, _ time.Time) (DailyRewardRecord, error) {
	if f.records == nil {
		f.records = map[string]DailyRewardRecord{}
	}
	key := playerID + "#" + day
	if record, ok := f.records[key]; ok {
		return record, nil
	}
	record := DailyRewardRecord{Amount: amount, Status: StatusPending}
	f.records[key] = record
	if f.seenPlayers == nil {
		f.seenPlayers = map[string]bool{}
	}
	f.seenPlayers[playerID] = true
	return record, nil
}

func (f *fakeSpinStore) IsFirstReward(_ context.Context, playerID string) (bool, error) {
	return !f.seenPlayers[playerID], nil
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

func (f *fakeSpinStore) Get(_ context.Context, playerID, day string) (DailyRewardRecord, error) {
	if f.records == nil {
		f.records = map[string]DailyRewardRecord{}
	}
	key := playerID + "#" + day
	if record, ok := f.records[key]; ok {
		return record, nil
	}
	return DailyRewardRecord{}, nil
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

func fixedService(wallet *fakeCredit, store *fakeSpinStore) *Service {
	s := NewService(wallet, store)
	s.now = func() time.Time { return time.Date(2026, 7, 19, 12, 0, 0, 0, brt) }
	s.pick = func() (int64, error) { return 500, nil }
	return s
}

func TestSpinPersistsThenCreditsAndCompletes(t *testing.T) {
	store, wallet := &fakeSpinStore{seenPlayers: map[string]bool{"p1": true}}, &fakeCredit{}
	amount, _, err := fixedService(wallet, store).Spin(context.Background(), "p1")
	if err != nil || amount != 500 {
		t.Fatalf("amount=%d err=%v", amount, err)
	}
	if len(wallet.amounts) != 1 || store.completeCalls != 1 {
		t.Fatalf("credits=%v completes=%d", wallet.amounts, store.completeCalls)
	}
}

func TestPendingSpinRetriesSamePrizeAndIdempotencyKey(t *testing.T) {
	store, wallet := &fakeSpinStore{seenPlayers: map[string]bool{"p1": true}}, &fakeCredit{fail: true}
	svc := fixedService(wallet, store)
	if _, _, err := svc.Spin(context.Background(), "p1"); err == nil {
		t.Fatal("expected first wallet failure")
	}
	wallet.fail = false
	svc.pick = func() (int64, error) { return 1000, nil }
	amount, _, err := svc.Spin(context.Background(), "p1")
	if err != nil || amount != 500 {
		t.Fatalf("retry amount=%d err=%v", amount, err)
	}
	if len(wallet.amounts) != 2 || wallet.amounts[0] != wallet.amounts[1] || wallet.keys[0] != wallet.keys[1] {
		t.Fatalf("retry changed award/idempotency: amounts=%v keys=%v", wallet.amounts, wallet.keys)
	}
}

func TestCompletedSpinReturnsStoredPrizeWithoutCreditingAgain(t *testing.T) {
	store, wallet := &fakeSpinStore{seenPlayers: map[string]bool{"p1": true}}, &fakeCredit{}
	svc := fixedService(wallet, store)
	if _, _, err := svc.Spin(context.Background(), "p1"); err != nil {
		t.Fatal(err)
	}
	amount, _, err := svc.Spin(context.Background(), "p1")
	if err != nil || amount != 500 || len(wallet.amounts) != 1 {
		t.Fatalf("amount=%d credits=%d err=%v", amount, len(wallet.amounts), err)
	}
}

func TestFirstSpinEverAwardsFirstAwardRegardlessOfPick(t *testing.T) {
	store, wallet := &fakeSpinStore{}, &fakeCredit{}
	amount, _, err := fixedService(wallet, store).Spin(context.Background(), "new-player")
	if err != nil || amount != FirstAward {
		t.Fatalf("amount=%d err=%v", amount, err)
	}
}

func TestCompletionFailureRetriesWalletSafely(t *testing.T) {
	store, wallet := &fakeSpinStore{failComplete: true, seenPlayers: map[string]bool{"p1": true}}, &fakeCredit{}
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
