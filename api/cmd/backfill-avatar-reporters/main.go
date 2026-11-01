// Command backfill-avatar-reporters strips the legacy avatar_reporters String Set from every
// player profile. #220 stopped writing it (ReportAvatar REMOVEs it as it goes), but a profile
// nobody reported since still carries the reporters' ids, and the account-deletion purge cannot
// find them: that takes a Scan, which the service role is denied. Run by an operator through
// scripts/backfill-avatar-reporters.sh with their own AWS profile. Dry run unless -apply;
// safe to repeat.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"gopkg.aoctech.app/api-commons/dynamo"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	apply := flag.Bool("apply", false, "write; without it only count")
	flag.Parse()

	env := os.Getenv("ENVIRONMENT")
	if env == "" {
		slog.Error("ENVIRONMENT is required (use scripts/backfill-avatar-reporters.sh)")
		os.Exit(2)
	}
	ctx := context.Background()
	awsCfg, err := awscfg.LoadDefaultConfig(ctx)
	if err != nil {
		slog.Error("aws config", "err", err)
		os.Exit(1)
	}
	db := dynamodb.NewFromConfig(awsCfg)
	table := aws.String(dynamo.TableName(env, "poker_player_profiles"))

	var found, removed int
	var start map[string]types.AttributeValue
	for {
		out, err := db.Scan(ctx, &dynamodb.ScanInput{
			TableName:            table,
			FilterExpression:     aws.String("attribute_exists(avatar_reporters)"),
			ProjectionExpression: aws.String("pk"),
			ExclusiveStartKey:    start,
		})
		if err != nil {
			slog.Error("scan", "err", err)
			os.Exit(1)
		}
		for _, it := range out.Items {
			found++
			if !*apply {
				continue
			}
			if _, err := db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
				TableName:           table,
				Key:                 map[string]types.AttributeValue{"pk": it["pk"]},
				UpdateExpression:    aws.String("REMOVE avatar_reporters"),
				ConditionExpression: aws.String("attribute_exists(pk)"),
			}); err != nil {
				slog.Error("remove avatar_reporters", "err", err)
				os.Exit(1)
			}
			removed++
		}
		if len(out.LastEvaluatedKey) == 0 {
			break
		}
		start = out.LastEvaluatedKey
	}
	slog.Info("backfill-avatar-reporters", "apply", *apply, "profiles_with_set", found, "removed", removed)
}
