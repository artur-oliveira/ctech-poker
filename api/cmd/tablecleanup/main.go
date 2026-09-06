// Package main implements the scheduled Lambda job that archives poker
// tables idle past staleCutoff, refunding any seated players' sandbox chips
// first. Mirrors cmd/reconcile's shape (scheduled Lambda, SSM-resolved
// wallet credentials).
package main

import (
	"context"
	"fmt"
	"log"
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
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

// staleCutoff is how long a table may sit with no committed action before
// this job archives it. cmd/reconcile's analogous gracePeriod is 2 minutes
// (for a completed cash-out awaiting credit); a table being idle mid-session
// is a much slower signal;
const staleCutoff = 15 * time.Minute

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
	Delete(ctx context.Context, roomID string) error
}

type sandboxCredit interface {
	Credit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
}

// gameCashout settles a seated player's real-money stack against the
// ring-fenced game wallet, releasing the buy-in hold(s) that back it.
// Mirrors buyin.Service's walletMover.CashoutGame subset.
type gameCashout interface {
	CashoutGame(ctx context.Context, userID string, amount int64, tableRef string, holdIDs []string, idempotencyKey, reason string) error
}

// pendingRecorder is the subset of *reconcile.PendingStore this job needs to
// durably record a real-money settlement obligation before attempting it —
// same "record, then attempt, then resolve" shape as buyin.Service.settle,
// so a CashoutGame failure here is retried by cmd/reconcile instead of
// stranding the hold forever.
type pendingRecorder interface {
	Record(ctx context.Context, p reconcile.PendingCashout) error
	MarkResolved(ctx context.Context, id string) error
}

// timeNowFunc is overridden in tests that need a deterministic cutoff.
var timeNowFunc = time.Now

func run(ctx context.Context, stale staleQuerier, rooms roomLookup, wallet sandboxCredit, game gameCashout, pending pendingRecorder, cutoff time.Duration) error {
	olderThan := timeNowFunc().Add(-cutoff).Unix()
	tables, err := stale.QueryStaleActive(ctx, olderThan, queryBatchLimit)
	if err != nil {
		return fmt.Errorf("tablecleanup: query stale: %w", err)
	}
	slog.Info("tablecleanup: sweep complete", "stale_found", len(tables))

	for _, st := range tables {
		room, err := rooms.Get(ctx, st.TableID)
		if err != nil {
			slog.Error("tablecleanup: room lookup failed, skipping this pass", "table_id", st.TableID, "err", err)
			continue
		}
		// A missing room record is "unknown", never "assume sandbox": this
		// job must never credit a table it can't positively identify as
		// sandbox to the sandbox ledger (the currency_mode boundary is
		// load-bearing per api/CLAUDE.md). Skip and let a later pass, once
		// the room lookup succeeds (or the table itself expires/gets
		// archived by other means), resolve it.
		if room == nil {
			slog.Warn("tablecleanup: no room record for stale table, skipping (currency mode unknown)", "table_id", st.TableID)
			continue
		}

		switch room.CurrencyMode {
		case roomstore.CurrencyModeSandbox:
			if !refundSandboxAndArchive(ctx, stale, rooms, wallet, st) {
				continue
			}
		case roomstore.CurrencyModeReal:
			if !settleRealMoneyAndArchive(ctx, stale, rooms, game, pending, st) {
				continue
			}
		default:
			slog.Warn("tablecleanup: unrecognized currency_mode, skipping", "table_id", st.TableID, "currency_mode", room.CurrencyMode)
			continue
		}
	}
	return nil
}

// refundSandboxAndArchive refunds every seated sandbox player's stack, then
// archives the table and deletes its room. Returns false (and leaves the
// table active for the next sweep) if any refund or the archive itself
// fails — a sandbox refund has no durable recovery record of its own, so an
// archived table with a failed refund would strand those chips forever.
func refundSandboxAndArchive(ctx context.Context, stale staleQuerier, rooms roomLookup, wallet sandboxCredit, st tablestore.StoredTable) bool {
	refundFailed := false
	for _, p := range st.State.Players {
		if p.Stack <= 0 {
			continue
		}
		key := fmt.Sprintf("%s#%s#stale_archive_refund", st.TableID, p.ID)
		if err := wallet.Credit(ctx, p.ID, p.Stack, key, "poker_stale_table_refund"); err != nil {
			slog.Error("ALARM: tablecleanup refund failed, table left active for retry", "table_id", st.TableID, "player", p.ID, "amount", p.Stack, "err", err)
			refundFailed = true
			continue
		}
	}
	if refundFailed {
		slog.Error("tablecleanup: skipping archive for table with failed player refund(s)", "table_id", st.TableID)
		return false
	}
	return archiveTableAndRoom(ctx, stale, rooms, st)
}

// settleRealMoneyAndArchive releases every seated real-money player's
// game-wallet hold (crediting their final stack) before the table is
// archived. Each settlement is first recorded to poker_pending_cashouts —
// the same record-then-attempt-then-resolve shape buyin.Service.settle uses
// — so an immediate CashoutGame failure is not lost: cmd/reconcile's sweep
// retries it from the durable row, using the recorded hold IDs, until it
// resolves. Because the obligation is durable the moment Record succeeds,
// the table is safe to archive even if CashoutGame itself fails right now;
// only a Record failure (no durable obligation exists yet) blocks the
// archive, leaving the table active for the next sweep to retry.
//
// The table-entry entitlement (internal/entitlement) is deliberately left
// untouched here: it is a paid, non-refundable 3-hour reservation, not a
// fund hold, so there is nothing to "release" — it simply expires on its
// own TTL. See docs/plans/2026-08-21-entry-fee-entitlement.md.
func settleRealMoneyAndArchive(ctx context.Context, stale staleQuerier, rooms roomLookup, game gameCashout, pending pendingRecorder, st tablestore.StoredTable) bool {
	recordFailed := false
	for _, p := range st.State.Players {
		if p.Stack <= 0 {
			continue
		}
		key := fmt.Sprintf("%s#%s#stale_archive_cashout", st.TableID, p.ID)
		var holdIDs []string
		if p.HoldID != "" {
			holdIDs = []string{p.HoldID}
		}
		if err := pending.Record(ctx, reconcile.PendingCashout{
			ID: key, PlayerID: p.ID, Amount: p.Stack, CurrencyMode: roomstore.CurrencyModeReal,
			HoldIDs: holdIDs, TableRef: st.TableID, IdempotencyKey: key,
		}); err != nil {
			slog.Error("ALARM: tablecleanup real-money settlement record failed, table left active for retry", "table_id", st.TableID, "player", p.ID, "amount", p.Stack, "err", err)
			recordFailed = true
			continue
		}
		if err := game.CashoutGame(ctx, p.ID, p.Stack, st.TableID, holdIDs, key, "poker_stale_table_refund"); err != nil {
			slog.Error("tablecleanup: real-money cash-out failed after seat sweep, reconcile sweep will retry", "table_id", st.TableID, "player", p.ID, "amount", p.Stack, "hold_ids", holdIDs, "err", err)
			continue
		}
		if err := pending.MarkResolved(ctx, key); err != nil {
			slog.Error("tablecleanup: real-money cash-out succeeded but recovery row finalization failed", "table_id", st.TableID, "player", p.ID, "err", err)
		}
	}
	if recordFailed {
		slog.Error("tablecleanup: skipping archive for real-money table with failed recovery-record write(s)", "table_id", st.TableID)
		return false
	}
	return archiveTableAndRoom(ctx, stale, rooms, st)
}

// archiveTableAndRoom is the shared tail of both settlement paths: mark the
// table archived, then delete its room only once the archive is confirmed —
// a mid-sweep crash must never drop a room while its table is still
// live/joinable.
func archiveTableAndRoom(ctx context.Context, stale staleQuerier, rooms roomLookup, st tablestore.StoredTable) bool {
	if err := stale.MarkArchived(ctx, st.TableID, st.Version); err != nil {
		slog.Error("tablecleanup: archive failed (table may have just received a fresh action; skipping)", "table_id", st.TableID, "err", err)
		return false
	}
	if err := rooms.Delete(ctx, st.TableID); err != nil {
		slog.Error("tablecleanup: room delete failed, room will linger in lobby listing", "table_id", st.TableID, "err", err)
	}
	slog.Info("tablecleanup: archived stale table", "table_id", st.TableID, "seats_settled", len(st.State.Players))
	return true
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
	if err := os.Setenv("WALLET_URL", wURL); err != nil {
		return fmt.Errorf("set WALLET_URL: %w", err)
	}
	if err := os.Setenv("POKER_CLIENT_ID", cID); err != nil {
		return fmt.Errorf("set POKER_CLIENT_ID: %w", err)
	}
	if err := os.Setenv("POKER_CLIENT_SECRET", cSecret); err != nil {
		return fmt.Errorf("set POKER_CLIENT_SECRET: %w", err)
	}
	return nil
}

func handler(ctx context.Context) error {
	cfg, err := config.LoadForLambda()
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
	wallet := walletclient.New(cfg, cache.NewMemoryBackend(10))
	pendingStore := reconcile.NewPendingStore(db, cfg.Env)
	return run(ctx, store, rooms, wallet, wallet, pendingStore, staleCutoff)
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	// Resolve (and decrypt) the wallet SSM params once here, at cold start,
	// instead of on every invocation inside handler: this Lambda runs on a
	// rate(30 minutes) schedule and the warm execution environment is reused
	// across invocations, so re-fetching an unchanging SecureString on every
	// one of them was ~48 unnecessary KMS Decrypt calls/day. A cold start
	// still resolves it exactly once, and resolving here (rather than lazily
	// on first invocation) fails the Lambda fast and loudly if the parameter
	// is missing/misconfigured.
	if wURLParam := os.Getenv("WALLET_URL_PARAM"); wURLParam != "" {
		if err := resolveSSMParams(context.Background(), wURLParam, os.Getenv("POKER_CLIENT_ID_PARAM"), os.Getenv("POKER_CLIENT_SECRET_PARAM")); err != nil {
			log.Fatalf("tablecleanup: resolve SSM params: %v", err)
		}
	}

	lambda.Start(handler)
}
