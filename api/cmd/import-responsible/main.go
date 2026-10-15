// Command import-responsible copies the responsible-gambling state (consent, personal
// limits, self-exclusion, carried exclusions) from ctech-wallet's users table into Poker, once.
// Run by an operator through scripts/import-responsible.sh with their own AWS profile. Safe to
// repeat: a player already present is left alone.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"gopkg.aoctech.app/poker/api/internal/responsible"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	dry := flag.Bool("dry-run", false, "count what would be imported and write nothing")
	flag.Parse()

	ctx := context.Background()
	env := os.Getenv("ENVIRONMENT")
	if env == "" {
		slog.Error("ENVIRONMENT is required (use scripts/import-responsible.sh)")
		os.Exit(2)
	}
	awsCfg, err := awscfg.LoadDefaultConfig(ctx)
	if err != nil {
		slog.Error("aws config", "err", err)
		os.Exit(1)
	}
	db := dynamodb.NewFromConfig(awsCfg)
	store := responsible.NewDynamoStore(db, env)
	// The wallet's API role may not Scan, by design, so the one-off reads its users table with
	// the operator's own credentials.
	wallet := responsible.WalletTable{DB: db, Table: env + "_wallet_users"}

	counts, err := responsible.Import(ctx, wallet, store, *dry, time.Now())
	slog.Info("import-responsible", "dry_run", *dry, "users", counts.Users, "imported", counts.Imported,
		"already_there", counts.AlreadyThere, "carried_exclusions", counts.Carried)
	if err != nil {
		slog.Error("import-responsible failed", "err", err)
		os.Exit(1)
	}
}
