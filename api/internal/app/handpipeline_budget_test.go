package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/ws"
	"gopkg.aoctech.app/poker/api/internal/achievements"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/handreveal"
	"gopkg.aoctech.app/poker/api/internal/highlights"
	"gopkg.aoctech.app/poker/api/internal/leaderboard"
	"gopkg.aoctech.app/poker/api/internal/matchup"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/pokerstats"
	"gopkg.aoctech.app/poker/api/internal/recentplayers"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

// countingDynamo answers every DynamoDB call the pipeline makes with a canned
// response and tallies what it cost, so the whole post-hand pipeline's budget
// can be measured without a live table. Same shape as the per-module budget
// harnesses (leaderboard's countingDynamo, recentplayers', matchup's) — this
// one just sits under every store at once.
type countingDynamo struct {
	mu sync.Mutex
	// calls counts DynamoDB round trips by operation.
	calls map[string]int
	// writeItems counts items actually written: one per Put/Update/Delete,
	// and the real item count inside a BatchWriteItem/TransactWriteItems.
	writeItems int
	// tables records which table each call touched, so a store that silently
	// stopped writing shows up as a missing table rather than as a smaller
	// (apparently better) budget.
	tables map[string]int
}

func (c *countingDynamo) Do(req *http.Request) (*http.Response, error) {
	op := req.Header.Get("X-Amz-Target")
	if i := strings.LastIndex(op, "."); i >= 0 {
		op = op[i+1:]
	}
	raw, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	var body struct {
		TableName     string                     `json:"TableName"`
		RequestItems  map[string]json.RawMessage `json:"RequestItems"`
		TransactItems []map[string]struct {
			TableName string `json:"TableName"`
		} `json:"TransactItems"`
	}
	_ = json.Unmarshal(raw, &body)

	c.mu.Lock()
	c.calls[op]++
	if body.TableName != "" {
		c.tables[body.TableName]++
	}
	switch op {
	case "PutItem", "UpdateItem", "DeleteItem":
		c.writeItems++
	case "BatchWriteItem":
		for name, reqs := range body.RequestItems {
			c.tables[name]++
			var batch []json.RawMessage
			if json.Unmarshal(reqs, &batch) == nil {
				c.writeItems += len(batch)
			}
		}
	case "TransactWriteItems":
		c.writeItems += len(body.TransactItems)
		// A transaction carries its table names per item, not at the top level.
		for _, entry := range body.TransactItems {
			for _, op := range entry {
				if op.TableName != "" {
					c.tables[op.TableName]++
				}
			}
		}
	case "BatchGetItem":
		for name := range body.RequestItems {
			c.tables[name]++
		}
	}
	c.mu.Unlock()

	// A room has to resolve: tableCurrencyMode aborts the whole pipeline when
	// it does not, which would measure nothing. Everything else answers empty,
	// which every store treats as "no such row" — the worst case for writes,
	// since nothing is skipped as already-present.
	resp := `{}`
	switch op {
	case "GetItem":
		if strings.Contains(body.TableName, "rooms") {
			resp = `{"Item":{"currency_mode":{"S":"sandbox"},"big_blind":{"N":"100"}}}`
		}
	case "BatchGetItem":
		resp = `{"Responses":{},"UnprocessedKeys":{}}`
	case "Query", "Scan":
		resp = `{"Items":[],"Count":0}`
	case "UpdateItem":
		resp = `{"Attributes":{}}`
	case "BatchWriteItem":
		resp = `{"UnprocessedItems":{}}`
	}
	return &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/x-amz-json-1.0"}},
		Body:       io.NopCloser(bytes.NewReader([]byte(resp))),
		Request:    req,
	}, nil
}

func countingPipeline() (*handPipeline, *countingDynamo) {
	stub := &countingDynamo{calls: map[string]int{}, tables: map[string]int{}}
	db := dynamodb.New(dynamodb.Options{
		Region:           "us-east-1",
		Credentials:      credentials.NewStaticCredentialsProvider("dummy", "dummy", ""),
		HTTPClient:       stub,
		BaseEndpoint:     aws.String("https://dynamodb.us-east-1.amazonaws.com"),
		RetryMaxAttempts: 1,
	})
	const env = "budget_test"
	achv := achievements.NewService(achievements.NewStore(db, env))
	achv.SetCache(cache.NewMemoryBackend(64))
	return &handPipeline{
		reg:          noopRegistry{},
		achievements: achv,
		leaderboard:  leaderboard.NewServiceWithStore(leaderboard.NewStore(db, env)),
		rooms:        roomstore.NewStore(db, env),
		sessions:     sessionlog.NewStore(db, env),
		pokerStats:   pokerstats.NewStore(db, env),
		matchups:     matchup.NewStore(db, env),
		highlights:   highlights.NewStore(db, env),
		recent:       recentplayers.NewService(recentplayers.NewStore(db, env), nil, nil),
		players:      player.NewService(player.NewStore(db, env)),
		tables:       tablestore.NewStore(db, env),
		handReveals:  handreveal.NewStore(db, env),
		cfg:          &config.Config{AvatarBaseURL: "https://cdn.example.com"},
	}, stub
}

// TestHandPipelineDynamoBudget is the acceptance criterion of #204: the
// aggregate DynamoDB cost of one completed hand's post-game pipeline, measured
// end to end over the real stores at 2, 6 and 9 players, pinned so a
// regression in any single hook — or a new hook — cannot quietly multiply the
// per-hand bill again.
//
// It counts round trips and written items, not WCU: item *shape* is already
// pinned per module (TestHandHistoryItemWriteUnits here, TestRecordHandWriteBudget
// in leaderboard/achievements/recentplayers/matchup), and what this pipeline
// kept regressing on was the number of calls it fans out per hand.
//
// The measured ceiling below is the state after #244/#255/#257/#259/#261/#264;
// the numbers in issue #204's own text predate all of them.
func TestHandPipelineDynamoBudget(t *testing.T) {
	// The budget has three terms, because the pipeline's cost does:
	//
	//   fixed        the room read, the action-log query, the profile batch,
	//                the counter claim, the highlight and the reveal — one
	//                each per hand no matter how many seats.
	//   per seat     hand history, recent players, leaderboard, achievements
	//                and pokerstats each cost one item per participant.
	//   per pair     matchup is inherently C(seats,2) — every pair of players
	//                at the table gets its own row (#65/#201). This is the only
	//                quadratic term, and the reason a nine-handed hand costs
	//                roughly four times a six-handed one.
	//
	// Measured today (see the t.Logf below): 15 calls / 15 written items at 2
	// seats, 37 / 49 at 6, 64 / 85 at 9. The ceilings carry only enough
	// headroom to not be brittle — a new per-seat call or write trips them.
	const (
		callsPerHand      = 12
		callsPerSeat      = 2
		writeItemsPerHand = 6
		writeItemsPerSeat = 6
	)
	// Every store the pipeline is supposed to reach. A hook that stops writing
	// makes the budget look better, so the budget alone is not enough.
	wantTables := []string{
		"poker_player_hands", "poker_hand_reveals", "poker_table_highlights",
		"poker_achievement_progress", "poker_leaderboard_stats", "poker_player_poker_stats",
		"poker_player_matchups", "poker_recent_players",
	}

	for _, seats := range []int{2, 6, 9} {
		pipeline, stub := countingPipeline()
		outcome, names, _ := worstCaseHandOutcome(seats)
		outcome.WonWithoutShowdown, outcome.Winners = true, outcome.Participants[:1]
		// A real pot is what makes the highlights write happen at all.
		outcome.PotResults = []hand.PotResult{{
			Amount: 1000, PayoutAmount: 1000, EligiblePlayerIDs: outcome.Participants,
			Winners: outcome.Winners, Payouts: map[string]int64{outcome.Winners[0]: 1000},
		}}

		pipeline.run(context.Background(), "01M1HABKV5Z90SGHZ63K44EGHB", "01M1HABKV5Z90SGHZ63K44EGHC", outcome, names)

		totalCalls := 0
		for _, n := range stub.calls {
			totalCalls += n
		}
		pairs := seats * (seats - 1) / 2
		maxCalls := callsPerHand + callsPerSeat*seats + pairs
		maxWrites := writeItemsPerHand + writeItemsPerSeat*seats + pairs
		t.Logf("%d players: %d DynamoDB calls %v, %d written items, tables %v",
			seats, totalCalls, sortedOps(stub.calls), stub.writeItems, sortedOps(stub.tables))
		if totalCalls > maxCalls {
			t.Errorf("%d players: %d DynamoDB calls per hand, over the %d budget (%v)",
				seats, totalCalls, maxCalls, sortedOps(stub.calls))
		}
		if stub.writeItems > maxWrites {
			t.Errorf("%d players: %d written items per hand, over the %d budget",
				seats, stub.writeItems, maxWrites)
		}
		for _, name := range wantTables {
			if !touched(stub.tables, name) {
				t.Errorf("%d players: nothing was written to %s — a pipeline step stopped running", seats, name)
			}
		}
	}
}

// touched reports whether any call hit that table; store table names carry the
// environment prefix dynamo.NewBase adds.
func touched(tables map[string]int, name string) bool {
	for got, n := range tables {
		if n > 0 && strings.HasSuffix(got, name) {
			return true
		}
	}
	return false
}

func sortedOps(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+itoa(v))
	}
	sort.Strings(out)
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

type noopRegistry struct{}

func (noopRegistry) Start(context.Context) error               { return nil }
func (noopRegistry) Stop(context.Context) error                { return nil }
func (noopRegistry) Register(string, string, ws.Conn)          {}
func (noopRegistry) Unregister(string, string)                 {}
func (noopRegistry) Broadcast(context.Context, string, []byte) {}
