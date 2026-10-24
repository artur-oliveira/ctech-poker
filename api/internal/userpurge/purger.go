package userpurge

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/erasure"
	"gopkg.aoctech.app/poker/api/internal/matchup"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
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
	carry    CarryFunc
	now      func() time.Time
}

// CarryFunc keeps an active self-exclusion past the erasure (see responsible.Service.CarryOnErase).
type CarryFunc func(ctx context.Context, sub string) (carried bool, err error)

// WithCarrier makes the purge carry self-exclusions before it erases them.
func (p *Purger) WithCarrier(carry CarryFunc) *Purger { p.carry = carry; return p }

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
	return []func(context.Context) error{
		r.carryExclusion, r.sharedHands, r.socialGraph, r.notesAboutUser, r.avatarReports,
		r.reports, r.leaderboard, r.ownRows, r.cacheKeys, r.avatars,
	}
}

// carryExclusion keeps an active self-exclusion past the account, before any state is
// erased: if this step fails the purge stops and a retry carries it again.
func (r *run) carryExclusion(ctx context.Context) error {
	if r.p.carry == nil {
		return nil
	}
	carried, err := r.p.carry(ctx, r.sub)
	if err != nil {
		return fmt.Errorf("userpurge: carry self-exclusion: %w", err)
	}
	if carried {
		r.count(CountSelfExclusionCarried, 1)
	}
	return nil
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

// socialGraph erases both directions of every edge, and first the events and
// invite guards the user caused in each counterpart's inbox (found only
// through the edge, which is why the edge goes last).
func (r *run) socialGraph(ctx context.Context) error {
	return r.p.eachItem(ctx, r.p.partition(TableSocialEdges, r.sub), func(edge item) error {
		other := stringAttr(edge, "sk")
		if err := r.actorEvents(ctx, other); err != nil {
			return err
		}
		if err := r.deleteItem(ctx, TableSocialEdges, key(other, r.sub)); err != nil {
			return err
		}
		return r.deleteItem(ctx, TableSocialEdges, keyOf(edge))
	})
}

func (r *run) actorEvents(ctx context.Context, recipient string) error {
	// sk is a key attribute, so the invite guards need a key condition; the
	// events the user caused are found by a filter on the rest.
	byActor := r.p.partition(TableSocialEvents, recipient)
	byActor.FilterExpression = aws.String("actor_id = :sub")
	byActor.ExpressionAttributeValues[":sub"] = str(r.sub)
	guards := r.p.partition(TableSocialEvents, recipient)
	guards.KeyConditionExpression = aws.String("pk = :pk AND begins_with(sk, :guard)")
	guards.ExpressionAttributeValues[":guard"] = str("invite_guard#" + r.sub + "#")
	var keys []item
	for _, in := range []*dynamodb.QueryInput{byActor, guards} {
		if err := r.p.eachItem(ctx, in, func(it item) error {
			keys = append(keys, keyOf(it))
			return nil
		}); err != nil {
			return err
		}
	}
	r.count(TableSocialEvents+"_erased", len(keys))
	return r.p.deleteKeys(ctx, TableSocialEvents, keys)
}

// notesAboutUser erases notes others wrote about the user (sk = sub, no index).
func (r *run) notesAboutUser(ctx context.Context) error {
	var keys []item
	if err := r.p.eachScan(ctx, &dynamodb.ScanInput{ // standards:allow user purge scan
		TableName:                 aws.String(r.p.table(TablePlayerNotes)),
		FilterExpression:          aws.String("sk = :sub"),
		ExpressionAttributeValues: item{":sub": str(r.sub)},
		ConsistentRead:            aws.Bool(true),
	}, func(it item) error {
		keys = append(keys, keyOf(it))
		return nil
	}); err != nil {
		return err
	}
	r.count(TablePlayerNotes+"_erased", len(keys))
	return r.p.deleteKeys(ctx, TablePlayerNotes, keys)
}

// avatarReports erases the avatar-report guard rows by or about the user and
// removes the user from the legacy avatar_reporters sets (no index: scan).
func (r *run) avatarReports(ctx context.Context) error {
	return r.p.eachScan(ctx, &dynamodb.ScanInput{ // standards:allow user purge scan
		TableName:                 aws.String(r.p.table(TablePlayerProfiles)),
		FilterExpression:          aws.String("(begins_with(pk, :guard) AND (target_id = :sub OR reporter_id = :sub)) OR contains(avatar_reporters, :sub)"),
		ExpressionAttributeValues: item{":guard": str("avreport#"), ":sub": str(r.sub)},
		ConsistentRead:            aws.Bool(true),
	}, func(it item) error {
		if strings.HasPrefix(stringAttr(it, "pk"), "avreport#") {
			return r.deleteItem(ctx, TablePlayerProfiles, keyOf(it))
		}
		if _, err := r.p.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
			TableName:                 aws.String(r.p.table(TablePlayerProfiles)),
			Key:                       keyOf(it),
			UpdateExpression:          aws.String("DELETE avatar_reporters :sub"),
			ExpressionAttributeValues: item{":sub": &types.AttributeValueMemberSS{Value: []string{r.sub}}},
		}); err != nil {
			return fmt.Errorf("userpurge: avatar_reporters: %w", err)
		}
		r.count(TablePlayerProfiles+"_anonymized", 1)
		return nil
	})
}

// reports keeps reports for 1 year under the pseudonym (plan ruling R9) and
// drops the text the erased user wrote: their copied chat on reports against
// them, their details on reports they filed.
func (r *run) reports(ctx context.Context) error {
	expires := r.p.now().Add(reportRetention).Unix()
	against := func(it item) error { return r.retainReport(ctx, it, "evidence_message", expires) }
	if err := r.p.eachItem(ctx, r.p.partition(TablePlayerReports, r.sub), against); err != nil {
		return err
	}
	return r.p.eachItem(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(r.p.table(TablePlayerReports)),
		IndexName:                 aws.String(gsiReportReporter),
		KeyConditionExpression:    aws.String("gsi_reporter_pk = :sub"),
		ExpressionAttributeValues: item{":sub": str(r.sub)},
	}, func(it item) error {
		// The index lags the table: re-read, so a retry cannot re-key twice.
		cur, err := r.getItem(ctx, TablePlayerReports, keyOf(it))
		if err != nil || len(cur) == 0 {
			return err
		}
		return r.retainReport(ctx, cur, "details", expires)
	})
}

func (r *run) retainReport(ctx context.Context, it item, dropText string, expires int64) error {
	next, _ := anonymizeMap(it, r.sub, r.pseudo)
	delete(next, dropText)
	if cur, ok := next["ttl"].(*types.AttributeValueMemberN); !ok || mustInt(cur.Value) > expires {
		next["ttl"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(expires, 10)}
	}
	return r.rekey(ctx, TablePlayerReports, it, next)
}

func mustInt(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 1<<63 - 1
	}
	return n
}

// leaderboard re-keys the user's board rows to the pseudonym so other
// players' ranks do not move, and drops the display name.
func (r *run) leaderboard(ctx context.Context) error {
	return r.p.eachItem(ctx, r.p.partition(TableLeaderboardStats, r.sub), func(it item) error {
		next, _ := anonymizeMap(it, r.sub, r.pseudo)
		delete(next, "player_name")
		return r.rekey(ctx, TableLeaderboardStats, it, next)
	})
}

// erasedBySub are the tables partitioned by the user's sub: the whole
// partition is the user's own data.
var erasedBySub = []string{
	TableAchievementProgress, TableDailyReward, TablePlayerSessions, TablePlayerNotes, TableHandMeta,
	TableSandboxPurchases, TableReactionEntitlements, TableReactionPurchases, TableCosmeticEntitlements,
	TableCosmeticPurchases, TableCosmeticLoadouts, TableChatPrefs, TableBotCheckContests, TableResponsible,
	TableWalletAlertPrefs, TablePromoRedemptions, TableTableEntitlements, TableSocialEvents,
	TableRecentPlayers, TablePlayerHands,
}

func (r *run) ownRows(ctx context.Context) error {
	for _, table := range erasedBySub {
		if err := r.erasePartition(ctx, table, r.sub); err != nil {
			return err
		}
	}
	for _, part := range []struct{ table, pk string }{
		{TablePlayerSessions, "buyinguard#" + r.sub},
		{TablePokerStats, "stats#" + roomstore.CurrencyModeSandbox + "#" + r.sub},
		{TablePokerStats, "stats#" + roomstore.CurrencyModeReal + "#" + r.sub},
	} {
		if err := r.erasePartition(ctx, part.table, part.pk); err != nil {
			return err
		}
	}
	if err := r.ownByIndex(ctx, TableHandShares, gsiHandShareOwner, "owner_id", false); err != nil {
		return err
	}
	// Unresolved settlements are a blocker; resolvedOnly makes sure a money
	// row is never deleted even if one raced the eligibility check.
	if err := r.ownByIndex(ctx, TablePendingCashouts, gsiPlayerSettlements, "player_id", true); err != nil {
		return err
	}
	// The profile goes last: nothing above reads it, so any earlier crash
	// leaves a row a retry still finds.
	return r.erasePartition(ctx, TablePlayerProfiles, r.sub)
}

func (r *run) ownByIndex(ctx context.Context, table, index, attr string, resolvedOnly bool) error {
	in := &dynamodb.QueryInput{
		TableName:                 aws.String(r.p.table(table)),
		IndexName:                 aws.String(index),
		KeyConditionExpression:    aws.String("#a = :sub"),
		ExpressionAttributeNames:  map[string]string{"#a": attr},
		ExpressionAttributeValues: item{":sub": str(r.sub)},
	}
	if resolvedOnly {
		in.FilterExpression = aws.String("resolved = :true")
		in.ExpressionAttributeValues[":true"] = &types.AttributeValueMemberBOOL{Value: true}
	}
	var keys []item
	if err := r.p.eachItem(ctx, in, func(it item) error {
		keys = append(keys, keyOf(it))
		return nil
	}); err != nil {
		return err
	}
	r.count(table+"_erased", len(keys))
	return r.p.deleteKeys(ctx, table, keys)
}

// cacheKeys deletes the user's Valkey keys. Rate-limit windows, the
// leaderboard rank mirror and table-keyed keys expire within minutes (R11).
func (r *run) cacheKeys(ctx context.Context) error {
	for _, k := range []string{
		"poker:presence:connections:" + r.sub,
		"poker:presence:table:" + r.sub,
		"chatprefs:extra:" + r.sub,
		"poker:achv:lastpocketpair:" + roomstore.CurrencyModeSandbox + "#" + r.sub,
		"poker:achv:lastpocketpair:" + roomstore.CurrencyModeReal + "#" + r.sub,
	} {
		if err := r.p.cache.Delete(ctx, k); err != nil {
			return fmt.Errorf("userpurge: cache delete: %w", err)
		}
	}
	if err := r.p.cache.DeletePrefix(ctx, "reaction-owned:"+r.sub+":"); err != nil {
		return fmt.Errorf("userpurge: cache delete prefix: %w", err)
	}
	return nil
}
