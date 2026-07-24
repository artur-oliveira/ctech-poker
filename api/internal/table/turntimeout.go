package table

import "time"

// DefaultTurnTimeout is used for every public room and any private room that
// never configured its own turn_timeout_seconds.
const DefaultTurnTimeout = 30 * time.Second

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
const NextHandDelay = 12 * time.Second

// RevealGrace is added on top of the normal per-turn deadline the first time
// a new street (Flop/Turn/River) is dealt, so the board-card reveal
// animation has time to finish before the countdown visibly starts
// pressuring the next player to act. Only the first arm after a stage
// transition gets it — see broadcastAll's stage-change check in actor.go.
const RevealGrace = 2200 * time.Millisecond

// RunoutStreetDelay paces an all-in runout: how long the engine waits after
// dealing one community-card street before dealing the next, once two or
// more streets remain to be revealed after an all-in is accepted.
const RunoutStreetDelay = 2600 * time.Millisecond

// AFKSweepInterval is how often the actor checks every seated player's
// LastActionAt for staleness, independent of whose turn it is — see
// armAFKSweepTimer's doc comment.
const AFKSweepInterval = time.Minute
