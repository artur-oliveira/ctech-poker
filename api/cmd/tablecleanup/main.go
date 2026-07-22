// Package main implements the scheduled Lambda job that archives poker
// tables idle past staleCutoff, refunding any seated players' sandbox chips
// first. Mirrors cmd/reconcile's shape (scheduled Lambda, SSM-resolved
// wallet credentials).
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
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

// staleCutoff is how long a table may sit with no committed action before
// this job archives it. cmd/reconcile's analogous gracePeriod is 2 minutes
// (for a completed cash-out awaiting credit); a table being idle mid-session
// is a much slower signal, so this is measured in hours, not minutes.
const staleCutoff = 6 * time.Hour

// queryBatchLimit bounds how many stale tables one invocation processes.
// Any remainder is picked up on the next scheduled run since last_action_at
// does not change for a still-stale table between runs.
const queryBatchLimit = 25

type staleQuerier interface {
	QueryStaleActive(ctx context.Context, olderThanUnix int64, limit int) ([]tablestore.StoredTable, error)
	MarkArchived(ctx context.Context, tableID string, expectedVersion int) error
}

type roomLookup interface {
	Get(ctx context.Context, roomID string) (*roomstore.Room, error)
}

type sandboxCredit interface {
	Credit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
}

// timeNowFunc is overridden in tests that need a deterministic cutoff.
var timeNowFunc = time.Now

func run(ctx context.Context, stale staleQuerier, rooms roomLookup, wallet sandboxCredit, cutoff time.Duration) error {
	olderThan := timeNowFunc().Add(-cutoff).Unix()
	tables, err := stale.QueryStaleActive(ctx, olderThan, queryBatchLimit)
	if err != nil {
		return fmt.Errorf("tablecleanup: query stale: %w", err)
	}

	for _, st := range tables {
		room, err := rooms.Get(ctx, st.TableID)
		if err != nil {
			slog.Error("tablecleanup: room lookup failed, skipping this pass", "table_id", st.TableID, "err", err)
			continue
		}
		// Sandbox isolation is load-bearing (api/CLAUDE.md): this job never
		// touches a real-money table. A missing room record is treated the
		// same as sandbox, since every table this codebase creates today is
		// sandbox-only end-to-end.
		if room != nil && room.CurrencyMode != "sandbox" {
			continue
		}

		for _, p := range st.State.Players {
			if p.Stack <= 0 {
				continue
			}
			key := fmt.Sprintf("%s#%s#stale_archive_refund", st.TableID, p.ID)
			if err := wallet.Credit(ctx, p.ID, p.Stack, key, "poker_stale_table_refund"); err != nil {
				slog.Error("ALARM: tablecleanup refund failed, table left active for retry", "table_id", st.TableID, "player", p.ID, "amount", p.Stack, "err", err)
				continue
			}
		}

		if err := stale.MarkArchived(ctx, st.TableID, st.Version); err != nil {
			slog.Error("tablecleanup: archive failed (table may have just received a fresh action; skipping)", "table_id", st.TableID, "err", err)
			continue
		}
		slog.Info("tablecleanup: archived stale table", "table_id", st.TableID, "seats_refunded", len(st.State.Players))
	}
	return nil
}

func resolveSSMParams(ctx context.Context, walletURLParam, clientIDParam, clientSecretParam string) error {
	awsCfg, err := awscfg.LoadDefaultConfig(ctx)
	if err != nil {
		return err
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
		return err
	}
	cID, err := get(clientIDParam, false)
	if err != nil {
		return err
	}
	cSecret, err := get(clientSecretParam, true)
	if err != nil {
		return err
	}
	_ = os.Setenv("WALLET_URL", wURL)
	_ = os.Setenv("POKER_CLIENT_ID", cID)
	_ = os.Setenv("POKER_CLIENT_SECRET", cSecret)
	return nil
}

func handler(ctx context.Context) error {
	if wURLParam := os.Getenv("WALLET_URL_PARAM"); wURLParam != "" {
		if err := resolveSSMParams(ctx, wURLParam, os.Getenv("POKER_CLIENT_ID_PARAM"), os.Getenv("POKER_CLIENT_SECRET_PARAM")); err != nil {
			return fmt.Errorf("tablecleanup: resolve SSM params: %w", err)
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
	store := tablestore.NewStore(db, cfg.Env)
	rooms := roomstore.NewStore(db, cfg.Env)
	wallet := walletclient.New(cfg)
	return run(ctx, store, rooms, wallet, staleCutoff)
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	lambda.Start(handler)
}
