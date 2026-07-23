package table

import "time"

// DefaultTurnTimeout is used for every public room and any private room that
// never configured its own turn_timeout_seconds.
const DefaultTurnTimeout = 15 * time.Second

// TurnTimeoutFor resolves a room's configured turn_timeout_seconds (0 means
// "not configured") to a duration.
func TurnTimeoutFor(seconds int) time.Duration {
	if seconds <= 0 {
		return DefaultTurnTimeout
	}
	return time.Duration(seconds) * time.Second
}

// NextHandDelay is how long the table waits after a hand reaches Complete
// before auto-starting the next one.
const NextHandDelay = 5 * time.Second
