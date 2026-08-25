package v1

import (
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func TestConvertSnapshotPreservesVersionPresenceAndHand(t *testing.T) {
	startStack := int64(500)
	converted := ConvertSnapshot(hand.Snapshot{
		Stage:             "pre_flop",
		BoardTwo:          []string{"2c", "3d"},
		BoardSplitAt:      3,
		SnapshotVersion:   42,
		HandID:            "hand-42",
		IdleRemovalUnixMs: 123456,
		Seats: []hand.SeatView{{
			PlayerID: "p1", Name: "Ana", PlaystyleBadge: "selective", State: "folded",
			ConnectionState: "disconnected", DealtIn: true, Ready: false,
			HoleCardsRevealed: []bool{true, false},
			StackAtHandStart:  &startStack,
			TimeBankMs:        27000,
			HandScore:         4321, RunItTwice: true,
		}},
		PotResults: []hand.PotResultView{{
			Amount: 300, PayoutAmount: 293,
			EligiblePlayerIDs: []string{"p1", "p2"}, WinnerPlayerIDs: []string{"p1"},
			Payouts: map[string]int64{"p1": 293}, Runout: 1,
		}},
		ChatMessages:             []hand.ChatMessageView{{ID: "chat-1", PlayerID: "p1", Message: "oi", Timestamp: 100}},
		Reactions:                []hand.ReactionView{{ID: "react-1", PlayerID: "p1", ReactionID: "wow", Timestamp: 101, ExpiresAt: 2501}},
		ActionPreselection:       "check_fold",
		ActionPreselectionAmount: 40,
		ProspectiveCallAmount:    80,
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
	if converted.Seats[0].GetPlaystyleBadge() != "selective" {
		t.Fatal("playstyle badge lost during protobuf conversion")
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
	if converted.Seats[0].StackAtHandStart == nil || converted.Seats[0].GetStackAtHandStart() != 500 {
		t.Fatalf("pre-blind stack lost during protobuf conversion: %+v", converted.Seats[0])
	}
	if converted.ProtocolVersion != 11 || converted.IdleRemovalUnixMs != 123456 ||
		converted.Seats[0].TimeBankMs != 27000 || converted.Seats[0].HandScore != 4321 || len(converted.PotResults) != 1 ||
		!converted.Seats[0].GetRunItTwice() || converted.PotResults[0].Runout != 1 ||
		len(converted.BoardTwo) != 2 || converted.BoardSplitAt != 3 ||
		converted.PotResults[0].WinnerPlayerIds[0] != "p1" ||
		converted.PotResults[0].Payouts["p1"] != 293 || len(converted.ChatMessages) != 1 ||
		converted.ChatMessages[0].Id != "chat-1" || len(converted.Reactions) != 1 ||
		converted.Reactions[0].Id != "react-1" || converted.ActionPreselection != "check_fold" ||
		converted.ActionPreselectionAmount != 40 || converted.ProspectiveCallAmount != 80 {
		t.Fatalf("result protocol fields lost during conversion: %+v", converted)
	}
}

func TestConvertSnapshotCarriesThePendingWinnerCardsRequest(t *testing.T) {
	snap := hand.Snapshot{PendingWinnerCards: &hand.WinnerCardsRequestView{
		RequesterID: "p1", RequesterName: "Ana", WinnerID: "p2", Fee: 40, ExpiresAtUnixMs: 1700,
	}}
	got := ConvertSnapshot(snap).GetPendingWinnerCards()
	if got == nil || got.GetRequesterId() != "p1" || got.GetRequesterName() != "Ana" ||
		got.GetWinnerId() != "p2" || got.GetFee() != 40 || got.GetExpiresAtUnixMs() != 1700 {
		t.Fatalf("pending winner cards request lost during conversion: %+v", got)
	}
	if ConvertSnapshot(hand.Snapshot{}).GetPendingWinnerCards() != nil {
		t.Fatal("a snapshot with no pending request must not fabricate one")
	}
}

func TestConvertSnapshotMapsAutoRebuyToOptionalBool(t *testing.T) {
	snap := hand.Snapshot{Seats: []hand.SeatView{
		{PlayerID: "p1", AutoRebuy: true},
		{PlayerID: "p2", AutoRebuy: false},
	}}
	proto := ConvertSnapshot(snap)
	if proto.Seats[0].AutoRebuy == nil || !*proto.Seats[0].AutoRebuy {
		t.Fatal("expected p1's auto_rebuy to be set true")
	}
	if proto.Seats[1].AutoRebuy != nil {
		t.Fatal("expected p2's auto_rebuy to be nil (unset), matching run_it_twice's false-is-absent convention")
	}
}

func TestConvertSnapshotPreservesPartialDeckProof(t *testing.T) {
	converted := ConvertSnapshot(hand.Snapshot{
		RootCommitHash: "root",
		RevealedCardSalts: map[int]hand.RevealedSaltView{
			5: {Card: "As", SaltHex: "salt"},
		},
		UnrevealedCardHashes: map[int]string{6: "hash"},
		RunoutCards:          []string{"As"},
	})
	if converted.RootCommitHash != "root" ||
		converted.RevealedCardSalts[5].Card != "As" ||
		converted.UnrevealedCardHashes[6] != "hash" ||
		len(converted.RunoutCards) != 1 {
		t.Fatalf("partial proof lost during protobuf conversion: %+v", converted)
	}
}

func TestEveryStateChangingOrAmplifiableMessageIsRateLimited(t *testing.T) {
	for _, messageType := range []string{"act", "chat", "reaction", "preselect_action", "sync_state", "ready", "post_big_blind", "show_cards", "keep_seat", "set_run_it_twice", "ping"} {
		if !rateLimitedTableMessage(messageType) {
			t.Errorf("%q bypasses the per-seat limiter", messageType)
		}
	}
	for _, messageType := range []string{"unknown"} {
		if rateLimitedTableMessage(messageType) {
			t.Errorf("%q should not consume the action limiter", messageType)
		}
	}
}

func TestReactionLimiterAllowsOnlyOneBurstPerWindow(t *testing.T) {
	limiter := newWindowSeatLimiter(1, 2*time.Second)
	if !limiter.Allow("p1") {
		t.Fatal("first reaction should be allowed")
	}
	if limiter.Allow("p1") {
		t.Fatal("second reaction in the same window should be blocked")
	}
	if !limiter.Allow("p2") {
		t.Fatal("one player's reaction limit must not block another player")
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
