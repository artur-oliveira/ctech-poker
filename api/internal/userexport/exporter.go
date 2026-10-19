// Package userexport builds poker's part of the personal-data export: a zip with
// one JSON file per table that holds the person's own data, plus a README. It
// is the dataexport.PartStreamFunc of the poker service
// (ctech-account docs/specs/2026-10-17-data-export.md).
package userexport

import (
	"archive/zip"
	"cmp"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dataexport"
	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
)

// maxItems bounds one file. A hand history past it is cut and the file says so.
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
	// maxItems overrides the package's maxItems (tests).
	maxItems int
}

func New(db Query, env string) *Exporter { return &Exporter{db: db, env: env, now: time.Now} }

// Write writes the zip for sub to w as it reads, a page of a table at a time, so
// a long hand history costs one page of memory. It is idempotent: a redelivery
// builds it again. It is the dataexport.PartStreamFunc of the poker service.
func (e *Exporter) Write(ctx context.Context, sub string, w io.Writer) error {
	zw := zip.NewWriter(w)
	var truncated, files []string
	for _, s := range sources {
		name := strings.TrimPrefix(s.table, "poker_") + ".json"
		wrote, cut, err := e.write(ctx, zw, name, s, sub)
		if err != nil {
			return err
		}
		if wrote {
			files = append(files, name)
		}
		if cut {
			truncated = append(truncated, name)
		}
	}
	sort.Strings(files)
	rw, err := zw.Create("README.txt")
	if err != nil {
		return err
	}
	if _, err := rw.Write([]byte(readme(e.now(), files, truncated))); err != nil {
		return err
	}
	return zw.Close()
}

// write streams one source into its file, up to maxItems rows over all its
// partitions; nothing stored means no file.
func (e *Exporter) write(ctx context.Context, zw *zip.Writer, name string, s source, sub string) (wrote, cut bool, err error) {
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
	var it *dataexport.Items
	limit := cmp.Or(e.maxItems, maxItems)
	for _, in := range queries {
		for {
			page, err := e.db.Query(ctx, in)
			if err != nil {
				return false, false, fmt.Errorf("userexport: query %s: %w", s.table, err)
			}
			for _, row := range page.Items {
				if it != nil && it.Count() >= limit {
					return true, true, it.End(true)
				}
				var m map[string]any
				if err := attributevalue.UnmarshalMap(row, &m); err != nil {
					return false, false, fmt.Errorf("userexport: decode %s: %w", s.table, err)
				}
				if it == nil {
					if it, err = dataexport.BeginItems(zw, name); err != nil {
						return false, false, err
					}
				}
				if err := it.Add(scrub(m, sub)); err != nil {
					return false, false, err
				}
			}
			if len(page.LastEvaluatedKey) == 0 {
				break
			}
			in.ExclusiveStartKey = page.LastEvaluatedKey
		}
	}
	if it == nil {
		return false, false, nil
	}
	return true, false, it.End(false)
}

func readme(at time.Time, files, truncated []string) string {
	var b strings.Builder
	b.WriteString("CTech Poker — a copy of your personal data\nGenerated at " + at.UTC().Format(time.RFC3339) + "\n\nFiles (one per table that holds your data):\n")
	for _, f := range files {
		b.WriteString("  " + f + "\n")
	}
	b.WriteString("\nEach file is {items, truncated}, named after its table. Timestamps are epoch milliseconds or RFC3339 as stored.\n")
	b.WriteString("Other players appear only as opaque ids: their names, avatars and chat are not included.\n")
	b.WriteString("Not included: notes others wrote about you, reports, other players' actions at shared tables, and passwords or tokens (poker holds none).\n")
	b.WriteString("Payments are part of the CTech Ledger export.\n")
	if len(truncated) > 0 {
		b.WriteString("\nThese files were cut at " + fmt.Sprint(maxItems) + " items: " + strings.Join(truncated, ", ") + "\n")
	}
	return b.String()
}
