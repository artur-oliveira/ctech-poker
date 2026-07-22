//go:build integration

package tablemanager

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
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

func archivedTestClient(t *testing.T) *dynamodb.Client {
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

// mustCreateArchivedTestTables creates the three tablestore tables (with the
// gsi_active_last_action GSI on poker_table_state) under env — a copy of
// tablestore's own unexported helper, following this repo's established
// per-package-copy convention for DynamoDB Local test schema setup.
func mustCreateArchivedTestTables(ctx context.Context, t *testing.T, db *dynamodb.Client, env string) {
	t.Helper()
	create := func(name string, withSK bool, gsis []types.GlobalSecondaryIndex) {
		attrs := []types.AttributeDefinition{{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS}}
		keys := []types.KeySchemaElement{{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash}}
		if withSK {
			attrs = append(attrs, types.AttributeDefinition{AttributeName: aws.String("sk"), AttributeType: types.ScalarAttributeTypeS})
			keys = append(keys, types.KeySchemaElement{AttributeName: aws.String("sk"), KeyType: types.KeyTypeRange})
		}
		if len(gsis) > 0 {
			attrs = append(attrs,
				types.AttributeDefinition{AttributeName: aws.String("gsi_active"), AttributeType: types.ScalarAttributeTypeS},
				types.AttributeDefinition{AttributeName: aws.String("last_action_at"), AttributeType: types.ScalarAttributeTypeN},
			)
		}
		input := &dynamodb.CreateTableInput{
			TableName: aws.String(name), AttributeDefinitions: attrs, KeySchema: keys, BillingMode: types.BillingModePayPerRequest,
		}
		if len(gsis) > 0 {
			input.GlobalSecondaryIndexes = gsis
		}
		if _, err := db.CreateTable(ctx, input); err != nil {
			var inUse *types.ResourceInUseException
			if !errors.As(err, &inUse) {
				t.Fatalf("create table %s: %v", name, err)
			}
		}
	}
	create(env+"_poker_action_guards", false, nil)
	create(env+"_poker_action_log", true, nil)
	create(env+"_poker_table_state", false, []types.GlobalSecondaryIndex{{
		IndexName: aws.String("gsi_active_last_action"),
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("gsi_active"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("last_action_at"), KeyType: types.KeyTypeRange},
		},
		Projection: &types.Projection{ProjectionType: types.ProjectionTypeKeysOnly},
	}})
}

func TestGetOrCreateActorRejectsArchivedTable(t *testing.T) {
	db := archivedTestClient(t)
	env := fmt.Sprintf("tablemanager_test_%d", time.Now().UnixNano())
	store := tablestore.NewStore(db, env)
	ctx := context.Background()
	mustCreateArchivedTestTables(ctx, t, db, env)

	if err := store.SeedTable(ctx, "archived-table", hand.State{Stage: hand.WaitingForPlayers}); err != nil {
		t.Fatalf("SeedTable: %v", err)
	}
	if err := store.MarkArchived(ctx, "archived-table", 1); err != nil {
		t.Fatalf("MarkArchived: %v", err)
	}

	mgr := NewManager(tablelease.NewService(cache.NewMemoryBackend(16)), store, nil, nil)
	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }

	_, err := mgr.GetOrCreateActor(ctx, "archived-table", seed)
	if !errors.Is(err, ErrTableArchived) {
		t.Fatalf("expected ErrTableArchived, got %v", err)
	}
}
