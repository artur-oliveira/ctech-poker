# Pay To See Winner's Cards (History) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a player buy a peek at the winner's hole cards for a sandbox hand that already ended without a showdown, from hand history/replay — not just live at the table (that's the prerequisite live spec, already shipped).

**Architecture:** A new leaf package `handreveal` owns two new DynamoDB tables: `poker_hand_reveals` (one immutable-ish record per eligible hand, written by the same post-hand hooks that already write `sessionlog.HandItem`) and `poker_hand_reveal_payments` (one row per buyer per hand, tracking the purchase). A new authenticated REST endpoint pair (`POST`/`GET /players/me/hands/:handId/reveal-winner`) debits the buyer and credits the winner through the existing wallet client (money never touches a local DynamoDB transaction — sandbox balance lives in ctech-wallet, not in this service), then returns the winner's true hole cards to the buyer only. The frontend replay view gets a buy button mirroring the live table's `WinnerCards` component, wired to the new REST calls instead of a WS command.

**Tech Stack:** Go (Fiber v3, AWS SDK v2 DynamoDB, `gopkg.aoctech.app/api-commons/dynamo`), AWS CDK (TypeScript), Next.js/React (TanStack Query, vitest).

**Spec:** `docs/specs/2026-08-21-pay-to-see-winner-cards-history.md` (hard-dependent on `docs/specs/2026-08-21-pay-to-see-winner-cards-live.md`, already shipped — verified: `Table.RequestWinnerCards`/`RequestWinnerCardsCmd`/`request_winner_cards` WS command and `WinnerCards.tsx` all exist in code today).

## Global Constraints

- Sandbox tables only. The archive row is only ever written when `outcome.WonWithoutShowdown` is true, exactly one winner exists, and the table's `currency_mode == "sandbox"` — never for real-money hands, structurally (see Task 3).
- Never touch `sessionlog.HandItem`, `hand-shares`, or their existing write-time redaction. This feature is a wholly separate store.
- No retroactive backfill for hands that completed before this ships.
- Fee = the hand's big blind (integer), same as the live spec. Winner gets `fee/2` (integer division), remainder is uncredited rake — identical split behavior to `Table.RequestWinnerCards` in `api/internal/engine/hand/hand.go:1082-1092`.
- Player identity for every new handler comes from the JWT `sub` (`c.Locals(localsUserID)`), never a client-supplied id (`api/CLAUDE.md`'s IDOR rule).
- Named constants, not magic strings, for table names/paths (`api/CLAUDE.md`, `cdk/CLAUDE.md`).
- DynamoDB: on-demand billing with explicit `maxRead/WriteRequestUnits` (`cdk/CLAUDE.md`); no table below 100 RCU/WCU-equivalent cap.
- Every code change ships with its documentation update in the same change (`api/CLAUDE.md`, `cdk/CLAUDE.md`, `ui/CLAUDE.md` "Mandatory Documentation Policy").

## Deviations from the literal spec text (read before implementing)

The spec is the design source of truth, but three of its details don't survive contact with how this codebase actually works. Each is called out again inline at the task that implements it:

1. **"One DynamoDB `TransactWriteItems`" for `PayForReveal` is not possible.** Sandbox balance is not a local DynamoDB item — it lives in the external `ctech-wallet` service, reached through `internal/walletclient` (confirmed: `buyin.Service.SandboxBalance` calls `walletclient.Client.Balances`; `dailyreward.Service.Spin` credits sandbox balance via `wallet.Credit`, a plain HTTP call, not a Dynamo transaction). `PayForReveal` instead does: claim a local idempotency row → `wallet.Debit` the buyer → `wallet.Credit` the winner half → mark the row complete. Every step carries a stable idempotency key, so a retry (client retry, or a request that failed partway) always resumes safely without double-charging — this mirrors `dailyreward`'s own debit/credit-via-wallet pattern, which also has no local transaction wrapping the wallet calls.
2. **`HandRecord.PK` cannot be `table_id#hand_id`.** The buy/check endpoints only receive `:handId` in the URL — there is no way to construct that key without already knowing `table_id`, which isn't known yet. Hand IDs are already globally unique (confirmed: `sessionlog.Store.GetHand(ctx, playerID, mode, handID)` looks up a hand by `handID` alone, no `tableID` needed). `HandRecord`'s Dynamo key is `pk = hand_id` (no sort key); `table_id` is kept as a plain attribute for reference only.
3. **No TTL on `poker_hand_reveals` or `poker_hand_reveal_payments`.** The spec says the archive's TTL "matches `poker_player_hands` retention" — but `poker_player_hands` itself has no TTL (`cdk/lib/dynamodb-stack.ts:190`, `table('poker_player_hands', true)` — no third `withTTL` argument). Match the real retention: no TTL on either new table, same as `poker_sandbox_purchases`/`poker_player_matchups` (permanent-record tables).
4. **The `GET` endpoint must return the fee even before purchase.** The spec's frontend section says the buy button's copy needs the fee amount, but `sessionlog.HandItem` has no big-blind field (and Deviation constraint #2 above forbids adding one). So `GET /players/me/hands/:handId/reveal-winner` returns `{"fee": <int>, "already_paid": <bool>, "cards": [...] }` (cards present only once paid) instead of a bare 404-until-paid — 404 is reserved for "no archive exists at all" (showdown hand, real-money hand, multi-way split, or pre-feature hand). This is a strictly additive response shape; nothing in the spec's `POST` contract changes.
5. **No `readscopes.go` change needed.** `enforceReadOnlyScope` (`api/internal/api/v1/readscopes.go:57-63`) already rejects any non-`GET` request from a non-first-party session outright, so `POST .../reveal-winner` is already gated. `requiredReadScope`'s existing `/v1.0/players/me/hands` prefix match (`readscopes.go:88`) already covers the new `GET .../reveal-winner` path too. Verify this with a quick grep before starting Task 5, don't just trust this plan.

---

## File Structure

- `api/internal/handreveal/store.go` — `HandRecord`, `PlayerHandCode`, `Store` (table `poker_hand_reveals`).
- `api/internal/handreveal/payments.go` — `PaymentStore` (table `poker_hand_reveal_payments`), `StatusPending`/`StatusCompleted`.
- `api/internal/handreveal/service.go` — `Service` (wallet debit/credit + payment bookkeeping).
- `api/internal/handreveal/store_integration_test.go` — DynamoDB Local test for `Store`/`PaymentStore`.
- `api/internal/handreveal/service_test.go` — unit test for `Service.PayForReveal`/`HasPaid` with fakes.
- `api/internal/api/v1/handreveal.go` — `RegisterHandReveal`, handlers.
- `api/internal/api/v1/handreveal_test.go` — handler route tests with fakes.
- Modify: `api/internal/app/app.go` — provider wiring + `persistHandReveal` hook in `newTableManager`.
- Modify: `api/internal/api/v1/router.go` — route registration + fx param plumbing.
- Modify: `cdk/lib/dynamodb-stack.ts` — two new tables.
- Modify: `ui/src/lib/api/player.ts` — `getHandRevealWinner`/`revealHandWinner` + types.
- Create: `ui/src/components/hands/RevealWinnerButton.tsx` + test.
- Modify: `ui/src/components/hands/HandReplayer.tsx` — wire the button in.
- Modify: `api/CLAUDE.md`, `cdk/CLAUDE.md` — mandatory doc updates.

---

## Task 1: CDK — two new DynamoDB tables

**Files:**
- Modify: `cdk/lib/dynamodb-stack.ts`

**Interfaces:**
- Produces: DynamoDB tables `poker_hand_reveals` (pk `pk` string, no sort key, no TTL) and `poker_hand_reveal_payments` (pk `pk` string, no sort key, no TTL), both on-demand with `maxReadRequestUnits`/`maxWriteRequestUnits` 1000 (inherited from the shared `table()` helper).

- [ ] **Step 1: Add both table names to the `TableName` union type**

In `cdk/lib/dynamodb-stack.ts`, extend the union (around line 11-20):

```ts
export type TableName =
  'poker_table_state' | 'poker_table_state_history' | 'poker_action_log' | 'poker_action_guards' |
  'poker_rooms' | 'poker_player_profiles' | 'poker_achievement_progress' | 'poker_leaderboard_stats' |
  'poker_daily_reward' | 'poker_pending_cashouts' | 'poker_player_sessions' | 'poker_player_hands' |
  'poker_player_notes' | 'poker_hand_shares' | 'poker_player_poker_stats' | 'poker_player_matchups' |
  'poker_sandbox_purchases' |
  'poker_reaction_entitlements' | 'poker_reaction_purchases' |
  'poker_cosmetic_entitlements' | 'poker_cosmetic_purchases' |
  'poker_table_entitlements' | 'poker_table_highlights' |
  'poker_hand_reveals' | 'poker_hand_reveal_payments' |
  (typeof DYNAMO_TABLE)[keyof typeof DYNAMO_TABLE];
```

- [ ] **Step 2: Add the two tables next to `poker_hand_shares`**

Right after the existing `table('poker_hand_shares', false, true);` line (around line 109), add:

```ts
    // poker_hand_reveals: one permanent row per sandbox hand that ended
    // without a showdown with exactly one winner (pk = hand_id — globally
    // unique already, so the buy/check endpoints never need the table id to
    // look a hand up). Holds every participant's true hole cards regardless
    // of whether they were ever shown, gated entirely by the paid-reveal
    // endpoint that is the only reader of this table — sessionlog.HandItem
    // and hand-shares are untouched and keep their existing write-time
    // redaction as their only guarantee. No TTL: matches poker_player_hands'
    // real (permanent) retention, not a TTL'd table.
    // See docs/specs/2026-08-21-pay-to-see-winner-cards-history.md.
    table('poker_hand_reveals', false);
    // poker_hand_reveal_payments: one permanent row per (hand, buyer) pair
    // recording a paid reveal purchase — kept in its own table so a payment
    // write never races the poker_hand_reveals write (that one is written
    // once, by the hand-complete/hand-updated hooks, and never touched
    // again). No TTL, mirrors poker_sandbox_purchases' permanent history.
    table('poker_hand_reveal_payments', false);
```

- [ ] **Step 3: Build and typecheck**

Run: `cd cdk && npm run build`
Expected: compiles with no errors.

- [ ] **Step 4: Update `cdk/CLAUDE.md`'s table count**

Change `**27 DynamoDB tables**` to `**29 DynamoDB tables**` in `cdk/CLAUDE.md`'s "Architecture facts" section, and add one line noting the two new tables (mirror the existing terse style of that bullet list).

- [ ] **Step 5: Commit**

```bash
git add cdk/lib/dynamodb-stack.ts cdk/CLAUDE.md
git commit -m "feat(cdk): add poker_hand_reveals and poker_hand_reveal_payments tables"
```

---

## Task 2: `handreveal` package — `HandRecord` store

**Files:**
- Create: `api/internal/handreveal/store.go`
- Create: `api/internal/handreveal/store_integration_test.go`

**Interfaces:**
- Produces:
  - `type PlayerHandCode struct { Cards [2]string }`
  - `type HandRecord struct { HandID, TableID, WinnerID string; BigBlind int64; WinnerShown bool; PlayerHands map[string]PlayerHandCode; EndedAt int64 }`
  - `func NewStore(db *dynamodb.Client, env string) *Store`
  - `func (s *Store) Put(ctx context.Context, record HandRecord) error`
  - `func (s *Store) Get(ctx context.Context, handID string) (*HandRecord, error)`

- [ ] **Step 1: Write `store.go`**

```go
// Package handreveal implements the paid history reveal of an
// uncontested winner's hole cards for a hand that already ended and was
// archived — the history counterpart to the live table's
// Table.RequestWinnerCards (engine/hand). See
// docs/specs/2026-08-21-pay-to-see-winner-cards-history.md.
package handreveal

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const tableHandReveals = "poker_hand_reveals"

// PlayerHandCode is one participant's two hole cards in "Ah"/"Tc" notation,
// matching hand.PlayerHandInfo.HoleCards' wire format.
type PlayerHandCode struct {
	Cards [2]string `dynamodbav:"cards"`
}

// HandRecord is the one-per-hand archive of a sandbox hand that ended
// without a showdown with exactly one winner — written once by the
// hand-complete hook and refreshed by the hand-updated hook if the winner
// later voluntarily shows (see api/internal/app/app.go's persistHandReveal).
// Unlike sessionlog.HandItem, this holds every participant's TRUE hole
// cards regardless of whether they were ever shown — nothing except the
// paid-reveal endpoint (api/internal/api/v1/handreveal.go) ever reads it.
type HandRecord struct {
	// PK is hand_id alone, not "table_id#hand_id": hand IDs are already
	// globally unique (sessionlog.Store.GetHand looks one up with no table
	// id), and the buy/check endpoints only ever have :handId from the URL.
	PK          string                    `dynamodbav:"pk"`
	TableID     string                    `dynamodbav:"table_id"`
	HandID      string                    `dynamodbav:"hand_id"`
	BigBlind    int64                     `dynamodbav:"big_blind"`
	WinnerID    string                    `dynamodbav:"winner_id"`
	WinnerShown bool                      `dynamodbav:"winner_shown"`
	PlayerHands map[string]PlayerHandCode `dynamodbav:"player_hands"`
	EndedAt     int64                     `dynamodbav:"ended_at"`
}

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tableHandReveals)}
}

// Put upserts record, keyed by record.HandID. Called from both the
// hand-complete hook (first write) and the hand-updated hook (refresh
// WinnerShown if the winner later voluntarily shows) — safe to call
// repeatedly with the same HandID.
func (s *Store) Put(ctx context.Context, record HandRecord) error {
	record.PK = record.HandID
	item, err := dynamo.Encode(record)
	if err != nil {
		return fmt.Errorf("handreveal: encode: %w", err)
	}
	if err := s.base.PutItem(ctx, item); err != nil {
		return fmt.Errorf("handreveal: put: %w", err)
	}
	return nil
}

// Get returns nil, nil if no archive exists for handID — a showdown hand, a
// real-money hand, a 2+-winner split, or a hand that predates this feature
// all look identical: nothing was ever written.
func (s *Store) Get(ctx context.Context, handID string) (*HandRecord, error) {
	item, err := s.base.GetItem(ctx, handID)
	if err != nil {
		return nil, fmt.Errorf("handreveal: get: %w", err)
	}
	if item == nil {
		return nil, nil
	}
	record, err := dynamo.Decode[HandRecord](item)
	if err != nil {
		return nil, fmt.Errorf("handreveal: decode: %w", err)
	}
	return record, nil
}
```

- [ ] **Step 2: Write the integration test**

```go
//go:build integration

package handreveal

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
)

func handRevealTestClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
	)
	if err != nil {
		t.Fatal(err)
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String("http://localhost:8000")
	})
}

func createHandRevealsTable(t *testing.T, db *dynamodb.Client, env string) {
	t.Helper()
	_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: aws.String(env + "_" + tableHandReveals), BillingMode: types.BillingModePayPerRequest,
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
}

func TestStorePutThenGetRoundTrips(t *testing.T) {
	db := handRevealTestClient(t)
	env := fmt.Sprintf("handreveal_test_%d", time.Now().UnixNano())
	createHandRevealsTable(t, db, env)
	store := NewStore(db, env)

	record := HandRecord{
		HandID: "hand-1", TableID: "table-1", BigBlind: 200, WinnerID: "winner",
		PlayerHands: map[string]PlayerHandCode{
			"winner": {Cards: [2]string{"Ah", "Kd"}},
			"loser":  {Cards: [2]string{"2c", "7s"}},
		},
		EndedAt: 1000,
	}
	if err := store.Put(context.Background(), record); err != nil {
		t.Fatal(err)
	}

	got, err := store.Get(context.Background(), "hand-1")
	if err != nil || got == nil {
		t.Fatalf("get: %+v err=%v", got, err)
	}
	if got.WinnerID != "winner" || got.BigBlind != 200 || got.WinnerShown {
		t.Fatalf("unexpected record: %+v", got)
	}
	if got.PlayerHands["winner"].Cards != [2]string{"Ah", "Kd"} {
		t.Fatalf("winner cards not round-tripped: %+v", got.PlayerHands)
	}
}

func TestStoreGetMissingHandReturnsNilNil(t *testing.T) {
	db := handRevealTestClient(t)
	env := fmt.Sprintf("handreveal_test_%d", time.Now().UnixNano())
	createHandRevealsTable(t, db, env)
	store := NewStore(db, env)

	got, err := store.Get(context.Background(), "no-such-hand")
	if err != nil || got != nil {
		t.Fatalf("expected nil, nil for a missing hand, got %+v err=%v", got, err)
	}
}

func TestStorePutTwiceUpdatesWinnerShown(t *testing.T) {
	db := handRevealTestClient(t)
	env := fmt.Sprintf("handreveal_test_%d", time.Now().UnixNano())
	createHandRevealsTable(t, db, env)
	store := NewStore(db, env)

	base := HandRecord{
		HandID: "hand-2", TableID: "table-1", BigBlind: 200, WinnerID: "winner",
		PlayerHands: map[string]PlayerHandCode{"winner": {Cards: [2]string{"Ah", "Kd"}}},
	}
	if err := store.Put(context.Background(), base); err != nil {
		t.Fatal(err)
	}
	base.WinnerShown = true
	if err := store.Put(context.Background(), base); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), "hand-2")
	if err != nil || got == nil || !got.WinnerShown {
		t.Fatalf("expected WinnerShown=true after the second Put, got %+v err=%v", got, err)
	}
}
```

- [ ] **Step 3: Start DynamoDB Local and run the test**

Run: `cd api && docker compose -f docker-compose.test.yml up -d && go test -tags integration ./internal/handreveal/... -run TestStore -v`
Expected: all three tests PASS.

- [ ] **Step 4: Commit**

```bash
git add api/internal/handreveal/store.go api/internal/handreveal/store_integration_test.go
git commit -m "feat(handreveal): add HandRecord archive store"
```

---

## Task 3: `handreveal` package — payment store

**Files:**
- Create: `api/internal/handreveal/payments.go`
- Modify: `api/internal/handreveal/store_integration_test.go` (add payment tests, same file, same build tag)

**Interfaces:**
- Consumes: nothing outside this package.
- Produces:
  - `const StatusPending = "pending"`, `const StatusCompleted = "completed"`
  - `func NewPaymentStore(db *dynamodb.Client, env string) *PaymentStore`
  - `func (s *PaymentStore) ClaimPayment(ctx context.Context, handID, viewerID string) (status string, err error)`
  - `func (s *PaymentStore) CompletePayment(ctx context.Context, handID, viewerID string) error`
  - `func (s *PaymentStore) HasPaid(ctx context.Context, handID, viewerID string) (bool, error)`

- [ ] **Step 1: Write `payments.go`**

```go
package handreveal

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const tableHandRevealPayments = "poker_hand_reveal_payments"

const (
	StatusPending   = "pending"
	StatusCompleted = "completed"
)

type paymentItem struct {
	PK     string `dynamodbav:"pk"` // hand_id#viewer_id
	Status string `dynamodbav:"status"`
}

func paymentKey(handID, viewerID string) string { return handID + "#" + viewerID }

type PaymentStore struct{ base dynamo.Base }

func NewPaymentStore(db *dynamodb.Client, env string) *PaymentStore {
	return &PaymentStore{base: dynamo.NewBase(db, env, tableHandRevealPayments)}
}

// ClaimPayment conditionally creates a pending payment row for (handID,
// viewerID) and returns its status. A retried call (network retry, or a
// caller resuming after a wallet failure partway through Service.PayForReveal)
// finds the existing row instead of creating a second one and returns its
// current status — mirrors dailyreward.Store.Claim's idempotent-claim shape.
func (s *PaymentStore) ClaimPayment(ctx context.Context, handID, viewerID string) (string, error) {
	item, err := dynamo.Encode(paymentItem{PK: paymentKey(handID, viewerID), Status: StatusPending})
	if err != nil {
		return "", fmt.Errorf("handreveal: encode payment: %w", err)
	}
	err = s.base.TransactWrite(ctx, []types.TransactWriteItem{s.base.BuildPutTxItemIfAbsent(item)})
	if err == nil {
		return StatusPending, nil
	}
	if !dynamo.IsConditionFailed(err) {
		return "", fmt.Errorf("handreveal: claim payment: %w", err)
	}
	existing, err := s.base.GetItem(ctx, paymentKey(handID, viewerID))
	if err != nil {
		return "", fmt.Errorf("handreveal: load claimed payment: %w", err)
	}
	if existing == nil {
		return "", fmt.Errorf("handreveal: payment claim disappeared")
	}
	decoded, err := dynamo.Decode[paymentItem](existing)
	if err != nil {
		return "", fmt.Errorf("handreveal: decode payment: %w", err)
	}
	return decoded.Status, nil
}

func (s *PaymentStore) CompletePayment(ctx context.Context, handID, viewerID string) error {
	ok, err := s.base.UpdateItem(ctx, paymentKey(handID, viewerID), nil, map[string]any{"status": StatusCompleted})
	if err != nil {
		return fmt.Errorf("handreveal: complete payment: %w", err)
	}
	if !ok {
		return fmt.Errorf("handreveal: payment claim not found for %s#%s", handID, viewerID)
	}
	return nil
}

func (s *PaymentStore) HasPaid(ctx context.Context, handID, viewerID string) (bool, error) {
	item, err := s.base.GetItem(ctx, paymentKey(handID, viewerID))
	if err != nil {
		return false, fmt.Errorf("handreveal: has paid: %w", err)
	}
	if item == nil {
		return false, nil
	}
	decoded, err := dynamo.Decode[paymentItem](item)
	if err != nil {
		return false, fmt.Errorf("handreveal: decode payment: %w", err)
	}
	return decoded.Status == StatusCompleted, nil
}
```

- [ ] **Step 2: Append payment tests to `store_integration_test.go`**

Add a table creator and three tests to the same file from Task 2:

```go
func createHandRevealPaymentsTable(t *testing.T, db *dynamodb.Client, env string) {
	t.Helper()
	_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: aws.String(env + "_" + tableHandRevealPayments), BillingMode: types.BillingModePayPerRequest,
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
}

func TestClaimPaymentIsIdempotent(t *testing.T) {
	db := handRevealTestClient(t)
	env := fmt.Sprintf("handreveal_test_%d", time.Now().UnixNano())
	createHandRevealPaymentsTable(t, db, env)
	store := NewPaymentStore(db, env)

	first, err := store.ClaimPayment(context.Background(), "hand-1", "buyer")
	if err != nil || first != StatusPending {
		t.Fatalf("first claim: %q err=%v", first, err)
	}
	second, err := store.ClaimPayment(context.Background(), "hand-1", "buyer")
	if err != nil || second != StatusPending {
		t.Fatalf("second claim should return the existing pending row: %q err=%v", second, err)
	}
}

func TestCompletePaymentThenHasPaid(t *testing.T) {
	db := handRevealTestClient(t)
	env := fmt.Sprintf("handreveal_test_%d", time.Now().UnixNano())
	createHandRevealPaymentsTable(t, db, env)
	store := NewPaymentStore(db, env)

	if _, err := store.ClaimPayment(context.Background(), "hand-1", "buyer"); err != nil {
		t.Fatal(err)
	}
	paid, err := store.HasPaid(context.Background(), "hand-1", "buyer")
	if err != nil || paid {
		t.Fatalf("expected not yet paid, got paid=%v err=%v", paid, err)
	}
	if err := store.CompletePayment(context.Background(), "hand-1", "buyer"); err != nil {
		t.Fatal(err)
	}
	paid, err = store.HasPaid(context.Background(), "hand-1", "buyer")
	if err != nil || !paid {
		t.Fatalf("expected paid after CompletePayment, got paid=%v err=%v", paid, err)
	}
}

func TestHasPaidFalseForUnknownBuyer(t *testing.T) {
	db := handRevealTestClient(t)
	env := fmt.Sprintf("handreveal_test_%d", time.Now().UnixNano())
	createHandRevealPaymentsTable(t, db, env)
	store := NewPaymentStore(db, env)

	paid, err := store.HasPaid(context.Background(), "hand-1", "nobody")
	if err != nil || paid {
		t.Fatalf("expected false for an unclaimed payment, got paid=%v err=%v", paid, err)
	}
}
```

- [ ] **Step 3: Run all `handreveal` integration tests**

Run: `cd api && go test -tags integration ./internal/handreveal/... -v`
Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add api/internal/handreveal/payments.go api/internal/handreveal/store_integration_test.go
git commit -m "feat(handreveal): add idempotent payment claim store"
```

---

## Task 4: `handreveal` package — `Service.PayForReveal`

**Files:**
- Create: `api/internal/handreveal/service.go`
- Create: `api/internal/handreveal/service_test.go`

**Interfaces:**
- Consumes: `PaymentStore.ClaimPayment`/`CompletePayment`/`HasPaid` (Task 3).
- Produces:
  - `func NewService(wallet wallet, payments *PaymentStore) *Service`
  - `func (s *Service) HasPaid(ctx context.Context, handID, viewerID string) (bool, error)`
  - `func (s *Service) PayForReveal(ctx context.Context, buyerID, winnerID, handID string, fee int64) error`

- [ ] **Step 1: Write `service.go`**

```go
package handreveal

import (
	"context"
	"fmt"
)

// wallet is the narrow slice of walletclient.Client this package needs —
// mirrors dailyreward's `credit` interface so tests use a fake, not a real
// HTTP client.
type wallet interface {
	Debit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
	Credit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
}

const (
	reasonDebit  = "hand_reveal_history"
	reasonCredit = "hand_reveal_history_payout"
)

type Service struct {
	wallet   wallet
	payments *PaymentStore
}

func NewService(wallet wallet, payments *PaymentStore) *Service {
	return &Service{wallet: wallet, payments: payments}
}

func (s *Service) HasPaid(ctx context.Context, handID, viewerID string) (bool, error) {
	return s.payments.HasPaid(ctx, handID, viewerID)
}

// PayForReveal debits buyerID the full fee, credits winnerID half (integer
// division — the remainder is uncredited rake, same split as
// Table.RequestWinnerCards), and records the purchase. Idempotent per
// (handID, buyerID): ClaimPayment reuses an existing pending row instead of
// creating a second one, and both wallet calls carry a stable idempotency
// key, so a retried call after a partial failure (e.g. the debit succeeded
// but the credit didn't) resumes safely instead of double-charging the
// buyer or double-crediting the winner.
func (s *Service) PayForReveal(ctx context.Context, buyerID, winnerID, handID string, fee int64) error {
	status, err := s.payments.ClaimPayment(ctx, handID, buyerID)
	if err != nil {
		return fmt.Errorf("handreveal: claim payment: %w", err)
	}
	if status == StatusCompleted {
		return nil
	}
	debitKey := handID + "#" + buyerID + "#debit"
	if err := s.wallet.Debit(ctx, buyerID, fee, debitKey, reasonDebit); err != nil {
		return err
	}
	creditKey := handID + "#" + buyerID + "#credit"
	if err := s.wallet.Credit(ctx, winnerID, fee/2, creditKey, reasonCredit); err != nil {
		return err
	}
	return s.payments.CompletePayment(ctx, handID, buyerID)
}
```

- [ ] **Step 2: Write `service_test.go`**

```go
package handreveal

import (
	"context"
	"errors"
	"testing"
)

type fakeWallet struct {
	debits, credits         []int64
	debitKeys, creditKeys   []string
	failDebit, failCredit   bool
}

func (f *fakeWallet) Debit(_ context.Context, _ string, amount int64, key, _ string) error {
	f.debits = append(f.debits, amount)
	f.debitKeys = append(f.debitKeys, key)
	if f.failDebit {
		return errors.New("wallet: insufficient funds")
	}
	return nil
}

func (f *fakeWallet) Credit(_ context.Context, _ string, amount int64, key, _ string) error {
	f.credits = append(f.credits, amount)
	f.creditKeys = append(f.creditKeys, key)
	if f.failCredit {
		return errors.New("wallet: unavailable")
	}
	return nil
}

func TestPayForRevealDebitsFullFeeAndCreditsHalf(t *testing.T) {
	wallet := &fakeWallet{}
	payments := NewPaymentStore(nil, "test")
	svc := &Service{wallet: wallet, payments: payments}
	_ = svc // payments needs a real Dynamo client for ClaimPayment; see integration test below instead.
}

func TestPayForRevealSplitArithmeticIsPure(t *testing.T) {
	// fee/2 with integer division: an odd fee leaves the extra unit as rake,
	// never credited — same rule as Table.RequestWinnerCards.
	fee := int64(201)
	if fee/2 != 100 {
		t.Fatalf("expected floor division, got %d", fee/2)
	}
}
```

- [ ] **Step 3: Note the DynamoDB dependency and switch to an integration test**

`PayForReveal` needs a real (or DynamoDB Local) `*PaymentStore` for `ClaimPayment`/`CompletePayment` — those aren't mockable through an interface per this package's design (mirrors `handshare.Store`/`matchup.Store` being concrete types, not interfaces, elsewhere in this codebase). Delete the placeholder `TestPayForRevealDebitsFullFeeAndCreditsHalf` above and instead add this to `store_integration_test.go` (same build tag, same file, so it shares the Dynamo Local setup):

```go
func TestServicePayForRevealDebitsFullFeeCreditsHalfAndCompletes(t *testing.T) {
	db := handRevealTestClient(t)
	env := fmt.Sprintf("handreveal_test_%d", time.Now().UnixNano())
	createHandRevealPaymentsTable(t, db, env)
	payments := NewPaymentStore(db, env)
	wallet := &fakeWallet{}
	svc := NewService(wallet, payments)

	if err := svc.PayForReveal(context.Background(), "buyer", "winner", "hand-1", 200); err != nil {
		t.Fatal(err)
	}
	if len(wallet.debits) != 1 || wallet.debits[0] != 200 {
		t.Fatalf("expected one 200 debit, got %+v", wallet.debits)
	}
	if len(wallet.credits) != 1 || wallet.credits[0] != 100 {
		t.Fatalf("expected one 100 credit (fee/2), got %+v", wallet.credits)
	}
	paid, err := payments.HasPaid(context.Background(), "hand-1", "buyer")
	if err != nil || !paid {
		t.Fatalf("expected HasPaid=true after PayForReveal, got %v err=%v", paid, err)
	}

	// A second call must be a no-op: already completed, no new wallet calls.
	if err := svc.PayForReveal(context.Background(), "buyer", "winner", "hand-1", 200); err != nil {
		t.Fatal(err)
	}
	if len(wallet.debits) != 1 || len(wallet.credits) != 1 {
		t.Fatalf("expected no additional wallet calls on retry, got debits=%+v credits=%+v", wallet.debits, wallet.credits)
	}
}

func TestServicePayForRevealLeavesPendingOnDebitFailure(t *testing.T) {
	db := handRevealTestClient(t)
	env := fmt.Sprintf("handreveal_test_%d", time.Now().UnixNano())
	createHandRevealPaymentsTable(t, db, env)
	payments := NewPaymentStore(db, env)
	wallet := &fakeWallet{failDebit: true}
	svc := NewService(wallet, payments)

	if err := svc.PayForReveal(context.Background(), "buyer", "winner", "hand-1", 200); err == nil {
		t.Fatal("expected the debit failure to propagate")
	}
	paid, err := payments.HasPaid(context.Background(), "hand-1", "buyer")
	if err != nil || paid {
		t.Fatalf("a failed debit must leave the payment row pending, not paid: %v err=%v", paid, err)
	}
}
```

Move `fakeWallet` from `service_test.go` into `store_integration_test.go` (it needs to live under the `integration` build tag now), and delete `service_test.go`'s DynamoDB-dependent stub — keep only `TestPayForRevealSplitArithmeticIsPure` there as the one pure-Go unit test in that file.

- [ ] **Step 4: Run the tests**

Run: `cd api && go test ./internal/handreveal/... -v && go test -tags integration ./internal/handreveal/... -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/handreveal/service.go api/internal/handreveal/service_test.go api/internal/handreveal/store_integration_test.go
git commit -m "feat(handreveal): add PayForReveal wallet debit/credit service"
```

---

## Task 5: Wire the hand-complete/hand-updated hooks to write `HandRecord`

**Files:**
- Modify: `api/internal/app/app.go`

**Interfaces:**
- Consumes: `handreveal.NewStore`, `handreveal.HandRecord`, `handreveal.PlayerHandCode` (Task 2); `roomstore.Room.BigBlind`, `roomstore.CurrencyModeSandbox` (existing); `hand.HandOutcome.WonWithoutShowdown`/`Winners`/`PlayerHands` (existing, `api/internal/engine/hand/hand.go:176-206`).
- Produces: every eligible sandbox hand gets a `poker_hand_reveals` row within the same hook that already writes `sessionlog.HandItem`.

- [ ] **Step 1: Add the provider function**

In `api/internal/app/app.go`, near `newHandShareStore` (around line 278), add:

```go
func newHandRevealStore(db *dynamodb.Client, cfg *config.Config) *handreveal.Store {
	return handreveal.NewStore(db, cfg.Env)
}
```

Add `"gopkg.aoctech.app/poker/api/internal/handreveal"` to the import block.

- [ ] **Step 2: Register the provider with fx**

In the `fx.Provide(` list (around line 65-90, next to `newMatchupStore`), add `newHandRevealStore,`.

- [ ] **Step 3: Thread `*handreveal.Store` into `newTableManager`**

Change the `newTableManager` signature (around line 449) to accept it:

```go
func newTableManager(leases *tablelease.Service, store *tablestore.Store, reg ws.Registry, achv *achievements.Service, leaderboardSvc *leaderboard.Service, rooms *roomstore.Store, sessionStore *sessionlog.Store, pokerStatsStore *pokerstats.Store, matchupStore *matchup.Store, highlightsStore *highlights.Store, recentSvc *recentplayers.Service, players *player.Service, cfg *config.Config, handRevealStore *handreveal.Store) *tablemanager.Manager {
```

fx resolves the new parameter automatically from the provider added in Step 2 — no call site needs editing.

- [ ] **Step 4: Add `persistHandReveal` next to `persistHandHistory`**

Right after the `persistHandHistory` closure definition (around line 457-474), add:

```go
	persistHandReveal := func(tableID, handID, mode string, outcome hand.HandOutcome) {
		if handRevealStore == nil || mode != roomstore.CurrencyModeSandbox {
			return
		}
		if !outcome.WonWithoutShowdown || len(outcome.Winners) != 1 {
			return
		}
		room, err := rooms.Get(context.Background(), tableID)
		if err != nil || room == nil {
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
		if err := handRevealStore.Put(context.Background(), record); err != nil {
			slog.Error("handreveal: record hand failed", "table", tableID, "hand", handID, "err", err)
		}
	}
```

- [ ] **Step 5: Call it from both hooks**

In `onHandComplete` (around line 535, right after `persistHandHistory(tableID, handID, mode, outcome, names)`), add:

```go
		persistHandReveal(tableID, handID, mode, outcome)
```

In `mgr.SetOnHandUpdated(...)` (around line 572-579, right after its own `persistHandHistory(tableID, handID, mode, outcome, names)` call), add the same line:

```go
		persistHandReveal(tableID, handID, mode, outcome)
```

This is what keeps `HandRecord.WinnerShown` correct if the winner voluntarily shows after the hand completed but before anyone buys the reveal — `outcome.PlayerHands[winnerID].Revealed` flips to `true` the moment `Table.RevealHoleCard` runs (`api/internal/engine/hand/hand.go:943-990`), and `SetOnHandUpdated` is the hook that re-fires with that updated outcome.

- [ ] **Step 6: Build**

Run: `cd api && go build ./...`
Expected: compiles clean.

- [ ] **Step 7: Commit**

```bash
git add api/internal/app/app.go
git commit -m "feat(handreveal): write hand-reveal archive from the hand-complete/hand-updated hooks"
```

---

## Task 6: REST endpoints — `POST`/`GET /players/me/hands/:handId/reveal-winner`

**Files:**
- Create: `api/internal/api/v1/handreveal.go`
- Create: `api/internal/api/v1/handreveal_test.go`
- Modify: `api/internal/api/v1/router.go`
- Modify: `api/internal/app/app.go` (provider wiring for `*handreveal.PaymentStore` / `*handreveal.Service`)

**Interfaces:**
- Consumes: `sessionLogReader.GetHand` (existing, `api/internal/api/v1/player.go:35-40`); `handreveal.Store.Get` (Task 2); `handreveal.Service.HasPaid`/`PayForReveal` (Task 4); `roomstore.CurrencyModeSandbox` (existing); `walletOrInternalProblem` (existing, `api/internal/api/v1/dailyreward.go:50-56`).
- Produces: `func RegisterHandReveal(router fiber.Router, auth fiber.Handler, sessions sessionLogReader, records handRevealStore, svc handRevealService, limiter *RateLimiter)`.

- [ ] **Step 1: Write the handler file**

```go
package v1

import (
	"context"
	"net/url"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/handreveal"
	"gopkg.aoctech.app/poker/api/internal/problem"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
)

// handRevealStore and handRevealService are narrow interfaces over
// *handreveal.Store / *handreveal.Service (both satisfy them automatically)
// so handler tests can use fakes instead of a real DynamoDB/wallet client —
// same pattern as sessionLogReader/historyStore elsewhere in this package.
type handRevealStore interface {
	Get(ctx context.Context, handID string) (*handreveal.HandRecord, error)
}

type handRevealService interface {
	HasPaid(ctx context.Context, handID, viewerID string) (bool, error)
	PayForReveal(ctx context.Context, buyerID, winnerID, handID string, fee int64) error
}

type handRevealHandlers struct {
	sessions sessionLogReader
	records  handRevealStore
	svc      handRevealService
}

func RegisterHandReveal(router fiber.Router, auth fiber.Handler, sessions sessionLogReader, records handRevealStore, svc handRevealService, limiter *RateLimiter) {
	h := &handRevealHandlers{sessions: sessions, records: records, svc: svc}
	g := router.Group("/players/me/hands/:handId", auth)
	g.Post("/reveal-winner", rateLimit(limiter, ipKey("handreveal:create")), h.reveal)
	g.Get("/reveal-winner", h.check)
}

func (h *handRevealHandlers) loadEligibleRecord(c fiber.Ctx, playerID, handID string) (*handreveal.HandRecord, *problem.Problem) {
	// The caller must have been dealt into this hand — the only place that
	// already ties a player to a hand id today (mirrors handshares.go's
	// create handler, which does the same lookup for the same reason).
	participant, err := h.sessions.GetHand(c.Context(), playerID, roomstore.CurrencyModeSandbox, handID)
	if err != nil {
		return nil, problem.InternalServer("failed to load hand", c, err)
	}
	if participant == nil {
		return nil, problem.NotFound("hand not found")
	}
	record, err := h.records.Get(c.Context(), handID)
	if err != nil {
		return nil, problem.InternalServer("failed to load hand reveal", c, err)
	}
	if record == nil {
		return nil, problem.NotFound("hand reveal not available")
	}
	return record, nil
}

func (h *handRevealHandlers) reveal(c fiber.Ctx) error {
	playerID := c.Locals(localsUserID).(string)
	handID, err := url.PathUnescape(c.Params("handId"))
	if err != nil || handID == "" {
		return problem.BadRequest("hand id is invalid").Send(c)
	}
	record, prob := h.loadEligibleRecord(c, playerID, handID)
	if prob != nil {
		return prob.Send(c)
	}
	if record.WinnerShown {
		return problem.BadRequest("the winner's cards were already shown").Send(c)
	}
	if playerID == record.WinnerID {
		return problem.BadRequest("the winner cannot buy their own hand").Send(c)
	}
	paid, err := h.svc.HasPaid(c.Context(), handID, playerID)
	if err != nil {
		return problem.InternalServer("failed to check payment", c, err).Send(c)
	}
	if paid {
		return problem.BadRequest("already purchased").Send(c)
	}
	if err := h.svc.PayForReveal(c.Context(), playerID, record.WinnerID, handID, record.BigBlind); err != nil {
		return walletOrInternalProblem(err, "reveal purchase failed", c).Send(c)
	}
	cards := record.PlayerHands[record.WinnerID].Cards
	return c.JSON(fiber.Map{"cards": cards[:]})
}

func (h *handRevealHandlers) check(c fiber.Ctx) error {
	playerID := c.Locals(localsUserID).(string)
	handID, err := url.PathUnescape(c.Params("handId"))
	if err != nil || handID == "" {
		return problem.BadRequest("hand id is invalid").Send(c)
	}
	record, prob := h.loadEligibleRecord(c, playerID, handID)
	if prob != nil {
		return prob.Send(c)
	}
	paid, err := h.svc.HasPaid(c.Context(), handID, playerID)
	if err != nil {
		return problem.InternalServer("failed to check payment", c, err).Send(c)
	}
	resp := fiber.Map{"fee": record.BigBlind, "already_paid": paid}
	if paid {
		cards := record.PlayerHands[record.WinnerID].Cards
		resp["cards"] = cards[:]
	}
	return c.JSON(resp)
}
```

- [ ] **Step 2: Write handler tests with fakes**

```go
package v1

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/handreveal"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
)

type fakeHandRevealSessions struct {
	hand *sessionlog.HandItem
}

func (f *fakeHandRevealSessions) ListSessions(context.Context, string, int, map[string]types.AttributeValue) ([]sessionlog.SessionItem, map[string]types.AttributeValue, error) {
	return nil, nil, nil
}
func (f *fakeHandRevealSessions) ListHands(context.Context, string, string, int, map[string]types.AttributeValue) ([]sessionlog.HandItem, map[string]types.AttributeValue, error) {
	return nil, nil, nil
}
func (f *fakeHandRevealSessions) ListHandsByTable(context.Context, string, string, string, int, map[string]types.AttributeValue) ([]sessionlog.HandItem, map[string]types.AttributeValue, error) {
	return nil, nil, nil
}
func (f *fakeHandRevealSessions) GetHand(context.Context, string, string, string) (*sessionlog.HandItem, error) {
	return f.hand, nil
}

type fakeHandRevealStore struct {
	record *handreveal.HandRecord
}

func (f *fakeHandRevealStore) Get(context.Context, string) (*handreveal.HandRecord, error) {
	return f.record, nil
}

type fakeHandRevealService struct {
	paid    bool
	payErr  error
	payCall int
}

func (f *fakeHandRevealService) HasPaid(context.Context, string, string) (bool, error) { return f.paid, nil }
func (f *fakeHandRevealService) PayForReveal(context.Context, string, string, string, int64) error {
	f.payCall++
	return f.payErr
}

func newHandRevealApp(playerID string, sessions sessionLogReader, records handRevealStore, svc handRevealService) *fiber.App {
	app := fiber.New()
	auth := func(c fiber.Ctx) error {
		c.Locals(localsUserID, playerID)
		return c.Next()
	}
	RegisterHandReveal(app.Group("/v1.0"), auth, sessions, records, svc, nil)
	return app
}

func TestRevealWinnerRejectsNonParticipant(t *testing.T) {
	sessions := &fakeHandRevealSessions{hand: nil}
	app := newHandRevealApp("buyer", sessions, &fakeHandRevealStore{}, &fakeHandRevealService{})

	req := httptest.NewRequest(http.MethodPost, "/v1.0/players/me/hands/hand-1/reveal-winner", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for a non-participant, got %d", resp.StatusCode)
	}
}

func TestRevealWinnerRejectsMissingArchive(t *testing.T) {
	sessions := &fakeHandRevealSessions{hand: &sessionlog.HandItem{HandID: "hand-1", TableID: "table-1"}}
	app := newHandRevealApp("buyer", sessions, &fakeHandRevealStore{record: nil}, &fakeHandRevealService{})

	req := httptest.NewRequest(http.MethodPost, "/v1.0/players/me/hands/hand-1/reveal-winner", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 when no archive exists (showdown/real-money/pre-feature hand), got %d", resp.StatusCode)
	}
}

func TestRevealWinnerRejectsWinnerBuyingOwnHand(t *testing.T) {
	sessions := &fakeHandRevealSessions{hand: &sessionlog.HandItem{HandID: "hand-1", TableID: "table-1"}}
	records := &fakeHandRevealStore{record: &handreveal.HandRecord{HandID: "hand-1", WinnerID: "buyer", BigBlind: 200}}
	app := newHandRevealApp("buyer", sessions, records, &fakeHandRevealService{})

	req := httptest.NewRequest(http.MethodPost, "/v1.0/players/me/hands/hand-1/reveal-winner", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for the winner buying their own hand, got %d", resp.StatusCode)
	}
}

func TestRevealWinnerRejectsAlreadyShown(t *testing.T) {
	sessions := &fakeHandRevealSessions{hand: &sessionlog.HandItem{HandID: "hand-1", TableID: "table-1"}}
	records := &fakeHandRevealStore{record: &handreveal.HandRecord{HandID: "hand-1", WinnerID: "winner", WinnerShown: true, BigBlind: 200}}
	app := newHandRevealApp("buyer", sessions, records, &fakeHandRevealService{})

	req := httptest.NewRequest(http.MethodPost, "/v1.0/players/me/hands/hand-1/reveal-winner", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 when the winner already voluntarily showed, got %d", resp.StatusCode)
	}
}

func TestRevealWinnerSuccessReturnsCards(t *testing.T) {
	sessions := &fakeHandRevealSessions{hand: &sessionlog.HandItem{HandID: "hand-1", TableID: "table-1"}}
	records := &fakeHandRevealStore{record: &handreveal.HandRecord{
		HandID: "hand-1", WinnerID: "winner", BigBlind: 200,
		PlayerHands: map[string]handreveal.PlayerHandCode{"winner": {Cards: [2]string{"Ah", "Kd"}}},
	}}
	svc := &fakeHandRevealService{}
	app := newHandRevealApp("buyer", sessions, records, svc)

	req := httptest.NewRequest(http.MethodPost, "/v1.0/players/me/hands/hand-1/reveal-winner", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body struct{ Cards []string `json:"cards"` }
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Cards) != 2 || body.Cards[0] != "Ah" || body.Cards[1] != "Kd" {
		t.Fatalf("unexpected cards in response: %+v", body.Cards)
	}
	if svc.payCall != 1 {
		t.Fatalf("expected PayForReveal to be called once, got %d", svc.payCall)
	}
}

func TestCheckReturnsFeeAndCardsOnlyWhenPaid(t *testing.T) {
	sessions := &fakeHandRevealSessions{hand: &sessionlog.HandItem{HandID: "hand-1", TableID: "table-1"}}
	records := &fakeHandRevealStore{record: &handreveal.HandRecord{
		HandID: "hand-1", WinnerID: "winner", BigBlind: 200,
		PlayerHands: map[string]handreveal.PlayerHandCode{"winner": {Cards: [2]string{"Ah", "Kd"}}},
	}}
	app := newHandRevealApp("buyer", sessions, records, &fakeHandRevealService{paid: false})

	req := httptest.NewRequest(http.MethodGet, "/v1.0/players/me/hands/hand-1/reveal-winner", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body struct {
		Fee         int64    `json:"fee"`
		AlreadyPaid bool     `json:"already_paid"`
		Cards       []string `json:"cards"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Fee != 200 || body.AlreadyPaid || body.Cards != nil {
		t.Fatalf("expected fee=200, already_paid=false, no cards before purchase; got %+v", body)
	}

	app = newHandRevealApp("buyer", sessions, records, &fakeHandRevealService{paid: true})
	req = httptest.NewRequest(http.MethodGet, "/v1.0/players/me/hands/hand-1/reveal-winner", nil)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.AlreadyPaid || len(body.Cards) != 2 {
		t.Fatalf("expected already_paid=true with cards once paid; got %+v", body)
	}
}

func TestCheckReturns404WithNoArchive(t *testing.T) {
	sessions := &fakeHandRevealSessions{hand: &sessionlog.HandItem{HandID: "hand-1", TableID: "table-1"}}
	app := newHandRevealApp("buyer", sessions, &fakeHandRevealStore{record: nil}, &fakeHandRevealService{})

	req := httptest.NewRequest(http.MethodGet, "/v1.0/players/me/hands/hand-1/reveal-winner", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 when no archive exists, got %d", resp.StatusCode)
	}
}

var _ = errors.New // silence unused import if trimmed later; remove if errors ends up used directly.
```

(Drop the trailing `var _ = errors.New` line if the `errors` import ends up genuinely unused after writing the file — check with `go vet`.)

- [ ] **Step 3: Wire providers in `app.go`**

Next to `newHandRevealStore` (Task 5, Step 1), add:

```go
func newHandRevealPaymentStore(db *dynamodb.Client, cfg *config.Config) *handreveal.PaymentStore {
	return handreveal.NewPaymentStore(db, cfg.Env)
}
func newHandRevealService(wallet *walletclient.Client, payments *handreveal.PaymentStore) *handreveal.Service {
	return handreveal.NewService(wallet, payments)
}
```

Add both to the `fx.Provide(` list.

- [ ] **Step 4: Register the route in `router.go`**

Add `"gopkg.aoctech.app/poker/api/internal/handreveal"` to `router.go`'s imports. Add `handRevealStore *handreveal.Store, handRevealSvc *handreveal.Service,` to `Register`'s parameter list (next to `matchupStore *matchup.Store,` around line 63). Reuse the existing `purchaseLimiter` (already built at `router.go:100`, already passed to `RegisterSandboxPurchase`/`RegisterReactionPurchase`/`RegisterCosmeticPurchase`):

```go
	RegisterHandReveal(router, auth, sessionStore, handRevealStore, handRevealSvc, purchaseLimiter)
```

Add this line right after `RegisterHandShares(router, auth, sessionStore, tableStore, handShareStore)` (around line 115).

- [ ] **Step 5: Verify no `readscopes.go` change is needed**

Run: `grep -n "players/me/hands" api/internal/api/v1/readscopes.go`
Expected: the existing `strings.HasPrefix(path, "/v1.0/players/me/hands")` case already matches `/v1.0/players/me/hands/:handId/reveal-winner` — confirm no edit needed (Deviation #5 above). `enforceReadOnlyScope`'s method check already blocks the `POST` for non-first-party sessions.

- [ ] **Step 6: Build and run tests**

Run: `cd api && go build ./... && go test ./internal/api/v1/... -run TestRevealWinner -v && go test ./internal/api/v1/... -run TestCheck -v`
Expected: compiles, all new tests PASS.

- [ ] **Step 7: Commit**

```bash
git add api/internal/api/v1/handreveal.go api/internal/api/v1/handreveal_test.go api/internal/api/v1/router.go api/internal/app/app.go
git commit -m "feat(api): add paid history reveal endpoints for winner cards"
```

---

## Task 7: Frontend — API client functions

**Files:**
- Modify: `ui/src/lib/api/player.ts`

**Interfaces:**
- Produces:
  - `export interface HandRevealCheck { fee: number; already_paid: boolean; cards?: [string, string]; }`
  - `export async function getHandRevealWinner(handId: string): Promise<HandRevealCheck>`
  - `export async function revealHandWinner(handId: string): Promise<{ cards: [string, string] }>`

- [ ] **Step 1: Add the types and functions**

At the end of `ui/src/lib/api/player.ts` (after `getHand`, around line 155), add:

```ts
export interface HandRevealCheck {
  fee: number;
  already_paid: boolean;
  cards?: [string, string];
}

// GET .../reveal-winner always returns 200 with the fee once an archive
// exists for handId (a sandbox hand that ended without a showdown with
// exactly one winner) — 404 means no archive exists at all (showdown hand,
// real-money hand, split pot, or a hand that predates this feature).
export async function getHandRevealWinner(handId: string) {
  return (await apiClient.get<HandRevealCheck>(
    `/v1.0/players/me/hands/${encodeURIComponent(handId)}/reveal-winner`, {silentError: true}
  )).data;
}

export async function revealHandWinner(handId: string) {
  return (await apiClient.post<{ cards: [string, string] }>(
    `/v1.0/players/me/hands/${encodeURIComponent(handId)}/reveal-winner`, {}, {silentError: true}
  )).data;
}
```

- [ ] **Step 2: Add a retry helper mirroring `shouldRetryHighlightFetch`**

In the same file (or inline where the component uses it — see Task 8), the retry policy is: don't retry a 404 (permanent — no archive), retry other failures up to 3 times. Reuse `isNotFound` from `@/lib/api/client` (already imported by `client.ts` consumers elsewhere) directly in the component instead of duplicating a helper here, to keep this file free of React-specific concerns — see Task 8, Step 2.

- [ ] **Step 3: Typecheck**

Run: `cd ui && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add ui/src/lib/api/player.ts
git commit -m "feat(ui): add hand-reveal-winner API client functions"
```

---

## Task 8: Frontend — `RevealWinnerButton` component and `HandReplayer` wiring

**Files:**
- Create: `ui/src/components/hands/RevealWinnerButton.tsx`
- Create: `ui/src/components/hands/RevealWinnerButton.test.tsx`
- Modify: `ui/src/components/hands/HandReplayer.tsx`

**Interfaces:**
- Consumes: `getHandRevealWinner`, `revealHandWinner` (Task 7); `OpponentSummary` (existing, `ui/src/lib/api/player.ts:105-111`); `isNotFound` (existing, `ui/src/lib/api/client.ts`).
- Produces: `export function RevealWinnerButton({handId, winner, bigBlind, onRevealedAction}: {...})`.

- [ ] **Step 1: Write `RevealWinnerButton.tsx`**

```tsx
'use client';
import {useEffect, useState} from 'react';
import {useQuery} from '@tanstack/react-query';
import {Eye} from 'lucide-react';
import {getHandRevealWinner, revealHandWinner} from '@/lib/api/player';
import {isNotFound} from '@/lib/api/client';

// Buy button for a history/replay view — the REST-driven counterpart to
// components/table/WinnerCards.tsx, which does the same purchase live over
// WS. Renders nothing until the check query resolves; a 404 (no archive —
// showdown/real-money/pre-feature hand) or an already-visible winner hand
// both mean there is nothing to sell.
export function RevealWinnerButton({handId, winnerName, alreadyRevealed, onRevealedAction}: {
  handId: string;
  winnerName?: string;
  alreadyRevealed: boolean;
  onRevealedAction: (cards: [string, string]) => void;
}) {
  const [pending, setPending] = useState(false);
  const check = useQuery({
    queryKey: ['hand-reveal-winner', handId],
    queryFn: () => getHandRevealWinner(handId),
    retry: (count, err) => !isNotFound(err) && count < 3,
  });

  useEffect(() => {
    if (check.data?.already_paid && check.data.cards) {
      onRevealedAction(check.data.cards);
    }
  }, [check.data, onRevealedAction]);

  if (alreadyRevealed || !check.data || check.data.already_paid) return null;

  const handleClick = async () => {
    setPending(true);
    try {
      const result = await revealHandWinner(handId);
      onRevealedAction(result.cards);
    } finally {
      setPending(false);
    }
  };

  return <aside className="winner-cards" aria-live="polite">
    <button type="button" disabled={pending} onClick={handleClick}>
      <Eye aria-hidden="true"/>
      <span><b>{`Ver a mão de ${winnerName || 'vencedor'} por ${check.data.fee.toLocaleString('pt-BR')} fichas`}</b>
        <small>As cartas aparecem apenas para você</small></span>
    </button>
  </aside>;
}
```

- [ ] **Step 2: Write the component test**

```tsx
import {describe, expect, test, vi, beforeEach} from 'vitest';
import {render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import {RevealWinnerButton} from './RevealWinnerButton';
import * as playerApi from '@/lib/api/player';

vi.mock('@/lib/api/player', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/player')>('@/lib/api/player');
  return {...actual, getHandRevealWinner: vi.fn(), revealHandWinner: vi.fn()};
});

function renderWithClient(ui: React.ReactElement) {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}});
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>);
}

describe('RevealWinnerButton', () => {
  beforeEach(() => vi.clearAllMocks());

  test('renders nothing while the check is loading or on 404', async () => {
    vi.mocked(playerApi.getHandRevealWinner).mockRejectedValue({response: {status: 404}});
    const onRevealed = vi.fn();
    renderWithClient(<RevealWinnerButton handId="hand-1" winnerName="Ana" alreadyRevealed={false} onRevealedAction={onRevealed}/>);
    await waitFor(() => expect(playerApi.getHandRevealWinner).toHaveBeenCalledWith('hand-1'));
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  test('renders nothing when alreadyRevealed is true, without calling the API result', async () => {
    vi.mocked(playerApi.getHandRevealWinner).mockResolvedValue({fee: 200, already_paid: false});
    renderWithClient(<RevealWinnerButton handId="hand-1" winnerName="Ana" alreadyRevealed onRevealedAction={vi.fn()}/>);
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  test('shows the buy button with the fee once the check resolves as unpaid', async () => {
    vi.mocked(playerApi.getHandRevealWinner).mockResolvedValue({fee: 200, already_paid: false});
    renderWithClient(<RevealWinnerButton handId="hand-1" winnerName="Ana" alreadyRevealed={false} onRevealedAction={vi.fn()}/>);
    expect(await screen.findByRole('button')).toHaveTextContent('200');
  });

  test('clicking the button purchases the reveal and surfaces the cards', async () => {
    vi.mocked(playerApi.getHandRevealWinner).mockResolvedValue({fee: 200, already_paid: false});
    vi.mocked(playerApi.revealHandWinner).mockResolvedValue({cards: ['Ah', 'Kd']});
    const onRevealed = vi.fn();
    renderWithClient(<RevealWinnerButton handId="hand-1" winnerName="Ana" alreadyRevealed={false} onRevealedAction={onRevealed}/>);
    const button = await screen.findByRole('button');
    await userEvent.click(button);
    await waitFor(() => expect(onRevealed).toHaveBeenCalledWith(['Ah', 'Kd']));
  });

  test('surfaces cards automatically when the check reports already_paid', async () => {
    vi.mocked(playerApi.getHandRevealWinner).mockResolvedValue({fee: 200, already_paid: true, cards: ['2c', '7s']});
    const onRevealed = vi.fn();
    renderWithClient(<RevealWinnerButton handId="hand-1" winnerName="Ana" alreadyRevealed={false} onRevealedAction={onRevealed}/>);
    await waitFor(() => expect(onRevealed).toHaveBeenCalledWith(['2c', '7s']));
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });
});
```

- [ ] **Step 3: Run the new tests**

Run: `cd ui && npx vitest run src/components/hands/RevealWinnerButton.test.tsx`
Expected: all PASS.

- [ ] **Step 4: Wire the button into `HandReplayer.tsx`**

In `ui/src/components/hands/HandReplayer.tsx`:

Add the import near the top (with the other component imports, around line 5-9):

```tsx
import {RevealWinnerButton} from '@/components/hands/RevealWinnerButton';
```

Add local state for a purchased reveal, right after the existing `const [speed, setSpeed] = useState(1);` (around line 82):

```tsx
  const [revealedWinnerCards, setRevealedWinnerCards] = useState<string[] | null>(null);
```

After `const opponents = new Map(...)` (around line 108) and `const showFinalCards = ...` (around line 110), add a derived winner lookup — this only makes sense once the replay has reached the final frame:

```tsx
  const winnerOpponent = frame.stage === 'complete'
    ? [...opponents.values()].find(o => o.won && (!o.hole_cards || o.hole_cards.length === 0))
    : undefined;
```

Change `holeCardsFor` (around line 112-116) to fall back to the purchased reveal:

```tsx
  const holeCardsFor = (playerId: string) => {
    if (playerId === viewerId) return hand.hole_cards;
    if (winnerOpponent && playerId === winnerOpponent.player_id && revealedWinnerCards) return revealedWinnerCards;
    if (!showFinalCards && !shownPlayers.has(playerId)) return undefined;
    return opponents.get(playerId)?.hole_cards;
  };
```

Render the button next to the outcome badge in the header (around line 143-147, inside `<div className="replay-header-end">`), right after the `OutcomeBadge`:

```tsx
        {frame.stage === 'complete' && <OutcomeBadge outcome={hand.outcome}/>}
        {winnerOpponent && <RevealWinnerButton
          handId={hand.hand_id}
          winnerName={winnerOpponent.name}
          alreadyRevealed={Boolean(revealedWinnerCards)}
          onRevealedAction={cards => setRevealedWinnerCards(cards)}
        />}
```

- [ ] **Step 5: Run the existing `HandReplayer` tests to check for regressions**

Run: `cd ui && npx vitest run src/components/hands` (or wherever `HandReplayer`'s tests live — check for a `HandReplayer.test.tsx`; if none exists, run the full `hands` app test suite: `npx vitest run src/app/hands`)
Expected: all PASS, no regressions from the new import/state/render.

- [ ] **Step 6: Full frontend quality gate**

Run: `cd ui && npx vitest run && npx tsc --noEmit && npx eslint src --max-warnings 0`
Expected: zero errors, zero warnings (per `ui/CLAUDE.md`'s quality gate). If coverage thresholds fail because of the new files, the tests in Step 2 must already cover error/empty/disabled branches — re-check `RevealWinnerButton.test.tsx` covers: 404/loading (empty), unpaid (button shown), disabled-while-pending, success, and already-paid-on-load. Add a `disabled` assertion mid-click if coverage flags the `pending` branch:

```tsx
  test('disables the button while the purchase is in flight', async () => {
    vi.mocked(playerApi.getHandRevealWinner).mockResolvedValue({fee: 200, already_paid: false});
    let resolvePurchase: (v: {cards: [string, string]}) => void = () => {};
    vi.mocked(playerApi.revealHandWinner).mockReturnValue(new Promise(resolve => { resolvePurchase = resolve; }));
    renderWithClient(<RevealWinnerButton handId="hand-1" winnerName="Ana" alreadyRevealed={false} onRevealedAction={vi.fn()}/>);
    const button = await screen.findByRole('button');
    userEvent.click(button);
    await waitFor(() => expect(button).toBeDisabled());
    resolvePurchase({cards: ['Ah', 'Kd']});
  });
```

- [ ] **Step 7: Commit**

```bash
git add ui/src/components/hands/RevealWinnerButton.tsx ui/src/components/hands/RevealWinnerButton.test.tsx ui/src/components/hands/HandReplayer.tsx
git commit -m "feat(ui): add paid winner-cards reveal button to hand replay"
```

---

## Task 9: Documentation updates

**Files:**
- Modify: `api/CLAUDE.md`
- Modify: `cdk/CLAUDE.md` (table count already done in Task 1, Step 4 — verify it's still accurate)

**Interfaces:** none (documentation only).

- [ ] **Step 1: Update `api/CLAUDE.md`'s Layout line**

In the `## Layout` section, add `handreveal` to the money-adjacent/player-scoped package list — it belongs with `handshare` (both are player-scoped, paid, gated single-purpose stores):

```
`internal/{player,playernotes,pokerstats,matchup,sessionlog,handshare,handreveal,highlights}` (player-scoped data) ·
```

- [ ] **Step 2: Add a short note to `api/CLAUDE.md`'s intro paragraph**

Mirror the existing terse style used for the live paid-rabbit-hunt/winner-cards features (see the "B32 fixed" bullet under "Other known issues") — add one bullet:

```
- `handreveal` (`poker_hand_reveals` + `poker_hand_reveal_payments`) extends the live paid
  winner-cards reveal to hand history: `POST`/`GET /players/me/hands/:handId/reveal-winner`.
  Sandbox-only, one row per eligible hand, written by the same hand-complete/hand-updated hooks
  that already write `sessionlog.HandItem`. See `docs/specs/2026-08-21-pay-to-see-winner-cards-history.md`.
```

- [ ] **Step 3: Verify `cdk/CLAUDE.md`'s table count is correct**

Run: `grep -n "DynamoDB tables" cdk/CLAUDE.md`
Expected: reads `**29 DynamoDB tables**` (updated in Task 1, Step 4) — fix if it wasn't.

- [ ] **Step 4: Commit**

```bash
git add api/CLAUDE.md cdk/CLAUDE.md
git commit -m "docs: document the paid history winner-cards reveal feature"
```

---

## Task 10: Full-stack verification

**Files:** none modified — verification only.

- [ ] **Step 1: Full backend test suite**

Run: `cd api && go test ./... -race`
Expected: all PASS.

- [ ] **Step 2: Backend integration suite**

Run: `cd api && docker compose -f docker-compose.test.yml up -d && go test -tags integration -race ./...`
Expected: all PASS.

- [ ] **Step 3: CDK tests**

Run: `cd cdk && npm run build && npx jest`
Expected: all PASS.

- [ ] **Step 4: Frontend quality gate (repeat, full)**

Run: `cd ui && npx vitest run && npx tsc --noEmit && npx eslint src --max-warnings 0 && npm run build`
Expected: all PASS, zero warnings, coverage thresholds (lines/functions/statements/branches ≥90) met.

- [ ] **Step 5: Manual smoke check (if a local stack is available)**

Start the API + DynamoDB Local + UI dev server. Play a sandbox hand to completion where one player folds pre-showdown to a single winner who never shows. Open that hand's replay from `/hands/history`. Confirm the buy button appears with the correct big-blind price, purchasing it reveals the winner's true cards in place of `"back"`, and reloading the replay page shows the cards immediately (via the `GET` check) without re-charging.

- [ ] **Step 6: Final commit (if anything was fixed during verification)**

```bash
git add -A
git commit -m "fix: address issues found during full-stack verification"
```

(Skip if verification found nothing to fix.)
