package table

import (
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
)

// SeedForRoom builds the first hand.Table for a room. It always configures rake
// from the room's currency mode — the createRoom seed path previously omitted
// ConfigureRake, which would have produced a rake-misconfigured (rakeBPS=0)
// table the moment real-money ships. All three seed call sites use this so the
// contract can't drift.
func SeedForRoom(room *roomstore.Room) *hand.Table {
	t := hand.NewTable(nil, room.SmallBlind, room.BigBlind)
	t.ConfigureRake(room.CurrencyMode)
	return t
}
