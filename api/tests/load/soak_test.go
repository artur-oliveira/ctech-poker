//go:build load

// In-process load / soak harness for pre-release concurrency validation
// (GitHub issue #123). Unlike cmd/loadtest (synthetic WebSocket clients against
// a deployed prod-like stack), this variant drives the real
// tablemanager.Manager / table.Actor / tablestore stack directly against
// DynamoDB Local, so it runs from a laptop with only `podman compose -f
// docker-compose.test.yml up` — no server, wallet, or JWKS needed.
//
// It exercises the same hot path the WS gateway does (JoinCmd -> ReadyCmd ->
// ActCmd, with Connect/Disconnect churn and busted-player rebuys) across N
// tables served by a simulated fleet of M manager instances, and reports:
//   - p50/p95/p99 action-commit latency (Actor.Dispatch round-trip)
//   - hands completed
//   - errors by class
//   - Connect/Disconnect churn events
//   - process RSS versus live tables and process-local actors
//
// Run:
//
//	podman compose -f docker-compose.test.yml up -d
//	LOADTEST_DURATION=2m LOADTEST_TABLES=20 LOADTEST_PLAYERS=6 \
//	  go test -tags load -run TestSoak -timeout 30m ./tests/load
//
// With LOADTEST_DURATION unset the test skips (it is a deliberate, on-demand
// tool, not part of the default suite).
package load

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

const (
	soakEnv        = "load_soak"
	soakBuyIn      = int64(2000)
	soakSmallBlind = int64(10)
	soakBigBlind   = int64(20)
)

type soakConfig struct {
	tables   int
	players  int
	servers  int
	duration time.Duration
	thinkMin time.Duration
	thinkMax time.Duration
	churnPct int // percent chance per action loop of a Disconnect/Reconnect churn
}

func soakConfigFromEnv(t *testing.T) soakConfig {
	dur := os.Getenv("LOADTEST_DURATION")
	if dur == "" {
		t.Skip("set LOADTEST_DURATION (e.g. 2m) to run the soak harness")
	}
	d, err := time.ParseDuration(dur)
	if err != nil {
		t.Fatalf("LOADTEST_DURATION: %v", err)
	}
	cfg := soakConfig{
		tables:   envInt("LOADTEST_TABLES", 20),
		players:  envInt("LOADTEST_PLAYERS", 6),
		servers:  envInt("LOADTEST_SERVERS", 3),
		duration: d,
		thinkMin: time.Duration(envInt("LOADTEST_THINK_MIN_MS", 20)) * time.Millisecond,
		thinkMax: time.Duration(envInt("LOADTEST_THINK_MAX_MS", 120)) * time.Millisecond,
		churnPct: envInt("LOADTEST_CHURN_PCT", 2),
	}
	if cfg.players < 2 {
		t.Fatal("LOADTEST_PLAYERS must be >= 2")
	}
	if cfg.servers < 1 {
		cfg.servers = 1
	}
	return cfg
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// ---- metrics ---------------------------------------------------------

type soakMetrics struct {
	mu          sync.Mutex
	commitLat   []time.Duration
	handsDone   map[string]struct{}
	errsByClass map[string]int64
	actions     atomic.Int64
	churnEvents atomic.Int64
	rebuys      atomic.Int64
	liveTables  atomic.Int64
	memory      []memorySample
}

type memorySample struct {
	at         time.Duration
	liveTables int64
	liveActors int
	rssBytes   uint64
}

func newSoakMetrics() *soakMetrics {
	return &soakMetrics{handsDone: map[string]struct{}{}, errsByClass: map[string]int64{}}
}

func (m *soakMetrics) commit(d time.Duration, err error) {
	m.actions.Add(1)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commitLat = append(m.commitLat, d)
	if err != nil {
		m.errsByClass[classify(err)]++
	}
}

func (m *soakMetrics) handComplete(id string) {
	if id == "" {
		return
	}
	m.mu.Lock()
	m.handsDone[id] = struct{}{}
	m.mu.Unlock()
}

func (m *soakMetrics) sampleMemory(start time.Time, managers []*tablemanager.Manager) {
	rss, err := processRSSBytes()
	if err != nil {
		return
	}
	actors := 0
	for _, manager := range managers {
		actors += manager.LiveActorCount()
	}
	m.mu.Lock()
	m.memory = append(m.memory, memorySample{
		at: time.Since(start), liveTables: m.liveTables.Load(), liveActors: actors, rssBytes: rss,
	})
	m.mu.Unlock()
}

// processRSSBytes reads Linux's resident page count. The soak harness targets
// the same Linux runtime as EC2; unsupported hosts simply omit the curve.
func processRSSBytes() (uint64, error) {
	b, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(b))
	if len(fields) < 2 {
		return 0, fmt.Errorf("unexpected /proc/self/statm: %q", b)
	}
	pages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, err
	}
	return pages * uint64(os.Getpagesize()), nil
}

func classify(err error) string {
	switch {
	case errors.Is(err, tablestore.ErrUnavailable):
		return "unavailable"
	case errors.Is(err, table.ErrActorStopped):
		return "actor_stopped"
	case err != nil && contains(err.Error(), "version"):
		return "version_conflict"
	case err != nil && contains(err.Error(), "stale action state"):
		return "stale_state"
	default:
		return "invalid_action"
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func pct(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	return sorted[int(p/100*float64(len(sorted)-1))]
}

func (m *soakMetrics) report(t *testing.T, cfg soakConfig, elapsed time.Duration) (p99 time.Duration, errRate float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	lat := append([]time.Duration(nil), m.commitLat...)
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	var errs int64
	for _, c := range m.errsByClass {
		errs += c
	}
	total := m.actions.Load()
	if total > 0 {
		errRate = float64(errs) / float64(total)
	}
	p99 = pct(lat, 99)
	t.Logf("\n==================== soak results ====================")
	t.Logf("elapsed:            %s", elapsed.Round(time.Second))
	t.Logf("tables x players:   %d x %d  (%d seats) across %d servers", cfg.tables, cfg.players, cfg.tables*cfg.players, cfg.servers)
	t.Logf("actions committed:  %d", total)
	t.Logf("hands completed:    %d", len(m.handsDone))
	t.Logf("throughput:         %.1f actions/s, %.2f hands/s", float64(total)/elapsed.Seconds(), float64(len(m.handsDone))/elapsed.Seconds())
	t.Logf("churn events:       %d", m.churnEvents.Load())
	t.Logf("rebuys:             %d", m.rebuys.Load())
	if len(lat) > 0 {
		t.Logf("commit latency:     p50=%s p95=%s p99=%s max=%s", pct(lat, 50), pct(lat, 95), pct(lat, 99), lat[len(lat)-1])
	}
	t.Logf("errors:             %d total (%.3f%%)", errs, errRate*100)
	classes := make([]string, 0, len(m.errsByClass))
	for c := range m.errsByClass {
		classes = append(classes, c)
	}
	sort.Strings(classes)
	for _, c := range classes {
		t.Logf("  %-18s %d", c, m.errsByClass[c])
	}
	if len(m.memory) > 0 {
		t.Log("rss curve (elapsed, live_tables, local_actors, rss_mib):")
		for _, sample := range m.memory {
			t.Logf("  %s,%d,%d,%.1f", sample.at.Round(time.Second), sample.liveTables,
				sample.liveActors, float64(sample.rssBytes)/(1<<20))
		}
	}
	t.Logf("=====================================================")
	return p99, errRate
}

// ---- DynamoDB Local plumbing (mirrors tests/integration helpers) -----

func soakDynamoClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := awscfg.LoadDefaultConfig(context.Background(),
		awscfg.WithRegion("us-east-1"),
		awscfg.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")))
	if err != nil {
		t.Fatalf("aws config: %v", err)
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String("http://localhost:8555") })
}

func mustCreatePokerTables(t *testing.T, db *dynamodb.Client, env string) {
	t.Helper()
	pkOnly := []string{env + "_poker_table_state", env + "_poker_action_guards"}
	pkSk := []string{env + "_poker_action_log", env + "_poker_pending_cashouts"}
	create := func(name string, withSK bool) {
		attrs := []types.AttributeDefinition{{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS}}
		keys := []types.KeySchemaElement{{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash}}
		if withSK {
			attrs = append(attrs, types.AttributeDefinition{AttributeName: aws.String("sk"), AttributeType: types.ScalarAttributeTypeS})
			keys = append(keys, types.KeySchemaElement{AttributeName: aws.String("sk"), KeyType: types.KeyTypeRange})
		}
		_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
			TableName: aws.String(name), AttributeDefinitions: attrs, KeySchema: keys,
			BillingMode: types.BillingModePayPerRequest,
		})
		var inUse *types.ResourceInUseException
		if err != nil && !errors.As(err, &inUse) {
			t.Fatalf("create table %s: %v (is DynamoDB Local up on :8555? `podman compose -f docker-compose.test.yml up -d`)", name, err)
		}
	}
	for _, n := range pkOnly {
		create(n, false)
	}
	for _, n := range pkSk {
		create(n, true)
	}
}

// ---- the harness ----------------------------------------------------

func TestSoak(t *testing.T) {
	cfg := soakConfigFromEnv(t)
	db := soakDynamoClient(t)
	mustCreatePokerTables(t, db, soakEnv)
	store := tablestore.NewStore(db, soakEnv)
	leaseBackend := cache.NewMemoryBackend(1024)

	m := newSoakMetrics()

	// Simulated fleet: M managers sharing only the store + lease backend,
	// exactly like ARCHITECTURE.md §2's real instances.
	managers := make([]*tablemanager.Manager, cfg.servers)
	for i := range managers {
		managers[i] = tablemanager.NewManager(
			tablelease.NewService(leaseBackend), store,
			func(tableID, viewerID string, snap hand.Snapshot) {
				if snap.Stage == "complete" {
					m.handComplete(snap.HandID)
				}
			}, nil)
	}

	runStart := time.Now()
	deadline := runStart.Add(cfg.duration)
	ctx, cancel := context.WithDeadline(context.Background(), deadline.Add(30*time.Second))
	defer cancel()
	m.sampleMemory(runStart, managers)
	sampleCtx, stopSamples := context.WithCancel(ctx)
	defer stopSamples()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-sampleCtx.Done():
				return
			case <-ticker.C:
				m.sampleMemory(runStart, managers)
			}
		}
	}()

	var wg sync.WaitGroup
	for ti := 0; ti < cfg.tables; ti++ {
		ti := ti
		wg.Add(1)
		go func() {
			defer wg.Done()
			driveTable(ctx, t, cfg, m, managers, store, ti, deadline)
		}()
		// Ramp: stagger table starts across the first 10% of the run.
		if gap := cfg.duration / 10 / time.Duration(max1(cfg.tables)); gap > 0 {
			time.Sleep(gap)
		}
	}
	wg.Wait()
	stopSamples()
	m.sampleMemory(runStart, managers)

	elapsed := time.Since(runStart)
	for _, mgr := range managers {
		for ti := 0; ti < cfg.tables; ti++ {
			mgr.Release(fmt.Sprintf("soak-table-%d", ti))
		}
	}

	p99, errRate := m.report(t, cfg, elapsed)

	// Pass/fail thresholds (see docs/plans/2026-09-02-load-soak-test-harness.md).
	if m.actions.Load() == 0 {
		t.Fatal("no actions committed — harness did not drive any play")
	}
	if p99 > 750*time.Millisecond {
		t.Errorf("FAIL: p99 commit latency %s exceeds 750ms threshold", p99)
	}
	if errRate > 0.01 {
		t.Errorf("FAIL: error rate %.3f%% exceeds 1%% threshold", errRate*100)
	}
}

func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}

func driveTable(ctx context.Context, t *testing.T, cfg soakConfig, m *soakMetrics,
	managers []*tablemanager.Manager, store *tablestore.Store, tableIdx int, deadline time.Time) {

	tableID := fmt.Sprintf("soak-table-%d", tableIdx)
	m.liveTables.Add(1)
	rng := rand.New(rand.NewSource(int64(tableIdx)*7919 + time.Now().UnixNano()))

	playerIDs := make([]string, cfg.players)
	players := make([]*hand.Player, cfg.players)
	for i := range playerIDs {
		playerIDs[i] = fmt.Sprintf("t%dp%d", tableIdx, i)
		players[i] = &hand.Player{ID: playerIDs[i], Stack: soakBuyIn, Ready: true}
	}
	seed := func() *hand.Table { return hand.NewTable(players, soakSmallBlind, soakBigBlind) }

	// Each player is pinned to one server for the whole run (their WS
	// connection would be), commands for that player always go through it.
	serverFor := make(map[string]*tablemanager.Manager, cfg.players)
	for i, id := range playerIDs {
		serverFor[id] = managers[i%len(managers)]
	}

	actorFor := func(playerID string) (*table.Actor, error) {
		mgr := serverFor[playerID]
		if mgr == nil {
			// Any identity the engine surfaces that we did not seed (should not
			// happen, but never nil-panic the harness) goes through server 0.
			mgr = managers[0]
		}
		return mgr.GetOrCreateActor(ctx, tableID, seed)
	}

	dispatch := func(playerID string, cmd table.Command) error {
		a, err := actorFor(playerID)
		if err != nil {
			return err
		}
		return a.Dispatch(cmd)
	}

	// Kick the first hand off.
	_ = dispatch(playerIDs[0], table.ReadyCmd{PlayerID: playerIDs[0], ActionID: "soak-start", Ready: true, Reply: make(chan error, 1)})

	connected := map[string]string{} // playerID -> connID currently open

	for time.Now().Before(deadline) && ctx.Err() == nil {
		stored, err := store.LoadTable(ctx, tableID)
		if err != nil || stored == nil {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		tbl := hand.NewTableFromState(stored.State)

		switch tbl.Stage() {
		case hand.Complete:
			m.handComplete(stored.HandID)
			time.Sleep(50 * time.Millisecond)
		case hand.WaitingForPlayers:
			// Rebuy anyone busted, re-ready everyone.
			for _, p := range stored.State.Players {
				if p.Stack <= 0 {
					if err := dispatch(p.ID, table.JoinCmd{PlayerID: p.ID, Stack: soakBuyIn, Reply: make(chan error, 1)}); err == nil {
						m.rebuys.Add(1)
					}
				}
			}
			for _, id := range playerIDs {
				_ = dispatch(id, table.ReadyCmd{PlayerID: id, ActionID: fmt.Sprintf("ready-%s-%d", id, time.Now().UnixNano()), Ready: true, Reply: make(chan error, 1)})
			}
			time.Sleep(30 * time.Millisecond)
		default:
			current := tbl.CurrentPlayerIDForActor()
			if current == "" {
				time.Sleep(20 * time.Millisecond)
				continue
			}
			legal := tbl.ViewFor(current).LegalActions
			if legal == nil || len(legal.Actions) == 0 {
				time.Sleep(20 * time.Millisecond)
				continue
			}
			think := cfg.thinkMin
			if d := cfg.thinkMax - cfg.thinkMin; d > 0 {
				think += time.Duration(rng.Int63n(int64(d)))
			}
			time.Sleep(think)

			action, amount := pickAction(rng, legal)
			start := time.Now()
			derr := dispatch(current, table.ActCmd{
				PlayerID: current, ActionID: fmt.Sprintf("act-%s-%d", current, time.Now().UnixNano()),
				Action: action, Amount: amount, Reply: make(chan error, 1),
			})
			m.commit(time.Since(start), derr)
		}

		// Connect/Disconnect churn — a fraction of loops toggle a random
		// player's socket, exercising the presence + rearm paths under load.
		if rng.Intn(100) < cfg.churnPct {
			pid := playerIDs[rng.Intn(len(playerIDs))]
			if cid, open := connected[pid]; open {
				_ = dispatch(pid, table.DisconnectCmd{PlayerID: pid, ConnID: cid, Reply: make(chan error, 1)})
				delete(connected, pid)
			} else {
				cid := fmt.Sprintf("conn-%s-%d", pid, time.Now().UnixNano())
				if err := dispatch(pid, table.ConnectCmd{PlayerID: pid, ConnID: cid, Reply: make(chan error, 1)}); err == nil {
					connected[pid] = cid
				}
			}
			m.churnEvents.Add(1)
		}
	}
}

func pickAction(rng *rand.Rand, legal *hand.LegalActions) (betting.Action, int64) {
	has := func(a string) bool {
		for _, x := range legal.Actions {
			if x == a {
				return true
			}
		}
		return false
	}
	// Weighted toward keeping hands moving: mostly check/call, sometimes fold,
	// rarely a min-raise.
	switch {
	case has("check") && rng.Intn(10) != 0:
		return betting.ActionCheck, 0
	case has("raise") && rng.Intn(12) == 0:
		return betting.ActionRaise, legal.MinRaiseTo
	case has("call") && rng.Intn(6) != 0:
		return betting.ActionCall, legal.CallAmount
	case has("fold"):
		return betting.ActionFold, 0
	case has("check"):
		return betting.ActionCheck, 0
	case has("call"):
		return betting.ActionCall, legal.CallAmount
	default:
		return betting.ActionFold, 0
	}
}
