package app

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	goproto "google.golang.org/protobuf/proto"
	"gopkg.aoctech.app/api-commons/ws"
	"gopkg.aoctech.app/poker/api/internal/achievements"
	pokerproto "gopkg.aoctech.app/poker/api/internal/api/v1/proto"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/handreveal"
	"gopkg.aoctech.app/poker/api/internal/highlights"
	"gopkg.aoctech.app/poker/api/internal/leaderboard"
	"gopkg.aoctech.app/poker/api/internal/matchup"
	"gopkg.aoctech.app/poker/api/internal/metrics"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/pokerstats"
	"gopkg.aoctech.app/poker/api/internal/recentplayers"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

// Pipeline execution limits (#204). The pipeline is per completed hand and is
// pure bookkeeping, so the two things worth bounding are how much of it hits
// DynamoDB at once and how long any one run may hold a slot.
const (
	// maxConcurrentHandPipelines caps how many completed hands are being
	// written back at the same time across every table this process serves.
	// Nothing a player sees depends on the pipeline (broadcastAll has already
	// gone out by the time it is dispatched), so its only job is to keep the
	// per-hand DynamoDB burst — see handPipelineWriteBudget — from multiplying
	// by however many tables happen to finish a hand in the same second.
	maxConcurrentHandPipelines = 16

	// maxQueuedHandPipelines is the finite queue in front of that limit. Past
	// it the hand's bookkeeping is dropped with a loud log rather than queued
	// forever: a backlog this deep means the fleet is far past capacity, and
	// unbounded queueing would trade a bounded loss of gamification for
	// unbounded memory and a write burst that arrives long after the hand.
	maxQueuedHandPipelines = 256

	// handPipelineTimeout bounds one run. Every step is idempotent per
	// (table, hand) or a plain overwrite, so a run cut short is retried by
	// nothing and simply loses that hand's remaining derived rows — the same
	// outcome as the process dying mid-flight, which the pipeline has always
	// tolerated (see internal/handhook).
	handPipelineTimeout = 30 * time.Second
)

var (
	handPipelineSlots = make(chan struct{}, maxConcurrentHandPipelines)
	handPipelineDepth atomic.Int64
)

// handPipelineQueueDepth reports how many hand pipelines are queued or
// running right now. It is what the budget test asserts on; the same number is
// emitted as HandPipelineQueueDepth on every dispatch.
func handPipelineQueueDepth() int64 { return handPipelineDepth.Load() }

// #204 asked for runtime instrumentation of this pipeline and a runtime alert
// for a budget violation, which had nowhere to go until #279. These four are
// what the budget is actually made of:
//
//	HandPipelineQueueDepth   saturation — the Maximum statistic is the alarm,
//	                         since maxQueuedHandPipelines is the hard ceiling
//	HandPipelineDropped      the ceiling actually being hit: bookkeeping lost
//	HandPipelineDuration     p95 against handPipelineTimeout
//	HandPipelineStepFailures which hook is failing, dimensioned by Step
//	HandPipelineStepDuration how long each hook takes, same Step dimension
//	                         (#290 follow-up to #204/#279)
//
// Sampled ReturnConsumedCapacity — the other half of #204's ask — lives one
// level down, in gopkg.aoctech.app/api-commons/dynamo (SetCapacityRecorder),
// wired in internal/app/app.go.
const (
	metricQueueDepth   = "HandPipelineQueueDepth"
	metricDropped      = "HandPipelineDropped"
	metricDuration     = "HandPipelineDuration"
	metricStepFailures = "HandPipelineStepFailures"
	metricStepDuration = "HandPipelineStepDuration"
	metricPanics       = "HandPipelinePanics"
)

// stepFailed counts one failed step of the post-hand pipeline. Step values come
// from the fixed set below; the table and hand ids stay in the log line next to
// the call and never become dimensions (internal/metrics, "Dimensions are
// money").
func stepFailed(step string) {
	metrics.Record(metricStepFailures, metrics.Count, metrics.Dims{"Step": step}, 1)
}

// recordStepDuration reports how long one pipeline step took, dimensioned by
// the exact same Step values stepFailed uses — so a step that is slow and a
// step that is failing show up under the same name. Call with the step's own
// start time, measured around the DynamoDB call itself rather than the whole
// enclosing function, so CPU-only work (pokerstats.Analyze, the peek scan)
// doesn't inflate a step that is really about the store round trip.
func recordStepDuration(step string, started time.Time) {
	metrics.Record(metricStepDuration, metrics.Milliseconds, metrics.Dims{"Step": step}, float64(time.Since(started).Milliseconds()))
}

// dispatchGamificationPipeline detaches a completed hand's gamification
// bookkeeping (pipeline) onto its own goroutine so the table actor's own
// goroutine — which calls onHandComplete synchronously from
// table/actor.go's notifyHandComplete — never blocks on it (#61). A panic
// inside pipeline is recovered and logged rather than crashing the whole
// process, since this goroutine runs outside any request's recover
// middleware, the same reasoning as tablews.go's own ws handler recover.
//
// The goroutine is created immediately and only then waits for a slot, so the
// actor is never blocked by the concurrency limit either — what backs up under
// load is this queue, not the table (#204).
func dispatchGamificationPipeline(tableID, handID string, pipeline func(context.Context)) {
	depth := handPipelineDepth.Add(1)
	metrics.Record(metricQueueDepth, metrics.Count, nil, float64(depth))
	if depth > maxQueuedHandPipelines {
		handPipelineDepth.Add(-1)
		metrics.Record(metricDropped, metrics.Count, nil, 1)
		slog.Error("gamification: pipeline queue saturated, dropping hand bookkeeping",
			"table", tableID, "hand", handID, "queued", depth-1, "limit", maxQueuedHandPipelines)
		return
	}
	// Measured from dispatch, not from the moment a slot is granted: what #204
	// cares about is how long a hand's bookkeeping takes to land, and queueing
	// behind maxConcurrentHandPipelines is part of that.
	started := time.Now()
	go func() {
		defer handPipelineDepth.Add(-1)
		defer func() {
			if r := recover(); r != nil {
				metrics.Record(metricPanics, metrics.Count, nil, 1)
				slog.Error("gamification: onHandComplete panic recovered", "table", tableID, "hand", handID, "panic", r)
			}
		}()
		handPipelineSlots <- struct{}{}
		defer func() { <-handPipelineSlots }()
		ctx, cancel := context.WithTimeout(context.Background(), handPipelineTimeout)
		defer cancel()
		pipeline(ctx)
		metrics.Record(metricDuration, metrics.Milliseconds, nil, float64(time.Since(started).Milliseconds()))
	}()
}

// handPipeline is everything a completed hand's post-game bookkeeping writes
// to. It was newTableManager's onHandComplete closure; it is a type so the
// per-hand DynamoDB budget (#204) can be measured end to end against the real
// stores instead of module by module.
type handPipeline struct {
	reg          ws.Registry
	achievements *achievements.Service
	leaderboard  *leaderboard.Service
	rooms        *roomstore.Store
	sessions     *sessionlog.Store
	pokerStats   *pokerstats.Store
	matchups     *matchup.Store
	highlights   *highlights.Store
	recent       *recentplayers.Service
	players      *player.Service
	tables       *tablestore.Store
	handReveals  *handreveal.Store
	cfg          *config.Config
}

func (p *handPipeline) persistHandHistory(ctx context.Context, tableID, handID, mode string, outcome hand.HandOutcome, names map[string]string) {
	if p.sessions == nil {
		return
	}
	// One BatchGetItem for the whole table instead of one GetOrCreate per
	// participant (#200): this runs on the write the client actively waits
	// for, so nine sequential profile reads were nine round trips of pure
	// latency in front of it. GetMany is read-only where GetOrCreate would
	// create a row, which costs nothing here — every participant was dealt
	// in, so buyin.Service already created their profile at buy-in
	// (internal/buyin/service.go). Participants missing from the batch
	// (deleted profile, or a partial failure) simply get no avatar, the
	// same fallback the per-player GetOrCreate error path had.
	profileStart := time.Now()
	profiles, profileErr := p.players.GetMany(ctx, outcome.Participants)
	recordStepDuration("profiles", profileStart)
	avatarURLs := make(map[string]string, len(outcome.Participants))
	if profileErr != nil {
		stepFailed("profiles")
		slog.Error("sessionlog: batch resolve participant avatars failed", "table", tableID, "hand", handID, "err", profileErr)
	} else {
		for id, profile := range profiles {
			avatarURLs[id] = player.AvatarURL(&profile, p.cfg.AvatarBaseURL)
		}
	}
	endedAt := time.Now().UnixMilli()
	items := make([]sessionlog.HandItem, 0, len(outcome.Participants))
	for _, id := range outcome.Participants {
		item := handItemForWithAvatars(outcome, id, names, avatarURLs)
		item.PK, item.TableID, item.HandID, item.CurrencyMode, item.EndedAt = id, tableID, handID, mode, endedAt
		items = append(items, item)
	}
	// One BatchWriteItem for every participant's row — see
	// sessionlog.Store.RecordHands for why the history stays N redacted
	// per-player copies rather than one canonical hand record.
	writeStart := time.Now()
	err := p.sessions.RecordHands(ctx, items)
	recordStepDuration("handhistory", writeStart)
	if err != nil {
		stepFailed("handhistory")
		slog.Error("sessionlog: record hand failed", "table", tableID, "hand", handID, "err", err)
	}
}

func (p *handPipeline) persistHandReveal(ctx context.Context, tableID, handID, mode string, outcome hand.HandOutcome) {
	if p.handReveals == nil || mode != roomstore.CurrencyModeSandbox {
		return
	}
	if !outcome.WonWithoutShowdown || len(outcome.Winners) != 1 {
		return
	}
	defer func(started time.Time) { recordStepDuration("reveal", started) }(time.Now())
	room, err := p.rooms.Get(ctx, tableID)
	if err != nil || room == nil {
		stepFailed("reveal")
		slog.Error("handreveal: load room for big blind failed", "table", tableID, "hand", handID, "err", err)
		return
	}
	winnerID := outcome.Winners[0]
	playerHands := make(map[string]handreveal.PlayerHandCode, len(outcome.PlayerHands))
	for id, info := range outcome.PlayerHands {
		playerHands[id] = handreveal.PlayerHandCode{Cards: info.HoleCards}
	}
	record := handreveal.HandRecord{
		HandID: handID, TableID: tableID, BigBlind: room.BigBlind,
		WinnerID: winnerID, WinnerShown: outcome.PlayerHands[winnerID].Revealed,
		PlayerHands: playerHands, EndedAt: time.Now().UnixMilli(),
	}
	if err := p.handReveals.Put(ctx, record); err != nil {
		stepFailed("reveal")
		slog.Error("handreveal: record hand failed", "table", tableID, "hand", handID, "err", err)
	}
}

// run is the pipeline itself. It is invoked off the actor goroutine (see
// dispatchGamificationPipeline) — everything it does is gamification
// bookkeeping, none of which the actor's own state or any client-visible
// broadcast depends on: broadcastAll already sent every player their post-hand
// "state" snapshot before onHandComplete is ever reached, and
// notifyHandComplete's handhook SET NX claim (fleet-wide once-per-hand dedup,
// internal/handhook) has already been taken synchronously.
//
// The handhook claim bounds which instance may run this at all, but it fails
// OPEN on its own Valkey error (see internal/handhook), so a Valkey blip can
// still let two instances both reach here for the same hand. That is only
// harmless for the writers below that are already idempotent per
// (table_id, hand_id) on their own (pokerstats, matchup, and the
// plain-overwrite session/hand-history, highlights, recent players writes) —
// the achievements/leaderboard counters are bare ADDs with no guard of their
// own, so they are additionally gated behind achievements.ClaimHandCounters
// (issue #66).
func (p *handPipeline) run(ctx context.Context, tableID, handID string, outcome hand.HandOutcome, names map[string]string) {
	roomStart := time.Now()
	mode, err := tableCurrencyMode(ctx, p.rooms, tableID)
	recordStepDuration("room", roomStart)
	if err != nil {
		stepFailed("room")
		slog.Error("gamification: load room mode failed", "table", tableID, "err", err)
		return
	}
	// Player-visible reads (the last-winners strip's hand history and
	// the "maior pote de hoje" highlight) are written first: the client
	// invalidates both query keys the instant it sees the `complete`
	// snapshot, so anything ahead of these writes in the pipeline was
	// making its refetch race ahead of the row and show the finished hand
	// a whole hand late. All three are plain idempotent overwrites and
	// depend only on the outcome, not on any metrics computed below.
	p.persistHandHistory(ctx, tableID, handID, mode, outcome, names)
	p.persistHandReveal(ctx, tableID, handID, mode, outcome)
	highlightsStart := time.Now()
	err = p.highlights.RecordHand(ctx, tableID, handID, outcome, names)
	recordStepDuration("highlights", highlightsStart)
	if err != nil {
		stepFailed("highlights")
		slog.Error("highlights: record hand failed", "table", tableID, "hand", handID, "err", err)
	}
	var metrics []pokerstats.HandMetric
	actionsStart := time.Now()
	actions, metricsErr := p.tables.LoadActionsSince(ctx, tableID, handID, 0)
	recordStepDuration("actions", actionsStart)
	if metricsErr != nil {
		stepFailed("actions")
		slog.Error("pokerstats: load hand actions failed", "table", tableID, "hand", handID, "err", metricsErr)
	} else {
		metrics = pokerstats.Analyze(outcome.Participants, actions)
	}
	// peeked scans the whole hand's action log (unlike pokerstats.Analyze,
	// which intentionally stops at the flop) since a player can peek at
	// their own cards at any street, not just preflop.
	peeked := make(map[string]bool)
	// Time bank is charged per action, so one hand can carry several
	// charges for the same player.
	timeBankMs := make(map[string]int64)
	for _, entry := range actions {
		if entry.Action == "peek_cards" {
			peeked[entry.PlayerID] = true
		}
		timeBankMs[entry.PlayerID] += entry.TimeBankMs
	}
	achievementMetrics := make([]achievements.HandMetric, len(metrics))
	for i, metric := range metrics {
		achievementMetrics[i] = achievements.HandMetric{
			PlayerID:   metric.PlayerID,
			VPIP:       metric.VPIP,
			ThreeBet:   metric.ThreeBet,
			Peeked:     peeked[metric.PlayerID],
			TimeBankMs: timeBankMs[metric.PlayerID],
		}
	}
	// ClaimHandCounters is the correctness backstop for the two
	// non-idempotent ADD-based writers below (achievements' counters,
	// leaderboard's hands_played/hands_won/achievement points):
	// internal/handhook's own claim already dedupes which instance may
	// reach this pipeline at all, but it fails OPEN on a Valkey error, so
	// a Valkey blip during hand completion can let two instances both pass
	// it and both reach here — and unlike matchup/pokerstats (which guard
	// their own write) neither of these had a per-hand guard of their own
	// before issue #66. Skip both entirely when the claim is lost or
	// errors — an error here fails CLOSED (never runs the increments)
	// rather than open, because this guard IS the source of truth for
	// "already counted", not an optional accelerator.
	// matchup/pokerstats/sessionlog/highlights/recent players are
	// unaffected: they stay outside this guard exactly as before, since
	// each is already idempotent on its own.
	counterguardStart := time.Now()
	claimed, err := p.achievements.ClaimHandCounters(ctx, tableID, handID)
	recordStepDuration("counterguard", counterguardStart)
	if err != nil {
		stepFailed("counterguard")
		slog.Error("gamification: hand counter guard failed, skipping achievement/leaderboard increments", "table", tableID, "hand", handID, "err", err)
	} else if !claimed {
		slog.Info("gamification: hand counters already claimed for this hand, skipping duplicate achievement/leaderboard increments", "table", tableID, "hand", handID)
	} else {
		achievementsStart := time.Now()
		unlocks, err := p.achievements.RecordHand(ctx, tableID, mode, outcome, achievementMetrics)
		recordStepDuration("achievements", achievementsStart)
		if err != nil {
			stepFailed("achievements")
			slog.Error("achievements record hand failed", "table", tableID, "err", err)
		}
		for _, unlock := range unlocks {
			data, err := goproto.Marshal(&pokerproto.ServerMessage{
				Type:  "achievement_unlocked",
				Key:   unlock.Key,
				Stars: int32(unlock.Stars),
			})
			if err == nil {
				p.reg.Broadcast(ctx, tableID+"#"+unlock.PlayerID, data)
			}
		}
		leaderboardStart := time.Now()
		unlocksErr := p.leaderboard.RecordUnlocks(ctx, mode, unlocks)
		handErr := p.leaderboard.RecordHand(ctx, mode, outcome, names)
		recordStepDuration("leaderboard", leaderboardStart)
		if unlocksErr != nil {
			stepFailed("leaderboard")
			slog.Error("leaderboard achievement points failed", "table", tableID, "err", unlocksErr)
		}
		if handErr != nil {
			stepFailed("leaderboard")
			slog.Error("leaderboard record hand failed", "table", tableID, "err", handErr)
		}
	}
	if metricsErr == nil {
		pokerstatsStart := time.Now()
		err := p.pokerStats.RecordHand(ctx, mode, tableID, handID, metrics)
		recordStepDuration("pokerstats", pokerstatsStart)
		if err != nil {
			stepFailed("pokerstats")
			slog.Error("pokerstats: record hand failed", "table", tableID, "hand", handID, "err", err)
		}
	}
	matchupStart := time.Now()
	err = p.matchups.RecordHand(ctx, mode, tableID, handID, outcome)
	recordStepDuration("matchup", matchupStart)
	if err != nil {
		stepFailed("matchup")
		slog.Error("matchup: record hand failed", "table", tableID, "hand", handID, "err", err)
	}
	if p.recent != nil {
		recentStart := time.Now()
		err := p.recent.RecordHand(ctx, tableID, handID, outcome.Participants, time.Now())
		recordStepDuration("recentplayers", recentStart)
		if err != nil {
			stepFailed("recentplayers")
			slog.Error("recent players: record hand failed", "table", tableID, "hand", handID, "err", err)
		}
	}
}
