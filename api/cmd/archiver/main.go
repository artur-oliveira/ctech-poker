// archiver is a Lambda subscribed to poker_action_log's DynamoDB Stream
// (cdk/lib/archiver-stack.ts): it ships every inserted ActionLogEntry to S3
// before logTTLDays (tablestore's hot-table TTL) ever reaps it. DynamoDB
// serves the recent window; S3 is the indefinite archive.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// s3Putter is the minimal surface main needs from *s3.Client — narrowed so
// buildBatch's caller can be tested against a fake without a live bucket.
type s3Putter interface {
	PutObject(ctx context.Context, bucket, key string, body []byte) error
}

type realS3Putter struct{ client *s3.Client }

func (p *realS3Putter) PutObject(ctx context.Context, bucket, key string, body []byte) error {
	_, err := p.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), Body: bytes.NewReader(body)})
	return err
}

// buildBatch renders every INSERT record's NewImage as one JSON line (JSON
// Lines format, so a later consumer processes the archive without loading a
// whole batch into memory as a single document) and derives an S3 key
// partitioned by table_id/hand_id (poker_action_log's pk is
// "table_id#hand_id" — see tablestore.CommitAction). Non-INSERT records
// (TTL-expiry emits REMOVE) are skipped: an expiring item already reached S3
// on its own INSERT, so archiving its REMOVE would just duplicate it.
func buildBatch(e events.DynamoDBEvent) (batch []byte, key string, err error) {
	var buf bytes.Buffer
	var firstPK, lastEventID string
	for _, r := range e.Records {
		if r.EventName != "INSERT" {
			continue
		}
		if firstPK == "" {
			firstPK = r.Change.NewImage["pk"].String()
		}
		lastEventID = r.EventID
		rendered, err := attributeMapToJSON(r.Change.NewImage)
		if err != nil {
			return nil, "", fmt.Errorf("archiver: encode record: %w", err)
		}
		buf.Write(rendered)
		buf.WriteByte('\n')
	}
	if buf.Len() == 0 {
		return nil, "", nil
	}
	partition := strings.ReplaceAll(firstPK, "#", "/")
	key = fmt.Sprintf("%s/%d-%s.jsonl", partition, time.Now().UnixNano(), lastEventID)
	return buf.Bytes(), key, nil
}

// attributeMapToJSON converts one DynamoDB Stream NewImage into a compact
// JSON object — events.DynamoDBAttributeValue has no built-in JSON
// marshaler, so this recurses over its DataType() itself.
func attributeMapToJSON(m map[string]events.DynamoDBAttributeValue) ([]byte, error) {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = attributeValueToInterface(v)
	}
	return json.Marshal(out)
}

func attributeValueToInterface(v events.DynamoDBAttributeValue) any {
	switch v.DataType() {
	case events.DataTypeString:
		return v.String()
	case events.DataTypeNumber:
		n, _ := strconv.ParseFloat(v.Number(), 64)
		return n
	case events.DataTypeBoolean:
		return v.Boolean()
	case events.DataTypeNull:
		return nil
	case events.DataTypeList:
		list := v.List()
		out := make([]any, len(list))
		for i, item := range list {
			out[i] = attributeValueToInterface(item)
		}
		return out
	case events.DataTypeMap:
		return mapToInterface(v.Map())
	default:
		return nil // Binary/*Set: not present in ActionLogEntry, skipped rather than guessed at
	}
}

func mapToInterface(m map[string]events.DynamoDBAttributeValue) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = attributeValueToInterface(v)
	}
	return out
}

func handle(putter s3Putter, bucket string) func(context.Context, events.DynamoDBEvent) error {
	return func(ctx context.Context, e events.DynamoDBEvent) error {
		batch, key, err := buildBatch(e)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		return putter.PutObject(ctx, bucket, key, batch)
	}
}

func main() {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		panic(fmt.Errorf("archiver: load AWS config: %w", err))
	}
	bucket := os.Getenv("ARCHIVE_BUCKET")
	lambda.Start(handle(&realS3Putter{client: s3.NewFromConfig(cfg)}, bucket))
}
