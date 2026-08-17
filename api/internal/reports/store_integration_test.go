//go:build integration

package reports

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

func TestDynamoCreateIsIdempotentAndResolutionAddsRetention(t *testing.T) {
	db := reportTestClient(t)
	env := fmt.Sprintf("reports_test_%d", time.Now().UnixNano())
	name := dynamo.TableName(env, tablePlayerReports)
	_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: aws.String(name), BillingMode: types.BillingModePayPerRequest,
		AttributeDefinitions:   []types.AttributeDefinition{{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS}, {AttributeName: aws.String("sk"), AttributeType: types.ScalarAttributeTypeS}, {AttributeName: aws.String("gsi_status_pk"), AttributeType: types.ScalarAttributeTypeS}, {AttributeName: aws.String("gsi_status_sk"), AttributeType: types.ScalarAttributeTypeS}},
		KeySchema:              []types.KeySchemaElement{{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash}, {AttributeName: aws.String("sk"), KeyType: types.KeyTypeRange}},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{{IndexName: aws.String(gsiReportStatus), KeySchema: []types.KeySchemaElement{{AttributeName: aws.String("gsi_status_pk"), KeyType: types.KeyTypeHash}, {AttributeName: aws.String("gsi_status_sk"), KeyType: types.KeyTypeRange}}, Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db, env)
	input := Report{TargetPlayerID: "target", ReporterPlayerID: "reporter", Category: CategoryHarassment, Surface: SurfaceProfile, Details: "private"}
	first, inserted, err := store.Create(context.Background(), input, "idem")
	if err != nil || !inserted {
		t.Fatalf("inserted=%v err=%v", inserted, err)
	}
	second, inserted, err := store.Create(context.Background(), input, "idem")
	if err != nil || inserted || second.ReportID != first.ReportID {
		t.Fatalf("second=%+v inserted=%v err=%v", second, inserted, err)
	}
	if err := store.SetStatus(context.Background(), first.TargetPlayerID, first.StorageKey, StatusReviewing, "", "moderator"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetStatus(context.Background(), first.TargetPlayerID, first.StorageKey, StatusResolved, ResolutionNoAction, "moderator"); err != nil {
		t.Fatal(err)
	}
	resolved, err := store.Get(context.Background(), first.TargetPlayerID, first.StorageKey)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != StatusResolved || resolved.Resolution != ResolutionNoAction || resolved.TTL < time.Now().Add(179*24*time.Hour).Unix() || resolved.Details != "private" {
		t.Fatalf("resolved=%+v", resolved)
	}
}

func reportTestClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion("us-east-1"), awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")))
	if err != nil {
		t.Fatal(err)
	}
	return dynamodb.NewFromConfig(cfg, func(options *dynamodb.Options) { options.BaseEndpoint = aws.String("http://localhost:8555") })
}
