//go:build integration

package table

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// stopActor cancels an Actor's Run context and blocks until its goroutine has
// actually exited (Done() closes in Run's deferred close(a.done)). Cancelling
// alone only signals Run's select loop -- if Run is mid-handle() on a long
// synchronous chain (e.g. a runout timer cascade), it keeps running for a
// while after cancel() returns. A bare t.Cleanup(cancel) races: the next test
// in this binary can start, and its own goroutine reads/writes package-level
// test state (like timebank_test.go's timeNowFunc) while this one is still
// live, producing a real, if rare, data race between unrelated tests.
func stopActor(t *testing.T, a *Actor, cancel context.CancelFunc) {
	t.Helper()
	t.Cleanup(func() {
		cancel()
		<-a.Done()
	})
}

var uniqueTableIDSeq atomic.Int64

// uniqueTableID returns a tableID scoped to both this test's name AND this
// specific process invocation -- t.Name() alone repeats identically across
// separate `go test` runs, so against a persistent (non-restarted) DynamoDB
// Local, tablestore.SeedTable's put-if-absent semantics would silently reuse
// whatever state a PREVIOUS run of the same test left behind instead of
// seeding fresh.
func uniqueTableID(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("table-%s-%d-%d", t.Name(), time.Now().UnixNano(), uniqueTableIDSeq.Add(1))
}

func testClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"), config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")))
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String("http://localhost:8555") })
}

func mustCreateTestTables(t *testing.T, db *dynamodb.Client, env string) {
	t.Helper()
	pkOnly := []string{env + "_poker_table_state", env + "_poker_action_guards"}
	pkSk := []string{env + "_poker_action_log", env + "_poker_table_state_history"}
	for _, name := range pkOnly {
		createTestTable(t, db, name, false)
	}
	for _, name := range pkSk {
		createTestTable(t, db, name, true)
	}
}

func createTestTable(t *testing.T, db *dynamodb.Client, name string, withSK bool) {
	t.Helper()
	attrs := []types.AttributeDefinition{{AttributeName: new("pk"), AttributeType: types.ScalarAttributeTypeS}}
	keys := []types.KeySchemaElement{{AttributeName: new("pk"), KeyType: types.KeyTypeHash}}
	if withSK {
		attrs = append(attrs, types.AttributeDefinition{AttributeName: new("sk"), AttributeType: types.ScalarAttributeTypeS})
		keys = append(keys, types.KeySchemaElement{AttributeName: new("sk"), KeyType: types.KeyTypeRange})
	}
	tableName := name
	_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{TableName: &tableName, AttributeDefinitions: attrs, KeySchema: keys, BillingMode: types.BillingModePayPerRequest})
	if err != nil {
		var inUse *types.ResourceInUseException
		if !errors.As(err, &inUse) {
			t.Fatalf("create table %s: %v", name, err)
		}
	}
}

func new(s string) *string { return &s }
