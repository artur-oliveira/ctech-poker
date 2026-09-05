package handshare

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

// stubTable is just enough DynamoDB to exercise ListByOwner: it answers one
// canned Query and records every request it saw, so a test can pin both the
// operation count and the fact that a list performs no write at all.
type stubTable struct {
	items    string // the Items JSON array the Query answers with
	lastKey  string // LastEvaluatedKey JSON object, empty for "no next page"
	targets  []string
	requests []string
}

func (s *stubTable) client(t *testing.T) *dynamodb.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		target := r.Header.Get("X-Amz-Target")
		s.targets = append(s.targets, target[strings.LastIndex(target, ".")+1:])
		s.requests = append(s.requests, string(body))
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		last := ""
		if s.lastKey != "" {
			last = fmt.Sprintf(`,"LastEvaluatedKey":%s`, s.lastKey)
		}
		fmt.Fprintf(w, `{"Items":%s%s}`, s.items, last)
	}))
	t.Cleanup(srv.Close)
	cfg := aws.Config{Region: "us-east-1", Credentials: credentials.NewStaticCredentialsProvider("dummy", "dummy", "")}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String(srv.URL) })
}

func shareItem(token, ownerID string, createdAt, expiresAt int64) string {
	return fmt.Sprintf(`{"pk":{"S":%q},"owner_id":{"S":%q},"kind":{"S":"brag"},"outcome":{"S":"won"},`+
		`"net_change":{"N":"500"},"created_at":{"N":"%d"},"expires_at":{"N":"%d"}}`,
		token, ownerID, createdAt, expiresAt)
}

func TestListByOwnerIsOneQueryAndNoWrite(t *testing.T) {
	now := time.Now().UnixMilli()
	future := time.Now().Add(24 * time.Hour).UnixMilli()
	stub := &stubTable{
		items: "[" + strings.Join([]string{
			shareItem("new", "u1", now, future),
			// Already past its expiry: the TTL sweep is eventual, so the row
			// can still be in the index and must not reach the caller.
			shareItem("expired", "u1", now-2000, now-1),
			shareItem("old", "u1", now-1000, future),
		}, ",") + "]",
		lastKey: `{"pk":{"S":"old"},"owner_id":{"S":"u1"},"created_at":{"N":"1"}}`,
	}
	store := &Store{base: dynamo.NewBase(stub.client(t), "test", tableHandShares)}

	got, next, err := store.ListByOwner(context.Background(), "u1", 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	// One page = exactly one DynamoDB operation, and it is a read.
	if len(stub.targets) != 1 || stub.targets[0] != "Query" {
		t.Fatalf("expected exactly one Query and no other operation, got %v", stub.targets)
	}
	if len(got) != 2 || got[0].Token != "new" || got[1].Token != "old" {
		t.Fatalf("expected the two live shares in index order, got %+v", got)
	}
	if len(next) == 0 {
		t.Fatal("expected the LastEvaluatedKey to be handed back as the next page cursor")
	}
	var request struct {
		IndexName        string
		Limit            int
		ScanIndexForward bool
	}
	if err := json.Unmarshal([]byte(stub.requests[0]), &request); err != nil {
		t.Fatal(err)
	}
	if request.IndexName != ownerIndex || request.ScanIndexForward {
		t.Fatalf("expected a descending query on %s, got %+v", ownerIndex, request)
	}
	if request.Limit != 2 {
		t.Fatalf("expected the caller's page size to reach DynamoDB, got %d", request.Limit)
	}
}

func TestListByOwnerCapsThePageSizeAndForwardsTheCursor(t *testing.T) {
	stub := &stubTable{items: "[]"}
	store := &Store{base: dynamo.NewBase(stub.client(t), "test", tableHandShares)}
	start := map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: "t1"}}

	if _, _, err := store.ListByOwner(context.Background(), "u1", MaxListPageSize+500, start); err != nil {
		t.Fatal(err)
	}
	var request struct {
		Limit             int
		ExclusiveStartKey map[string]map[string]string
	}
	if err := json.Unmarshal([]byte(stub.requests[0]), &request); err != nil {
		t.Fatal(err)
	}
	if request.Limit != MaxListPageSize {
		t.Fatalf("a client-supplied page size must be capped at %d, got %d", MaxListPageSize, request.Limit)
	}
	if request.ExclusiveStartKey["pk"]["S"] != "t1" {
		t.Fatalf("expected the cursor to reach DynamoDB, got %+v", request.ExclusiveStartKey)
	}
}

func TestListByOwnerRejectsAnEmptyOwner(t *testing.T) {
	store := &Store{}
	if _, _, err := store.ListByOwner(context.Background(), "  ", 10, nil); err != ErrNotOwner {
		t.Fatalf("an unauthenticated list must never reach the index: %v", err)
	}
}
