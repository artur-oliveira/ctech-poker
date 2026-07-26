package main

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/reconcile"
)

type fakePendingLister struct {
	unresolved []reconcile.PendingCashout
	resolved   []string
}

func (f *fakePendingLister) ListUnresolved(context.Context, time.Duration) ([]reconcile.PendingCashout, error) {
	return f.unresolved, nil
}
func (f *fakePendingLister) MarkResolved(_ context.Context, id string) error {
	f.resolved = append(f.resolved, id)
	return nil
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
