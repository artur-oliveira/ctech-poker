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
type archiveFile struct {
	key     string
	payload []byte
}

// buildBatches groups INSERT stream records by partition key (poker_action_log's
// pk is "table_id#hand_id") into separate S3 objects. Non-INSERT records
// (TTL-expiry emits REMOVE) are skipped: an expiring item already reached S3
// on its own INSERT, so archiving its REMOVE would just duplicate it.
func buildBatches(e events.DynamoDBEvent) ([]archiveFile, error) {
	groups := make(map[string]*bytes.Buffer)
	lastEventIDs := make(map[string]string)
	for _, r := range e.Records {
		if r.EventName != "INSERT" {
			continue
		}
		pkAttr, ok := r.Change.NewImage["pk"]
		if !ok || pkAttr.String() == "" {
			continue
		}
		pk := pkAttr.String()
		buf, exists := groups[pk]
		if !exists {
			buf = &bytes.Buffer{}
			groups[pk] = buf
		}
		lastEventIDs[pk] = r.EventID
		rendered, err := attributeMapToJSON(r.Change.NewImage)
		if err != nil {
			return nil, fmt.Errorf("archiver: encode record: %w", err)
		}
		buf.Write(rendered)
		buf.WriteByte('\n')
	}

	files := make([]archiveFile, 0, len(groups))
	for pk, buf := range groups {
		if buf.Len() == 0 {
			continue
		}
		partition := strings.ReplaceAll(pk, "#", "/")
		key := fmt.Sprintf("%s/%d-%s.jsonl", partition, time.Now().UnixNano(), lastEventIDs[pk])
		files = append(files, archiveFile{key: key, payload: buf.Bytes()})
	}
	return files, nil
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
		// json.Number preserves DynamoDB's number exactly as stored — it is
		// itself a string, but encoding/json emits it as a bare numeric
		// token (never quoted) as long as it's a valid JSON number literal,
		// which every DynamoDB Number attribute already is. Routing this
		// through float64 (the old behaviour) silently loses precision past
		// 2^53, which matters here: this archive is the permanent audit
		// trail for every chip amount and payout (#55).
		return json.Number(v.Number())
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
		files, err := buildBatches(e)
		if err != nil {
			return err
		}
		for _, f := range files {
			if err := putter.PutObject(ctx, bucket, f.key, f.payload); err != nil {
				return fmt.Errorf("archiver: put %s: %w", f.key, err)
			}
		}
		return nil
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
