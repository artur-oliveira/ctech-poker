package v1

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"

	"gopkg.aoctech.app/poker/api/internal/cosmetics"
	"gopkg.aoctech.app/poker/api/internal/player"
)

func TestIdentityCmdForIncludesDecor(t *testing.T) {
	store := &fakePlayerStore{profile: player.PlayerProfile{EquippedFrameID: "gold", EquippedHatID: "crown", EquippedAccessoryID: "shades", EquippedBadgeIDs: []string{"b2", "b1"}}}
	cmd, ok := identityCmdFor(context.Background(), player.NewService(store), nil, nil, "u1")
	want := cosmetics.Decor{Frame: "gold", Hat: "crown", Accessory: "shades", Badges: []string{"b2", "b1"}}
	if !ok || cmd.Decor.Frame != want.Frame || !cmd.Decor.Equal(want) {
		t.Fatalf("ok=%v decor=%+v", ok, cmd.Decor)
	}
}

type countingPusher struct{ n int }

func (p *countingPusher) push(context.Context, string) { p.n++ }

func TestEquipPushesIdentityToTable(t *testing.T) {
	restore := cosmetics.WithCatalogForTest([]cosmetics.CatalogEntry{{Kind: cosmetics.KindHat, ID: "crown", Source: cosmetics.SourceSeason}})
	defer restore()
	patch := func(body string) int {
		pusher := &countingPusher{}
		h := &playerHandlers{players: player.NewService(&fakePlayerStore{}).WithCosmetics(ownsAll{true}), identity: pusher}
		app := fiber.New()
		app.Post("/players/me", func(c fiber.Ctx) error { c.Locals(localsUserID, "u1"); return c.Next() }, h.updateMe)
		req := httptest.NewRequest(fiber.MethodPost, "/players/me", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if resp, err := app.Test(req); err != nil || resp.StatusCode != fiber.StatusOK {
			t.Fatalf("%s: %v %v", body, resp, err)
		}
		return pusher.n
	}
	if n := patch(`{"equipped_hat_id":"crown"}`); n != 1 {
		t.Fatalf("equip pushes = %d, want 1", n)
	}
	if n := patch(`{"bet_preset_mode":"bb"}`); n != 0 {
		t.Fatalf("non-equip pushes = %d, want 0", n)
	}
	if n := patch(`{"name":"Ana","equipped_hat_id":"crown"}`); n != 1 {
		t.Fatalf("rename+equip pushes = %d, want 1", n)
	}
}
