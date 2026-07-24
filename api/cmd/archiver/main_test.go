package main

import (
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

func TestBuildBatchRendersOneJSONLinePerInsert(t *testing.T) {
	e := events.DynamoDBEvent{
		Records: []events.DynamoDBEventRecord{
			{
				EventName: "INSERT",
				EventID:   "evt-1",
				Change: events.DynamoDBStreamRecord{
					NewImage: map[string]events.DynamoDBAttributeValue{
						"pk":        events.NewStringAttribute("table-1#hand-1"),
						"sk":        events.NewStringAttribute("0000000002"),
						"player_id": events.NewStringAttribute("p1"),
						"action":    events.NewStringAttribute("call"),
						"amount":    events.NewNumberAttribute("0"),
					},
				},
			},
			{
				// TTL-expiry emits REMOVE — never archive those, the item
				// already reached S3 on its own INSERT.
				EventName: "REMOVE",
				EventID:   "evt-2",
				Change: events.DynamoDBStreamRecord{
					NewImage: map[string]events.DynamoDBAttributeValue{"pk": events.NewStringAttribute("table-1#hand-1")},
				},
			},
		},
	}

	batch, key, err := buildBatch(e)
	if err != nil {
		t.Fatalf("buildBatch: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(batch), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected exactly 1 line (INSERT only), got %d: %q", len(lines), string(batch))
	}
	if !strings.Contains(lines[0], `"player_id":"p1"`) {
		t.Fatalf("expected the JSON line to contain player_id, got %q", lines[0])
	}
	if !strings.HasPrefix(key, "table-1/hand-1/") || !strings.HasSuffix(key, ".jsonl") {
		t.Fatalf("expected key partitioned as table_id/hand_id/*.jsonl, got %q", key)
	}
}

func TestBuildBatchReturnsEmptyWhenNothingToInsert(t *testing.T) {
	e := events.DynamoDBEvent{Records: []events.DynamoDBEventRecord{{EventName: "REMOVE"}}}
	batch, key, err := buildBatch(e)
	if err != nil || batch != nil || key != "" {
		t.Fatalf("expected no-op for an all-REMOVE batch, got batch=%q key=%q err=%v", batch, key, err)
	}
}
