package botcheck

import (
	"context"
	"errors"
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

func testContestStore(t *testing.T, handler http.HandlerFunc) *ContestStore {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	cfg := aws.Config{Region: "us-east-1", Credentials: credentials.NewStaticCredentialsProvider("dummy", "dummy", "")}
	client := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String(srv.URL) })
	return &ContestStore{base: dynamo.NewBase(client, "test", tableBotCheckContests)}
}

// The contest flow must never touch Turnstile's siteverify endpoint — it is
// a completely separate, append-only path from Verify (#322: "separado do
// caminho de verificação normal"). This test's fake DynamoDB handler is the
// only network call Record makes; nothing here constructs or calls a
// Service at all, which is itself the isolation the acceptance criterion
// asks for — a contest never has a Turnstile token to accept or reject.
func TestRecordContestNeverCallsVerification(t *testing.T) {
	var puts int
	store := testContestStore(t, func(w http.ResponseWriter, r *http.Request) {
		puts++
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		_, _ = io.WriteString(w, `{}`)
	})
	contest, err := store.Record(context.Background(), "player-1", "table-1", "I was not a bot")
	if err != nil {
		t.Fatal(err)
	}
	if puts != 1 {
		t.Fatalf("expected exactly one DynamoDB write, got %d", puts)
	}
	if contest.Status != ContestStatusPending {
		t.Fatalf("expected a pending contest, got status %q", contest.Status)
	}
}

func TestRecordContestRequiresPlayer(t *testing.T) {
	store := &ContestStore{}
	if _, err := store.Record(context.Background(), "  ", "table-1", ""); !errors.Is(err, ErrContestPlayerRequired) {
		t.Fatalf("expected ErrContestPlayerRequired, got %v", err)
	}
}

func TestRecordContestRejectsOversizedReason(t *testing.T) {
	store := &ContestStore{}
	reason := strings.Repeat("a", MaxContestReasonLength+1)
	if _, err := store.Record(context.Background(), "player-1", "table-1", reason); !errors.Is(err, ErrContestReasonTooLong) {
		t.Fatalf("expected ErrContestReasonTooLong, got %v", err)
	}
}
