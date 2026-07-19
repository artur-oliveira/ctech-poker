//go:build integration

package tablestore

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func testClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(aws.AnonymousCredentials{}),
	)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String("http://localhost:8555")
	})
}

func TestSaveAndLoadSnapshot(t *testing.T) {
	db := testClient(t)
	s := NewStore(db, "test")
	ctx := context.Background()
	mustCreateTestTables(ctx, t, db, "test")

	snap := hand.Snapshot{Stage: "pre_flop"}
	if err := s.SaveSnapshot(ctx, "table-1", "hand-1", 3, snap); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	got, err := s.LoadSnapshot(ctx, "table-1")
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if got == nil || got.HandID != "hand-1" || got.Seq != 3 || got.State.Stage != "pre_flop" {
		t.Fatalf("unexpected snapshot: %+v", got)
	}
}

func TestAppendAndLoadActionsSince(t *testing.T) {
	db := testClient(t)
	s := NewStore(db, "test")
	ctx := context.Background()
	mustCreateTestTables(ctx, t, db, "test")

	for i := 1; i <= 3; i++ {
		entry := ActionLogEntry{TableID: "table-2", HandID: "hand-1", Seq: i, PlayerID: "p1", ActionID: "a" + string(rune('0'+i)), Action: "call"}
		if err := s.AppendAction(ctx, "table-2", "hand-1", i, entry); err != nil {
			t.Fatalf("AppendAction seq %d: %v", i, err)
		}
	}

	got, err := s.LoadActionsSince(ctx, "table-2", "hand-1", 1)
	if err != nil {
		t.Fatalf("LoadActionsSince: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 actions after seq 1, got %d", len(got))
	}
	if got[0].Seq != 2 || got[1].Seq != 3 {
		t.Fatalf("expected ordered seq 2,3, got %d,%d", got[0].Seq, got[1].Seq)
	}
}
