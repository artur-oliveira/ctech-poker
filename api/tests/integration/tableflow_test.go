//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

func testDynamoClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"), config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")))
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String("http://localhost:8555") })
}

func mustCreatePokerTables(t *testing.T, db *dynamodb.Client, env string) {
	t.Helper()
	pkOnly := []string{env + "_poker_table_state", env + "_poker_action_guards"}
	pkSk := []string{env + "_poker_action_log"}
	create := func(name string, withSK bool) {
		attrs := []types.AttributeDefinition{{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS}}
		keys := []types.KeySchemaElement{{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash}}
		if withSK {
			attrs = append(attrs, types.AttributeDefinition{AttributeName: aws.String("sk"), AttributeType: types.ScalarAttributeTypeS})
			keys = append(keys, types.KeySchemaElement{AttributeName: aws.String("sk"), KeyType: types.KeyTypeRange})
		}
		tableName := name
		_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{TableName: &tableName, AttributeDefinitions: attrs, KeySchema: keys, BillingMode: types.BillingModePayPerRequest})
		var inUse *types.ResourceInUseException
		if err != nil && !errors.As(err, &inUse) {
			t.Fatalf("create table %s: %v", name, err)
		}
	}
	for _, n := range pkOnly {
		create(n, false)
	}
	for _, n := range pkSk {
		create(n, true)
	}
}

func TestTwoInstancesRacingSameTableResolveDeterministically(t *testing.T) {
	backend := cache.NewMemoryBackend(16)
	db := testDynamoClient(t)
	store := tablestore.NewStore(db, "flow_test")
	mustCreatePokerTables(t, db, "flow_test")

	seed := func() *hand.Table {
		return hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}, {ID: "p2", Stack: 1000}}, 10, 20)
	}

	// Two "instances", neither holding the lease — both must re-read before
	// every command, so neither trusts a stale cache.
	mgrA := tablemanager.NewManager(tablelease.NewService(backend), store, nil)
	mgrB := tablemanager.NewManager(tablelease.NewService(backend), store, nil)

	actorA, err := mgrA.GetOrCreateActor(context.Background(), "table-race", seed)
	if err != nil {
		t.Fatalf("acquire on instance A: %v", err)
	}
	actorB, err := mgrB.GetOrCreateActor(context.Background(), "table-race", seed)
	if err != nil {
		t.Fatalf("acquire on instance B: %v", err)
	}

	replyA := make(chan error, 1)
	if err := actorA.Dispatch(table.ReadyCmd{PlayerID: "p1", Ready: true, Reply: replyA}); err != nil {
		t.Fatalf("ready p1 via A: %v", err)
	}
	replyB := make(chan error, 1)
	if err := actorB.Dispatch(table.ReadyCmd{PlayerID: "p2", Ready: true, Reply: replyB}); err != nil {
		t.Fatalf("ready p2 via B (must survive A's concurrent version bump): %v", err)
	}

	stored, err := store.LoadTable(context.Background(), "table-race")
	if err != nil || stored == nil || stored.State.Stage == hand.WaitingForPlayers {
		t.Fatalf("expected the hand to have started after both readies landed, got %+v err=%v", stored, err)
	}
}

func TestFreshInstanceReadsCurrentStateWithNoReplayNeeded(t *testing.T) {
	backend := cache.NewMemoryBackend(16)
	db := testDynamoClient(t)
	store := tablestore.NewStore(db, "flow_test")
	mustCreatePokerTables(t, db, "flow_test")

	seed := func() *hand.Table {
		return hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}, {ID: "p2", Stack: 1000}}, 10, 20)
	}

	mgrA := tablemanager.NewManager(tablelease.NewService(backend), store, nil)
	actorA, err := mgrA.GetOrCreateActor(context.Background(), "table-crash", seed)
	if err != nil {
		t.Fatalf("acquire on instance A: %v", err)
	}
	reply := make(chan error, 1)
	_ = actorA.Dispatch(table.ReadyCmd{PlayerID: "p1", Ready: true, Reply: reply})
	reply2 := make(chan error, 1)
	_ = actorA.Dispatch(table.ReadyCmd{PlayerID: "p2", Ready: true, Reply: reply2})
	// Instance A "crashes" here — nothing more happens on it. Under this
	// revision there is nothing to fail over: the next instance just reads
	// current DynamoDB state directly (ARCHITECTURE.md §3).

	mgrB := tablemanager.NewManager(tablelease.NewService(backend), store, nil)
	actorB, err := mgrB.GetOrCreateActor(context.Background(), "table-crash", seed)
	if err != nil {
		t.Fatalf("acquire on instance B: %v", err)
	}
	// TableForTest() is populated only after the Actor has processed at
	// least one command — re-affirm p1's existing Ready:true as a no-op
	// that still forces ensureLoaded to run.
	reply3 := make(chan error, 1)
	if err := actorB.Dispatch(table.ReadyCmd{PlayerID: "p1", Ready: true, Reply: reply3}); err != nil {
		t.Fatalf("ready p1 via B: %v", err)
	}
	view := actorB.TableForTest().ViewFor("p1")
	if view.Stage == "waiting_for_players" {
		t.Fatalf("expected instance B to see the hand already in progress, got stage %s", view.Stage)
	}
}
