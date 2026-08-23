# Head-to-Head Player Comparator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:
> executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a two-player head-to-head comparator: an incrementally-updated DynamoDB aggregate (`internal/matchup`),
written at the same hand-complete hook as `pokerstats`/`leaderboard`, read by a new
`GET /players/me/matchups/:opponentId` endpoint, and surfaced as a small card on the existing public profile showcase
route.

**Architecture:** New package `internal/matchup` mirrors `internal/pokerstats`'s `Store{base dynamo.Base}` shape
exactly — one new DynamoDB table (`poker_player_matchups`), one `TransactWriteItems` call per completed hand covering
every unordered pair within `outcome.Participants`, each pair carrying its own create-only idempotency guard so a
duplicate hook invocation double-counts nothing. The pair's key is always `idLow`/`idHigh` (lexicographic), so a
"viewer/opponent" remapping happens once, in the read handler, never in storage.

**Tech Stack:** Go (Fiber v3, `aws-sdk-go-v2/dynamodb`, `gopkg.aoctech.app/api-commons/dynamo`, `go.uber.org/fx`), AWS
CDK (TypeScript, `aws-cdk-lib/aws-dynamodb`), Next.js 16 App Router (TanStack Query, `lucide-react`).

**Spec:** `docs/specs/2026-08-21-head-to-head-stats.md`

## Global Constraints

- Sandbox and real chip results must never mix into one number — every key and query is scoped by `mode` (`sandbox`/
  `real`), exactly like `pokerstats.Store.Get`'s `"stats#"+mode+"#"+playerID` key.
- Correctness = DynamoDB conditional writes. Never read-then-write against shared counters; every increment goes through
  `dynamo.Base.BuildRawUpdateTxItem`'s `ADD` expression inside a `TransactWriteItems` call guarded by a create-only
  idempotency row.
- Player identity comes from the JWT `sub` (`claims.Sub` / `c.Locals(localsUserID)`), never from a client-supplied id —
  the only client-supplied id anywhere in this feature is the *opponent*'s id.
- Named constants / no magic strings for table names, route paths, and DynamoDB field names.
- **Deviation from the spec, called out explicitly:** the spec's data model has a single `NetChangeLow` field and has
  the read handler negate it for the `idHigh` viewer. That's wrong once rake is involved — `HandOutcome.Payouts` is
  already net-of-rake (`hand.go:175-176`'s doc comment) but `Contributions` is gross, so a heads-up hand's two
  `Payouts[id]-Contributions[id]` results do **not** sum to zero; `idHigh`'s true net is not `-NetChangeLow`. This plan
  stores both `NetChangeLow` and `NetChangeHigh`, each computed directly from that side's own `Payouts`/`Contributions`,
  so no negation is ever needed. Every other field in the spec's data model is unchanged.
- Every code change ships with its documentation update in the same task (`api/CLAUDE.md`, `cdk/CLAUDE.md`,
  `ui/CLAUDE.md` each carry a "Mandatory Documentation Policy").

---

## Task 1: `internal/matchup` — data model, pure delta computation, and the write/read store

**Files:**

- Create: `api/internal/matchup/store.go`
- Test: `api/internal/matchup/store_test.go`

**Interfaces:**

- Produces: `matchup.Stats` (dynamodbav-tagged), `matchup.PairStats{IDLow, IDHigh string; Stats Stats}`,
  `matchup.Store{}`, `matchup.NewStore(db *dynamodb.Client, env string) *Store`,
  `(*Store) RecordHand(ctx context.Context, mode, tableID, handID string, outcome hand.HandOutcome) error`,
  `(*Store) Get(ctx context.Context, mode, playerA, playerB string) (PairStats, error)`.
- Consumes: `gopkg.aoctech.app/api-commons/dynamo` (`Base`, `NewBase`, `Encode`, `Decode`, `NowStr`,
  `IsConditionFailed`), `gopkg.aoctech.app/poker/api/internal/engine/hand` (`HandOutcome`, fields `Winners []string`,
  `Participants []string`, `Payouts map[string]int64`, `Contributions map[string]int64`).

- [ ] **Step 1: Write the package**

```go
// Package matchup materializes an incrementally-updated head-to-head
// aggregate for every unordered pair of players who have shared a hand,
// following internal/pokerstats's exact shape (Store{base dynamo.Base}) so
// a per-opponent comparator never has to page a player's entire hand
// history at query time (see docs/specs/2026-08-21-head-to-head-stats.md's
// "Why not derive this from existing history at query time").
package matchup

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

const (
	tableMatchups = "poker_player_matchups"
	guardTTLDays  = 90
)

// Stats is one unordered player pair's cumulative head-to-head record,
// scoped by currency mode. Every field is relative to idLow/idHigh (see
// pairKey), never to a "viewer" — the same item is correct regardless of
// which of the two players queries it; viewer/opponent remapping happens
// once, in the read handler (internal/api/v1), not here.
type Stats struct {
	HandsTogether        int64 `dynamodbav:"hands_together"`
	WinsLow              int64 `dynamodbav:"wins_low"`
	WinsHigh             int64 `dynamodbav:"wins_high"`
	Ties                 int64 `dynamodbav:"ties"`
	HeadsUpHandsTogether int64 `dynamodbav:"heads_up_hands_together"`
	// NetChangeLow/NetChangeHigh are each player's own cumulative
	// Payouts-Contributions across heads-up hands only (a 3+-way pot has no
	// single correct per-opponent attribution — see deltasFor). Two
	// independent fields, not one negated field: HandOutcome.Payouts is net
	// of rake but Contributions is gross, so a heads-up hand is not
	// zero-sum between the two players and idHigh's result can never be
	// safely derived as -NetChangeLow.
	NetChangeLow  int64 `dynamodbav:"net_change_low"`
	NetChangeHigh int64 `dynamodbav:"net_change_high"`
}

// PairStats is one Get result: the raw idLow/idHigh-relative Stats plus
// which supplied player landed on which side, so a caller can remap to
// viewer/opponent.
type PairStats struct {
	IDLow  string
	IDHigh string
	Stats  Stats
}

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tableMatchups)}
}

// pairKey returns the pair's item key plus the two ids in lexicographic
// order, so the same unordered pair always resolves to the same item
// regardless of who's "viewer."
func pairKey(mode, a, b string) (key, idLow, idHigh string) {
	idLow, idHigh = a, b
	if idHigh < idLow {
		idLow, idHigh = idHigh, idLow
	}
	return "pair#" + mode + "#" + idLow + "#" + idHigh, idLow, idHigh
}

// pairDelta is one hand's contribution to one unordered pair, computed pure
// (no I/O) so the counting logic is unit-testable without DynamoDB.
type pairDelta struct {
	key, idLow, idHigh                     string
	handsTogether, winsLow, winsHigh, ties int64
	headsUp                                bool
	netLow, netHigh                        int64
}

// deltasFor computes one pairDelta per unordered pair within
// outcome.Participants. Hand-win/loss/tie is well-defined for any table
// size using membership in outcome.Winners (the same check
// internal/app's handItemFor uses); NetChangeLow/NetChangeHigh only move on
// a genuine heads-up hand (exactly 2 participants) — publishing a
// per-opponent chip result for a 3+-way pot would be quietly wrong for
// every hand a third player contributed to or won.
func deltasFor(mode string, outcome hand.HandOutcome) []pairDelta {
	if len(outcome.Participants) < 2 {
		return nil
	}
	winners := make(map[string]bool, len(outcome.Winners))
	for _, id := range outcome.Winners {
		winners[id] = true
	}
	headsUp := len(outcome.Participants) == 2
	var deltas []pairDelta
	for i := 0; i < len(outcome.Participants); i++ {
		for j := i + 1; j < len(outcome.Participants); j++ {
			a, b := outcome.Participants[i], outcome.Participants[j]
			if a == "" || b == "" || a == b {
				continue
			}
			key, idLow, idHigh := pairKey(mode, a, b)
			d := pairDelta{key: key, idLow: idLow, idHigh: idHigh, handsTogether: 1, headsUp: headsUp}
			switch {
			case winners[idLow] && winners[idHigh]:
				d.ties = 1
			case winners[idLow]:
				d.winsLow = 1
			case winners[idHigh]:
				d.winsHigh = 1
			}
			if headsUp {
				d.netLow = outcome.Payouts[idLow] - outcome.Contributions[idLow]
				d.netHigh = outcome.Payouts[idHigh] - outcome.Contributions[idHigh]
			}
			deltas = append(deltas, d)
		}
	}
	return deltas
}

// RecordHand atomically applies one completed hand to every unordered pair
// within outcome.Participants. Each pair carries its own create-only
// idempotency guard inside the same TransactWriteItems call (mirrors
// pokerstats.Store.RecordHand's guard-plus-increments shape, extended to
// per-pair guards since one hand now touches many independent items), so a
// duplicate onHandComplete invocation for the same hand double-counts no
// pair — and, since all guards ride in one transaction, either the whole
// hand's pairs commit or none do. tableID disambiguates the guard because
// hand ids are only unique within a table (mirrors
// pokerstats.Store.RecordHand's "guard#"+tableID+"#"+handID key). A 9-max
// table caps this at C(9,2)=36 pairs * 2 items (guard + update) = 72 items,
// under DynamoDB's 100-item TransactWriteItems limit.
func (s *Store) RecordHand(ctx context.Context, mode, tableID, handID string, outcome hand.HandOutcome) error {
	if tableID == "" || handID == "" {
		return nil
	}
	deltas := deltasFor(mode, outcome)
	if len(deltas) == 0 {
		return nil
	}
	items := make([]types.TransactWriteItem, 0, len(deltas)*2)
	for _, d := range deltas {
		guard, err := dynamo.Encode(struct {
			PK  string `dynamodbav:"pk"`
			TTL int64  `dynamodbav:"ttl"`
		}{
			PK:  "guard#" + tableID + "#" + handID + "#" + d.key,
			TTL: time.Now().Add(guardTTLDays * 24 * time.Hour).Unix(),
		})
		if err != nil {
			return fmt.Errorf("matchup: encode guard: %w", err)
		}
		items = append(items, s.base.BuildPutTxItemIfAbsent(guard))

		values := map[string]types.AttributeValue{
			":hands":   number(d.handsTogether),
			":winLow":  number(d.winsLow),
			":winHigh": number(d.winsHigh),
			":tie":     number(d.ties),
			":now":     &types.AttributeValueMemberS{Value: dynamo.NowStr()},
		}
		updateExpr := "ADD hands_together :hands, wins_low :winLow, wins_high :winHigh, ties :tie SET updated_at = :now"
		if d.headsUp {
			values[":huHands"] = number(1)
			values[":netLow"] = number(d.netLow)
			values[":netHigh"] = number(d.netHigh)
			updateExpr = "ADD hands_together :hands, wins_low :winLow, wins_high :winHigh, ties :tie, " +
				"heads_up_hands_together :huHands, net_change_low :netLow, net_change_high :netHigh SET updated_at = :now"
		}
		items = append(items, s.base.BuildRawUpdateTxItem(d.key, nil, updateExpr, "", nil, values))
	}
	if err := s.base.TransactWrite(ctx, items); err != nil {
		if dynamo.IsConditionFailed(err) {
			return nil
		}
		return fmt.Errorf("matchup: record hand: %w", err)
	}
	return nil
}

// Get returns playerA/playerB's head-to-head stats, zeroed (not an error)
// when the pair has never shared a hand — a pair with no history isn't an
// error, same as pokerstats.Store.Get's item==nil branch.
func (s *Store) Get(ctx context.Context, mode, playerA, playerB string) (PairStats, error) {
	key, idLow, idHigh := pairKey(mode, playerA, playerB)
	result := PairStats{IDLow: idLow, IDHigh: idHigh}
	item, err := s.base.GetItem(ctx, key)
	if err != nil {
		return PairStats{}, fmt.Errorf("matchup: get: %w", err)
	}
	if item == nil {
		return result, nil
	}
	stats, err := dynamo.Decode[Stats](item)
	if err != nil {
		return PairStats{}, fmt.Errorf("matchup: decode: %w", err)
	}
	result.Stats = *stats
	return result, nil
}

func number(v int64) types.AttributeValue {
	return &types.AttributeValueMemberN{Value: strconv.FormatInt(v, 10)}
}
```

- [ ] **Step 2: Write the failing unit tests**

```go
package matchup

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func TestPairKeyIsOrderIndependent(t *testing.T) {
	k1, low1, high1 := pairKey("sandbox", "b", "a")
	k2, low2, high2 := pairKey("sandbox", "a", "b")
	if k1 != k2 || low1 != low2 || high1 != high2 {
		t.Fatalf("pairKey not order independent: %q/%q/%q vs %q/%q/%q", k1, low1, high1, k2, low2, high2)
	}
	if low1 != "a" || high1 != "b" {
		t.Fatalf("expected lexicographic a/b, got %s/%s", low1, high1)
	}
}

func TestDeltasForMultiWayHandCountsHandsWinsTiesButNoNetChange(t *testing.T) {
	outcome := hand.HandOutcome{
		Winners:       []string{"a"},
		Participants:  []string{"a", "b", "c"},
		Payouts:       map[string]int64{"a": 300},
		Contributions: map[string]int64{"a": 100, "b": 100, "c": 100},
	}
	deltas := deltasFor("sandbox", outcome)
	if len(deltas) != 3 {
		t.Fatalf("expected 3 pairs, got %d", len(deltas))
	}
	for _, d := range deltas {
		if d.handsTogether != 1 {
			t.Fatalf("pair %s/%s handsTogether = %d, want 1", d.idLow, d.idHigh, d.handsTogether)
		}
		if d.headsUp || d.netLow != 0 || d.netHigh != 0 {
			t.Fatalf("pair %s/%s should carry no net change in a 3-way hand: %+v", d.idLow, d.idHigh, d)
		}
		wantWinLow, wantWinHigh := int64(0), int64(0)
		if d.idLow == "a" {
			wantWinLow = 1
		}
		if d.idHigh == "a" {
			wantWinHigh = 1
		}
		if d.winsLow != wantWinLow || d.winsHigh != wantWinHigh || d.ties != 0 {
			t.Fatalf("pair %s/%s win split = winLow=%d winHigh=%d ties=%d", d.idLow, d.idHigh, d.winsLow, d.winsHigh, d.ties)
		}
	}
}

func TestDeltasForHeadsUpHandTracksNetChangeBothSides(t *testing.T) {
	outcome := hand.HandOutcome{
		Winners:       []string{"z"},
		Participants:  []string{"a", "z"},
		Payouts:       map[string]int64{"z": 190},
		Contributions: map[string]int64{"a": 100, "z": 100},
	}
	deltas := deltasFor("sandbox", outcome)
	if len(deltas) != 1 {
		t.Fatalf("expected 1 pair, got %d", len(deltas))
	}
	d := deltas[0]
	if d.idLow != "a" || d.idHigh != "z" {
		t.Fatalf("expected a/z ordering, got %s/%s", d.idLow, d.idHigh)
	}
	// Payouts is net of rake, Contributions is gross: 200 contributed, 190
	// paid out to z (10 rake). netLow=-100 (a lost its whole 100 buy-in),
	// netHigh=90 (z won 90 net of rake) — deliberately not netLow's
	// negation, see the Stats.NetChangeLow/NetChangeHigh doc comment.
	if !d.headsUp || d.netLow != -100 || d.netHigh != 90 {
		t.Fatalf("heads-up net change = %+v, want netLow=-100 netHigh=90", d)
	}
	if d.winsLow != 0 || d.winsHigh != 1 || d.ties != 0 {
		t.Fatalf("win split = %+v, want winHigh=1", d)
	}
}

func TestDeltasForTiedHandCountsBothSidesAsATie(t *testing.T) {
	outcome := hand.HandOutcome{
		Winners:       []string{"a", "z"},
		Participants:  []string{"a", "z"},
		Payouts:       map[string]int64{"a": 95, "z": 95},
		Contributions: map[string]int64{"a": 100, "z": 100},
	}
	deltas := deltasFor("sandbox", outcome)
	if len(deltas) != 1 || deltas[0].ties != 1 || deltas[0].winsLow != 0 || deltas[0].winsHigh != 0 {
		t.Fatalf("split-pot deltas = %+v, want a single tie", deltas)
	}
}

func TestDeltasForSkipsEmptyAndSelfPairs(t *testing.T) {
	outcome := hand.HandOutcome{Participants: []string{"a", "", "a"}}
	if deltas := deltasFor("sandbox", outcome); len(deltas) != 0 {
		t.Fatalf("expected no pairs from an empty/self participant list, got %+v", deltas)
	}
}
```

- [ ] **Step 3: Run the tests, verify they fail with "undefined: pairKey" etc if written before Step 1, or pass if Step
  1 already applied**

Run: `cd api && go test ./internal/matchup/... -run . -v`
Expected: PASS (Step 1's implementation and Step 2's tests are written together above; run this to confirm before
committing).

- [ ] **Step 4: Commit**

```bash
git add api/internal/matchup/store.go api/internal/matchup/store_test.go
git commit -m "feat(matchup): add head-to-head pair aggregate store"
```

---

## Task 2: `internal/matchup` integration test (DynamoDB Local)

**Files:**

- Create: `api/internal/matchup/store_integration_test.go`

**Interfaces:**

- Consumes: `matchup.NewStore`, `matchup.Store.RecordHand`, `matchup.Store.Get` (Task 1). Same DynamoDB Local endpoint
  (`http://localhost:8555`) and `dummy`/`dummy` static credentials `internal/pokerstats/store_integration_test.go`
  already uses.

- [ ] **Step 1: Write the integration test**

```go
//go:build integration

package matchup

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func TestRecordHandIsIdempotentAndGetIsSymmetric(t *testing.T) {
	db := matchupTestClient(t)
	env := fmt.Sprintf("matchup_test_%d", time.Now().UnixNano())
	_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: aws.String(env + "_" + tableMatchups), BillingMode: types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db, env)
	outcome := hand.HandOutcome{
		Winners:       []string{"z"},
		Participants:  []string{"a", "z"},
		Payouts:       map[string]int64{"z": 190},
		Contributions: map[string]int64{"a": 100, "z": 100},
	}
	for range 2 {
		if err := store.RecordHand(context.Background(), "sandbox", "table-1", "hand-1", outcome); err != nil {
			t.Fatal(err)
		}
	}
	fromA, err := store.Get(context.Background(), "sandbox", "a", "z")
	if err != nil || fromA.Stats.HandsTogether != 1 || fromA.Stats.HeadsUpHandsTogether != 1 ||
		fromA.Stats.WinsHigh != 1 || fromA.Stats.NetChangeLow != -100 || fromA.Stats.NetChangeHigh != 90 {
		t.Fatalf("fromA=%+v err=%v", fromA, err)
	}
	fromZ, err := store.Get(context.Background(), "sandbox", "z", "a")
	if err != nil || fromZ.IDLow != "a" || fromZ.IDHigh != "z" || fromZ.Stats.HandsTogether != 1 {
		t.Fatalf("fromZ=%+v err=%v", fromZ, err)
	}
	empty, err := store.Get(context.Background(), "sandbox", "a", "nobody")
	if err != nil || empty.Stats.HandsTogether != 0 {
		t.Fatalf("expected zeroed stats for an unseen pair, got %+v err=%v", empty, err)
	}
}

func matchupTestClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
	)
	if err != nil {
		t.Fatal(err)
	}
	return dynamodb.NewFromConfig(cfg, func(options *dynamodb.Options) {
		options.BaseEndpoint = aws.String("http://localhost:8555")
	})
}
```

- [ ] **Step 2: Run it against DynamoDB Local**

Run: `cd api && docker compose -f docker-compose.test.yml up -d && go test -tags integration ./internal/matchup/... -v`
Expected: PASS (`TestRecordHandIsIdempotentAndGetIsSymmetric`).

- [ ] **Step 3: Commit**

```bash
git add api/internal/matchup/store_integration_test.go
git commit -m "test(matchup): add DynamoDB Local idempotency/symmetry integration test"
```

---

## Task 3: CDK — `poker_player_matchups` table

**Files:**

- Modify: `cdk/lib/dynamodb-stack.ts:11-19` (TableName union), `cdk/lib/dynamodb-stack.ts:109-116` (table declarations)
- Modify: `cdk/test/dynamodb-stack.test.ts:11` (resource count), add a new test near
  `cdk/test/dynamodb-stack.test.ts:91-98`
- Modify: `cdk/CLAUDE.md` (table count doc)

**Interfaces:**

- Produces: physical table `<env>_poker_player_matchups` — PK-only (`pk`), TTL on `ttl` (guard rows), no GSI. Matches
  `internal/matchup/store.go`'s `tableMatchups = "poker_player_matchups"` exactly.

- [ ] **Step 1: Add the table name to the union type**

In `cdk/lib/dynamodb-stack.ts`, change:

```ts
  'poker_player_notes' | 'poker_hand_shares' | 'poker_player_poker_stats' | 'poker_sandbox_purchases' |
```

to:

```ts
  'poker_player_notes' | 'poker_hand_shares' | 'poker_player_poker_stats' | 'poker_player_matchups' |
'poker_sandbox_purchases' |
```

- [ ] **Step 2: Declare the table**

In `cdk/lib/dynamodb-stack.ts`, right after the existing:

```ts
    // One permanent private aggregate per player plus short-lived idempotency
    // guards for completed hands.
table('poker_player_poker_stats', false, true);
```

add:

```ts
    // poker_player_matchups: one permanent aggregate per unordered player
    // pair (pk "pair#<mode>#<idLow>#<idHigh>") plus short-lived per-pair
    // idempotency guards for completed hands (pk "guard#<table>#<hand>#pair#...") —
    // same PK-only, TTL'd shape as poker_player_poker_stats.
    // See docs/specs/2026-08-21-head-to-head-stats.md.
table('poker_player_matchups', false, true);
```

- [ ] **Step 3: Update the resource count and add a table-specific test**

In `cdk/test/dynamodb-stack.test.ts`, change:

```ts
  template.resourceCountIs('AWS::DynamoDB::GlobalTable', 26);
```

to:

```ts
  template.resourceCountIs('AWS::DynamoDB::GlobalTable', 27);
```

Then, right after the existing `'creates private poker stats with expiring hand guards'` test, add:

```ts
test('creates head-to-head matchup stats with expiring pair guards', () => {
    const app = new App();
    const stack = new DynamoDBStack(app, 'TestMatchupStack', {environment: 'dev'});
    Template.fromStack(stack).hasResourceProperties('AWS::DynamoDB::GlobalTable', {
        TableName: 'dev_poker_player_matchups',
        TimeToLiveSpecification: {AttributeName: 'ttl', Enabled: true},
    });
});
```

- [ ] **Step 4: Build and run the CDK tests**

Run: `cd cdk && npm run build && npx jest test/dynamodb-stack.test.ts`
Expected: PASS, including the new `'creates head-to-head matchup stats with expiring pair guards'` test.

- [ ] **Step 5: Update `cdk/CLAUDE.md`'s table count**

Change the line:

```
- **26 DynamoDB tables** (`dynamodb-stack.ts`) — an older revision of this file undercounted (15, before it, 8).
```

to:

```
- **27 DynamoDB tables** (`dynamodb-stack.ts`) — an older revision of this file undercounted (15, before it, 8, then 26).
```

- [ ] **Step 6: Commit**

```bash
git add cdk/lib/dynamodb-stack.ts cdk/test/dynamodb-stack.test.ts cdk/CLAUDE.md
git commit -m "feat(cdk): add poker_player_matchups table"
```

---

## Task 4: Write path — wire `matchup.Store.RecordHand` into `onHandComplete`

**Files:**

- Modify: `api/internal/app/app.go`

**Interfaces:**

- Consumes: `matchup.NewStore`, `matchup.Store.RecordHand` (Task 1).
- Produces: `*matchup.Store` as an Fx-provided type, threaded into `newTableManager` and (for the read path, Task 5)into
  `registerRoutesWithSocialRuntime` / `v1.Register`.

- [ ] **Step 1: Add the import**

In `api/internal/app/app.go`'s import block, between `"gopkg.aoctech.app/poker/api/internal/leaderboard"` and
`"gopkg.aoctech.app/poker/api/internal/player"`, add:

```go
    "gopkg.aoctech.app/poker/api/internal/matchup"
```

- [ ] **Step 2: Register the Fx provider**

In the `fx.Provide(...)` list, right after `newPokerStatsStore,`, add:

```go
            newMatchupStore,
```

Right after the `newPokerStatsStore` function definition:

```go
func newPokerStatsStore(db *dynamodb.Client, cfg *config.Config) *pokerstats.Store {
return pokerstats.NewStore(db, cfg.Env)
}
```

add:

```go
func newMatchupStore(db *dynamodb.Client, cfg *config.Config) *matchup.Store {
return matchup.NewStore(db, cfg.Env)
}
```

- [ ] **Step 3: Thread the store into `newTableManager` and call `RecordHand` from `onHandComplete`**

Change the `newTableManager` signature from:

```go
func newTableManager(leases *tablelease.Service, store *tablestore.Store, reg ws.Registry, achv *achievements.Service, leaderboardSvc *leaderboard.Service, rooms *roomstore.Store, sessionStore *sessionlog.Store, pokerStatsStore *pokerstats.Store, highlightsStore *highlights.Store, recentSvc *recentplayers.Service, players *player.Service, cfg *config.Config) *tablemanager.Manager {
```

to:

```go
func newTableManager(leases *tablelease.Service, store *tablestore.Store, reg ws.Registry, achv *achievements.Service, leaderboardSvc *leaderboard.Service, rooms *roomstore.Store, sessionStore *sessionlog.Store, pokerStatsStore *pokerstats.Store, matchupStore *matchup.Store, highlightsStore *highlights.Store, recentSvc *recentplayers.Service, players *player.Service, cfg *config.Config) *tablemanager.Manager {
```

Inside `onHandComplete`, right after the existing:

```go
        if metricsErr == nil {
if err := pokerStatsStore.RecordHand(ctx, mode, tableID, handID, metrics); err != nil {
slog.Error("pokerstats: record hand failed", "table", tableID, "hand", handID, "err", err)
}
}
```

add:

```go
        if err := matchupStore.RecordHand(ctx, mode, tableID, handID, outcome); err != nil {
slog.Error("matchup: record hand failed", "table", tableID, "hand", handID, "err", err)
}
```

(`matchupStore.RecordHand` takes the full `outcome`, not the pre-flop-only `metrics` — unlike `pokerstats`, it doesn't
need `metricsErr` to have succeeded.)

- [ ] **Step 4: Thread the store to the route registration seam**

Change `registerRoutesWithSocialRuntime`'s signature from:

```go
    pokerStatsStore *pokerstats.Store,
highlightsStore *highlights.Store,
```

to:

```go
    pokerStatsStore *pokerstats.Store,
matchupStore *matchup.Store,
highlightsStore *highlights.Store,
```

and its `v1.Register(...)` call from:

```go
    v1.Register(app, cfg, db, verifier, manager, reg, roomBackedSeed(rooms), cacheBackend, rooms, buyinSvc, players, leaderboardSvc, dailyRewardSvc, tableStore, sessionStore, achievementStore, playerNoteStore, handShareStore, pokerStatsStore, highlightsStore, avatars, sandboxPurchaseSvc, reactionPurchaseSvc, cosmeticPurchaseSvc, socialSvc, presenceSvc, recentSvc, reportSvc)
}

// registerRoutes retains the narrow construction seam used by older unit
```

to:

```go
    v1.Register(app, cfg, db, verifier, manager, reg, roomBackedSeed(rooms), cacheBackend, rooms, buyinSvc, players, leaderboardSvc, dailyRewardSvc, tableStore, sessionStore, achievementStore, playerNoteStore, handShareStore, pokerStatsStore, matchupStore, highlightsStore, avatars, sandboxPurchaseSvc, reactionPurchaseSvc, cosmeticPurchaseSvc, socialSvc, presenceSvc, recentSvc, reportSvc)
}

// registerRoutes retains the narrow construction seam used by older unit
```

Leave `registerRoutes` (the narrow test seam) untouched except its own `v1.Register(...)` call — change:

```go
    v1.Register(app, cfg, db, verifier, manager, reg, roomBackedSeed(rooms), cacheBackend, rooms, buyinSvc, players, leaderboardSvc, dailyRewardSvc, tableStore, sessionStore, achievementStore, playerNoteStore, handShareStore, pokerStatsStore, highlightsStore, avatars, sandboxPurchaseSvc, reactionPurchaseSvc, cosmeticPurchaseSvc, socialSvc, nil, nil, nil)
}
```

to:

```go
    v1.Register(app, cfg, db, verifier, manager, reg, roomBackedSeed(rooms), cacheBackend, rooms, buyinSvc, players, leaderboardSvc, dailyRewardSvc, tableStore, sessionStore, achievementStore, playerNoteStore, handShareStore, pokerStatsStore, nil, highlightsStore, avatars, sandboxPurchaseSvc, reactionPurchaseSvc, cosmeticPurchaseSvc, socialSvc, nil, nil, nil)
}
```

(`registerRoutes` doesn't gain a `matchupStore` parameter of its own — it already hardcodes `nil` for `presenceSvc`/
`recentSvc`/`reportSvc` the same way; `nil` for `*matchup.Store` is the same pattern, and `api/internal/app/app_test.go`
's existing `registerRoutes(...)` call needs no change since `registerRoutes`'s own signature didn't change.)

This step won't compile until Task 5 adds the `matchupStore *matchup.Store` parameter to `v1.Register`'s signature — do
Task 5 before running `go build` here, or do both in the same commit.

- [ ] **Step 5: Build**

Run: `cd api && go build ./...`
Expected: succeeds once Task 5's `v1.Register` signature change is also applied.

- [ ] **Step 6: Commit** (combine with Task 5's commit, since the two are mutually dependent for compilation)

---

## Task 5: Read path — `GET /players/me/matchups/:opponentId`

**Files:**

- Create: `api/internal/api/v1/matchups.go`
- Test: `api/internal/api/v1/matchups_test.go`
- Modify: `api/internal/api/v1/router.go`
- Modify: `api/CLAUDE.md`

**Interfaces:**

- Consumes: `matchup.PairStats`, `matchup.Stats` (Task 1).
- Produces: `v1.RegisterMatchups(router fiber.Router, auth fiber.Handler, store matchupReader)`, wired into`v1.Register`
  's new `matchupStore *matchup.Store` parameter (completes Task 4 Step 4).

- [ ] **Step 1: Write the handler**

```go
package v1

import (
	"context"
	"net/url"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/matchup"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

type matchupReader interface {
	Get(ctx context.Context, mode, playerA, playerB string) (matchup.PairStats, error)
}

// RegisterMatchups mounts the read-only head-to-head comparator. The path
// param names the opponent only — viewer identity always comes from the JWT
// (localsUserID), never the client — same IDOR-safe pattern as
// RegisterPlayerNotes's save handler.
func RegisterMatchups(router fiber.Router, auth fiber.Handler, store matchupReader) {
	router.Get("/players/me/matchups/:opponentId", auth, func(c fiber.Ctx) error {
		viewerID := c.Locals(localsUserID).(string)
		opponentID, err := url.PathUnescape(c.Params("opponentId"))
		if err != nil || opponentID == "" || opponentID == viewerID {
			return problem.BadRequest("opponent id is invalid").Send(c)
		}
		pair, err := store.Get(c.Context(), currencyModeParam(c), viewerID, opponentID)
		if err != nil {
			return problem.InternalServer("failed to load matchup stats", c, err).Send(c)
		}
		// pair.Stats is always idLow/idHigh-relative; remap to
		// viewer/opponent here, once, rather than in storage.
		viewerWins, opponentWins, netChangeViewer := pair.Stats.WinsLow, pair.Stats.WinsHigh, pair.Stats.NetChangeLow
		if viewerID != pair.IDLow {
			viewerWins, opponentWins, netChangeViewer = pair.Stats.WinsHigh, pair.Stats.WinsLow, pair.Stats.NetChangeHigh
		}
		return c.JSON(fiber.Map{
			"hands_together":          pair.Stats.HandsTogether,
			"viewer_wins":             viewerWins,
			"opponent_wins":           opponentWins,
			"ties":                    pair.Stats.Ties,
			"heads_up_hands_together": pair.Stats.HeadsUpHandsTogether,
			"net_change_viewer":       netChangeViewer,
		})
	})
}
```

- [ ] **Step 2: Write the handler test**

```go
package v1

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/matchup"
)

type fakeMatchupStore struct {
	stats matchup.PairStats
	err   error
}

func (f *fakeMatchupStore) Get(_ context.Context, _, _, _ string) (matchup.PairStats, error) {
	return f.stats, f.err
}

func newMatchupsTestApp(store *fakeMatchupStore) *fiber.App {
	app := fiber.New()
	auth := func(c fiber.Ctx) error { c.Locals(localsUserID, "player-1"); return c.Next() }
	RegisterMatchups(app.Group("/v1.0"), auth, store)
	return app
}

func TestMatchups_ZeroedStatsWhenPairHasNoHistory(t *testing.T) {
	app := newMatchupsTestApp(&fakeMatchupStore{stats: matchup.PairStats{IDLow: "player-1", IDHigh: "player-2"}})
	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/v1.0/players/me/matchups/player-2", nil))
	if err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200 for a pair with no shared history, got %d err %v", resp.StatusCode, err)
	}
}

func TestMatchups_RemapsStatsRelativeToViewer(t *testing.T) {
	// viewer is "player-1", which sorts as IDHigh against "player-0" —
	// WinsHigh/NetChangeHigh must surface as the viewer's own numbers.
	app := newMatchupsTestApp(&fakeMatchupStore{stats: matchup.PairStats{
		IDLow: "player-0", IDHigh: "player-1",
		Stats: matchup.Stats{
			HandsTogether: 5, WinsLow: 1, WinsHigh: 4, Ties: 0,
			HeadsUpHandsTogether: 3, NetChangeLow: -300, NetChangeHigh: 250,
		},
	}})
	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/v1.0/players/me/matchups/player-0", nil))
	if err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d err %v", resp.StatusCode, err)
	}
	var body struct {
		ViewerWins      int64 `json:"viewer_wins"`
		OpponentWins    int64 `json:"opponent_wins"`
		NetChangeViewer int64 `json:"net_change_viewer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.ViewerWins != 4 || body.OpponentWins != 1 || body.NetChangeViewer != 250 {
		t.Fatalf("remap = %+v", body)
	}
}

func TestMatchups_RejectsSelfAsOpponent(t *testing.T) {
	app := newMatchupsTestApp(&fakeMatchupStore{})
	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/v1.0/players/me/matchups/player-1", nil))
	if err != nil || resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400 for opponent==viewer, got %d err %v", resp.StatusCode, err)
	}
}
```

- [ ] **Step 3: Wire `RegisterMatchups` and thread `matchupStore` through `router.go`**

Add the import in `api/internal/api/v1/router.go`, between `"gopkg.aoctech.app/poker/api/internal/leaderboard"` and
`"gopkg.aoctech.app/poker/api/internal/oauthresource"`:

```go
    "gopkg.aoctech.app/poker/api/internal/matchup"
```

Change `Register`'s signature — right after `pokerStatsStore *pokerstats.Store,` add:

```go
    matchupStore *matchup.Store,
```

Right after the existing:

```go
    RegisterPokerStats(router, auth, pokerStatsStore)
```

add:

```go
    RegisterMatchups(router, auth, matchupStore)
```

- [ ] **Step 4: Build and test**

Run: `cd api && go build ./... && go test ./internal/api/v1/... ./internal/app/... ./internal/matchup/... -v`
Expected: PASS, including the three new `TestMatchups_*` tests. This also completes Task 4's build (the `v1.Register`
signature now has the `matchupStore` parameter both call sites in `app.go` were updated for).

- [ ] **Step 5: Update `api/CLAUDE.md`'s Layout line**

Change:

```
`internal/{player,playernotes,pokerstats,sessionlog,handshare,highlights}` (player-scoped data) ·
```

to:

```
`internal/{player,playernotes,pokerstats,matchup,sessionlog,handshare,highlights}` (player-scoped data) ·
```

- [ ] **Step 6: Commit (covers Task 4 + Task 5 together, since they're mutually dependent for compilation)**

```bash
git add api/internal/app/app.go api/internal/api/v1/matchups.go api/internal/api/v1/matchups_test.go api/internal/api/v1/router.go api/CLAUDE.md
git commit -m "feat(api): wire head-to-head matchup write and read paths"
```

---

## Task 6: UI — head-to-head card on the public profile showcase

**Files:**

- Modify: `ui/src/lib/api/player.ts`
- Modify: `ui/src/app/profile/page.tsx`
- Modify: `ui/src/app/globals.css`
- Modify: `ui/src/app/profile/page.test.tsx`
- Modify: `ui/CLAUDE.md`

**Interfaces:**

- Consumes: `GET /v1.0/players/me/matchups/:opponentId` (Task 5), response shape
  `{hands_together, viewer_wins, opponent_wins, ties, heads_up_hands_together, net_change_viewer}` (all `number`).
- Produces: `MatchupStats` (exported type), `getMatchupStats(opponentId: string): Promise<MatchupStats>` in
  `lib/api/player.ts`.

- [ ] **Step 1: Add the API client function**

In `ui/src/lib/api/player.ts`, right after the existing `getProfileShowcase` function:

```ts
export interface MatchupStats {
    hands_together: number;
    viewer_wins: number;
    opponent_wins: number;
    ties: number;
    heads_up_hands_together: number;
    net_change_viewer: number;
}

export async function getMatchupStats(opponentId: string) {
    return (await apiClient.get<MatchupStats>(
        `/v1.0/players/me/matchups/${encodeURIComponent(opponentId)}`,
        {silentError: true}
    )).data;
}
```

- [ ] **Step 2: Add the query and card to the profile page**

In `ui/src/app/profile/page.tsx`, add `Swords` to the `lucide-react` import:

```tsx
import {ChevronLeft, Sparkles, Swords, Trophy} from 'lucide-react';
```

Add `getMatchupStats` and the `MatchupStats` type to the existing import:

```tsx
import {getMatchupStats, getProfileShowcase} from '@/lib/api/player';
```

Right after the existing `relationship` query, add:

```tsx
  const matchup = useQuery({
    queryKey: ['profile-matchup', playerID],
    queryFn: () => getMatchupStats(playerID),
    enabled: authed && Boolean(playerID),
    retry: false
});
```

Inside `.profile-showcase-content`, right after the closing `</section>` of the "Melhor Vitória Recente" section (and
still inside the outer `<div className="profile-showcase-content">`), add:

```tsx
            {
    matchup.data && matchup.data.hands_together > 0 && <section className="profile-matchup">
        <h2><Swords aria-hidden="true"/> Cara a Cara</h2>
        <p>
            Vocês já jogaram {matchup.data.hands_together.toLocaleString('pt-BR')} mãos juntos,
            {' '}você venceu {matchup.data.viewer_wins.toLocaleString('pt-BR')},{' '}
            {showcase.data.name || 'Jogador'} venceu {matchup.data.opponent_wins.toLocaleString('pt-BR')}.
        </p>
    </section>
}
```

- [ ] **Step 3: Add the card's CSS**

In `ui/src/app/globals.css`, right after the existing `.profile-best-hand b { color: var(--success) }` rule (and before
the `@media (max-width: 640px)` block for `.profile-showcase-content`), add:

```css
.profile-matchup {
    grid-column: 1 / -1;
    padding: 16px;
    border-radius: var(--rounded-card);
    background: var(--surface-seat)
}

.profile-matchup p {
    margin: 0;
    font-size: 13px
}
```

- [ ] **Step 4: Update the test mock and add coverage**

In `ui/src/app/profile/page.test.tsx`, add `matchupQuery` to the hoisted mocks:

```tsx
const mocks = vi.hoisted(() => ({
    playerID: 'player-42',
    session: {authed: true, checking: false},
    query: {} as Record<string, unknown>,
    relationshipQuery: {} as Record<string, unknown>,
    matchupQuery: {} as Record<string, unknown>,
    queryOptions: undefined as unknown,
    invalidateQueries: vi.fn(),
}));
```

Change the `useQuery` mock from:

```tsx
vi.mock('@tanstack/react-query', () => ({
    useQuery: (options: unknown) => {
        if ((options as { queryKey: string[] }).queryKey[1] === 'relationship') return mocks.relationshipQuery;
        mocks.queryOptions = options;
        return mocks.query;
    },
    useQueryClient: () => ({invalidateQueries: mocks.invalidateQueries}),
}));
```

to:

```tsx
vi.mock('@tanstack/react-query', () => ({
    useQuery: (options: unknown) => {
        const key = (options as { queryKey: string[] }).queryKey;
        if (key[1] === 'relationship') return mocks.relationshipQuery;
        if (key[0] === 'profile-matchup') return mocks.matchupQuery;
        mocks.queryOptions = options;
        return mocks.query;
    },
    useQueryClient: () => ({invalidateQueries: mocks.invalidateQueries}),
}));
```

In `beforeEach`, right after `mocks.relationshipQuery = {data: undefined, isLoading: false, isError: true};`, add:

```tsx
    mocks.matchupQuery = {data: undefined, isLoading: false, isError: true};
```

Add two new tests at the end of the `describe` block, right before its closing `});`:

```tsx
  test('shows the head-to-head card once the pair has shared hands', () => {
    mocks.matchupQuery = {
        data: {
            hands_together: 12,
            viewer_wins: 7,
            opponent_wins: 5,
            ties: 0,
            heads_up_hands_together: 4,
            net_change_viewer: 380
        },
        isLoading: false, isError: false,
    };
    render(<ProfilePage/>);
    expect(screen.getByText(/12 mãos juntos/)).toBeInTheDocument();
    expect(screen.getByText(/você venceu 7/)).toBeInTheDocument();
    expect(screen.getByText(/Ás da Mesa venceu 5/)).toBeInTheDocument();
});

test('hides the head-to-head card for a pair that never shared a table', () => {
    mocks.matchupQuery = {
        data: {
            hands_together: 0,
            viewer_wins: 0,
            opponent_wins: 0,
            ties: 0,
            heads_up_hands_together: 0,
            net_change_viewer: 0
        },
        isLoading: false, isError: false,
    };
    render(<ProfilePage/>);
    expect(screen.queryByText(/mãos juntos/)).not.toBeInTheDocument();
});
```

- [ ] **Step 5: Run the UI test suite, typecheck, and lint**

Run: `cd ui && npx vitest run src/app/profile/page.test.tsx`
Expected: PASS, including the two new tests.

Run: `cd ui && npx tsc --noEmit && npx eslint src/app/profile src/lib/api/player.ts --max-warnings 0`
Expected: zero errors, zero warnings.

- [ ] **Step 6: Manually verify in the browser**

Start the dev server (`cd ui && npm run dev`), sign in, open `/profile?id=<a-player-you-have-shared-a-table-with>`, and
confirm the "Cara a Cara" card renders with the expected counts; open`/profile?id=<a-player-you-have-never-played-with>`
and confirm no card renders (not even an empty one).

- [ ] **Step 7: Update `ui/CLAUDE.md`**

Right after the existing sentence:

```
Profile **editing** is `components/lobby/ProfileMenu.tsx` + `ProfileShowcaseDialog.tsx`, not
`app/profile/` — that route is the public read-only showcase of another player.
```

add:

```
The showcase also renders a "Cara a Cara" head-to-head card (`app/profile/page.tsx`) fetched from
`GET /v1.0/players/me/matchups/:id`, shown only when `hands_together > 0` — no empty-state card for
pairs that have never shared a table.
```

- [ ] **Step 8: Commit**

```bash
git add ui/src/lib/api/player.ts ui/src/app/profile/page.tsx ui/src/app/globals.css ui/src/app/profile/page.test.tsx ui/CLAUDE.md
git commit -m "feat(ui): show a head-to-head card on the public profile showcase"
```

---

## Post-implementation checklist

- [ ] `cd api && go test ./... -race` passes.
- [ ] `cd api && go test -tags integration -race ./internal/matchup/...` passes against DynamoDB Local.
- [ ] `cd cdk && npm run build && npx jest` passes.
- [ ] `cd ui && npx vitest run && npx tsc --noEmit && npx eslint src --max-warnings 0 && npm run build` all pass with
  zero errors/warnings.
- [ ] Every doc touched (`api/CLAUDE.md`, `cdk/CLAUDE.md`, `ui/CLAUDE.md`) reflects the new table, endpoint, and card.
