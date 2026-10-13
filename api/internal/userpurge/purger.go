package userpurge

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/erasure"
	"gopkg.aoctech.app/poker/api/internal/matchup"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
)

// reportRetention bounds how long an anonymized report outlives the erasure
// (anti-abuse; plan ruling R9).
const reportRetention = 365 * 24 * time.Hour

// Config names what the purge touches. An empty bucket skips that step (dev).
type Config struct {
	Env           string
	AvatarBucket  string
	ArchiveBucket string
}

// BlockersFunc lists what stops a user's erasure right now (Eligibility.Blockers).
type BlockersFunc func(ctx context.Context, sub string) ([]erasure.Blocker, error)

// Purger is poker's erasure.PurgeFunc. Every step is idempotent and re-reads
// from scratch, so a crash or a redelivered message resumes. Rows that drive
// discovery (the user's own hand rows, their social edges) are deleted only
// after what they point to.
type Purger struct {
	db       *dynamodb.Client
	objects  S3API
	cache    cache.Backend
	cfg      Config
	blockers BlockersFunc
	now      func() time.Time
}

func NewPurger(db *dynamodb.Client, objects S3API, c cache.Backend, cfg Config, blockers BlockersFunc) *Purger {
	return &Purger{db: db, objects: objects, cache: c, cfg: cfg, blockers: blockers, now: time.Now}
}

// run is one Purge invocation. pseudo is never persisted next to sub.
type run struct {
	p      *Purger
	sub    string
	pseudo string
	counts map[string]int
	tables map[string]bool // table ids whose table-keyed rows this run already anonymized
}

func (r *run) count(key string, n int) {
	if n > 0 {
		r.counts[key] += n
	}
}

// Purge erases or anonymizes m.Sub (data inventory §7). It re-checks the
// blockers first and never half-purges a blocked user. m.Organizations is
// ignored: poker holds no organization-scoped data.
func (p *Purger) Purge(ctx context.Context, m erasure.Message) (erasure.Ack, error) {
	blockers, err := p.blockers(ctx, m.Sub)
	if err != nil {
		return erasure.Ack{}, fmt.Errorf("userpurge: eligibility: %w", err)
	}
	if len(blockers) > 0 {
		return erasure.Ack{Result: erasure.ResultBlocked, Blockers: blockers}, nil
	}
	r := &run{p: p, sub: m.Sub, pseudo: newPseudonym(), counts: map[string]int{}, tables: map[string]bool{}}
	for _, step := range r.steps() {
		if err := step(ctx); err != nil {
			return erasure.Ack{}, err
		}
	}
	return erasure.Ack{Result: erasure.ResultDone, Counts: r.counts}, nil
}

// steps is the purge in order.
func (r *run) steps() []func(context.Context) error {
	return []func(context.Context) error{r.sharedHands}
}

// sharedHands walks the user's own hand rows, the only index of the hands
// they played. Per hand, it anonymizes everything others keep about it and
// then deletes the user's row, so a retry resumes at the first hand left.
func (r *run) sharedHands(ctx context.Context) error {
	return r.p.eachItem(ctx, r.p.partition(TablePlayerHands, r.sub), func(it item) error {
		var h sessionlog.HandItem
		if err := attributevalue.UnmarshalMap(it, &h); err != nil {
			return fmt.Errorf("userpurge: decode hand %s: %w", stringAttr(it, "sk"), err)
		}
		if err := r.tableRows(ctx, h.TableID); err != nil {
			return err
		}
		if err := r.handRows(ctx, h); err != nil {
			return err
		}
		return r.deleteItem(ctx, TablePlayerHands, keyOf(it))
	})
}

// tableRows anonymizes the table-keyed rows (state snapshots, daily
// highlights) once per table per run.
func (r *run) tableRows(ctx context.Context, tableID string) error {
	if tableID == "" || r.tables[tableID] {
		return nil
	}
	for _, table := range []string{TableTableStateHistory, TableTableHighlights} {
		if err := r.p.eachItem(ctx, r.p.partition(table, tableID), func(it item) error {
			return r.rewrite(ctx, table, it)
		}); err != nil {
			return err
		}
	}
	r.tables[tableID] = true
	return nil
}

// handRows anonymizes one hand everywhere other players see it.
func (r *run) handRows(ctx context.Context, h sessionlog.HandItem) error {
	if err := r.p.eachItem(ctx, r.p.partition(TableActionLog, h.TableID+"#"+h.HandID), func(it item) error {
		return r.rewrite(ctx, TableActionLog, it)
	}); err != nil {
		return err
	}
	if err := r.archive(ctx, h.TableID, h.HandID); err != nil {
		return err
	}
	if err := r.rewriteKey(ctx, TableHandReveals, key(h.HandID, "")); err != nil {
		return err
	}
	if err := r.deleteItem(ctx, TableHandRevealPayments, key(h.HandID+"#"+r.sub, "")); err != nil {
		return err
	}
	for _, o := range h.Opponents {
		if o.PlayerID == "" || o.PlayerID == r.sub {
			continue
		}
		if err := r.rewriteKey(ctx, TablePlayerHands, key(o.PlayerID, h.SK)); err != nil {
			return err
		}
		if err := r.rewriteKey(ctx, TableRecentPlayers, key(o.PlayerID, "hand#"+h.HandID)); err != nil {
			return err
		}
		if err := r.deleteItem(ctx, TableMatchups, key(matchup.PairKey(h.CurrencyMode, r.sub, o.PlayerID), "")); err != nil {
			return err
		}
	}
	return nil
}
