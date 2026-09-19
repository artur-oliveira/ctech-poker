package v1

import (
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/cosmeticpurchase"
	"gopkg.aoctech.app/poker/api/internal/cosmetics"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

// RegisterAvatarCosmeticsOwnership mounts the one read the seasonal
// frame/badge equip UI (#292) needs: which ids of a given kind this player
// owns. Deliberately not RegisterCosmeticPurchase's `/wallet/cosmetic-purchase`
// group — that group's :kind param, catalog and create/refund routes are
// wallet-priced deck/felt purchases, and frame/badge ownership is granted
// (cosmeticpurchase.Service.Grant), never sold.
func RegisterAvatarCosmeticsOwnership(router fiber.Router, auth fiber.Handler, svc *cosmeticpurchase.Service) {
	router.Get("/players/me/cosmetics/:kind/owned", auth, validFrameOrBadgeKind, func(c fiber.Ctx) error {
		kind := cosmetics.Kind(c.Params("kind"))
		userID := c.Locals(localsUserID).(string)
		owned, err := svc.OwnedIDs(c.Context(), userID, kind)
		if err != nil {
			return problem.InternalServer("list owned cosmetics failed", c, err).Send(c)
		}
		ids := make([]string, 0, len(owned))
		for id := range owned {
			ids = append(ids, id)
		}
		return sendPage(c, ids, nil, "")
	})
}

func validFrameOrBadgeKind(c fiber.Ctx) error {
	kind := cosmetics.Kind(c.Params("kind"))
	if kind != cosmetics.KindFrame && kind != cosmetics.KindBadge {
		return problem.BadRequest("kind must be \"frame\" or \"badge\"").Send(c)
	}
	return c.Next()
}
