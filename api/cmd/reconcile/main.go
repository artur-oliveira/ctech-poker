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
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/reconcile"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

const gracePeriod = 2 * time.Minute

var timeNow = time.Now

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

type feeDebiter interface {
	DebitReal(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
}

func run(ctx context.Context, pending pendingLister, game gameCredit, sandbox sandboxCredit, fee feeDebiter) error {
	entries, err := pending.ListUnresolved(ctx, gracePeriod)
	if err != nil {
		return fmt.Errorf("reconcile: list unresolved: %w", err)
	}
	logPendingCashouts(entries)
	for _, e := range entries {
		var opErr error
		switch e.Kind {
		case reconcile.KindFeeDebit:
			opErr = fee.DebitReal(ctx, e.PlayerID, e.Amount, e.IdempotencyKey, "poker_table_fee_reconcile")
		default:
			switch e.CurrencyMode {
			case "real":
				tableRef := e.TableRef
				if tableRef == "" {
					tableRef = "unknown"
				}
				opErr = game.CashoutGame(ctx, e.PlayerID, e.Amount, tableRef, e.HoldIDs, e.IdempotencyKey, "poker_cashout_reconcile")
			default:
				if sandbox != nil {
					opErr = sandbox.Credit(ctx, e.PlayerID, e.Amount, e.IdempotencyKey, "poker_cashout_reconcile")
				}
			}
		}

		if opErr != nil {
			slog.Error("ALARM: reconcile operation failed, needs manual review",
				"pending_id", e.ID, "kind", e.Kind, "player", e.PlayerID, "amount", e.Amount, "err", opErr)
			continue
		}
		if err := pending.MarkResolved(ctx, e.ID); err != nil {
			slog.Error("ALARM: reconcile resolved operation but failed to mark pending entry resolved",
				"pending_id", e.ID, "err", err)
		}
	}
	return nil
}

// logPendingCashouts replaces the PendingCashouts / OldestPendingCashoutAgeSeconds
// EMF metrics (removed 2026-08-19 along with every other custom metric). The
// backlog and its oldest entry are the money-in-limbo signal, so they stay
// visible — as a log line in /ctech-poker/<env>/app, which costs nothing extra.
func logPendingCashouts(entries []reconcile.PendingCashout) {
	var oldestSeconds float64
	now := timeNow()
	for _, entry := range entries {
		recordedAt, err := time.Parse(time.RFC3339Nano, entry.RecordedAt)
		if err != nil {
			continue
		}
		if age := now.Sub(recordedAt).Seconds(); age > oldestSeconds {
			oldestSeconds = age
		}
	}
	slog.Info("reconcile pending cashouts",
		"pending_cashouts", len(entries),
		"oldest_pending_cashout_age_seconds", oldestSeconds)
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

	cfg, err := config.LoadForLambda()
	if err != nil {
		return err
	}
	awsCfg, err := awscfg.LoadDefaultConfig(ctx)
	if err != nil {
		return err
	}
	db := dynamodb.NewFromConfig(awsCfg)
	pendingStore := reconcile.NewPendingStore(db, cfg.Env)
	wallet := walletclient.New(cfg, cache.NewMemoryBackend(10))
	return run(ctx, pendingStore, wallet, wallet, wallet)
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	lambda.Start(handler)
}
