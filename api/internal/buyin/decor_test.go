//go:build integration

package buyin

import (
	"context"
	"slices"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/player"
)

type decorProfiles struct{ unacceptedProfiles }

func (decorProfiles) GetOrCreate(context.Context, string) (*player.PlayerProfile, error) {
	return &player.PlayerProfile{UserID: "user-1", Name: "Ann", PokerTermsVersion: player.CurrentPokerTermsVersion, TermsAcceptedAt: "2026-01-01T00:00:00Z",
		EquippedFrameID: "gold", EquippedHatID: "crown", EquippedAccessoryID: "shades", EquippedBadgeIDs: []string{"b2", "b1"}}, nil
}

func TestBuyinIdentityIncludesDecor(t *testing.T) {
	mgr := testManager(t)
	svc := NewService(&fakeWallet{}, mgr, testRoomLookup()).WithPlayers(player.NewService(decorProfiles{}))
	ctx := context.Background()
	actor, err := mgr.GetOrCreateActor(ctx, "room-1", func() *hand.Table { return hand.NewTable(nil, 10, 20) })
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.BuyIn(ctx, "room-1", "user-1", 400, false, ""); err != nil {
		t.Fatalf("buyin: %v", err)
	}
	for _, s := range actor.TableForTest().ViewFor("user-1").Seats {
		if s.PlayerID == "user-1" {
			if s.FrameID != "gold" || s.HatID != "crown" || s.AccessoryID != "shades" || !slices.Equal(s.BadgeIDs, []string{"b2", "b1"}) {
				t.Fatalf("seat decor = %+v", s)
			}
			return
		}
	}
	t.Fatal("user-1 not seated")
}
