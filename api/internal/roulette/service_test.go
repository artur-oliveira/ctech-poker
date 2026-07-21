package roulette

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeSpinStore struct {
	records       map[string]SpinRecord
	completeCalls int
	failComplete  bool
}

func (f *fakeSpinStore) Claim(_ context.Context, playerID, day string, amount int64, _ time.Time) (SpinRecord, error) {
	if f.records == nil {
		f.records = map[string]SpinRecord{}
	}
	key := playerID + "#" + day
	if record, ok := f.records[key]; ok {
		return record, nil
	}
	record := SpinRecord{Amount: amount, Status: StatusPending}
	f.records[key] = record
	return record, nil
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

func (f *fakeSpinStore) Get(_ context.Context, playerID, day string) (SpinRecord, error) {
	if f.records == nil {
		f.records = map[string]SpinRecord{}
	}
	key := playerID + "#" + day
	if record, ok := f.records[key]; ok {
		return record, nil
	}
	return SpinRecord{}, nil
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
	store, wallet := &fakeSpinStore{}, &fakeCredit{}
	amount, err := fixedService(wallet, store).Spin(context.Background(), "p1")
	if err != nil || amount != 500 {
		t.Fatalf("amount=%d err=%v", amount, err)
	}
	if len(wallet.amounts) != 1 || store.completeCalls != 1 {
		t.Fatalf("credits=%v completes=%d", wallet.amounts, store.completeCalls)
	}
}

func TestPendingSpinRetriesSamePrizeAndIdempotencyKey(t *testing.T) {
	store, wallet := &fakeSpinStore{}, &fakeCredit{fail: true}
	svc := fixedService(wallet, store)
	if _, err := svc.Spin(context.Background(), "p1"); err == nil {
		t.Fatal("expected first wallet failure")
	}
	wallet.fail = false
	svc.pick = func() (int64, error) { return 1000, nil }
	amount, err := svc.Spin(context.Background(), "p1")
	if err != nil || amount != 500 {
		t.Fatalf("retry amount=%d err=%v", amount, err)
	}
	if len(wallet.amounts) != 2 || wallet.amounts[0] != wallet.amounts[1] || wallet.keys[0] != wallet.keys[1] {
		t.Fatalf("retry changed award/idempotency: amounts=%v keys=%v", wallet.amounts, wallet.keys)
	}
}

func TestCompletedSpinReturnsStoredPrizeWithoutCreditingAgain(t *testing.T) {
	store, wallet := &fakeSpinStore{}, &fakeCredit{}
	svc := fixedService(wallet, store)
	if _, err := svc.Spin(context.Background(), "p1"); err != nil {
		t.Fatal(err)
	}
	amount, err := svc.Spin(context.Background(), "p1")
	if err != nil || amount != 500 || len(wallet.amounts) != 1 {
		t.Fatalf("amount=%d credits=%d err=%v", amount, len(wallet.amounts), err)
	}
}

func TestCompletionFailureRetriesWalletSafely(t *testing.T) {
	store, wallet := &fakeSpinStore{failComplete: true}, &fakeCredit{}
	svc := fixedService(wallet, store)
	if _, err := svc.Spin(context.Background(), "p1"); err == nil {
		t.Fatal("expected completion failure")
	}
	store.failComplete = false
	if _, err := svc.Spin(context.Background(), "p1"); err != nil {
		t.Fatal(err)
	}
	if len(wallet.keys) != 2 || wallet.keys[0] != wallet.keys[1] {
		t.Fatalf("expected same wallet idempotency key, got %v", wallet.keys)
	}
}
