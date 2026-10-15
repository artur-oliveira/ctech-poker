// Command import-responsible copies the responsible-gambling state (consent, personal
// limits, self-exclusion, carried exclusions) from ctech-wallet into Poker, once. Run by
// an operator through scripts/import-responsible.sh, which reads the M2M credentials
// from SSM. Safe to repeat: a player already present is left alone.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/api-commons/cache"

	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/responsible"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	dry := flag.Bool("dry-run", false, "count what would be imported and write nothing")
	flag.Parse()

	ctx := context.Background()
	cfg := &config.Config{
		Env:               os.Getenv("ENVIRONMENT"),
		CtechURL:          os.Getenv("CTECH_URL"),
		WalletURL:         os.Getenv("WALLET_URL"),
		PokerClientID:     os.Getenv("POKER_CLIENT_ID"),
		PokerClientSecret: os.Getenv("POKER_CLIENT_SECRET"),
	}
	if cfg.Env == "" || cfg.CtechURL == "" || cfg.WalletURL == "" || cfg.PokerClientID == "" || cfg.PokerClientSecret == "" {
		slog.Error("ENVIRONMENT, CTECH_URL, WALLET_URL, POKER_CLIENT_ID and POKER_CLIENT_SECRET are required (use scripts/import-responsible.sh)")
		os.Exit(2)
	}
	awsCfg, err := awscfg.LoadDefaultConfig(ctx)
	if err != nil {
		slog.Error("aws config", "err", err)
		os.Exit(1)
	}
	store := responsible.NewDynamoStore(dynamodb.NewFromConfig(awsCfg), cfg.Env)
	wallet := walletclient.New(cfg, cache.NewMemoryBackend(10))

	counts, err := responsible.Import(ctx, wallet, store, *dry, time.Now())
	slog.Info("import-responsible", "dry_run", *dry, "users", counts.Users, "imported", counts.Imported,
		"already_there", counts.AlreadyThere, "carried_exclusions", counts.Carried)
	if err != nil {
		slog.Error("import-responsible failed", "err", err)
		os.Exit(1)
	}
}
