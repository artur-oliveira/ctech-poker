// Package presence defines ephemeral friend-visible availability. A room
// identifier is carried only so the social API can offer "join my friend's
// table"; publishing it requires the player's own opt-in (player.TablePublic)
// AND a public, joinable room — both gated in api/v1/social.go, never here.
package presence

type Status string

const (
	StatusOffline Status = "offline"
	StatusOnline  Status = "online"
	StatusInTable Status = "in_table"
)

type PlayerPresence struct {
	PlayerID string
	Status   Status
	// RoomID is the room this player is sitting in, or "" when it is unknown
	// (offline, not in a table, or a key written before rooms were tracked).
	// Never published without the gates in api/v1/social.go.
	RoomID string
}
