package main

import (
	"encoding/json"
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

	files, err := buildBatches(e)
	if err != nil {
		t.Fatalf("buildBatches: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	batch := files[0].payload
	key := files[0].key
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
	files, err := buildBatches(e)
	if err != nil || len(files) != 0 {
		t.Fatalf("expected no-op for an all-REMOVE batch, got files=%v err=%v", files, err)
	}
}

// TestNumericAttributesPreserveIntegerFidelity guards #55: a chip total or
// payout past 2^53 (9007199254740992) is not exactly representable as a
// float64, so a naive strconv.ParseFloat round-trip through the archive
// silently corrupts it. attributeValueToInterface must instead carry the
// DynamoDB Number attribute through as json.Number, so json.Marshal emits the
// original digits verbatim rather than a rounded float64 approximation.
func TestNumericAttributesPreserveIntegerFidelity(t *testing.T) {
	const largeChipTotal = "9007199254740993" // 2^53 + 1 — the smallest integer float64 cannot represent exactly
	const exactCents = "12345678901234567.89"

	e := events.DynamoDBEvent{
		Records: []events.DynamoDBEventRecord{
			{
				EventName: "INSERT",
				EventID:   "evt-1",
				Change: events.DynamoDBStreamRecord{
					NewImage: map[string]events.DynamoDBAttributeValue{
						"pk":          events.NewStringAttribute("table-1#hand-1"),
						"chip_total":  events.NewNumberAttribute(largeChipTotal),
						"exact_cents": events.NewNumberAttribute(exactCents),
					},
				},
			},
		},
	}

	files, err := buildBatches(e)
	if err != nil {
		t.Fatalf("buildBatches: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	line := strings.TrimRight(string(files[0].payload), "\n")

	// The archived JSON must contain the exact digit sequence, unquoted (a
	// JSON number, not a string) and not rewritten by float64 rounding.
	if !strings.Contains(line, `"chip_total":`+largeChipTotal) {
		t.Fatalf("expected chip_total to round-trip exactly as %s, got %q", largeChipTotal, line)
	}
	if !strings.Contains(line, `"exact_cents":`+exactCents) {
		t.Fatalf("expected exact_cents to round-trip exactly as %s, got %q", exactCents, line)
	}

	// Decoding back with json.Number (as any real consumer of this archive
	// must, to avoid the same float64 trap) must recover the exact string.
	dec := json.NewDecoder(strings.NewReader(line))
	dec.UseNumber()
	var decoded map[string]any
	if err := dec.Decode(&decoded); err != nil {
		t.Fatalf("decode archived line: %v", err)
	}
	if got := decoded["chip_total"].(json.Number).String(); got != largeChipTotal {
		t.Fatalf("chip_total round-trip: got %s, want %s", got, largeChipTotal)
	}
	if got := decoded["exact_cents"].(json.Number).String(); got != exactCents {
		t.Fatalf("exact_cents round-trip: got %s, want %s", got, exactCents)
	}
}
