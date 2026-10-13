package userpurge

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/erasure"
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

// steps is the purge in order. Tasks 6 and 7 add to it.
func (r *run) steps() []func(context.Context) error {
	return nil
}
