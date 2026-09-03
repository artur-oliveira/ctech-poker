package v1

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/sessionlog"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

func preflopFrame(smallBlindContrib, bigBlindContrib int64) *tablestore.ReplayFrame {
	return &tablestore.ReplayFrame{
		Stage: "pre_flop", SmallBlindPlayerID: "sb", BigBlindPlayerID: "bb",
		Seats: []tablestore.ReplaySeat{
			{PlayerID: "utg", Contributed: 0},
			{PlayerID: "sb", Contributed: smallBlindContrib},
			{PlayerID: "bb", Contributed: bigBlindContrib},
		},
	}
}

func TestBlindsForPrefersTheStoredLevel(t *testing.T) {
	// An escalated hand stores the level it was played at; the timeline must
	// never override it.
	sb, bb := blindsFor(
		&sessionlog.HandItem{SmallBlind: 50, BigBlind: 100},
		[]tablestore.ActionLogEntry{{Frame: preflopFrame(10, 20)}},
	)
	if sb != 50 || bb != 100 {
		t.Fatalf("expected the stored 50/100 level, got %d/%d", sb, bb)
	}
}

func TestBlindsForDerivesLegacyHandsFromTheFirstPreflopFrame(t *testing.T) {
	sb, bb := blindsFor(&sessionlog.HandItem{}, []tablestore.ActionLogEntry{
		{Frame: nil},
		{Frame: preflopFrame(10, 20)},
		// A later frame, after the BB has raised, must not win.
		{Frame: preflopFrame(10, 260)},
	})
	if sb != 10 || bb != 20 {
		t.Fatalf("expected 10/20 derived from the first pre-flop frame, got %d/%d", sb, bb)
	}
}

func TestBlindsForLeavesUnknownHandsAtZero(t *testing.T) {
	// No stored level and no usable frame: the replayer hides the marker
	// rather than rendering a hardcoded default.
	sb, bb := blindsFor(&sessionlog.HandItem{}, []tablestore.ActionLogEntry{
		{Frame: &tablestore.ReplayFrame{Stage: "flop", BigBlindPlayerID: "bb"}},
	})
	if sb != 0 || bb != 0 {
		t.Fatalf("expected an unknown blind level, got %d/%d", sb, bb)
	}
}
