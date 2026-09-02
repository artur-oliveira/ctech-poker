package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/reconcile"
)

func TestLogPendingCashoutsReportsCountAndOldestAge(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	previousNow := timeNow
	timeNow = func() time.Time { return now }
	t.Cleanup(func() { timeNow = previousNow })

	var out bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&out, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	logPendingCashouts([]reconcile.PendingCashout{
		{RecordedAt: now.Add(-3 * time.Minute).Format(time.RFC3339Nano)},
		{RecordedAt: now.Add(-10 * time.Minute).Format(time.RFC3339Nano)},
	})

	if got := out.String(); !strings.Contains(got, `"pending_cashouts":2`) ||
		!strings.Contains(got, `"oldest_pending_cashout_age_seconds":600`) {
		t.Fatalf("unexpected pending cashout log:\n%s", got)
	}
}

type fakePendingLister struct {
	unresolved  []reconcile.PendingCashout
	resolved    []string
	attempts    map[string]int
	quarantined []string
	recordErr   error
}

func (f *fakePendingLister) ListUnresolved(context.Context, time.Duration) ([]reconcile.PendingCashout, error) {
	return f.unresolved, nil
}
func (f *fakePendingLister) MarkResolved(_ context.Context, id string) error {
	f.resolved = append(f.resolved, id)
	return nil
}
func (f *fakePendingLister) RecordFailedAttempt(_ context.Context, e reconcile.PendingCashout, _ error) (int, bool, error) {
	if f.recordErr != nil {
		return 0, false, f.recordErr
	}
	if f.attempts == nil {
		f.attempts = map[string]int{}
	}
	n := e.Attempts + 1
	f.attempts[e.ID] = n
	quarantined := n >= reconcile.MaxAttempts
	if quarantined {
		f.quarantined = append(f.quarantined, e.ID)
	}
	return n, quarantined, nil
}

type stubErrGameCredit struct{ err error }

func (s stubErrGameCredit) CashoutGame(context.Context, string, int64, string, []string, string, string) error {
	return s.err
}

type fakeGameCredit struct {
	cashouts []reconcile.PendingCashout
}

func (f *fakeGameCredit) CashoutGame(_ context.Context, userID string, amount int64, tableRef string, holdIDs []string, idempotencyKey, reason string) error {
	f.cashouts = append(f.cashouts, reconcile.PendingCashout{
		PlayerID:       userID,
		Amount:         amount,
		TableRef:       tableRef,
		HoldIDs:        holdIDs,
		IdempotencyKey: idempotencyKey,
	})
	return nil
}

type fakeSandboxCredit struct {
	credits []reconcile.PendingCashout
}

func (f *fakeSandboxCredit) Credit(_ context.Context, userID string, amount int64, idempotencyKey, reason string) error {
	f.credits = append(f.credits, reconcile.PendingCashout{
		PlayerID:       userID,
		Amount:         amount,
		IdempotencyKey: idempotencyKey,
	})
	return nil
}

type fakeFeeDebiter struct {
	debits []reconcile.PendingCashout
}

func (f *fakeFeeDebiter) DebitReal(_ context.Context, userID string, amount int64, idempotencyKey, reason string) error {
	f.debits = append(f.debits, reconcile.PendingCashout{PlayerID: userID, Amount: amount, IdempotencyKey: idempotencyKey})
	return nil
}

func TestRunResolvesUnresolvedCashouts(t *testing.T) {
	pending := &fakePendingLister{
		unresolved: []reconcile.PendingCashout{
			{ID: "co-1", PlayerID: "user-1", Amount: 400, CurrencyMode: "real", TableRef: "room-1", HoldIDs: []string{"h1"}, IdempotencyKey: "k1"},
			{ID: "co-2", PlayerID: "user-2", Amount: 100, CurrencyMode: "sandbox", IdempotencyKey: "k2"},
		},
	}
	game := &fakeGameCredit{}
	sandbox := &fakeSandboxCredit{}

	if err := run(context.Background(), pending, game, sandbox, &fakeFeeDebiter{}); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(pending.resolved) != 2 {
		t.Fatalf("expected 2 resolved entries, got %v", pending.resolved)
	}
	if len(game.cashouts) != 1 || game.cashouts[0].PlayerID != "user-1" {
		t.Fatalf("expected 1 real cashout, got %+v", game.cashouts)
	}
	if len(sandbox.credits) != 1 || sandbox.credits[0].PlayerID != "user-2" {
		t.Fatalf("expected 1 sandbox credit, got %+v", sandbox.credits)
	}
}

func TestRunCountsTransientFailureWithoutFailingInvocation(t *testing.T) {
	pending := &fakePendingLister{
		unresolved: []reconcile.PendingCashout{
			{ID: "co-1", PlayerID: "user-1", Amount: 400, CurrencyMode: "real", TableRef: "room-1", Attempts: 0},
			{ID: "co-2", PlayerID: "user-2", Amount: 100, CurrencyMode: "sandbox"},
		},
	}
	game := stubErrGameCredit{err: errors.New("wallet down")}

	if err := run(context.Background(), pending, game, &fakeSandboxCredit{}, &fakeFeeDebiter{}); err != nil {
		t.Fatalf("early transient failure must not fail the invocation, got %v", err)
	}
	if pending.attempts["co-1"] != 1 {
		t.Fatalf("expected co-1 attempt counter incremented to 1, got %d", pending.attempts["co-1"])
	}
	// The healthy sandbox entry must still be processed despite co-1 failing.
	if len(pending.resolved) != 1 || pending.resolved[0] != "co-2" {
		t.Fatalf("poison entry blocked the batch: resolved=%v", pending.resolved)
	}
}

func TestRunEscalatesEntryThatExhaustsRetries(t *testing.T) {
	pending := &fakePendingLister{
		unresolved: []reconcile.PendingCashout{
			{ID: "co-1", PlayerID: "user-1", Amount: 400, CurrencyMode: "real", TableRef: "room-1", Attempts: reconcile.MaxAttempts - 1},
			{ID: "co-2", PlayerID: "user-2", Amount: 100, CurrencyMode: "sandbox"},
		},
	}
	game := stubErrGameCredit{err: errors.New("wallet down")}

	err := run(context.Background(), pending, game, &fakeSandboxCredit{}, &fakeFeeDebiter{})
	if err == nil {
		t.Fatal("expected run to return an aggregated error so the Lambda DLQ fires")
	}
	if !strings.Contains(err.Error(), "co-1") || !strings.Contains(err.Error(), "quarantined") {
		t.Fatalf("expected error to name the quarantined entry, got %v", err)
	}
	if len(pending.quarantined) != 1 || pending.quarantined[0] != "co-1" {
		t.Fatalf("expected co-1 quarantined, got %v", pending.quarantined)
	}
	if len(pending.resolved) != 1 || pending.resolved[0] != "co-2" {
		t.Fatalf("healthy entry must still resolve, got %v", pending.resolved)
	}
}

func TestRunRetriesFeeDebit(t *testing.T) {
	pending := &fakePendingLister{
		unresolved: []reconcile.PendingCashout{
			{ID: "fee-1", PlayerID: "user-1", Amount: 100, CurrencyMode: "real", Kind: reconcile.KindFeeDebit, IdempotencyKey: "k1"},
		},
	}
	fee := &fakeFeeDebiter{}

	if err := run(context.Background(), pending, &fakeGameCredit{}, &fakeSandboxCredit{}, fee); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(pending.resolved) != 1 || pending.resolved[0] != "fee-1" {
		t.Fatalf("expected fee-1 resolved, got %v", pending.resolved)
	}
	if len(fee.debits) != 1 || fee.debits[0].PlayerID != "user-1" || fee.debits[0].Amount != 100 {
		t.Fatalf("expected one 100-cent fee retry for user-1, got %+v", fee.debits)
	}
}
