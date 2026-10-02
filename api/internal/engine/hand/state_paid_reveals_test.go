package hand

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
)

// dynamoRoundTrip pushes a State through the same attributevalue encoding
// tablestore.CommitAction persists and LoadTable reads back.
func dynamoRoundTrip(t *testing.T, s State) State {
	t.Helper()
	item, err := attributevalue.MarshalMap(struct {
		State State `dynamodbav:"state"`
	}{s})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out struct {
		State State `dynamodbav:"state"`
	}
	if err := attributevalue.UnmarshalMap(item, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out.State
}

func TestPaidRevealStateSurvivesPersistence(t *testing.T) {
	table, first, second, winnerID, _ := threeWayWinnerCardsSetup(t)
	now := time.Unix(1_700_000_000, 0)
	if _, err := table.RequestRabbitHunt(first, boardSlotPtr(2)); err != nil {
		t.Fatalf("RequestRabbitHunt: %v", err)
	}
	_, _ = table.RequestWinnerCards(first, now)
	_, _ = table.RequestWinnerCards(second, now)

	rebuilt := NewTableFromState(dynamoRoundTrip(t, table.ExportState()))
	if rebuilt.RabbitHuntSlotsFor(first) != 1<<2 {
		t.Fatalf("rabbit slots lost: %b", rebuilt.RabbitHuntSlotsFor(first))
	}
	if batch := rebuilt.PendingWinnerCards(); len(batch) != 2 || batch[0].WinnerID != winnerID {
		t.Fatalf("winner-cards batch lost: %+v", batch)
	}
	_ = rebuilt.DeclineWinnerCards(winnerID)
	if again := NewTableFromState(dynamoRoundTrip(t, rebuilt.ExportState())); !again.WinnerCardsClosed() {
		t.Fatal("the closed flag must survive a reload, or a decline could be re-asked past after a handoff")
	}
}

// A hand persisted by a pre-per-card / pre-batch instance must keep its
// purchase and its pending fee when a new instance loads it mid-deploy.
func TestLegacyPaidRevealStateIsUpgradedOnLoad(t *testing.T) {
	table, requesterID, winnerID, _, _ := winnerCardsSetup(t, "sandbox", 20)
	legacy := table.ExportState()
	legacy.RabbitHuntPaid = map[string]bool{requesterID: true}
	legacy.PendingWinnerCards = &WinnerCardsRequest{RequesterID: requesterID, WinnerID: winnerID, Fee: 20, ExpiresAt: 1}

	rebuilt := NewTableFromState(dynamoRoundTrip(t, legacy))
	if got := rebuilt.RabbitHuntSlotsFor(requesterID); got != 0b11111 {
		t.Fatalf("a legacy whole-runout purchase must cover every undealt slot, got %b", got)
	}
	if batch := rebuilt.PendingWinnerCards(); len(batch) != 1 || batch[0].RequesterID != requesterID {
		t.Fatalf("a legacy pending request must become a one-entry batch, got %+v", batch)
	}
	if out := rebuilt.ExportState(); out.RabbitHuntPaid != nil || out.PendingWinnerCards != nil {
		t.Fatal("the legacy fields are read-only and must not be written back")
	}
}
