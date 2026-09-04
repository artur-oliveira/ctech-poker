package recentplayers

import (
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// TestRecordHandWriteBudget pins issue #199's ceiling: one row per
// participant per hand — 2 / 6 / 9 writes for 2 / 6 / 9 players, in a single
// BatchWriteItem call — against the 9x8=72 directed updates plus a guard, in
// one transaction (~146 WCU), the aggregate model wrote for a full ring.
func TestRecordHandWriteBudget(t *testing.T) {
	for _, seats := range []int{2, 6, 9} {
		players := make([]string, seats)
		for i := range players {
			players[i] = fmt.Sprintf("p%d", i)
		}
		events, err := eventsFor(HandCompletion{TableID: "t", HandID: "h", Players: players, PlayedAt: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		if len(events) != seats {
			t.Fatalf("%d seats: %d rows, want %d", seats, len(events), seats)
		}
		for _, event := range events {
			if len(event.Opponents) != seats-1 {
				t.Fatalf("%d seats: row %s lists %d opponents, want %d", seats, event.PK, len(event.Opponents), seats-1)
			}
			// The key must not carry the timestamp: a retried hand hook has to
			// rewrite the same row, which is what makes this store idempotent
			// without a guard row.
			if event.SK != eventSKPrefix+"h" {
				t.Fatalf("row key %q is not derived from the hand id alone", event.SK)
			}
		}
	}
}

func TestEventsForRejectsOverfullHandAndSkipsIncompleteOnes(t *testing.T) {
	tooMany := make([]string, maxPlayersPerHand+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("p%d", i)
	}
	if _, err := eventsFor(HandCompletion{TableID: "t", HandID: "h", Players: tooMany}); err == nil {
		t.Fatal("expected an error for a hand with more players than seats")
	}
	for _, hand := range []HandCompletion{
		{TableID: "", HandID: "h", Players: []string{"a", "b"}},
		{TableID: "t", HandID: "", Players: []string{"a", "b"}},
		{TableID: "t", HandID: "h", Players: []string{"a", "a", ""}},
	} {
		events, err := eventsFor(hand)
		if err != nil || len(events) != 0 {
			t.Fatalf("hand %+v: events=%d err=%v, want none", hand, len(events), err)
		}
	}
}

// TestPageFromPaginatesCoalescedListByOffset covers the cursor the coalesced
// read hands back: opponents come from many rows, so the cursor is a position
// in the result, and it must neither repeat nor skip an opponent.
func TestPageFromPaginatesCoalescedListByOffset(t *testing.T) {
	players := make([]Player, 5)
	for i := range players {
		players[i] = Player{OpponentPlayerID: fmt.Sprintf("o%d", i)}
	}
	var seen []string
	offset := 0
	for range len(players) {
		page := pageFrom(players, offset, 2)
		for _, p := range page.Players {
			seen = append(seen, p.OpponentPlayerID)
		}
		if page.LastKey == nil {
			break
		}
		offset = offsetFrom(page.LastKey)
	}
	if len(seen) != len(players) {
		t.Fatalf("paged over %v, want all %d opponents exactly once", seen, len(players))
	}
	for i, id := range seen {
		if id != players[i].OpponentPlayerID {
			t.Fatalf("page order %v does not match the coalesced order", seen)
		}
	}
	if page := pageFrom(players, 99, 2); len(page.Players) != 0 || page.LastKey != nil {
		t.Fatalf("past-the-end offset should be an empty last page, got %+v", page)
	}
	// A cursor from before #199 was a row key, not an offset: it must read as
	// "start from the beginning" rather than panic or skip the list.
	stale := map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: "viewer"}}
	if offsetFrom(stale) != 0 {
		t.Fatal("a stale row-key cursor should decode to offset 0")
	}
}
