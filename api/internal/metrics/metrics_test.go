package metrics

import (
	"encoding/json"
	"testing"
	"time"
)

// testRegistry is a registry that captures EMF documents instead of writing
// them to stdout, with a fixed clock so the emitted timestamp is assertable.
func testRegistry(t *testing.T) (*registry, *[]map[string]any) {
	t.Helper()
	r := newRegistry()
	r.namespace = "TestNamespace"
	r.env = "test"
	r.nowFunc = func() time.Time { return time.UnixMilli(1_700_000_000_000) }
	var captured []map[string]any
	r.emit = func(payload map[string]any) { captured = append(captured, payload) }
	return r, &captured
}

// TestRecordAggregatesInsteadOfEmittingPerEvent is the property the whole
// package exists for: the 2026-09-02 storm produced 5,779 log lines for one
// table, so a metric on a high-volume event must never be one line per
// occurrence. 250 observations must not be 250 documents.
func TestRecordAggregatesInsteadOfEmittingPerEvent(t *testing.T) {
	r, captured := testRegistry(t)

	for i := 0; i < 250; i++ {
		r.record("TableCommits", Count, Dims{"Outcome": "conflict"}, 1)
	}
	// 250 samples = two full buckets flushed at maxValuesPerMetric, 50 pending.
	if len(*captured) != 2 {
		t.Fatalf("250 observations emitted %d documents, want 2 (one per %d samples)", len(*captured), maxValuesPerMetric)
	}
	r.flush()
	if len(*captured) != 3 {
		t.Fatalf("after flush: %d documents, want 3", len(*captured))
	}

	total := 0.0
	for _, doc := range *captured {
		for _, v := range doc["TableCommits"].([]float64) {
			total += v
		}
	}
	// Splitting a bucket must not lose a sample: the Sum statistic across the
	// three documents is what a counter is actually read back as.
	if total != 250 {
		t.Errorf("summed value across documents = %v, want 250 — samples were dropped", total)
	}
}

// TestPayloadIsValidEmf pins the document shape against the CloudWatch
// specification: _aws with a millisecond Timestamp, the namespace, one
// dimension set naming root-level string members, and a metric definition
// naming a root-level numeric (or numeric array) member.
func TestPayloadIsValidEmf(t *testing.T) {
	r, captured := testRegistry(t)

	r.record("HandPipelineDuration", Milliseconds, Dims{"Step": "matchup"}, 42)
	r.flush()

	if len(*captured) != 1 {
		t.Fatalf("got %d documents, want 1", len(*captured))
	}
	// Round-tripping through JSON is the point: this is what CloudWatch Logs
	// actually parses, and it catches a value type that does not marshal.
	raw, err := json.Marshal((*captured)[0])
	if err != nil {
		t.Fatalf("payload does not marshal: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}

	aws, ok := doc["_aws"].(map[string]any)
	if !ok {
		t.Fatal("document has no _aws metadata node")
	}
	if ts, ok := aws["Timestamp"].(float64); !ok || int64(ts) != 1_700_000_000_000 {
		t.Errorf("_aws.Timestamp = %v, want the clock's UnixMilli", aws["Timestamp"])
	}
	directives, ok := aws["CloudWatchMetrics"].([]any)
	if !ok || len(directives) != 1 {
		t.Fatalf("_aws.CloudWatchMetrics = %v, want exactly one directive", aws["CloudWatchMetrics"])
	}
	directive := directives[0].(map[string]any)
	if directive["Namespace"] != "TestNamespace" {
		t.Errorf("Namespace = %v, want TestNamespace", directive["Namespace"])
	}

	// Every dimension name must resolve to a string member on the root node,
	// and Environment must always be one of them.
	dimensionSets := directive["Dimensions"].([]any)
	names := dimensionSets[0].([]any)
	seenEnvironment := false
	for _, name := range names {
		key := name.(string)
		if _, ok := doc[key].(string); !ok {
			t.Errorf("dimension %q does not resolve to a string root member", key)
		}
		if key == environmentDimension {
			seenEnvironment = true
		}
	}
	if !seenEnvironment {
		t.Error("no Environment dimension — dev, stage and prod would share one series")
	}
	if doc["Step"] != "matchup" {
		t.Errorf("Step = %v, want matchup", doc["Step"])
	}

	// A single observation is a bare number, not a one-element array.
	definition := directive["Metrics"].([]any)[0].(map[string]any)
	if definition["Name"] != "HandPipelineDuration" || definition["Unit"] != "Milliseconds" {
		t.Errorf("metric definition = %v, want HandPipelineDuration/Milliseconds", definition)
	}
	if value, ok := doc["HandPipelineDuration"].(float64); !ok || value != 42 {
		t.Errorf("metric target = %v, want the numeric 42 on the root node", doc["HandPipelineDuration"])
	}
}

// TestDistinctDimensionsAreDistinctSeries: two outcomes of the same metric
// must not be summed together, and dimension order must not create a third
// series for the same pair of values.
func TestDistinctDimensionsAreDistinctSeries(t *testing.T) {
	r, captured := testRegistry(t)

	r.record("TableCommits", Count, Dims{"Outcome": "accepted"}, 1)
	r.record("TableCommits", Count, Dims{"Outcome": "conflict"}, 1)
	r.record("TableCommits", Count, Dims{"Outcome": "accepted"}, 1)
	r.flush()

	if len(*captured) != 2 {
		t.Fatalf("got %d documents, want one per Outcome value", len(*captured))
	}
	for _, doc := range *captured {
		switch doc["Outcome"] {
		case "accepted":
			if got := doc["TableCommits"].([]float64); len(got) != 2 {
				t.Errorf("accepted carried %v, want two samples", got)
			}
		case "conflict":
			if got := doc["TableCommits"].(float64); got != 1 {
				t.Errorf("conflict carried %v, want the single sample 1", got)
			}
		default:
			t.Errorf("unexpected Outcome %v", doc["Outcome"])
		}
	}
}

// TestSeriesCapDropsUnboundedDimensions is the billing backstop: a dimension
// value taken from a table id would otherwise create one custom metric per
// table. Past maxSeries the samples are dropped, not emitted.
func TestSeriesCapDropsUnboundedDimensions(t *testing.T) {
	r, captured := testRegistry(t)

	for i := 0; i < maxSeries*2; i++ {
		r.record("Leaky", Count, Dims{"TableID": string(rune('a'+i%26)) + "-" + time.Duration(i).String()}, 1)
	}
	r.flush()

	if len(*captured) > maxSeries {
		t.Errorf("emitted %d series, want at most the %d cap", len(*captured), maxSeries)
	}
	if r.dropped != 0 {
		t.Errorf("flush left %d dropped samples uncounted", r.dropped)
	}
}
