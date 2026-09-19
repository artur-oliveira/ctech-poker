package table

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/deck"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
)

func TestSeedForRoomPicksVariant(t *testing.T) {
	cases := []struct {
		name string
		room roomstore.Room
		want deck.Variant
	}{
		{"sandbox standard", roomstore.Room{CurrencyMode: roomstore.CurrencyModeSandbox}, deck.Standard},
		{"sandbox short deck", roomstore.Room{CurrencyMode: roomstore.CurrencyModeSandbox, Variant: roomstore.VariantShortDeck}, deck.ShortDeck},
		// Defence in depth: even if createRoom validation were bypassed, a
		// real-money room must never seed a variant table.
		{"real money ignores variant", roomstore.Room{CurrencyMode: roomstore.CurrencyModeReal, Variant: roomstore.VariantShortDeck}, deck.Standard},
	}
	for _, tc := range cases {
		room := tc.room
		room.SmallBlind, room.BigBlind = 10, 20
		if got := SeedForRoom(&room).Variant(); got != tc.want {
			t.Errorf("%s: variant=%v, want %v", tc.name, got, tc.want)
		}
	}
}
