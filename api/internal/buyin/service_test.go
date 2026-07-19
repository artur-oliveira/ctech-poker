//go:build integration

package buyin

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

type fakeWallet struct {
	credits []call
	debits  []call
}
type call struct {
	userID string
	amount int64
	key    string
}

func (f *fakeWallet) Credit(_ context.Context, userID string, amount int64, key, _ string) error {
	f.credits = append(f.credits, call{userID, amount, key})
	return nil
}
func (f *fakeWallet) Debit(_ context.Context, userID string, amount int64, key, _ string) error {
	f.debits = append(f.debits, call{userID, amount, key})
	return nil
}

func testManager(t *testing.T) *tablemanager.Manager {
	t.Helper()
	db := testClient(t)
	env := fmt.Sprintf("buyin_test_%d", time.Now().UnixNano())
	mustCreateTestTables(t, db, env)
	store := tablestore.NewStore(db, env)
	return tablemanager.NewManager(tablelease.NewService(cache.NewMemoryBackend(16)), store, nil)
}

func TestBuyInDebitsThenSeats(t *testing.T) {
	wallet := &fakeWallet{}
	mgr := testManager(t)
	svc := NewService(wallet, mgr, nil)
	ctx := context.Background()

	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	actor, err := mgr.GetOrCreateActor(ctx, "room-1", seed)
	if err != nil {
		t.Fatalf("get or create actor: %v", err)
	}

	if err := svc.BuyIn(ctx, "room-1", "user-1", 400, false); err != nil {
		t.Fatalf("buyin: %v", err)
	}
	if len(wallet.debits) != 1 || wallet.debits[0].amount != 400 {
		t.Fatalf("expected one 400-chip debit, got %+v", wallet.debits)
	}
	found := false
	for _, s := range actor.TableForTest().ViewFor("user-1").Seats {
		if s.PlayerID == "user-1" && s.Stack == 400 {
			found = true
		}
	}
	if !found {
		t.Fatal("expected user-1 seated with a 400-chip stack after buy-in")
	}
}

func TestCashOutRemovesThenCredits(t *testing.T) {
	wallet := &fakeWallet{}
	mgr := testManager(t)
	svc := NewService(wallet, mgr, nil)
	ctx := context.Background()

	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	if _, err := mgr.GetOrCreateActor(ctx, "room-2", seed); err != nil {
		t.Fatalf("get or create actor: %v", err)
	}
	if err := svc.BuyIn(ctx, "room-2", "user-1", 400, false); err != nil {
		t.Fatalf("buyin: %v", err)
	}

	stack, err := svc.CashOut(ctx, "room-2", "user-1")
	if err != nil {
		t.Fatalf("cashout: %v", err)
	}
	if stack != 400 {
		t.Fatalf("expected cash-out amount 400, got %d", stack)
	}
	if len(wallet.credits) != 1 || wallet.credits[0].amount != 400 {
		t.Fatalf("expected one 400-chip credit, got %+v", wallet.credits)
	}
}
