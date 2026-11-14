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

// ownsKinds owns only the listed kinds, so one request can mix an owned and an
// unowned equip.
type ownsKinds map[cosmetics.Kind]bool

func (o ownsKinds) IsOwned(_ context.Context, _ string, k cosmetics.Kind, _ string) (bool, error) {
	return o[k], nil
}

// A write that landed before a later field fails must still reach the table
// exactly once; a request that wrote no identity field pushes nothing.
func TestPartialFailureStillPushesIdentityOnce(t *testing.T) {
	restore := cosmetics.WithCatalogForTest([]cosmetics.CatalogEntry{
		{Kind: cosmetics.KindFrame, ID: "gold", Source: cosmetics.SourceSeason},
		{Kind: cosmetics.KindHat, ID: "crown", Source: cosmetics.SourceSeason},
	})
	defer restore()
	run := func(body string) (int, int) {
		pusher := &countingPusher{}
		owner := ownsKinds{cosmetics.KindFrame: true}
		h := &playerHandlers{players: player.NewService(&fakePlayerStore{}).WithCosmetics(owner), identity: pusher}
		app := fiber.New()
		app.Post("/players/me", func(c fiber.Ctx) error { c.Locals(localsUserID, "u1"); return c.Next() }, h.updateMe)
		req := httptest.NewRequest(fiber.MethodPost, "/players/me", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, pusher.n
	}
	for _, tc := range []struct {
		body string
		want int
	}{
		{`{"name":"Ana","wallet_mode":"bogus"}`, 1},
		{`{"equipped_frame_id":"gold","equipped_hat_id":"crown"}`, 1},
		{`{"wallet_mode":"bogus"}`, 0},
		{`{"equipped_hat_id":"crown"}`, 0},
	} {
		if code, n := run(tc.body); code != fiber.StatusBadRequest || n != tc.want {
			t.Fatalf("%s: status=%d pushes=%d, want 400 and %d", tc.body, code, n, tc.want)
		}
	}
}
