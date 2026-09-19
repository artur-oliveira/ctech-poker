package walletalert

import (
	"context"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

func TestEvaluateNoPreferenceConfiguredMeansNoAlert(t *testing.T) {
	if alerts := Evaluate(nil, walletclient.Balances{SandboxBalance: 0}); alerts != nil {
		t.Fatalf("expected no alerts with no preference configured, got %v", alerts)
	}
}

func TestEvaluateFiresOnLowSandboxBalance(t *testing.T) {
	prefs := &Prefs{MinSandboxBalance: 500}

	if alerts := Evaluate(prefs, walletclient.Balances{SandboxBalance: 499}); len(alerts) != 1 || alerts[0].Kind != AlertLowSandboxBalance {
		t.Fatalf("expected a low-balance alert, got %v", alerts)
	}
	if alerts := Evaluate(prefs, walletclient.Balances{SandboxBalance: 500}); alerts != nil {
		t.Fatalf("expected no alert exactly at the threshold, got %v", alerts)
	}
	if alerts := Evaluate(prefs, walletclient.Balances{SandboxBalance: 10000}); alerts != nil {
		t.Fatalf("expected no alert well above the threshold, got %v", alerts)
	}
}

func TestEvaluateZeroThresholdNeverFires(t *testing.T) {
	prefs := &Prefs{MinSandboxBalance: 0}
	if alerts := Evaluate(prefs, walletclient.Balances{SandboxBalance: 0}); alerts != nil {
		t.Fatalf("expected a zero (unconfigured) threshold to never fire, got %v", alerts)
	}
}

func TestEvaluatePurchaseFiresOverLimit(t *testing.T) {
	prefs := &Prefs{MaxPurchaseCents: 1000}
	if alerts := EvaluatePurchase(prefs, 1001); len(alerts) != 1 || alerts[0].Kind != AlertPurchaseLimitExceeded {
		t.Fatalf("expected a purchase-limit alert, got %v", alerts)
	}
	if alerts := EvaluatePurchase(prefs, 1000); alerts != nil {
		t.Fatalf("expected no alert exactly at the limit, got %v", alerts)
	}
	if alerts := EvaluatePurchase(nil, 999999); alerts != nil {
		t.Fatalf("expected no alert with no preference configured, got %v", alerts)
	}
}

// fakeStore is an in-memory stand-in for Store, for Service-level tests.
type fakeStore struct{ rows map[string]Prefs }

func newFakeStore() *fakeStore { return &fakeStore{rows: map[string]Prefs{}} }

func (f *fakeStore) Get(_ context.Context, playerID string) (*Prefs, error) {
	p, ok := f.rows[playerID]
	if !ok {
		return nil, nil
	}
	return &p, nil
}
func (f *fakeStore) Put(_ context.Context, prefs Prefs) error {
	f.rows[prefs.PlayerID] = prefs
	return nil
}
func (f *fakeStore) Delete(_ context.Context, playerID string) error {
	delete(f.rows, playerID)
	return nil
}

func TestServiceSetGetDeleteRoundTrip(t *testing.T) {
	svc := NewService(newFakeStore())
	ctx := context.Background()

	if prefs, err := svc.Get(ctx, "player-1"); err != nil || prefs != nil {
		t.Fatalf("expected no prefs before Set, got %+v err=%v", prefs, err)
	}

	if _, err := svc.Set(ctx, "player-1", 500, 2000); err != nil {
		t.Fatalf("Set: %v", err)
	}
	prefs, err := svc.Get(ctx, "player-1")
	if err != nil || prefs == nil || prefs.MinSandboxBalance != 500 || prefs.MaxPurchaseCents != 2000 {
		t.Fatalf("unexpected prefs after Set: %+v err=%v", prefs, err)
	}

	if err := svc.Delete(ctx, "player-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if prefs, err := svc.Get(ctx, "player-1"); err != nil || prefs != nil {
		t.Fatalf("expected no prefs after Delete, got %+v err=%v", prefs, err)
	}
}
