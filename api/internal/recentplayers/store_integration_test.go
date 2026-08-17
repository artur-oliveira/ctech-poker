//go:build integration

package recentplayers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

func TestDynamoRecordHandCreatesDirectedPairsOnce(t *testing.T) {
	db := recentTestClient(t)
	env := fmt.Sprintf("recent_test_%d", time.Now().UnixNano())
	name := dynamo.TableName(env, tableRecentPlayers)
	_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: aws.String(name), BillingMode: types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("sk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("gsi_recent_pk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("gsi_recent_sk"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash}, {AttributeName: aws.String("sk"), KeyType: types.KeyTypeRange}},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{{IndexName: aws.String(gsiRecentPlayers),
			KeySchema:  []types.KeySchemaElement{{AttributeName: aws.String("gsi_recent_pk"), KeyType: types.KeyTypeHash}, {AttributeName: aws.String("gsi_recent_sk"), KeyType: types.KeyTypeRange}},
			Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db, env)
	playedAt := time.Now().UTC().Truncate(time.Millisecond)
	hand := HandCompletion{TableID: "table", HandID: "hand", Players: []string{"a", "b", "c"}, PlayedAt: playedAt}
	if err := store.RecordHand(context.Background(), hand); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordHand(context.Background(), hand); err != nil {
		t.Fatal(err)
	}

	var page Page
	for attempt := 0; attempt < 20; attempt++ {
		page, err = store.List(context.Background(), "a", nil, 50)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Players) == 2 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if len(page.Players) != 2 {
		t.Fatalf("pairs=%+v", page.Players)
	}
	for _, item := range page.Players {
		if item.HandsTogether != 1 {
			t.Fatalf("duplicate increment: %+v", item)
		}
		if item.TTL < playedAt.Add(89*24*time.Hour).Unix() {
			t.Fatalf("ttl too short: %+v", item)
		}
	}
}

func recentTestClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion("us-east-1"), awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")))
	if err != nil {
		t.Fatal(err)
	}
	return dynamodb.NewFromConfig(cfg, func(options *dynamodb.Options) { options.BaseEndpoint = aws.String("http://localhost:8555") })
}
