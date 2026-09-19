package v1

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/cosmeticpurchase"
)

func TestAvatarCosmeticsOwnershipRejectsUnknownKind(t *testing.T) {
	svc := cosmeticpurchase.NewService(&fakeCosmeticWallet{}, cosmeticpurchase.NewEntitlementStore(stubDynamoClient(t), "test"), cosmeticpurchase.NewStore(nil, "test"))
	app := fiber.New()
	auth := func(c fiber.Ctx) error { c.Locals(localsUserID, "u1"); return c.Next() }
	RegisterAvatarCosmeticsOwnership(app, auth, svc)

	for _, kind := range []string{"deck", "felt", "not-a-kind"} {
		req := httptest.NewRequest(fiber.MethodGet, "/players/me/cosmetics/"+kind+"/owned", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Fatalf("kind=%q: status = %d, want 400", kind, resp.StatusCode)
		}
	}
}

func TestAvatarCosmeticsOwnershipReturnsOwnedIDs(t *testing.T) {
	svc := cosmeticpurchase.NewService(&fakeCosmeticWallet{}, cosmeticpurchase.NewEntitlementStore(stubDynamoClient(t), "test"), cosmeticpurchase.NewStore(nil, "test"))
	app := fiber.New()
	auth := func(c fiber.Ctx) error { c.Locals(localsUserID, "u1"); return c.Next() }
	RegisterAvatarCosmeticsOwnership(app, auth, svc)

	for _, kind := range []string{"frame", "badge"} {
		req := httptest.NewRequest(fiber.MethodGet, "/players/me/cosmetics/"+kind+"/owned", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("kind=%q: status = %d, want 200", kind, resp.StatusCode)
		}
	}
}
