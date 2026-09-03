package table

import "time"

// These are the product-level timing defaults. Keep the human-readable values
// in seconds here so changing either rule is a one-line edit; the duration
// constants below are what timers should consume.
const (
	DefaultTurnTimeoutSeconds = 15
	NextHandDelaySeconds      = 12

	// DefaultTurnTimeout is used for every public room and any private room
	// that never configured its own turn_timeout_seconds.
	DefaultTurnTimeout = DefaultTurnTimeoutSeconds * time.Second
	// NextHandDelay is how long the table waits after a hand reaches Complete
	// before auto-starting the next one.
	NextHandDelay = NextHandDelaySeconds * time.Second
)

// NextHandRetryDelay/MaxNextHandRetries bound handleNextHand's re-arm after a
// transient failure (a cancelled DynamoDB context on the load or the commit).
// The countdown timer that dispatched the command is already spent at that
// point, so without a re-arm a quiet table sits on Complete until some other
// command happens to reach the actor. Backoff is linear in the attempt count;
// past the cap the AFK sweep (the only unconditional tick) stays the
// last-resort watchdog, so retrying forever here buys nothing.
const (
	NextHandRetryDelay = 2 * time.Second
	MaxNextHandRetries = 5
)

// MaxNextHandArmsPerHand caps how many times armNextHandTimer will (re-)arm
// the post-hand timer for ONE hand. rearmTimersFromCache re-derives this timer
// on every reconnect, keepalive ping and AFK sweep, and handleNextHand clears
// nextHandArmedFor on entry (#136) so the "same hand" idempotence check stops
// throttling the moment the timer first fires. When the persisted next-hand
// deadline is already in the past the re-armed timer fires instantly — so on a
// wedged table (a seat that will not leave) a client reconnect loop became ~8
// rejected next-hand DynamoDB transactions per second, each still billed,
// until the table was cleaned up (2026-09-02 incident,
// docs/specs/2026-09-03-next-hand-rearm-storm.md).
//
// Past the cap the timer is left un-armed: a table this stuck recovers via
// tablecleanup's sweep or an operator, not by retrying a transaction that
// keeps being rejected. The count resets the moment a.handID changes (a hand
// actually started) or the table leaves Complete. handleNextHand's own
// transient-failure re-arm (retryNextHand, bounded by MaxNextHandRetries) does
// not go through armNextHandTimer and is counted separately.
const MaxNextHandArmsPerHand = 12

// TurnTimeoutFor resolves a room's configured turn_timeout_seconds (0 means
// "not configured") to a duration.
func TurnTimeoutFor(seconds int) time.Duration {
	if seconds <= 0 {
		return DefaultTurnTimeout
	}
	return time.Duration(seconds) * time.Second
}

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
