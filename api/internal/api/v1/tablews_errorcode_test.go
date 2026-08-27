package v1

import (
	"errors"
	"fmt"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

// A store or actor failure aborts a command before the engine ever judges it,
// so it must not be reported as an illegal move: "invalid_action" blames the
// player for an outage, and the client ends the command on it instead of
// resyncing and resubmitting. Wrapping matters as much as the sentinel — every
// real failure arrives wrapped in context by the time it reaches the gateway.
func TestActionErrorCodeSeparatesOutagesFromIllegalMoves(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"store unavailable", tablestore.ErrUnavailable, "unavailable"},
		{"store unavailable, wrapped", fmt.Errorf("%w: get table: throttled", tablestore.ErrUnavailable), "unavailable"},
		{"actor stopped", table.ErrActorStopped, "unavailable"},
		{"actor stopped, wrapped", fmt.Errorf("dispatch: %w", table.ErrActorStopped), "unavailable"},
		{"engine rejection", errors.New("hand: cards can only be revealed after the hand is complete"), "invalid_action"},
		{"version conflict is a real rejection", tablestore.ErrVersionConflict, "invalid_action"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := actionErrorCode(tc.err); got != tc.want {
				t.Fatalf("actionErrorCode(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}
