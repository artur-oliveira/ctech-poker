//go:build integration

package integration

import (
	"context"
	"flag"
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

const (
	fuzzBuyIn       = int64(1000)
	fuzzSmallBlind  = int64(10)
	fuzzBigBlind    = int64(20)
	fuzzTimerChance = 8
	fuzzTurnTimeout = 500 * time.Millisecond
	fuzzTimerWait   = 650 * time.Millisecond
)

type fuzzConfig struct {
	seed       int64
	numServers int
	numPlayers int
	numHands   int
}

type fuzzAction struct {
	hand     int
	player   string
	server   int
	action   betting.Action
	amount   int64
	timerArm bool
}

var multiServerFuzzSeed = flag.Int64("multiserver-fuzz-seed", 0, "run one multi-server fuzz seed")

func TestMultiServerFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("fuzz harness: slow, run explicitly")
	}
	if *multiServerFuzzSeed != 0 {
		runMultiServerFuzz(t, fuzzConfig{seed: *multiServerFuzzSeed, numServers: 3, numPlayers: 6, numHands: 5})
		return
	}
	base := time.Now().UnixNano()
	for i := 0; i < 25; i++ {
		seed := base + int64(i)
		t.Run(fmt.Sprintf("seed_%d", seed), func(t *testing.T) {
			t.Parallel()
			runMultiServerFuzz(t, fuzzConfig{
				seed: seed, numServers: 2 + i%2, numPlayers: 3 + i%4, numHands: 5,
			})
		})
	}
}

func runMultiServerFuzz(t *testing.T, cfg fuzzConfig) {
	t.Helper()
	if cfg.numServers < 2 || cfg.numServers > 3 || cfg.numPlayers < 3 || cfg.numPlayers > 6 || cfg.numHands < 1 {
		t.Fatalf("seed=%d: invalid fuzz config: %+v", cfg.seed, cfg)
	}

	// Cleanup output survives ordinary failures and test-goroutine panics, so
	// the exact run can be copied into a focused reproduction test.
	t.Cleanup(func() { t.Logf("multi-server fuzz seed=%d config=%+v", cfg.seed, cfg) })

	rng := rand.New(rand.NewPCG(uint64(cfg.seed), uint64(cfg.seed)^0x9e3779b97f4a7c15))
	db := testDynamoClient(t)
	store := tablestore.NewStore(db, "flow_test")
	mustCreatePokerTables(t, db, "flow_test")
	tableID := uniqueTableID(t)
	sharedLeaseBackend := cache.NewMemoryBackend(16)
	sink := newSnapshotSink()

	players := make([]*hand.Player, cfg.numPlayers)
	playerIDs := make([]string, cfg.numPlayers)
	for i := range players {
		id := fmt.Sprintf("p%d", i+1)
		playerIDs[i] = id
		players[i] = &hand.Player{ID: id, Stack: fuzzBuyIn, Ready: true}
	}
	seedTable := func() *hand.Table { return hand.NewTable(players, fuzzSmallBlind, fuzzBigBlind) }

	managers := make([]*tablemanager.Manager, cfg.numServers)
	actors := make([]*table.Actor, cfg.numServers)
	for i := range managers {
		managers[i] = tablemanager.NewManager(tablelease.NewService(sharedLeaseBackend), store, sink.record, nil)
		actor, err := managers[i].GetOrCreateActor(context.Background(), tableID, seedTable)
		if err != nil {
			t.Fatalf("seed=%d: server %d acquire actor: %v", cfg.seed, i, err)
		}
		actor.SetEquityEnabledForActor(false)
		// Configure before any command can arm a turn timer. Changing this
		// field after the actor is live would itself be a test data race.
		actor.SetTurnTimeoutForActor(fuzzTurnTimeout)
		actors[i] = actor
	}
	t.Cleanup(func() {
		for _, manager := range managers {
			manager.Release(tableID)
		}
	})

	serverFor := make(map[string]*table.Actor, cfg.numPlayers)
	serverIndexFor := make(map[string]int, cfg.numPlayers)
	for _, playerID := range playerIDs {
		serverIndex := rng.IntN(cfg.numServers)
		serverFor[playerID] = actors[serverIndex]
		serverIndexFor[playerID] = serverIndex
	}

	// All seats are present before the first StartHand. One ordinary Ready
	// command supplies the persisted transition and arms the first real timer.
	if err := serverFor[playerIDs[0]].Dispatch(table.ReadyCmd{
		PlayerID: playerIDs[0], ActionID: "fuzz-start", Ready: true, Reply: make(chan error, 1),
	}); err != nil {
		t.Fatalf("seed=%d: start first hand: %v", cfg.seed, err)
	}

	nextActionID := actionIDSeq()
	actionLog := make([]fuzzAction, 0, cfg.numHands*cfg.numPlayers*4)
	totalChips := int64(cfg.numPlayers) * fuzzBuyIn
	var previousHandID string

	for handNumber := 1; handNumber <= cfg.numHands; handNumber++ {
		if handNumber > 1 {
			waitForNextFuzzHand(t, cfg.seed, store, tableID, previousHandID, actionLog)
		}

		maxActions := cfg.numPlayers * 20
		for actionNumber := 0; ; actionNumber++ {
			stored := loadFuzzState(t, cfg.seed, store, tableID, actionLog)
			tbl := hand.NewTableFromState(stored.State)
			assertFuzzInvariants(t, cfg.seed, totalChips, stored, actionLog)
			if tbl.Stage() == hand.Complete {
				previousHandID = stored.HandID
				totalChips = replenishBustedFuzzPlayers(t, cfg.seed, fuzzBuyIn, totalChips, store, tableID, stored, serverFor, actionLog)
				break
			}
			if actionNumber >= maxActions {
				t.Fatalf("seed=%d: hand %d exceeded %d actions; stage=%v log=%+v", cfg.seed, handNumber, maxActions, tbl.Stage(), actionLog)
			}

			current := tbl.CurrentPlayerIDForActor()
			if current == "" {
				if tbl.IsAwaitingRunoutForActor() {
					waitForFuzzDecisionOrComplete(t, cfg.seed, totalChips, store, tableID, stored.Version, actionLog)
					continue
				}
				t.Fatalf("seed=%d: stalled with no current player at stage=%v log=%+v", cfg.seed, tbl.Stage(), actionLog)
			}
			legal := tbl.ViewFor(current).LegalActions
			if legal == nil || len(legal.Actions) == 0 {
				t.Fatalf("seed=%d: current player %s has no legal actions at stage=%v log=%+v", cfg.seed, current, tbl.Stage(), actionLog)
			}
			action, amount := pickRandomLegalAction(rng, legal)
			actor := serverFor[current]
			injectTimer := rng.IntN(fuzzTimerChance) == 0
			entry := fuzzAction{
				hand: handNumber, player: current, server: serverIndexFor[current],
				action: action, amount: amount, timerArm: injectTimer,
			}
			actionLog = append(actionLog, entry)
			dispatchErr := actor.Dispatch(table.ActCmd{
				PlayerID: current, ActionID: nextActionID(), Action: action, Amount: amount, Reply: make(chan error, 1),
			})
			post := loadFuzzState(t, cfg.seed, store, tableID, actionLog)
			assertFuzzInvariants(t, cfg.seed, totalChips, post, actionLog)
			if dispatchErr != nil && post.Version == stored.Version {
				t.Fatalf("seed=%d: dispatch %+v failed without a competing commit: %v; log=%+v", cfg.seed, entry, dispatchErr, actionLog)
			}
			if dispatchErr != nil {
				continue
			}
			if injectTimer {
				postTable := hand.NewTableFromState(post.State)
				timerPlayer := postTable.CurrentPlayerIDForActor()
				if timerPlayer != "" && timerPlayer != current {
					// Mark the new current player disconnected on THEIR fixed server.
					// Disconnect broadcasts and arms that actor's already-configured
					// short turn timer; its SitOut commit then races every other
					// server's stale cache exactly like the historical bug-3 path.
					connID := nextActionID()
					timerActor := serverFor[timerPlayer]
					if err := timerActor.Dispatch(table.ConnectCmd{PlayerID: timerPlayer, ConnID: connID, Reply: make(chan error, 1)}); err != nil {
						t.Fatalf("seed=%d: connect timer player %s: %v; log=%+v", cfg.seed, timerPlayer, err, actionLog)
					}
					if err := timerActor.Dispatch(table.DisconnectCmd{PlayerID: timerPlayer, ConnID: connID, Reply: make(chan error, 1)}); err != nil {
						t.Fatalf("seed=%d: disconnect timer player %s: %v; log=%+v", cfg.seed, timerPlayer, err, actionLog)
					}
					time.Sleep(fuzzTimerWait)
					if err := timerActor.Dispatch(table.ReconnectCmd{PlayerID: timerPlayer, Reply: make(chan error, 1)}); err != nil {
						t.Fatalf("seed=%d: reconnect timer player %s: %v; log=%+v", cfg.seed, timerPlayer, err, actionLog)
					}
					if err := timerActor.Dispatch(table.ReadyCmd{
						PlayerID: timerPlayer, ActionID: nextActionID(), Ready: true, Reply: make(chan error, 1),
					}); err != nil {
						t.Fatalf("seed=%d: ready timer player %s: %v; log=%+v", cfg.seed, timerPlayer, err, actionLog)
					}
					post = loadFuzzState(t, cfg.seed, store, tableID, actionLog)
					assertFuzzInvariants(t, cfg.seed, totalChips, post, actionLog)
				}
			}
		}
	}
}

// Random legal raises deliberately include all-in amounts. Rebuying a busted
// occupied seat is the production JoinCmd path and keeps a five-hand run from
// degenerating into WaitingForPlayers after an early showdown. wantTotal is
// increased only after the top-up commit, so conservation remains exact.
func replenishBustedFuzzPlayers(
	t *testing.T,
	seed, buyIn, wantTotal int64,
	store *tablestore.Store,
	tableID string,
	stored *tablestore.StoredTable,
	serverFor map[string]*table.Actor,
	log []fuzzAction,
) int64 {
	t.Helper()
	for _, player := range stored.State.Players {
		if player.Stack > 0 {
			continue
		}
		var rebuyErr error
		for attempt := 0; attempt < 10; attempt++ {
			rebuyErr = serverFor[player.ID].Dispatch(table.JoinCmd{
				PlayerID: player.ID, Stack: buyIn, Reply: make(chan error, 1),
			})
			post := loadFuzzState(t, seed, store, tableID, log)
			if persisted := findPlayer(hand.NewTableFromState(post.State), player.ID); persisted != nil && persisted.Stack > 0 {
				rebuyErr = nil
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if rebuyErr != nil {
			t.Fatalf("seed=%d: rebuy busted player %s after retries: %v; log=%+v", seed, player.ID, rebuyErr, log)
		}
		wantTotal += buyIn
		post := loadFuzzState(t, seed, store, tableID, log)
		assertFuzzInvariants(t, seed, wantTotal, post, log)
	}
	return wantTotal
}

func pickRandomLegalAction(rng *rand.Rand, legal *hand.LegalActions) (betting.Action, int64) {
	actionName := legal.Actions[rng.IntN(len(legal.Actions))]
	switch actionName {
	case "check":
		return betting.ActionCheck, 0
	case "call":
		return betting.ActionCall, legal.CallAmount
	case "fold":
		return betting.ActionFold, 0
	case "raise":
		span := legal.MaxRaiseTo - legal.MinRaiseTo + 1
		amount := legal.MinRaiseTo
		if span > 1 {
			amount += rng.Int64N(span)
		}
		return betting.ActionRaise, amount
	default:
		panic(fmt.Sprintf("unexpected legal poker action %q", actionName))
	}
}

func loadFuzzState(t *testing.T, seed int64, store *tablestore.Store, tableID string, log []fuzzAction) *tablestore.StoredTable {
	t.Helper()
	stored, err := store.LoadTable(context.Background(), tableID)
	if err != nil || stored == nil {
		t.Fatalf("seed=%d: load table: stored=%+v err=%v log=%+v", seed, stored, err, log)
	}
	return stored
}

func assertFuzzInvariants(t *testing.T, seed, wantTotal int64, stored *tablestore.StoredTable, log []fuzzAction) {
	t.Helper()
	playersByID := make(map[string]*hand.Player, len(stored.State.Players))
	var total int64
	for _, player := range stored.State.Players {
		playersByID[player.ID] = player
		total += player.Stack
	}
	if stored.State.Stage != hand.Complete && stored.State.Stage != hand.WaitingForPlayers {
		for _, ordered := range stored.State.HandOrder {
			player := playersByID[ordered.ID]
			if player == nil {
				t.Fatalf("seed=%d: handOrder player %s has no seated identity; log=%+v", seed, ordered.ID, log)
			}
			if ordered.Stack != player.Stack || ordered.Contributed != player.Contributed {
				t.Fatalf("seed=%d: handOrder/player desync for %s: order(stack=%d contributed=%d) players(stack=%d contributed=%d); log=%+v",
					seed, ordered.ID, ordered.Stack, ordered.Contributed, player.Stack, player.Contributed, log)
			}
			total += ordered.Contributed
		}
	}
	if total != wantTotal {
		t.Fatalf("seed=%d: chip conservation failed at version=%d stage=%v: got=%d want=%d; log=%+v",
			seed, stored.Version, stored.State.Stage, total, wantTotal, log)
	}
}

func waitForFuzzDecisionOrComplete(t *testing.T, seed, totalChips int64, store *tablestore.Store, tableID string, afterVersion int, log []fuzzAction) {
	t.Helper()
	deadline := time.Now().Add(12 * time.Second)
	for {
		stored := loadFuzzState(t, seed, store, tableID, log)
		assertFuzzInvariants(t, seed, totalChips, stored, log)
		tbl := hand.NewTableFromState(stored.State)
		if stored.Version > afterVersion && (tbl.Stage() == hand.Complete || tbl.CurrentPlayerIDForActor() != "") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("seed=%d: timed out awaiting paced runout; stage=%v version=%d log=%+v", seed, tbl.Stage(), stored.Version, log)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitForNextFuzzHand(t *testing.T, seed int64, store *tablestore.Store, tableID, previousHandID string, log []fuzzAction) {
	t.Helper()
	deadline := time.Now().Add(table.NextHandDelay + 5*time.Second)
	for {
		stored := loadFuzzState(t, seed, store, tableID, log)
		if stored.HandID != previousHandID && stored.State.Stage != hand.Complete {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("seed=%d: next hand did not start after hand %s; stage=%v log=%+v", seed, previousHandID, stored.State.Stage, log)
		}
		time.Sleep(25 * time.Millisecond)
	}
}
