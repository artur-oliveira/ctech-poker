package buyin

import "testing"

// A player can be seated, system-removed, rebuy, and be system-removed again
// from the SAME table for the SAME reason. The pending-cashouts row is written
// create-only and co-committed atomically with the seat removal, so if every
// one of those removals keyed the same string, the second removal's whole
// transaction failed its condition forever and the seat could never be pulled
// — the player was wedged "leaving…". The per-removal nonce is what keeps the
// keys distinct; the bare-key fallback only exists for a replayed pre-fix row.
func TestSystemLeaveKeyIsUniquePerRemoval(t *testing.T) {
	a := systemLeaveKey("room-1", "player-1", "exit_requested", "nonce-a")
	b := systemLeaveKey("room-1", "player-1", "exit_requested", "nonce-b")
	if a == b {
		t.Fatalf("two removals at the same table/player/reason must not share a key: %q", a)
	}

	legacy := systemLeaveKey("room-1", "player-1", "exit_requested", "")
	if legacy != "room-1#player-1#system_leave#exit_requested" {
		t.Fatalf("empty nonce must reproduce the legacy key, got %q", legacy)
	}
	if a == legacy {
		t.Fatalf("a nonce'd key must not collide with the legacy key: %q", a)
	}
}
