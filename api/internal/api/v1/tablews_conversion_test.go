package v1

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func TestConvertSnapshotPreservesVersionPresenceAndHand(t *testing.T) {
	converted := ConvertSnapshot(hand.Snapshot{
		Stage:           "pre_flop",
		SnapshotVersion: 42,
		HandID:          "hand-42",
		Seats: []hand.SeatView{{
			PlayerID: "p1", Name: "Ana", State: "folded",
			ConnectionState: "disconnected", DealtIn: true, Ready: false,
			HoleCardsRevealed: []bool{true, false},
		}},
		PotResults: []hand.PotResultView{{
			Amount: 300, PayoutAmount: 293,
			EligiblePlayerIDs: []string{"p1", "p2"}, WinnerPlayerIDs: []string{"p1"},
		}},
	})
	if converted.SnapshotVersion != 42 || converted.HandId != "hand-42" {
		t.Fatalf("snapshot identity lost: %+v", converted)
	}
	if len(converted.Seats) != 1 || converted.Seats[0].ConnectionState != "disconnected" {
		t.Fatalf("presence lost during protobuf conversion: %+v", converted.Seats)
	}
	if converted.Seats[0].State != "folded" {
		t.Fatal("transport presence must not overwrite poker state")
	}
	if !converted.Seats[0].GetDealtIn() {
		t.Fatal("hand membership lost during protobuf conversion")
	}
	if converted.Seats[0].Ready == nil || converted.Seats[0].GetReady() {
		t.Fatal("explicit paused state lost during protobuf conversion")
	}
	if got := converted.Seats[0].HoleCardsRevealed; len(got) != 2 || !got[0] || got[1] {
		t.Fatalf("per-card reveal mask lost during protobuf conversion: %v", got)
	}
	if converted.ProtocolVersion != 2 || len(converted.PotResults) != 1 ||
		converted.PotResults[0].WinnerPlayerIds[0] != "p1" {
		t.Fatalf("result protocol fields lost during conversion: %+v", converted)
	}
}

func TestTableConnectionTrackerReplaysAllLiveConnections(t *testing.T) {
	tracker := newTableConnectionTracker()
	tracker.add("table-a", "p1", "tab-1")
	tracker.add("table-a", "p1", "tab-1")
	tracker.add("table-a", "p1", "tab-2")
	tracker.add("table-a", "p2", "phone")
	tracker.add("table-b", "p3", "other")

	if got := len(tracker.listTable("table-a")); got != 3 {
		t.Fatalf("tracker must dedupe and retain every table connection, got %d", got)
	}
	tracker.remove("table-a", "p1", "tab-1")
	if got := len(tracker.listTable("table-a")); got != 2 {
		t.Fatalf("tracker removal removed the wrong scope, got %d", got)
	}
	if got := len(tracker.listTable("table-b")); got != 1 {
		t.Fatalf("table scopes leaked into each other, got %d", got)
	}
}
