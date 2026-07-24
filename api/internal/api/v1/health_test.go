package v1

import (
	"context"
	"testing"
	"time"
)

func TestDynamoDBFailureMakesDetailedHealthUnavailable(t *testing.T) {
	entry := checkDynamoDB(context.Background(), nil, "dev_poker_table_state", time.Now().UTC().Format(time.RFC3339Nano))
	if entry.Status != statusFail {
		t.Fatalf("status=%s", entry.Status)
	}
	overall, code := aggregate(map[string]healthEntry{componentDynamoDB: entry})
	if overall != statusFail || code != 503 {
		t.Fatalf("overall=%s code=%d", overall, code)
	}
}
