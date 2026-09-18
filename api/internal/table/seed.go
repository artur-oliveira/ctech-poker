package table

import (
	"gopkg.aoctech.app/poker/api/internal/engine/deck"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
)

// SeedForRoom builds the first hand.Table for a room. It always configures rake
// from the room's currency mode — the createRoom seed path previously omitted
// ConfigureRake, which would have produced a rake-misconfigured (rakeBPS=0)
// table the moment real-money ships. All three seed call sites use this so the
// contract can't drift.
//
// #296: the room's rule variant picks the deck/evaluator variant, but only
// for sandbox rooms — a real-money room with a non-empty Variant would be a
// createRoom validation bug reaching this far, so it is defended here too
// (fail to Standard) rather than trusting that the only caller of this
// function already enforced it.
func SeedForRoom(room *roomstore.Room) *hand.Table {
	variant := deck.Standard
	if room.CurrencyMode == roomstore.CurrencyModeSandbox && room.Variant == roomstore.VariantShortDeck {
		variant = deck.ShortDeck
	}
	t := hand.NewTableWithVariant(nil, room.SmallBlind, room.BigBlind, variant)
	t.ConfigureRake(room.CurrencyMode)
	return t
}
