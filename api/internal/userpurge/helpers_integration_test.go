//go:build integration

package userpurge

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/api-commons/erasure"
)

const (
	testU = "11111111-1111-4111-8111-111111111111" // the erased user ("Alice")
	testO = "22222222-2222-4222-8222-222222222222" // an opponent ("Bob")
)

// testClient mirrors the per-package DynamoDB Local helper (buyin/dynamo_helpers_test.go).
func testClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"), config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")))
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String("http://localhost:8555") })
}

type gsi struct {
	name, pk, sk string
	skNumber     bool
}

// purgeTables mirrors cdk/lib/dynamodb-stack.ts: table -> has a sort key.
var purgeTables = map[string]bool{
	TableTableStateHistory: true, TableActionLog: true, TablePlayerProfiles: false, TableAchievementProgress: true,
	TableLeaderboardStats: true, TableDailyReward: true, TablePlayerSessions: true, TablePlayerHands: true,
	TablePlayerNotes: true, TableHandMeta: true, TablePokerStats: false, TableMatchups: false,
	TableSandboxPurchases: true, TableReactionEntitlements: true, TableReactionPurchases: true,
	TableCosmeticEntitlements: true, TableCosmeticPurchases: true, TableCosmeticLoadouts: true,
	TableChatPrefs: false, TableBotCheckContests: true, TableWalletAlertPrefs: false, TablePromoRedemptions: true,
	TableTableEntitlements: true, TableTableHighlights: true, TableHandReveals: false, TableHandRevealPayments: false,
	TableSocialEdges: true, TableRecentPlayers: true, TableSocialEvents: true,
	TablePlayerReports: true, TableHandShares: false, TablePendingCashouts: true,
}

var purgeIndexes = map[string]gsi{
	TablePlayerReports:   {name: gsiReportReporter, pk: "gsi_reporter_pk", sk: "gsi_reporter_sk"},
	TableHandShares:      {name: gsiHandShareOwner, pk: "owner_id", sk: "created_at", skNumber: true},
	TablePendingCashouts: {name: gsiPlayerSettlements, pk: "player_id", sk: "recorded_at"},
}

func createPurgeTables(t *testing.T, db *dynamodb.Client, env string) {
	t.Helper()
	for table, withSK := range purgeTables {
		attrs := map[string]types.ScalarAttributeType{"pk": types.ScalarAttributeTypeS}
		keys := []types.KeySchemaElement{{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash}}
		if withSK {
			attrs["sk"] = types.ScalarAttributeTypeS
			keys = append(keys, types.KeySchemaElement{AttributeName: aws.String("sk"), KeyType: types.KeyTypeRange})
		}
		var indexes []types.GlobalSecondaryIndex
		if ix, ok := purgeIndexes[table]; ok {
			attrs[ix.pk] = types.ScalarAttributeTypeS
			skType := types.ScalarAttributeTypeS
			if ix.skNumber {
				skType = types.ScalarAttributeTypeN
			}
			attrs[ix.sk] = skType
			indexes = append(indexes, types.GlobalSecondaryIndex{
				IndexName: aws.String(ix.name),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String(ix.pk), KeyType: types.KeyTypeHash},
					{AttributeName: aws.String(ix.sk), KeyType: types.KeyTypeRange},
				},
				Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
			})
		}
		var defs []types.AttributeDefinition
		for name, typ := range attrs {
			defs = append(defs, types.AttributeDefinition{AttributeName: aws.String(name), AttributeType: typ})
		}
		if _, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
			TableName: aws.String(dynamo.TableName(env, table)), AttributeDefinitions: defs, KeySchema: keys,
			GlobalSecondaryIndexes: indexes, BillingMode: types.BillingModePayPerRequest,
		}); err != nil {
			t.Fatalf("create %s: %v", table, err)
		}
	}
}

func put(t *testing.T, db *dynamodb.Client, env, table string, fields map[string]any) {
	t.Helper()
	av, err := attributevalue.MarshalMap(fields)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	putAV(t, db, env, table, av)
}

func putAV(t *testing.T, db *dynamodb.Client, env, table string, av map[string]types.AttributeValue) {
	t.Helper()
	if _, err := db.PutItem(context.Background(), &dynamodb.PutItemInput{TableName: aws.String(dynamo.TableName(env, table)), Item: av}); err != nil {
		t.Fatalf("put %s: %v", table, err)
	}
}

func scanTable(t *testing.T, db *dynamodb.Client, env, table string) []map[string]types.AttributeValue {
	t.Helper()
	out, err := db.Scan(context.Background(), &dynamodb.ScanInput{TableName: aws.String(dynamo.TableName(env, table)), ConsistentRead: aws.Bool(true)})
	if err != nil {
		t.Fatalf("scan %s: %v", table, err)
	}
	return out.Items
}

// mentions reports whether needle appears anywhere in it, keys included.
func mentions(it map[string]types.AttributeValue, needle string) bool {
	_, found := anonymizeMap(it, needle, "\x00")
	return found
}

func noBlockers(context.Context, string) ([]erasure.Blocker, error) { return nil, nil }

func newTestPurger(t *testing.T, db *dynamodb.Client, objects S3API, c cache.Backend, blockers BlockersFunc) (*Purger, string) {
	t.Helper()
	env := fmt.Sprintf("userpurge_test_%d", time.Now().UnixNano())
	createPurgeTables(t, db, env)
	return NewPurger(db, objects, c, Config{Env: env, AvatarBucket: "avatars", ArchiveBucket: "archive"}, blockers), env
}

func purgeMsg() erasure.Message {
	return erasure.Message{Version: 1, Type: erasure.TypeErase, RequestID: "req-1", Sub: testU,
		Scope: erasure.ScopeAccount, Services: []string{"poker"}, IssuedAt: time.Now()}
}

// fakeS3 is an in-memory bucket store with one version per key.
type fakeS3 struct {
	mu      sync.Mutex
	objects map[string][]byte // bucket + "/" + key
}

func newFakeS3() *fakeS3 { return &fakeS3{objects: map[string][]byte{}} }

func (f *fakeS3) put(bucket, key string, body []byte) { f.objects[bucket+"/"+key] = body }

func (f *fakeS3) keys(bucket, prefix string) []string {
	var out []string
	for k := range f.objects {
		if rest, ok := strings.CutPrefix(k, bucket+"/"); ok && strings.HasPrefix(rest, prefix) {
			out = append(out, rest)
		}
	}
	sort.Strings(out)
	return out
}

func (f *fakeS3) ListObjectsV2(_ context.Context, in *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := &s3.ListObjectsV2Output{IsTruncated: aws.Bool(false)}
	for _, k := range f.keys(aws.ToString(in.Bucket), aws.ToString(in.Prefix)) {
		out.Contents = append(out.Contents, s3types.Object{Key: aws.String(k)})
	}
	return out, nil
}

func (f *fakeS3) GetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, ok := f.objects[aws.ToString(in.Bucket)+"/"+aws.ToString(in.Key)]
	if !ok {
		return nil, fmt.Errorf("no such key %s", aws.ToString(in.Key))
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(body))}, nil
}

func (f *fakeS3) PutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	body, err := io.ReadAll(in.Body)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.put(aws.ToString(in.Bucket), aws.ToString(in.Key), body)
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeS3) ListObjectVersions(_ context.Context, in *s3.ListObjectVersionsInput, _ ...func(*s3.Options)) (*s3.ListObjectVersionsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := &s3.ListObjectVersionsOutput{IsTruncated: aws.Bool(false)}
	for _, k := range f.keys(aws.ToString(in.Bucket), aws.ToString(in.Prefix)) {
		out.Versions = append(out.Versions, s3types.ObjectVersion{Key: aws.String(k), VersionId: aws.String("v1")})
	}
	return out, nil
}

func (f *fakeS3) DeleteObjects(_ context.Context, in *s3.DeleteObjectsInput, _ ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, o := range in.Delete.Objects {
		delete(f.objects, aws.ToString(in.Bucket)+"/"+aws.ToString(o.Key))
	}
	return &s3.DeleteObjectsOutput{}, nil
}

// seedSharedHand writes one sandbox hand h1 at table t1 between U and O.
func seedSharedHand(t *testing.T, db *dynamodb.Client, env string, objects *fakeS3) {
	t.Helper()
	avatar := "https://poker-api.aoctech.app/v1.0/avatars/" + testU + "/1.jpg"
	put(t, db, env, TablePlayerHands, map[string]any{"pk": testU, "sk": "sandbox#h1", "currency_mode": "sandbox", "table_id": "t1", "hand_id": "h1",
		"opponents": []map[string]any{{"player_id": testO, "name": "Bob"}}})
	put(t, db, env, TablePlayerHands, map[string]any{"pk": testO, "sk": "sandbox#h1", "currency_mode": "sandbox", "table_id": "t1", "hand_id": "h1",
		"opponents": []map[string]any{{"player_id": testU, "name": "Alice", "avatar_url": avatar, "hole_cards": []string{"As", "Ad"}}}})
	put(t, db, env, TableActionLog, map[string]any{"pk": "t1#h1", "sk": "0000000001", "player_id": testU, "message": "oi, sou a Alice",
		"frame": map[string]any{"seats": []map[string]any{{"player_id": testU, "name": "Alice"}, {"player_id": testO, "name": "Bob"}}}})
	put(t, db, env, TableTableStateHistory, map[string]any{"pk": "t1", "sk": "100",
		"state": map[string]any{"Players": []map[string]any{{"id": testU, "name": "Alice", "avatar_url": avatar}, {"id": testO, "name": "Bob"}},
			"Payouts": map[string]any{testU: 10}}})
	put(t, db, env, TableTableHighlights, map[string]any{"pk": "t1", "sk": "2026-10-07",
		"winners": []map[string]any{{"player_id": testU, "name": "Alice", "payout": 10}}})
	put(t, db, env, TableHandReveals, map[string]any{"pk": "h1", "winner_id": testU,
		"player_hands": map[string]any{testU: map[string]any{"cards": []string{"As", "Ad"}}, testO: map[string]any{"cards": []string{"2c", "7d"}}}})
	put(t, db, env, TableHandRevealPayments, map[string]any{"pk": "h1#" + testU})
	put(t, db, env, TableRecentPlayers, map[string]any{"pk": testO, "sk": "hand#h1", "opponents": []string{testU}})
	put(t, db, env, TableMatchups, map[string]any{"pk": "pair#sandbox#" + testU + "#" + testO, "hands_together": 1})
	objects.put("archive", "t1/h1/1-e1.jsonl",
		[]byte(`{"player_id":"`+testU+`","message":"oi, sou a Alice","amount":5,"frame":{"seats":[{"player_id":"`+testU+`","name":"Alice"},{"player_id":"`+testO+`","name":"Bob"}]}}`+"\n"))
}
