package leaderboard

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

// countingDynamo answers every DynamoDB call with a canned item and tallies
// the calls by operation, so a test can pin the per-hand write budget
// (issue #217) without a live table.
type countingDynamo struct {
	handsPlayed int
	calls       map[string]int
}

func (c *countingDynamo) Do(req *http.Request) (*http.Response, error) {
	op := req.Header.Get("X-Amz-Target")
	if i := strings.LastIndex(op, "."); i >= 0 {
		op = op[i+1:]
	}
	c.calls[op]++
	body := fmt.Sprintf(`{"Attributes":{"hands_played":{"N":"%d"},"hands_won":{"N":"1"}}}`, c.handsPlayed)
	return &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/x-amz-json-1.0"}},
		Body:       io.NopCloser(bytes.NewReader([]byte(body))),
		Request:    req,
	}, nil
}

func countingStore(handsPlayed int) (*Store, *countingDynamo) {
	stub := &countingDynamo{handsPlayed: handsPlayed, calls: map[string]int{}}
	db := dynamodb.New(dynamodb.Options{
		Region:       "us-east-1",
		Credentials:  credentials.NewStaticCredentialsProvider("dummy", "dummy", ""),
		HTTPClient:   stub,
		BaseEndpoint: aws.String("https://dynamodb.us-east-1.amazonaws.com"),
	})
	return NewStore(db, "writecount_test"), stub
}

func participantIDs(n int) []string {
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		ids = append(ids, fmt.Sprintf("p%d", i))
	}
	return ids
}

// TestRecordHandWriteBudget fixes the ceiling issue #217 asks for: one write
// per participant while the row is below the win_rate floor, and at most two
// once it is on the win_rate board (the second materializes the ratio the GSI
// sorts by, which DynamoDB cannot compute inside the counter update itself).
func TestRecordHandWriteBudget(t *testing.T) {
	for _, tc := range []struct {
		name          string
		handsPlayed   int
		writesPerSeat int
	}{
		{"sub-floor rows cost one write per seat", 5, 1},
		{"ranked rows cost two writes per seat", MinHandsForWinRateRank + 50, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, seats := range []int{2, 6, 9} {
				store, stub := countingStore(tc.handsPlayed)
				svc := NewServiceWithStore(store)
				ids := participantIDs(seats)
				outcome := hand.HandOutcome{Winners: ids[:1], Participants: ids}
				if err := svc.RecordHand(context.Background(), "sandbox", outcome, nil); err != nil {
					t.Fatalf("%d seats: %v", seats, err)
				}
				if want := seats * tc.writesPerSeat; stub.calls["UpdateItem"] != want {
					t.Errorf("%d seats: expected %d UpdateItem calls, got %d", seats, want, stub.calls["UpdateItem"])
				}
				if stub.calls["GetItem"] != 0 {
					t.Errorf("%d seats: uncontended hands must not read the row back, got %d GetItem", seats, stub.calls["GetItem"])
				}
			}
		})
	}
}

// TestIncrementAchievementPointsWriteBudget pins the other half: stars used to
// cost one AtomicIncrement each plus an upsert, a GetItem and a rank-key write.
func TestIncrementAchievementPointsWriteBudget(t *testing.T) {
	store, stub := countingStore(5)
	if err := store.IncrementAchievementPoints(context.Background(), "p0", "sandbox", 3); err != nil {
		t.Fatal(err)
	}
	if stub.calls["UpdateItem"] != 1 || stub.calls["GetItem"] != 0 {
		t.Fatalf("expected a single UpdateItem, got %v", stub.calls)
	}
}
