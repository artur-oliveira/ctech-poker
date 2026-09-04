//go:build integration

package cosmeticpurchase

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/poker/api/internal/cosmetics"
)

func TestEntitlementStorePutGetDelete(t *testing.T) {
	s := newTestEntitlementStore(t)
	ctx := context.Background()

	e := Entitlement{PlayerID: "player-1", Kind: string(cosmetics.KindDeck), ItemID: "golden", PurchaseMethod: "fichas", PurchaseID: "cp-1", CreatedAt: "2026-08-21T00:00:00Z"}
	if err := s.Put(ctx, e); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := s.Get(ctx, "player-1", cosmetics.KindDeck, "golden")
	if err != nil || got == nil || got.ItemID != "golden" {
		t.Fatalf("Get: %v, %+v", err, got)
	}
	// A felt entitlement with the same item id must not collide with the deck one.
	if got, err := s.Get(ctx, "player-1", cosmetics.KindFelt, "golden"); err != nil || got != nil {
		t.Fatalf("cross-kind lookup must not find the deck entitlement: %+v (err=%v)", got, err)
	}
	if err := s.Delete(ctx, "player-1", cosmetics.KindDeck, "golden"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err = s.Get(ctx, "player-1", cosmetics.KindDeck, "golden")
	if err != nil || got != nil {
		t.Fatalf("expected nil after Delete, got %+v (err=%v)", got, err)
	}
}

func TestStoreCreateIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	rec := Record{PlayerID: "player-1", PurchaseID: "cp-2", Kind: string(cosmetics.KindFelt), ItemID: "midnight", Method: "fichas", PriceFichas: 200_000, Status: "confirmed", CreatedAt: "2026-08-21T00:00:00Z", UpdatedAt: "2026-08-21T00:00:00Z"}
	got1, err := s.Create(ctx, rec)
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	got2, err := s.Create(ctx, rec)
	if err != nil || got2.PurchaseID != got1.PurchaseID {
		t.Fatalf("replay Create must return the existing row: %v, %+v", err, got2)
	}
}

// TestListPagesOneKindWithoutScanningTheOther is issue #219's acceptance: the
// deck history must never read a felt row, every page must be as full as the
// requested limit while rows remain, and the cursor must walk the whole
// history exactly once.
func TestListPagesOneKindWithoutScanningTheOther(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	// Interleaved ids, so an index that did not key on kind would hand back a
	// page that is half felt.
	for i := 0; i < 10; i++ {
		kind := cosmetics.KindDeck
		if i%2 == 1 {
			kind = cosmetics.KindFelt
		}
		rec := Record{
			PlayerID: "player-1", PurchaseID: fmt.Sprintf("cp-%02d", i), Kind: string(kind),
			ItemID: "item", Method: "fichas", PriceFichas: 100, Status: "confirmed",
			CreatedAt: fmt.Sprintf("2026-08-21T00:00:%02dZ", i), UpdatedAt: "2026-08-21T00:00:00Z",
		}
		if _, err := s.Create(ctx, rec); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}

	seen := map[string]bool{}
	var startKey = map[string]types.AttributeValue(nil)
	for page := 0; page < 3; page++ {
		records, next, err := s.List(ctx, "player-1", cosmetics.KindDeck, 2, startKey)
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		want := 2
		if page == 2 {
			want = 1
		}
		if len(records) != want {
			t.Fatalf("page %d: expected a full page of %d deck rows, got %d", page, want, len(records))
		}
		for _, rec := range records {
			if rec.Kind != string(cosmetics.KindDeck) {
				t.Fatalf("page %d returned a %s row: %+v", page, rec.Kind, rec)
			}
			if seen[rec.PurchaseID] {
				t.Fatalf("page %d repeated %s", page, rec.PurchaseID)
			}
			seen[rec.PurchaseID] = true
		}
		startKey = next
		if page < 2 && startKey == nil {
			t.Fatalf("page %d ended pagination early, saw %d of 5 deck rows", page, len(seen))
		}
	}
	if len(seen) != 5 {
		t.Fatalf("expected all 5 deck purchases across the pages, got %d", len(seen))
	}
	if startKey != nil {
		if records, _, err := s.List(ctx, "player-1", cosmetics.KindDeck, 2, startKey); err != nil || len(records) != 0 {
			t.Fatalf("expected the history to be exhausted, got %d rows (err=%v)", len(records), err)
		}
	}
}
