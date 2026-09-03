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
	"gopkg.aoctech.app/api-commons/dynamo"
)

// stubTable is just enough DynamoDB to exercise the owner index: GetItem by
// pk against a canned item map, and a success answer for the index's
// UpdateItem expressions (which it records so a test can assert the prune).
type stubTable struct {
	items   map[string]string // pk -> Item JSON
	updates []string          // pk of every UpdateItem call
}

func (s *stubTable) client(t *testing.T) *dynamodb.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Key map[string]struct{ S string } `json:"Key"`
		}
		_ = json.Unmarshal(body, &req)
		pk := req.Key["pk"].S
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		if strings.HasSuffix(r.Header.Get("X-Amz-Target"), ".GetItem") {
			if item, ok := s.items[pk]; ok {
				fmt.Fprintf(w, `{"Item":%s}`, item)
				return
			}
			_, _ = w.Write([]byte(`{}`))
			return
		}
		s.updates = append(s.updates, pk)
		_, _ = w.Write([]byte(`{}`))
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

func TestListByOwnerReturnsLiveSharesNewestFirstAndPrunesTheRest(t *testing.T) {
	now := time.Now().UnixMilli()
	future := time.Now().Add(24 * time.Hour).UnixMilli()
	stub := &stubTable{items: map[string]string{
		"owner#u1": `{"pk":{"S":"owner#u1"},"tokens":{"SS":["old","new","expired","revoked","someone-elses"]}}`,
		"old":      shareItem("old", "u1", now-1000, future),
		"new":      shareItem("new", "u1", now, future),
		// Past its expiry: Get already treats this as gone.
		"expired": shareItem("expired", "u1", now, now-1),
		// "revoked" is absent from the table entirely.
		"someone-elses": shareItem("someone-elses", "u2", now, future),
	}}
	store := &Store{base: dynamo.NewBase(stub.client(t), "test", tableHandShares)}

	got, err := store.ListByOwner(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected only the two live shares, got %+v", got)
	}
	if got[0].Token != "new" || got[1].Token != "old" {
		t.Fatalf("expected newest first, got %s then %s", got[0].Token, got[1].Token)
	}
	// Expired/revoked/foreign tokens must be pruned out of the index.
	if len(stub.updates) != 1 || stub.updates[0] != "owner#u1" {
		t.Fatalf("expected exactly one prune against the owner index, got %v", stub.updates)
	}
}

func TestListByOwnerWithNoIndexRowIsEmptyNotAnError(t *testing.T) {
	stub := &stubTable{items: map[string]string{}}
	store := &Store{base: dynamo.NewBase(stub.client(t), "test", tableHandShares)}
	got, err := store.ListByOwner(context.Background(), "u1")
	if err != nil || len(got) != 0 {
		t.Fatalf("expected an empty list, got %+v err=%v", got, err)
	}
	if len(stub.updates) != 0 {
		t.Fatalf("nothing to prune, so nothing should be written: %v", stub.updates)
	}
}

func TestListByOwnerRejectsAnEmptyOwner(t *testing.T) {
	store := &Store{}
	if _, err := store.ListByOwner(context.Background(), "  "); err != ErrNotOwner {
		t.Fatalf("an unauthenticated list must never fan out over every token: %v", err)
	}
}
