//go:build integration

package tablestore

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func testClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
	)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String("http://localhost:8555")
	})
}

func isolatedEnv() string { return fmt.Sprintf("tablestore_test_%d", time.Now().UnixNano()) }

func TestSeedThenCommitThenLoad(t *testing.T) {
	db := testClient(t)
	env := isolatedEnv()
	s := NewStore(db, env)
	ctx := context.Background()
	mustCreateTestTables(ctx, t, db, env)

	if err := s.SeedTable(ctx, "table-1", hand.State{Stage: hand.WaitingForPlayers}); err != nil {
		t.Fatalf("SeedTable: %v", err)
	}

	loaded, err := s.LoadTable(ctx, "table-1")
	if err != nil || loaded == nil || loaded.Version != 1 {
		t.Fatalf("expected version 1 after seed, got %+v err=%v", loaded, err)
	}

	newState := hand.State{Stage: hand.PreFlop}
	if err := s.CommitAction(ctx, "table-1", "hand-1", "act-1", 1, newState, ActionLogEntry{
		TableID: "table-1", HandID: "hand-1", Version: 2, PlayerID: "p1", ActionID: "act-1", Action: "call",
	}); err != nil {
		t.Fatalf("CommitAction: %v", err)
	}

	loaded, err = s.LoadTable(ctx, "table-1")
	if err != nil || loaded.Version != 2 || loaded.State.Stage != hand.PreFlop {
		t.Fatalf("expected version 2 pre_flop after commit, got %+v err=%v", loaded, err)
	}
}

func TestCommitActionRejectsStaleVersion(t *testing.T) {
	db := testClient(t)
	env := isolatedEnv()
	s := NewStore(db, env)
	ctx := context.Background()
	mustCreateTestTables(ctx, t, db, env)

	_ = s.SeedTable(ctx, "table-2", hand.State{Stage: hand.WaitingForPlayers})

	err := s.CommitAction(ctx, "table-2", "hand-1", "act-1", 99, hand.State{}, ActionLogEntry{TableID: "table-2", HandID: "hand-1", Version: 100, ActionID: "act-1"})
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("expected ErrVersionConflict against a stale expected version, got %v", err)
	}
}

func TestSeedAndCommitSetLastActionAt(t *testing.T) {
	db := testClient(t)
	env := isolatedEnv()
	s := NewStore(db, env)
	ctx := context.Background()
	mustCreateTestTables(ctx, t, db, env)

	timeNowFunc = func() time.Time { return time.Unix(1000, 0) }
	defer func() { timeNowFunc = time.Now }()

	if err := s.SeedTable(ctx, "table-4", hand.State{Stage: hand.WaitingForPlayers}); err != nil {
		t.Fatalf("SeedTable: %v", err)
	}
	loaded, err := s.LoadTable(ctx, "table-4")
	if err != nil || loaded == nil || loaded.LastActionAt != 1000 {
		t.Fatalf("expected last_action_at=1000 after seed, got %+v err=%v", loaded, err)
	}
	if loaded.Archived {
		t.Fatalf("expected a freshly seeded table to not be archived")
	}

	timeNowFunc = func() time.Time { return time.Unix(2000, 0) }
	if err := s.CommitAction(ctx, "table-4", "hand-1", "act-1", 1, hand.State{Stage: hand.PreFlop}, ActionLogEntry{
		TableID: "table-4", HandID: "hand-1", Version: 2, PlayerID: "p1", ActionID: "act-1", Action: "call",
	}); err != nil {
		t.Fatalf("CommitAction: %v", err)
	}
	loaded, err = s.LoadTable(ctx, "table-4")
	if err != nil || loaded.LastActionAt != 2000 {
		t.Fatalf("expected last_action_at=2000 after commit, got %+v err=%v", loaded, err)
	}
}

func TestCommitActionRejectsDuplicateActionID(t *testing.T) {
	db := testClient(t)
	env := isolatedEnv()
	s := NewStore(db, env)
	ctx := context.Background()
	mustCreateTestTables(ctx, t, db, env)

	_ = s.SeedTable(ctx, "table-3", hand.State{Stage: hand.WaitingForPlayers})
	entry := ActionLogEntry{TableID: "table-3", HandID: "hand-1", Version: 2, ActionID: "dup-1"}
	if err := s.CommitAction(ctx, "table-3", "hand-1", "dup-1", 1, hand.State{Stage: hand.PreFlop}, entry); err != nil {
		t.Fatalf("first commit: %v", err)
	}

	err := s.CommitAction(ctx, "table-3", "hand-1", "dup-1", 2, hand.State{Stage: hand.Flop}, ActionLogEntry{TableID: "table-3", HandID: "hand-1", Version: 3, ActionID: "dup-1"})
	if !errors.Is(err, ErrDuplicateAction) {
		t.Fatalf("expected ErrDuplicateAction on a replayed action_id, got %v", err)
	}
}
