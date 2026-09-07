package tablestore

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

// The timeline exists to be the small view: cosmetic events are dropped, and a
// row written before Seq existed still gets its position.
func TestTimelineFromDropsCosmeticEventsAndBackfillsSeq(t *testing.T) {
	items := encodeTimeline(t,
		TimelineEvent{PlayerID: "p1", Action: "post_blind", Amount: 10, Timestamp: 1},
		TimelineEvent{PlayerID: "p2", Action: "chat", Timestamp: 2},
		TimelineEvent{PlayerID: "p2", Action: "reaction", Timestamp: 3},
		TimelineEvent{Seq: 9, PlayerID: "p1", Action: "fold", Timestamp: 4},
	)

	events := timelineFrom(items)

	if len(events) != 2 {
		t.Fatalf("events=%+v", events)
	}
	if events[0].Action != "post_blind" || events[0].Seq != 1 {
		t.Fatalf("first=%+v", events[0])
	}
	// An explicit Seq is never overwritten by the result position.
	if events[1].Action != "fold" || events[1].Seq != 9 {
		t.Fatalf("second=%+v", events[1])
	}
}

func encodeTimeline(t *testing.T, events ...TimelineEvent) []map[string]types.AttributeValue {
	t.Helper()
	items := make([]map[string]types.AttributeValue, 0, len(events))
	for _, event := range events {
		item, err := dynamo.Encode(event)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		items = append(items, item)
	}
	return items
}
