package catalog

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/cosmetics"
)

// Every sellable cosmetic must have a Billing product, and avatar fichas prices are
// exactly the cents price x 1000 (one price list, two currencies).
func TestCosmeticSKUsMatchProducts(t *testing.T) {
	for _, k := range []cosmetics.Kind{cosmetics.KindDeck, cosmetics.KindFelt, cosmetics.KindFrame, cosmetics.KindHat, cosmetics.KindAccessory, cosmetics.KindBadge} {
		for _, e := range cosmetics.All(k) {
			if e.SKU == "" {
				continue
			}
			p, ok := ProductByID(e.SKU)
			if !ok {
				t.Errorf("%s/%s: SKU %s has no catalog product", e.Kind, e.ID, e.SKU)
				continue
			}
			if e.Source == cosmetics.SourcePurchase && e.PriceFichas != p.PriceCents*1000 {
				t.Errorf("%s/%s: %d fichas, want %d (cents x 1000)", e.Kind, e.ID, e.PriceFichas, p.PriceCents*1000)
			}
		}
	}
}
