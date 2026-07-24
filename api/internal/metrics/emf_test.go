package metrics

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestEmitTableMetricWritesValidEMFJSON(t *testing.T) {
	var buf bytes.Buffer
	EmitTableMetricTo(&buf, "dev", "HandsCompleted", 1, map[string]string{"table_id": "t1"})

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("expected valid JSON, got %v (line: %s)", err, buf.String())
	}
	if !strings.Contains(buf.String(), `"_aws"`) {
		t.Fatal("expected an EMF _aws metadata block")
	}
	if parsed["table_id"] != "t1" {
		t.Fatalf("expected table_id dimension present, got %v", parsed["table_id"])
	}
	if parsed["HandsCompleted"] != float64(1) {
		t.Fatalf("expected HandsCompleted=1, got %v", parsed["HandsCompleted"])
	}
}
