package player

import (
	"context"
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

// reportStub is just enough DynamoDB to exercise ReportAvatar: it answers the
// target's profile GetItem, optionally fails the guard PutItem the way a
// duplicate report would, and records every call so a test can assert what
// the report actually wrote.
type reportStub struct {
	guardExists bool
	calls       []string // operation name
	bodies      []string
}

func (s *reportStub) client(t *testing.T) *dynamodb.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		target := r.Header.Get("X-Amz-Target")
		op := target[strings.LastIndex(target, ".")+1:]
		s.calls = append(s.calls, op)
		s.bodies = append(s.bodies, string(body))
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		switch {
		case op == "GetItem":
			fmt.Fprint(w, `{"Item":{"pk":{"S":"target"},"name":{"S":"Alvo"}}}`)
		case op == "PutItem" && s.guardExists:
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"__type":"com.amazonaws.dynamodb.v20120810#ConditionalCheckFailedException",`+
				`"message":"The conditional request failed"}`)
		default:
			fmt.Fprint(w, `{}`)
		}
	}))
	t.Cleanup(srv.Close)
	cfg := aws.Config{Region: "us-east-1", Credentials: credentials.NewStaticCredentialsProvider("dummy", "dummy", "")}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String(srv.URL) })
}

func TestReportAvatarCountsOnceAndNeverGrowsTheProfileItem(t *testing.T) {
	stub := &reportStub{}
	store := &Store{base: dynamo.NewBase(stub.client(t), "test", tablePlayerProfiles)}

	if err := store.ReportAvatar(context.Background(), "target", "reporter"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(stub.calls, ","); got != "GetItem,PutItem,UpdateItem" {
		t.Fatalf("expected profile read, guard put, counter update — got %s", got)
	}
	guard := stub.bodies[1]
	if !strings.Contains(guard, avatarReportGuardPK("target", "reporter")) ||
		!strings.Contains(guard, "attribute_not_exists(pk)") {
		t.Fatalf("the distinct-reporter rule must be a conditional put of the guard row: %s", guard)
	}
	update := stub.bodies[2]
	// The whole point of #220: the report writes a bounded counter and drops
	// the legacy unbounded set, never adds to a set on the profile item.
	if strings.Contains(update, `"SS"`) || !strings.Contains(update, "ADD #count :one REMOVE #reporters") {
		t.Fatalf("a report must not write a string set onto the profile: %s", update)
	}
	if !strings.Contains(update, "#count < :cap") {
		t.Fatalf("the aggregate must be capped: %s", update)
	}
}

func TestReportAvatarIsIdempotentForTheSameReporter(t *testing.T) {
	stub := &reportStub{guardExists: true}
	store := &Store{base: dynamo.NewBase(stub.client(t), "test", tablePlayerProfiles)}

	if err := store.ReportAvatar(context.Background(), "target", "reporter"); err != nil {
		t.Fatalf("a repeated report is a no-op, not an error: %v", err)
	}
	if got := strings.Join(stub.calls, ","); got != "GetItem,PutItem" {
		t.Fatalf("a reporter who already reported this target must not bump the counter: %s", got)
	}
}

func TestReportAvatarRejectsSelfReports(t *testing.T) {
	store := &Store{}
	if err := store.ReportAvatar(context.Background(), "same", "same"); err == nil {
		t.Fatal("a player must not be able to report themselves")
	}
}
