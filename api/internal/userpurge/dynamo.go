package userpurge

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	batchWriteMax      = 25
	batchWriteAttempts = 5
)

type item = map[string]types.AttributeValue

func key(pk, sk string) item {
	k := item{"pk": str(pk)}
	if sk != "" {
		k["sk"] = str(sk)
	}
	return k
}

func keyOf(it item) item {
	k := item{"pk": it["pk"]}
	if sk, ok := it["sk"]; ok {
		k["sk"] = sk
	}
	return k
}

func stringAttr(it item, name string) string {
	if s, ok := it[name].(*types.AttributeValueMemberS); ok {
		return s.Value
	}
	return ""
}

func (p *Purger) table(name string) string { return dynamo.TableName(p.cfg.Env, name) }

// partition is a strongly consistent query of one base-table partition.
func (p *Purger) partition(table, pk string) *dynamodb.QueryInput {
	return &dynamodb.QueryInput{
		TableName:                 aws.String(p.table(table)),
		KeyConditionExpression:    aws.String("pk = :pk"),
		ExpressionAttributeValues: item{":pk": str(pk)},
		ConsistentRead:            aws.Bool(true),
	}
}

// eachItem pages in and calls fn per item. Deleting the items already seen
// is safe: ExclusiveStartKey resumes after a key even if it is gone.
func (p *Purger) eachItem(ctx context.Context, in *dynamodb.QueryInput, fn func(item) error) error {
	for {
		out, err := p.db.Query(ctx, in)
		if err != nil {
			return fmt.Errorf("userpurge: query %s: %w", aws.ToString(in.TableName), err)
		}
		for _, it := range out.Items {
			if err := fn(it); err != nil {
				return err
			}
		}
		if len(out.LastEvaluatedKey) == 0 {
			return nil
		}
		in.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

// eachScan is eachItem for a Scan. Used only where no index exists; erasures
// are rare, so a full read per erasure is the price of not adding a GSI.
func (p *Purger) eachScan(ctx context.Context, in *dynamodb.ScanInput, fn func(item) error) error {
	for {
		out, err := p.db.Scan(ctx, in)
		if err != nil {
			return fmt.Errorf("userpurge: scan %s: %w", aws.ToString(in.TableName), err)
		}
		for _, it := range out.Items {
			if err := fn(it); err != nil {
				return err
			}
		}
		if len(out.LastEvaluatedKey) == 0 {
			return nil
		}
		in.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

// deleteKeys batch-deletes keys (25 per call), retrying unprocessed items.
func (p *Purger) deleteKeys(ctx context.Context, table string, keys []item) error {
	name := p.table(table)
	for len(keys) > 0 {
		n := min(len(keys), batchWriteMax)
		reqs := make([]types.WriteRequest, n)
		for i, k := range keys[:n] {
			reqs[i] = types.WriteRequest{DeleteRequest: &types.DeleteRequest{Key: k}}
		}
		keys = keys[n:]
		pending := map[string][]types.WriteRequest{name: reqs}
		for attempt := 0; len(pending) > 0; attempt++ {
			if attempt == batchWriteAttempts {
				return fmt.Errorf("userpurge: batch delete %s: unprocessed items after %d attempts", name, attempt)
			}
			if attempt > 0 {
				time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
			}
			out, err := p.db.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{RequestItems: pending})
			if err != nil {
				return fmt.Errorf("userpurge: batch delete %s: %w", name, err)
			}
			pending = out.UnprocessedItems
		}
	}
	return nil
}

func (r *run) getItem(ctx context.Context, table string, k item) (item, error) {
	out, err := r.p.db.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(r.p.table(table)), Key: k, ConsistentRead: aws.Bool(true)})
	if err != nil {
		return nil, fmt.Errorf("userpurge: get %s: %w", table, err)
	}
	return out.Item, nil
}

// rewrite pseudonymizes it in place. A row reaped by TTL meanwhile is fine.
func (r *run) rewrite(ctx context.Context, table string, it item) error {
	out, changed := anonymizeItem(it, r.sub, r.pseudo)
	if !changed {
		return nil
	}
	_, err := r.p.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(r.p.table(table)),
		Item:                out,
		ConditionExpression: aws.String("attribute_exists(pk)"),
	})
	if dynamo.IsConditionFailed(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("userpurge: rewrite %s: %w", table, err)
	}
	r.count(table+"_anonymized", 1)
	return nil
}

func (r *run) rewriteKey(ctx context.Context, table string, k item) error {
	it, err := r.getItem(ctx, table, k)
	if err != nil || len(it) == 0 {
		return err
	}
	return r.rewrite(ctx, table, it)
}

// rekey moves old to next's key in one transaction (rows whose key holds the sub).
func (r *run) rekey(ctx context.Context, table string, old, next item) error {
	name := aws.String(r.p.table(table))
	_, err := r.p.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: []types.TransactWriteItem{
		{Delete: &types.Delete{TableName: name, Key: keyOf(old)}},
		{Put: &types.Put{TableName: name, Item: next}},
	}})
	if err != nil {
		return fmt.Errorf("userpurge: rekey %s: %w", table, err)
	}
	r.count(table+"_anonymized", 1)
	return nil
}

// deleteItem deletes one item and counts it only if it existed.
func (r *run) deleteItem(ctx context.Context, table string, k item) error {
	out, err := r.p.db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(r.p.table(table)), Key: k, ReturnValues: types.ReturnValueAllOld,
	})
	if err != nil {
		return fmt.Errorf("userpurge: delete %s: %w", table, err)
	}
	if len(out.Attributes) > 0 {
		r.count(table+"_erased", 1)
	}
	return nil
}

// erasePartition deletes every item whose pk is pk.
func (r *run) erasePartition(ctx context.Context, table, pk string) error {
	var keys []item
	if err := r.p.eachItem(ctx, r.p.partition(table, pk), func(it item) error {
		keys = append(keys, keyOf(it))
		return nil
	}); err != nil {
		return err
	}
	r.count(table+"_erased", len(keys))
	return r.p.deleteKeys(ctx, table, keys)
}
