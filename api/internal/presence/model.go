// Package presence defines ephemeral friend-visible availability. No table or
// room identifier is part of the public model.
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
}
