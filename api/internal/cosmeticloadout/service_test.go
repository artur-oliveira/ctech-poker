//go:build integration

package cosmeticloadout

import (
	"context"
	"errors"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/cosmetics"
)

// fakeOwnership lets tests control exactly which items are "owned" without
// standing up a real cosmeticpurchase.Service/wallet — Service only needs
// the one IsOwned method.
type fakeOwnership struct{ owned map[string]bool }

func (f *fakeOwnership) key(kind cosmetics.Kind, itemID string) string {
	return string(kind) + "#" + itemID
}
func (f *fakeOwnership) IsOwned(_ context.Context, _ string, kind cosmetics.Kind, itemID string) (bool, error) {
	return f.owned[f.key(kind, itemID)], nil
}
func (f *fakeOwnership) grant(kind cosmetics.Kind, itemID string) {
	f.owned[f.key(kind, itemID)] = true
}
func (f *fakeOwnership) revoke(kind cosmetics.Kind, itemID string) {
	delete(f.owned, f.key(kind, itemID))
}

func newFakeOwnership() *fakeOwnership { return &fakeOwnership{owned: map[string]bool{}} }

func TestCreateValidatesNameAndSelections(t *testing.T) {
	svc := NewService(newTestStore(t), newFakeOwnership())
	ctx := context.Background()

	if _, err := svc.Create(ctx, "player-1", "   ", map[string]string{"deck": "four-color"}); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("expected ErrInvalidName, got %v", err)
	}
	if _, err := svc.Create(ctx, "player-1", "My combo", map[string]string{}); !errors.Is(err, ErrInvalidSelections) {
		t.Fatalf("expected ErrInvalidSelections for empty selections, got %v", err)
	}
	if _, err := svc.Create(ctx, "player-1", "My combo", map[string]string{"badge": "whatever"}); !errors.Is(err, ErrInvalidSelections) {
		t.Fatalf("expected ErrInvalidSelections for unknown kind, got %v", err)
	}
	if _, err := svc.Create(ctx, "player-1", "My combo", map[string]string{"deck": "not-a-real-deck"}); !errors.Is(err, ErrInvalidSelections) {
		t.Fatalf("expected ErrInvalidSelections for unknown item, got %v", err)
	}

	loadout, err := svc.Create(ctx, "player-1", "My combo", map[string]string{"deck": "four-color", "felt": "classic"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if loadout.Name != "My combo" || loadout.Selections["deck"] != "four-color" || loadout.Selections["felt"] != "classic" {
		t.Fatalf("unexpected loadout: %+v", loadout)
	}
}

// TestCreateRejectsUnownedPremiumItem is issue #313's core acceptance
// criterion: a premium item a player does not own must never make it into a
// saved loadout, mirroring the ownership rule cosmeticpurchase itself
// enforces on selection.
func TestCreateRejectsUnownedPremiumItem(t *testing.T) {
	svc := NewService(newTestStore(t), newFakeOwnership())
	ctx := context.Background()

	if _, err := svc.Create(ctx, "player-1", "Golden combo", map[string]string{"deck": "golden"}); !errors.Is(err, ErrCosmeticNotOwned) {
		t.Fatalf("expected ErrCosmeticNotOwned, got %v", err)
	}
}

func TestCreateAllowsOwnedPremiumItem(t *testing.T) {
	ownership := newFakeOwnership()
	ownership.grant(cosmetics.KindDeck, "golden")
	svc := NewService(newTestStore(t), ownership)
	ctx := context.Background()

	loadout, err := svc.Create(ctx, "player-1", "Golden combo", map[string]string{"deck": "golden"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if loadout.Selections["deck"] != "golden" {
		t.Fatalf("unexpected loadout: %+v", loadout)
	}
}

// TestCreateEnforcesPerPlayerLimit is #313's other explicit acceptance
// criterion: cap the number of loadouts per player so the table (and the
// UI's choice surface) never grows unbounded (issue #220's lesson).
func TestCreateEnforcesPerPlayerLimit(t *testing.T) {
	svc := NewService(newTestStore(t), newFakeOwnership())
	ctx := context.Background()

	for i := 0; i < maxLoadoutsPerPlayer; i++ {
		if _, err := svc.Create(ctx, "player-1", "combo", map[string]string{"deck": "four-color"}); err != nil {
			t.Fatalf("Create #%d: %v", i, err)
		}
	}
	if _, err := svc.Create(ctx, "player-1", "one too many", map[string]string{"deck": "four-color"}); !errors.Is(err, ErrLoadoutLimit) {
		t.Fatalf("expected ErrLoadoutLimit, got %v", err)
	}
	// A different player's own cap is independent.
	if _, err := svc.Create(ctx, "player-2", "combo", map[string]string{"deck": "four-color"}); err != nil {
		t.Fatalf("Create for a different player: %v", err)
	}
}

func TestApplyCallsInjectedApplyFuncPerKind(t *testing.T) {
	ownership := newFakeOwnership()
	ownership.grant(cosmetics.KindFelt, "midnight")
	svc := NewService(newTestStore(t), ownership)
	ctx := context.Background()

	loadout, err := svc.Create(ctx, "player-1", "Combo", map[string]string{"deck": "four-color", "felt": "midnight"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	applied := map[string]string{}
	svc.SetApplyFunc(func(_ context.Context, playerID string, kind cosmetics.Kind, itemID string) error {
		if playerID != "player-1" {
			t.Fatalf("unexpected playerID %q", playerID)
		}
		applied[string(kind)] = itemID
		return nil
	})

	if _, err := svc.Apply(ctx, "player-1", loadout.LoadoutID); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if applied["deck"] != "four-color" || applied["felt"] != "midnight" {
		t.Fatalf("unexpected applied selections: %+v", applied)
	}
}

// TestApplyRevalidatesOwnershipAtApplyTime is #313's fraud-adjacent
// acceptance criterion: a loadout saved while an item was owned must never
// apply that item after it stopped being owned (e.g. refunded in between).
func TestApplyRevalidatesOwnershipAtApplyTime(t *testing.T) {
	ownership := newFakeOwnership()
	ownership.grant(cosmetics.KindDeck, "golden")
	svc := NewService(newTestStore(t), ownership)
	ctx := context.Background()

	loadout, err := svc.Create(ctx, "player-1", "Golden combo", map[string]string{"deck": "golden"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	applyCalled := false
	svc.SetApplyFunc(func(context.Context, string, cosmetics.Kind, string) error {
		applyCalled = true
		return nil
	})

	// Simulate a refund that happened after the loadout was saved.
	ownership.revoke(cosmetics.KindDeck, "golden")

	if _, err := svc.Apply(ctx, "player-1", loadout.LoadoutID); !errors.Is(err, ErrCosmeticNotOwned) {
		t.Fatalf("expected ErrCosmeticNotOwned, got %v", err)
	}
	if applyCalled {
		t.Fatalf("apply must never be called once ownership no longer holds")
	}
}

func TestApplyUnknownLoadoutReturnsNotFound(t *testing.T) {
	svc := NewService(newTestStore(t), newFakeOwnership())
	if _, err := svc.Apply(context.Background(), "player-1", "does-not-exist"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestListAndDelete(t *testing.T) {
	svc := NewService(newTestStore(t), newFakeOwnership())
	ctx := context.Background()

	loadout, err := svc.Create(ctx, "player-1", "combo", map[string]string{"deck": "four-color"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	loadouts, err := svc.List(ctx, "player-1")
	if err != nil || len(loadouts) != 1 {
		t.Fatalf("List: %v, %+v", err, loadouts)
	}
	if err := svc.Delete(ctx, "player-1", loadout.LoadoutID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := svc.Delete(ctx, "player-1", loadout.LoadoutID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound on repeat delete, got %v", err)
	}
	loadouts, err = svc.List(ctx, "player-1")
	if err != nil || len(loadouts) != 0 {
		t.Fatalf("List after delete: %v, %+v", err, loadouts)
	}
}
