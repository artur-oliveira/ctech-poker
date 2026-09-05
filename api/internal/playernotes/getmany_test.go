package playernotes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/api-commons/dynamo"
)

// batchStub answers one BatchGetItem with a canned partial response — only
// two of the three requested opponents have a note — and records the requests
// so a test can pin how many round-trips one screen costs.
type batchStub struct {
	table    string
	requests []string
}

func (s *batchStub) client(t *testing.T) *dynamodb.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.requests = append(s.requests, string(body))
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		fmt.Fprintf(w, `{"Responses":{%q:[`+
			`{"pk":{"S":"viewer"},"sk":{"S":"a"},"tag":{"S":"red"},"updated_at":{"S":"t"}},`+
			`{"pk":{"S":"viewer"},"sk":{"S":"c"},"note":{"S":"paga demais"},"updated_at":{"S":"t"}}`+
			`]},"UnprocessedKeys":{}}`, s.table)
	}))
	t.Cleanup(srv.Close)
	cfg := aws.Config{Region: "us-east-1", Credentials: credentials.NewStaticCredentialsProvider("dummy", "dummy", "")}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String(srv.URL) })
}

func TestGetManyReadsOnlyTheRequestedOpponentsInOneCall(t *testing.T) {
	stub := &batchStub{table: dynamo.TableName("test", tablePlayerNotes)}
	store := &Store{base: dynamo.NewBase(stub.client(t), "test", tablePlayerNotes)}

	// "viewer" and the duplicate "a" must not become keys: a viewer has no
	// note on themselves, and a repeated seat is still one read.
	notes, err := store.GetMany(context.Background(), "viewer", []string{"a", "b", "a", "", "viewer", "c"})
	if err != nil {
		t.Fatal(err)
	}
	if len(stub.requests) != 1 {
		t.Fatalf("one screen must cost one BatchGetItem, got %d", len(stub.requests))
	}
	request := stub.requests[0]
	if strings.Count(request, `"sk"`) != 3 {
		t.Fatalf("expected exactly the three distinct opponents as keys: %s", request)
	}
	if strings.Contains(request, `{"S":"viewer"}},{"pk":{"S":"viewer"},"sk":{"S":"viewer"}`) {
		t.Fatalf("the viewer must never be looked up as their own opponent: %s", request)
	}
	// "b" simply has no note; a partial answer must stay partial rather than
	// shift another player's note onto them.
	if len(notes) != 2 || notes[0].OpponentID != "a" || notes[1].OpponentID != "c" {
		t.Fatalf("expected only the notes that exist, keyed by their own opponent: %+v", notes)
	}
}

func TestGetManyRejectsAnOversizedIDList(t *testing.T) {
	store := &Store{}
	ids := make([]string, MaxBatchOpponentIDs+1)
	for i := range ids {
		ids[i] = fmt.Sprintf("p%d", i)
	}
	if _, err := store.GetMany(context.Background(), "viewer", ids); !errors.Is(err, ErrTooManyOpponents) {
		t.Fatalf("one request's fan-out must stay bounded: %v", err)
	}
}

func TestGetManyWithNoUsableIDsReadsNothing(t *testing.T) {
	store := &Store{}
	notes, err := store.GetMany(context.Background(), "viewer", []string{"", "viewer"})
	if err != nil || len(notes) != 0 {
		t.Fatalf("an empty seat list must not reach DynamoDB: %+v %v", notes, err)
	}
}
