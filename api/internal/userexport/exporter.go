// Package userexport builds poker's part of the personal-data export: a zip with
// one JSON file per table that holds the person's own data, plus a README. It
// is the dataexport.PartFunc of the poker service
// (ctech-account docs/specs/2026-10-17-data-export.md).
package userexport

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
)

// maxItems bounds one file. A hand history past it is cut and the file says so.
// ponytail: 100k items per table covers any real player; stream to S3 in pages
// if one ever outgrows it.
const maxItems = 100_000

// Query is the slice of the DynamoDB client the exporter uses.
type Query interface {
	Query(ctx context.Context, in *dynamodb.QueryInput, opts ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
}

// source is one file of the part: partitions of a table keyed by the sub, or a
// global index on an owner attribute.
type source struct {
	table string
	pks   func(sub string) []string // base-table partitions
	index string                    // or: a GSI queried on attr = sub
	attr  string
}

func bySub(sub string) []string { return []string{sub} }

var sources = []source{
	{table: "poker_player_profiles", pks: bySub},
	{table: "poker_achievement_progress", pks: bySub},
	{table: "poker_leaderboard_stats", pks: bySub},
	{table: "poker_daily_reward", pks: bySub},
	{table: "poker_player_sessions", pks: bySub},
	{table: "poker_player_hands", pks: bySub},
	{table: "poker_player_notes", pks: bySub},
	{table: "poker_hand_meta", pks: bySub},
	{table: "poker_player_poker_stats", pks: func(sub string) []string {
		return []string{"stats#" + roomstore.CurrencyModeSandbox + "#" + sub, "stats#" + roomstore.CurrencyModeReal + "#" + sub}
	}},
	{table: "poker_sandbox_purchases", pks: bySub},
	{table: "poker_reaction_entitlements", pks: bySub},
	{table: "poker_reaction_purchases", pks: bySub},
	{table: "poker_cosmetic_entitlements", pks: bySub},
	{table: "poker_cosmetic_purchases", pks: bySub},
	{table: "poker_cosmetic_loadouts", pks: bySub},
	{table: "poker_chat_prefs", pks: bySub},
	{table: "poker_botcheck_contests", pks: bySub},
	{table: "poker_wallet_alert_prefs", pks: bySub},
	{table: "poker_promo_redemptions", pks: bySub},
	{table: "poker_table_entitlements", pks: bySub},
	{table: "poker_social_edges", pks: bySub},
	{table: "poker_recent_players", pks: bySub},
	{table: "poker_social_events", pks: bySub},
	{table: "poker_responsible", pks: bySub},
	{table: "poker_hand_shares", index: "gsi_owner", attr: "owner_id"},
	{table: "poker_pending_cashouts", index: "gsi_player_settlements", attr: "player_id"},
}

type Exporter struct {
	db  Query
	env string
	now func() time.Time
}

func New(db Query, env string) *Exporter { return &Exporter{db: db, env: env, now: time.Now} }

// Part builds the zip for sub. It is idempotent: a redelivery builds it again.
func (e *Exporter) Part(ctx context.Context, sub string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	var truncated []string
	var files []string
	for _, s := range sources {
		items, cut, err := e.read(ctx, s, sub)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 && !cut {
			continue // nothing stored: no empty file
		}
		name := strings.TrimPrefix(s.table, "poker_") + ".json"
		data, err := json.MarshalIndent(map[string]any{"table": s.table, "truncated": cut, "items": items}, "", "  ")
		if err != nil {
			return nil, err
		}
		w, err := zw.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
		files = append(files, name)
		if cut {
			truncated = append(truncated, name)
		}
	}
	sort.Strings(files)
	w, err := zw.Create("README.txt")
	if err != nil {
		return nil, err
	}
	if _, err := w.Write([]byte(readme(e.now(), files, truncated))); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (e *Exporter) read(ctx context.Context, s source, sub string) ([]map[string]any, bool, error) {
	var out []map[string]any
	queries := []*dynamodb.QueryInput{}
	if s.index != "" {
		queries = append(queries, &dynamodb.QueryInput{
			TableName: aws.String(dynamo.TableName(e.env, s.table)), IndexName: aws.String(s.index),
			KeyConditionExpression:    aws.String("#a = :sub"),
			ExpressionAttributeNames:  map[string]string{"#a": s.attr},
			ExpressionAttributeValues: map[string]types.AttributeValue{":sub": &types.AttributeValueMemberS{Value: sub}},
		})
	} else {
		for _, pk := range s.pks(sub) {
			queries = append(queries, &dynamodb.QueryInput{
				TableName:                 aws.String(dynamo.TableName(e.env, s.table)),
				KeyConditionExpression:    aws.String("pk = :pk"),
				ExpressionAttributeValues: map[string]types.AttributeValue{":pk": &types.AttributeValueMemberS{Value: pk}},
				ConsistentRead:            aws.Bool(true),
			})
		}
	}
	for _, in := range queries {
		for {
			page, err := e.db.Query(ctx, in)
			if err != nil {
				return nil, false, fmt.Errorf("userexport: query %s: %w", s.table, err)
			}
			for _, it := range page.Items {
				if len(out) >= maxItems {
					return out, true, nil
				}
				var m map[string]any
				if err := attributevalue.UnmarshalMap(it, &m); err != nil {
					return nil, false, fmt.Errorf("userexport: decode %s: %w", s.table, err)
				}
				out = append(out, scrub(m, sub).(map[string]any))
			}
			if len(page.LastEvaluatedKey) == 0 {
				break
			}
			in.ExclusiveStartKey = page.LastEvaluatedKey
		}
	}
	return out, false, nil
}

func readme(at time.Time, files, truncated []string) string {
	var b strings.Builder
	b.WriteString("CTech Poker — a copy of your personal data\nGenerated at " + at.UTC().Format(time.RFC3339) + "\n\nFiles (one per table that holds your data):\n")
	for _, f := range files {
		b.WriteString("  " + f + "\n")
	}
	b.WriteString("\nEach file is {table, truncated, items}. Timestamps are epoch milliseconds or RFC3339 as stored.\n")
	b.WriteString("Other players appear only as opaque ids: their names, avatars and chat are not included.\n")
	b.WriteString("Not included: notes others wrote about you, reports, other players' actions at shared tables, and passwords or tokens (poker holds none).\n")
	b.WriteString("Payments are part of the CTech Ledger export.\n")
	if len(truncated) > 0 {
		b.WriteString("\nThese files were cut at " + fmt.Sprint(maxItems) + " items: " + strings.Join(truncated, ", ") + "\n")
	}
	return b.String()
}
