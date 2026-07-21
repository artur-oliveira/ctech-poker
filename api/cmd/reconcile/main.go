// Package main implements the scheduled Lambda job that sweeps poker_pending_cashouts
// for cash-outs whose credit to ctech-wallet was interrupted, retrying them safely.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/reconcile"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

const gracePeriod = 2 * time.Minute

type pendingLister interface {
	ListUnresolved(ctx context.Context, olderThan time.Duration) ([]reconcile.PendingCashout, error)
	MarkResolved(ctx context.Context, id string) error
}

type gameCredit interface {
	CashoutGame(ctx context.Context, userID string, amount int64, tableRef string, holdIDs []string, idempotencyKey, reason string) error
}

type sandboxCredit interface {
	Credit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
}

func run(ctx context.Context, pending pendingLister, game gameCredit, sandbox sandboxCredit) error {
	entries, err := pending.ListUnresolved(ctx, gracePeriod)
	if err != nil {
		return fmt.Errorf("reconcile: list unresolved: %w", err)
	}
	for _, e := range entries {
		var creditErr error
		switch e.CurrencyMode {
		case "real":
			tableRef := e.TableRef
			if tableRef == "" {
				tableRef = "unknown"
			}
			creditErr = game.CashoutGame(ctx, e.PlayerID, e.Amount, tableRef, e.HoldIDs, e.IdempotencyKey, "poker_cashout_reconcile")
		default:
			if sandbox != nil {
				creditErr = sandbox.Credit(ctx, e.PlayerID, e.Amount, e.IdempotencyKey, "poker_cashout_reconcile")
			}
		}

		if creditErr != nil {
			slog.Error("ALARM: reconcile credit failed, needs manual review",
				"pending_id", e.ID, "player", e.PlayerID, "amount", e.Amount, "err", creditErr)
			continue
		}
		if err := pending.MarkResolved(ctx, e.ID); err != nil {
			slog.Error("ALARM: reconcile resolved credit but failed to mark pending entry resolved",
				"pending_id", e.ID, "err", err)
		}
	}
	return nil
}

type resolvedParams struct {
	walletURL    string
	clientID     string
	clientSecret string
}

func resolveSSMParams(ctx context.Context, walletURLParam, clientIDParam, clientSecretParam string) (resolvedParams, error) {
	awsCfg, err := awscfg.LoadDefaultConfig(ctx)
	if err != nil {
		return resolvedParams{}, err
	}
	client := ssm.NewFromConfig(awsCfg)
	get := func(name string, withDecryption bool) (string, error) {
		out, err := client.GetParameter(ctx, &ssm.GetParameterInput{Name: aws.String(name), WithDecryption: aws.Bool(withDecryption)})
		if err != nil {
			return "", err
		}
		return *out.Parameter.Value, nil
	}

	wURL, err := get(walletURLParam, false)
	if err != nil {
		return resolvedParams{}, err
	}
	cID, err := get(clientIDParam, false)
	if err != nil {
		return resolvedParams{}, err
	}
	cSecret, err := get(clientSecretParam, true)
	if err != nil {
		return resolvedParams{}, err
	}
	return resolvedParams{walletURL: wURL, clientID: cID, clientSecret: cSecret}, nil
}

func handler(ctx context.Context) error {
	wURLParam := os.Getenv("WALLET_URL_PARAM")
	if wURLParam != "" {
		params, err := resolveSSMParams(ctx, wURLParam, os.Getenv("POKER_CLIENT_ID_PARAM"), os.Getenv("POKER_CLIENT_SECRET_PARAM"))
		if err == nil {
			_ = os.Setenv("WALLET_URL", params.walletURL)
			_ = os.Setenv("POKER_CLIENT_ID", params.clientID)
			_ = os.Setenv("POKER_CLIENT_SECRET", params.clientSecret)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	awsCfg, err := awscfg.LoadDefaultConfig(ctx)
	if err != nil {
		return err
	}
	db := dynamodb.NewFromConfig(awsCfg)
	pendingStore := reconcile.NewPendingStore(db, cfg.Env)
	wallet := walletclient.New(cfg)
	return run(ctx, pendingStore, wallet, wallet)
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	lambda.Start(handler)
}
