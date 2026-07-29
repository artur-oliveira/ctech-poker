# Real-Money Fixed Fee + Sandbox Rake Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:
> executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Flip the poker business model to be Brazil-legal: sandbox tables now take a rake (unchanged legally — virtual
currency), real-money tables never take rake and instead charge a flat, non-percentage table-entry fee (an "aluguel de
mesa") to every player who takes a seat, including the room's creator.

**Architecture:** `hand.Table.ConfigureRake` flips which currency mode gets the existing 2.5% rake engine (sandbox now,
not real). A new fixed fee (BRL cents, one of 4 risk tiers, never a % of blind) is computed once at room creation from a
static catalog and stored on `roomstore.Room`. `buyin.Service.BuyIn` — the single choke point every seat-entry (
create-and-join, join, and rebuy-after-bust all funnel through this one call) already passes through — charges that fee
via a new `ctech-wallet` call (`DebitReal`, hits the player's real/withdrawable wallet, *not* the ring-fenced game
wallet used for stakes) after the seat is committed, logging + queuing a reconciliation retry on failure rather than
unwinding the seat, mirroring this codebase's existing tolerance for post-commit wallet-call failures (see `CashOut`'s "
reconciliation job will retry" convention).

**Tech Stack:** Go 1.x (Fiber v3, DynamoDB on-demand), Next.js 16 (App Router) UI, ctech-wallet's existing
`POST /internal/wallet/real/debit` endpoint (already deployed, scope `internal:wallet:debit-real` already exists in
ctech-account's catalog — see Global Constraints).

## Global Constraints

- Fee is **flat per risk tier, never a formula against the blind** (Brazilian law: a cut of the pot/blind on a public
  real-money game is a bet and requires SPA authorization; a flat rental-style seat/table fee does not). Never write
  `fee = f(bigBlind)` — only table lookups.
- Real-money tables never collect rake (`ConfigureRake` must set `rakeBPS = 0` for `currencyMode == "real"`,
  unconditionally).
- Every seat-entry (create, join, rebuy) pays the fee — no special-casing "the creator" or "a rebuy":
  `buyin.Service.BuyIn` is already the one place all three paths converge, so one code path covers all three by
  construction.
- Fee tiers (BRL cents), fixed by business decision, do not change without a product/legal decision:

  | Bucket | Stakes (small/big, cents) | Fee (cents) |
    |---|---|---|
  | Micro | 5/10, 10/20, 25/50, 50/100 | 100 |
  | Low | 100/200, 200/500 | 200 |
  | Mid | 500/1000, 1000/2000 | 400 |
  | High | 2500/5000, 5000/10000 | 800 |

- **Cross-repo blocker (not part of this plan's tasks, must happen before this ships to prod):**
  `internal:wallet:debit-real` already exists in `ctech-account`'s scope catalog (`api/internal/scopes/catalog.go:152`)
  and is already wired end-to-end in `ctech-wallet` (`POST /internal/wallet/real/debit`, `WalletService.DebitReal`) for
  `ctech-billing`'s use — but poker's own M2M client has never been granted that scope. This is the same category of gap
  as the already-known missing `internal:wallet:game-status` grant (see `api/CLAUDE.md`): a data/config action in
  `ctech-account`, not a code change, and out of scope for this plan.
- No CDK changes anywhere in this plan — EC2/DynamoDB/wallet-client wiring is unaffected; `POKER_CLIENT_ID`/
  `POKER_CLIENT_SECRET` already flow into `walletclient.New`.

---

### Task 1: Real-money stake catalog + fixed fee table

**Files:**

- Modify: `api/internal/api/v1/stakes.go`
- Test: `api/internal/api/v1/stakes_test.go` (new)

**Interfaces:**

- Produces: `publicStake.FeeCents int64` (json `fee_cents,omitempty`),
  `realStakeFeeCents(smallBlind, bigBlind int64) (int64, bool)` — used by Task 2's `createRoom`.

- [ ] **Step 1: Replace the real-money stake list and add the fee lookup**

Replace the whole stakes list section of `api/internal/api/v1/stakes.go` (lines 1–27) with:

```go
package v1

type publicStake struct {
	SmallBlind int64 `json:"small_blind"`
	BigBlind   int64 `json:"big_blind"`
	// FeeCents is the fixed real-money table-entry fee for this stake tier, in
	// BRL cents. Zero for sandbox stakes (sandbox has no entry fee, only rake
	// — see hand.Table.ConfigureRake). Never derived from SmallBlind/BigBlind
	// at charge time — always a stored lookup (see Global Constraints).
	FeeCents int64 `json:"fee_cents,omitempty"`
}

// Values are stored in the smallest integer unit. Real mode interprets 5 as
// R$0,05; sandbox displays virtual chips without a currency symbol. Grouped
// by risk/compliance tier (see docs/plans/2026-07-25-realmoney-fixed-fee-and-sandbox-rake.md).
var realPublicStakes = []publicStake{
	// Micro — R$1,00 fee
	{SmallBlind: 5, BigBlind: 10, FeeCents: 100},
	{SmallBlind: 10, BigBlind: 20, FeeCents: 100},
	{SmallBlind: 25, BigBlind: 50, FeeCents: 100},
	{SmallBlind: 50, BigBlind: 100, FeeCents: 100},
	// Low — R$2,00 fee
	{SmallBlind: 100, BigBlind: 200, FeeCents: 200},
	{SmallBlind: 200, BigBlind: 500, FeeCents: 200},
	// Mid — R$4,00 fee
	{SmallBlind: 500, BigBlind: 1000, FeeCents: 400},
	{SmallBlind: 1000, BigBlind: 2000, FeeCents: 400},
	// High — R$8,00 fee
	{SmallBlind: 2500, BigBlind: 5000, FeeCents: 800},
	{SmallBlind: 5000, BigBlind: 10000, FeeCents: 800},
}

// sandboxPublicStakes mirrors realPublicStakes' blind pairs (sandbox chips
// happen to use the same denominations) plus three sandbox-only high tiers,
// but never carries a real-money fee — sandbox tables fund themselves via
// rake instead (hand.Table.ConfigureRake).
var sandboxPublicStakes = buildSandboxStakes()

func buildSandboxStakes() []publicStake {
	stakes := make([]publicStake, len(realPublicStakes))
	for i, s := range realPublicStakes {
		stakes[i] = publicStake{SmallBlind: s.SmallBlind, BigBlind: s.BigBlind}
	}
	return append(stakes,
		publicStake{SmallBlind: 10000, BigBlind: 25000},
		publicStake{SmallBlind: 25000, BigBlind: 50000},
		publicStake{SmallBlind: 50000, BigBlind: 100000},
	)
}

func isAllowedPublicStake(mode string, smallBlind, bigBlind int64) bool {
	stakes := sandboxPublicStakes
	if mode == "real" {
		stakes = realPublicStakes
	}
	for _, stake := range stakes {
		if stake.SmallBlind == smallBlind && stake.BigBlind == bigBlind {
			return true
		}
	}
	return false
}

// realStakeFeeCents looks up the fixed table-entry fee for a real-money
// stake pair. ok is false only if the pair matches no catalog tier — callers
// must have already validated the stake via isAllowedPublicStake("real", …)
// before relying on this to be true.
func realStakeFeeCents(smallBlind, bigBlind int64) (int64, bool) {
	for _, stake := range realPublicStakes {
		if stake.SmallBlind == smallBlind && stake.BigBlind == bigBlind {
			return stake.FeeCents, true
		}
	}
	return 0, false
}

func sandboxStakeCatalog() map[string]any {
	return map[string]any{
		"currency_mode": "sandbox",
		"unit":          "virtual_chip",
		"stakes":        sandboxPublicStakes,
	}
}

func realStakeCatalog() map[string]any {
	return map[string]any{
		"currency_mode": "real",
		"unit":          "brl_cent",
		"stakes":        realPublicStakes,
	}
}
```

- [ ] **Step 2: Write the failing test**

Create `api/internal/api/v1/stakes_test.go`:

```go
package v1

import "testing"

func TestRealStakeFeeCentsMatchesTierTable(t *testing.T) {
	cases := []struct {
		small, big, wantFee int64
	}{
		{5, 10, 100}, {50, 100, 100},
		{100, 200, 200}, {200, 500, 200},
		{500, 1000, 400}, {1000, 2000, 400},
		{2500, 5000, 800}, {5000, 10000, 800},
	}
	for _, c := range cases {
		fee, ok := realStakeFeeCents(c.small, c.big)
		if !ok || fee != c.wantFee {
			t.Fatalf("realStakeFeeCents(%d,%d) = %d,%v want %d,true", c.small, c.big, fee, ok, c.wantFee)
		}
	}
}

func TestRealStakeFeeCentsRejectsUnknownStake(t *testing.T) {
	if _, ok := realStakeFeeCents(7, 14); ok {
		t.Fatal("expected no fee match for an off-catalog stake pair")
	}
}

func TestSandboxStakesCarryNoFee(t *testing.T) {
	for _, s := range sandboxPublicStakes {
		if s.FeeCents != 0 {
			t.Fatalf("sandbox stake %d/%d leaked a real-money fee: %d", s.SmallBlind, s.BigBlind, s.FeeCents)
		}
	}
}
```

- [ ] **Step 3: Run the tests, verify they pass**

Run: `cd api && go test ./internal/api/v1/... -run 'TestRealStakeFeeCents|TestSandboxStakesCarryNoFee' -v`
Expected: PASS (this task is additive — no old test depends on the old stake values yet; Task 2 fixes the ones that do).

- [ ] **Step 4: Commit**

```bash
git add api/internal/api/v1/stakes.go api/internal/api/v1/stakes_test.go
git commit -m "feat(api): replace real-money stakes with 10-tier catalog + fixed entry fee table"
```

---

### Task 2: Room creation stores the fixed entry fee; catalog validation applies to private real-money rooms too

**Files:**

- Modify: `api/internal/roomstore/room.go`
- Modify: `api/internal/api/v1/rooms.go`
- Modify: `api/internal/api/v1/rooms_test.go`

**Interfaces:**

- Consumes: `realStakeFeeCents(smallBlind, bigBlind int64) (int64, bool)` (Task 1).
- Produces: `roomstore.Room.EntryFeeCents int64` — consumed by Task 6's `buyin.Service.BuyIn`.

**Why private rooms need the catalog restriction too:** `createRoom` currently only runs `isAllowedPublicStake` for
`Visibility == "public"` — a private room can pick arbitrary blinds. If that stayed true for `currency_mode: "real"`, a
private real-money room could have blinds matching no fee tier at all, and there would be nothing correct to charge.
Real-money rooms (public or private) must always use one of the fixed catalog stakes; only sandbox private rooms keep
free-form blinds.

- [ ] **Step 1: Add the field**

In `api/internal/roomstore/room.go`, add to the `Room` struct (after `BuyInMax`):

```go
	BuyInMax             int64            `dynamodbav:"buy_in_max" json:"buy_in_max"`
	// EntryFeeCents is the fixed real-money table-entry fee (BRL cents),
	// charged to every player who takes a seat (buyin.Service.BuyIn) — never
	// recomputed after creation, so a later change to the fee catalog never
	// retroactively changes an already-created room's fee. Always zero for
	// sandbox rooms (sandbox funds itself via rake instead).
	EntryFeeCents        int64            `dynamodbav:"entry_fee_cents,omitempty" json:"entry_fee_cents,omitempty"`
```

- [ ] **Step 2: Write the failing test**

Add to `api/internal/api/v1/rooms_test.go` (near `TestCreateRoomAcceptsRealMoneyWhenEnabled`):

```go
func TestCreateRoomStoresFixedEntryFeeForRealMoney(t *testing.T) {
	app := fiber.New()
	h := &roomHandlers{cfg: &config.Config{RealMoneyEnabled: true}}
	app.Post("/rooms", func(c fiber.Ctx) error { c.Locals(localsUserID, "u1"); return c.Next() }, h.createRoom)
	body := []byte(`{"visibility":"private","currency_mode":"real","small_blind":10,"big_blind":20,"max_seats":6,"buy_in_min":400,"buy_in_max":2000}`)
	req := httptest.NewRequest(fiber.MethodPost, "/rooms", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("got %d", resp.StatusCode)
	}
	var room roomstore.Room
	if err := json.NewDecoder(resp.Body).Decode(&room); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if room.EntryFeeCents != 100 {
		t.Fatalf("expected EntryFeeCents 100 for the 10/20 micro tier, got %d", room.EntryFeeCents)
	}
}

func TestCreateRoomRejectsPrivateRealMoneyRoomOffCatalog(t *testing.T) {
	app := fiber.New()
	h := &roomHandlers{cfg: &config.Config{RealMoneyEnabled: true}}
	app.Post("/rooms", func(c fiber.Ctx) error { c.Locals(localsUserID, "u1"); return c.Next() }, h.createRoom)
	// 7/14 matches no real-money tier — private real-money rooms must be
	// rejected the same as a public one would be, since there is no fee
	// tier to charge.
	body := []byte(`{"visibility":"private","currency_mode":"real","small_blind":7,"big_blind":14,"max_seats":6,"buy_in_min":280,"buy_in_max":1400}`)
	req := httptest.NewRequest(fiber.MethodPost, "/rooms", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("got %d, want 400", resp.StatusCode)
	}
}
```

Also fix `TestPublicSandboxStakesAreCurated` (Task 1 changed which pairs are curated) — replace its body with:

```go
func TestPublicSandboxStakesAreCurated(t *testing.T) {
	if !isAllowedPublicStake("sandbox", 5, 10) || !isAllowedPublicStake("sandbox", 50000, 100000) {
		t.Fatal("expected the lowest and highest sandbox stakes to be allowed")
	}
	if isAllowedPublicStake("sandbox", 7, 14) {
		t.Fatal("uncurated public stake was accepted")
	}
	if isAllowedPublicStake("real", 10000, 25000) {
		t.Fatal("sandbox-only high stake leaked into the real-money catalog")
	}
}
```

- [ ] **Step 3: Run the tests, verify they fail**

Run:
`cd api && go test ./internal/api/v1/... -run 'TestCreateRoomStoresFixedEntryFee|TestCreateRoomRejectsPrivateRealMoneyRoomOffCatalog|TestPublicSandboxStakesAreCurated' -v`
Expected: `TestCreateRoomStoresFixedEntryFeeForRealMoney` FAILs (`EntryFeeCents` always 0, field doesn't exist yet in
the create path), `TestCreateRoomRejectsPrivateRealMoneyRoomOffCatalog` FAILs (currently accepted — got 201 not 400),
`TestPublicSandboxStakesAreCurated` FAILs (10,20 no longer "uncurated" — wait, this one now asserts against `7,14` so it
should already pass once Task 1 landed; if it still fails here it means Task 1 wasn't committed — check that first).

- [ ] **Step 4: Wire validation + fee storage in `createRoom`**

In `api/internal/api/v1/rooms.go`, replace:

```go
	if req.Visibility == "public" && !isAllowedPublicStake(currencyMode, req.SmallBlind, req.BigBlind) {
		return problem.BadRequest("unsupported public stake for this currency mode").Send(c)
	}
```

with:

```go
	// Real-money rooms must always use one of the fixed catalog stakes,
	// public or private — the fixed entry fee below only exists for
	// catalog tiers, so an off-catalog real-money room would have nothing
	// correct to charge. Sandbox private rooms keep free-form blinds.
	if (req.Visibility == "public" || currencyMode == "real") && !isAllowedPublicStake(currencyMode, req.SmallBlind, req.BigBlind) {
		return problem.BadRequest("unsupported stake for this currency mode").Send(c)
	}
```

Then, in the same function, right before the `room := roomstore.Room{...}` literal, add:

```go
	entryFeeCents := int64(0)
	if currencyMode == "real" {
		entryFeeCents, _ = realStakeFeeCents(req.SmallBlind, req.BigBlind)
	}
```

And add the field to the `room` literal (after `BuyInMax: req.BuyInMax,`):

```go
		EntryFeeCents:        entryFeeCents,
```

- [ ] **Step 5: Run the tests, verify they pass**

Run: `cd api && go test ./internal/api/v1/... -v`
Expected: PASS, full package.

- [ ] **Step 6: Commit**

```bash
git add api/internal/roomstore/room.go api/internal/api/v1/rooms.go api/internal/api/v1/rooms_test.go
git commit -m "feat(api): store fixed real-money entry fee on room creation, require catalog stakes for private real rooms too"
```

---

### Task 3: Flip rake — sandbox gets it, real-money never does

**Files:**

- Modify: `api/internal/engine/hand/hand.go:208-216`
- Modify: `api/internal/engine/hand/rake_test.go`
- Modify: `api/internal/engine/hand/phase4_test.go`

**Interfaces:**

- No signature changes — `ConfigureRake(currencyMode string)` keeps the same signature; only which mode gets
  `rakeBPS = 250` flips. `table.SeedForRoom` (`api/internal/table/seed.go`) calls this unconditionally already and needs
  no change.

- [ ] **Step 1: Update the failing tests first**

In `api/internal/engine/hand/rake_test.go`, swap every `"real"`/`"sandbox"` argument and rename to match the new rule:

```go
package hand

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/deck"
)

func TestSandboxRakeUsesPercentageAndPlayerCaps(t *testing.T) {
	table := NewTable(nil, 50, 100)
	table.ConfigureRake("sandbox")
	table.board = make([]deck.Card, 3)

	table.handOrder = []*Player{{ID: "p1"}, {ID: "p2"}}
	if cap := table.rakeCap(); cap != 50 {
		t.Fatalf("heads-up cap = %d, want 50 (0.5 BB)", cap)
	}
	if rake := table.rakeForLayer(1000, table.rakeCap()); rake != 25 {
		t.Fatalf("2.5%% of 1000 = %d, want 25", rake)
	}
	if rake := table.rakeForLayer(10000, table.rakeCap()); rake != 50 {
		t.Fatalf("rake should be capped at 50, got %d", rake)
	}

	table.handOrder = []*Player{{}, {}, {}}
	if cap := table.rakeCap(); cap != 75 {
		t.Fatalf("3-player cap = %d, want 75 (0.75 BB)", cap)
	}
	table.handOrder = []*Player{{}, {}, {}, {}, {}}
	if cap := table.rakeCap(); cap != 100 {
		t.Fatalf("5-player cap = %d, want 100 (1 BB)", cap)
	}
}

func TestRealMoneyAndPreflopPotsHaveNoRake(t *testing.T) {
	table := NewTable(nil, 50, 100)
	table.handOrder = []*Player{{}, {}}
	table.board = make([]deck.Card, 3)
	table.ConfigureRake("real")
	if cap := table.rakeCap(); cap != 0 {
		t.Fatalf("real-money cap = %d, want zero (real-money never takes rake)", cap)
	}

	table.ConfigureRake("sandbox")
	table.board = nil
	if cap := table.rakeCap(); cap != 0 {
		t.Fatalf("preflop cap = %d, want zero", cap)
	}
}

func TestRakeConfigurationSurvivesPersistence(t *testing.T) {
	table := NewTable(nil, 50, 100)
	table.ConfigureRake("sandbox")
	table.rakeCollected = 17
	rebuilt := NewTableFromState(table.ExportState())
	if rebuilt.rakeBPS != 250 || rebuilt.rakeCollected != 17 {
		t.Fatalf("rake state was not preserved: bps=%d collected=%d", rebuilt.rakeBPS, rebuilt.rakeCollected)
	}
}
```

In `api/internal/engine/hand/phase4_test.go`, in `TestHandOutcomeUsesNetOfRakeWinnerAndExcludesRefunds`, change
`table.ConfigureRake("real")` to `table.ConfigureRake("sandbox")` (line 27) — the test's actual assertions (rake=5,
payout=195) are mode-agnostic, only the trigger mode needs to flip.

- [ ] **Step 2: Run the tests, verify they fail**

Run:
`cd api && go test ./internal/engine/hand/... -run 'TestSandboxRakeUsesPercentageAndPlayerCaps|TestRealMoneyAndPreflopPotsHaveNoRake|TestRakeConfigurationSurvivesPersistence|TestHandOutcomeUsesNetOfRakeWinnerAndExcludesRefunds' -v`
Expected: FAIL — `ConfigureRake` still gives `real` the 250bps, so `TestRealMoneyAndPreflopPotsHaveNoRake`'s first
assertion (`cap != 0` for `"real"`) fails, and `TestSandboxRakeUsesPercentageAndPlayerCaps` fails since `"sandbox"`
currently gives 0bps.

- [ ] **Step 3: Flip `ConfigureRake`**

In `api/internal/engine/hand/hand.go`, replace:

```go
// ConfigureRake enables the standard 2.5% real-money rake. Sandbox tables
// always remain rake-free. The setting is persisted with the table state.
func (t *Table) ConfigureRake(currencyMode string) {
	if currencyMode == "real" {
		t.rakeBPS = 250
		return
	}
	t.rakeBPS = 0
}
```

with:

```go
// ConfigureRake enables the standard 2.5% sandbox rake. Real-money tables
// are always rake-free — Brazilian law treats a cut of the pot/blind on a
// public real-money game as a bet requiring SPA authorization; poker's
// real-money revenue comes entirely from the fixed table-entry fee charged
// at buy-in instead (buyin.Service.BuyIn), never from the pot. The setting
// is persisted with the table state.
func (t *Table) ConfigureRake(currencyMode string) {
	if currencyMode == "sandbox" {
		t.rakeBPS = 250
		return
	}
	t.rakeBPS = 0
}
```

- [ ] **Step 4: Run the tests, verify they pass**

Run: `cd api && go test ./internal/engine/hand/... -v`
Expected: PASS, full package.

- [ ] **Step 5: Commit**

```bash
git add api/internal/engine/hand/hand.go api/internal/engine/hand/rake_test.go api/internal/engine/hand/phase4_test.go
git commit -m "feat(engine): sandbox tables now take rake, real-money tables never do"
```

---

### Task 4: `walletclient.DebitReal` — charge the fixed fee against the player's real wallet

**Files:**

- Modify: `api/internal/walletclient/client.go`
- Modify: `api/internal/walletclient/client_test.go`

**Interfaces:**

- Produces:
  `(*Client).DebitReal(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error` —
  consumed by Task 6 (`buyin.Service`) and Task 7 (`cmd/reconcile`).

- [ ] **Step 1: Write the failing test**

`client_test.go` already has two helpers used by every existing test: `fakeWalletServer(t, onMovement)` (a mux serving
`/v1.0/token` plus the sandbox credit/debit paths) and `fakeAuthServer(t)`. Neither mux registers
`/v1.0/internal/wallet/real/debit`, so this test stands up its own tiny mux instead of extending `fakeWalletServer` (
which is also used by sandbox-only tests and shouldn't grow a real-money path). Add:

```go
func TestDebitRealSendsExpectedRequestBody(t *testing.T) {
	var gotPath string
	var gotBody MovementRequest
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/internal/wallet/real/debit", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "entry-3"})
	})
	srv := httptest.NewServer(mux)
	authSrv := fakeAuthServer(t)
	defer srv.Close()
	defer authSrv.Close()

	c := New(&config.Config{WalletURL: srv.URL, CtechURL: authSrv.URL, PokerClientID: "poker", PokerClientSecret: "secret"}, cache.NewMemoryBackend(10))
	if err := c.DebitReal(t.Context(), "user-1", 100, "room-1#user-1#buyinfee#n1", "poker_table_fee"); err != nil {
		t.Fatalf("DebitReal: %v", err)
	}
	if gotPath != "/v1.0/internal/wallet/real/debit" {
		t.Fatalf("expected path /v1.0/internal/wallet/real/debit, got %s", gotPath)
	}
	if gotBody.UserID != "user-1" || gotBody.Amount != 100 || gotBody.Reason != "poker_table_fee" {
		t.Fatalf("unexpected request body: %+v", gotBody)
	}
}
```

- [ ] **Step 2: Run the test, verify it fails**

Run: `cd api && go test ./internal/walletclient/... -run TestDebitRealSendsExpectedRequestBody -v`
Expected: FAIL with `client.DebitReal undefined`.

- [ ] **Step 3: Add the method**

In `api/internal/walletclient/client.go`, add to the `const` block:

```go
	pathRealDebit = "/v1.0/internal/wallet/real/debit"
```

and:

```go
	scopeDebitReal   = "internal:wallet:debit-real"
```

Add to the `Client` struct:

```go
	debitRealTokens   *oauth2client.TokenManager
```

Wire it in `New`:

```go
		debitRealTokens:   oauth2client.New(httpClient, cacheB, baseAuth+pathToken, cfg.PokerClientID, cfg.PokerClientSecret, scopeDebitReal),
```

Add the method (near `Debit`):

```go
// DebitReal charges a fixed amount directly against the player's real
// (withdrawable) wallet — used for poker's fixed table-entry fee, which is
// platform revenue and never part of the at-stake game-wallet pot (see
// buyin.Service.BuyIn). ctech-wallet already exposes this endpoint for
// ctech-billing's subscription charges; poker's own M2M client additionally
// needs the internal:wallet:debit-real scope granted in ctech-account before
// this can succeed in any real environment (see this plan's Global Constraints
// — a cross-repo/config blocker, not a code gap here).
func (c *Client) DebitReal(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error {
	return c.movement(ctx, c.base+pathRealDebit, c.debitRealTokens, userID, amount, idempotencyKey, reason)
}
```

- [ ] **Step 4: Run the test, verify it passes**

Run: `cd api && go test ./internal/walletclient/... -v`
Expected: PASS, full package.

- [ ] **Step 5: Commit**

```bash
git add api/internal/walletclient/client.go api/internal/walletclient/client_test.go
git commit -m "feat(walletclient): add DebitReal for the real-money table-entry fee"
```

---

### Task 5: `reconcile.PendingCashout` gains a `Kind` so the same pending-retry table can also track a failed fee debit

**Files:**

- Modify: `api/internal/reconcile/pending.go`

**Interfaces:**

- Produces: `reconcile.PendingCashout.Kind string`, `reconcile.KindCashout = "cashout"`,
  `reconcile.KindFeeDebit = "fee_debit"` — consumed by Task 6 (`buyin.Service`, writes `KindFeeDebit` entries) and Task
  7 (`cmd/reconcile`, branches on `Kind`).

- [ ] **Step 1: Add the field and constants**

In `api/internal/reconcile/pending.go`, add above the `PendingCashout` struct:

```go
// Kind values for PendingCashout.Kind. Empty string means KindCashout, for
// backward compatibility with entries recorded before this field existed.
const (
	KindCashout  = "cashout"
	KindFeeDebit = "fee_debit"
)
```

Add the field to `PendingCashout` (after `CurrencyMode`):

```go
	CurrencyMode   string   `dynamodbav:"currency_mode" json:"currency_mode"` // "sandbox" | "real"
	// Kind distinguishes what this pending entry retries. Empty/KindCashout:
	// credit a player's final stack back to their wallet (the original use of
	// this store). KindFeeDebit: charge the fixed real-money table-entry fee
	// that failed after the player was already seated (buyin.Service.BuyIn)
	// — same retry-until-resolved shape, opposite direction of money movement.
	Kind           string   `dynamodbav:"kind,omitempty" json:"kind,omitempty"`
```

This task has no new test of its own — `pending.go`'s existing `Record`/`ListUnresolved`/`MarkResolved` logic doesn't
branch on any field value, so adding an optional field needs no new coverage here; Tasks 6 and 7 each add tests that
exercise `Kind` through their own call sites.

- [ ] **Step 2: Run the existing package tests, verify they still pass**

Run: `cd api && go test ./internal/reconcile/... -v` (the `pending_test.go` file is `//go:build integration` — this will
report "no test files" without `-tags integration`; that's expected and fine, it means nothing in this package needs a
live DynamoDB to compile-check).
Run: `cd api && go build ./...` to confirm the whole module still compiles.
Expected: build succeeds.

- [ ] **Step 3: Commit**

```bash
git add api/internal/reconcile/pending.go
git commit -m "feat(reconcile): add Kind to PendingCashout so fee-debit failures can reuse the same retry table"
```

---

### Task 6: `buyin.Service.BuyIn` charges the fixed entry fee after every successful seat

**Files:**

- Modify: `api/internal/buyin/service.go`
- Modify: `api/internal/buyin/service_test.go`
- Modify: `api/internal/buyin/terms_test.go`

**Interfaces:**

- Consumes: `roomstore.Room.EntryFeeCents` (Task 2), `walletMover.DebitReal` (Task 4, added to the interface here),
  `reconcile.KindFeeDebit` (Task 5).

- [ ] **Step 1: Add `DebitReal` to the `walletMover` interface**

In `api/internal/buyin/service.go`, extend the interface:

```go
type walletMover interface {
	Credit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
	Debit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
	HoldGame(ctx context.Context, userID string, amount int64, tableRef, idempotencyKey, reason string) (string, error)
	ReleaseHold(ctx context.Context, holdID string) error
	CashoutGame(ctx context.Context, userID string, amount int64, tableRef string, holdIDs []string, idempotencyKey, reason string) error
	DebitReal(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
}
```

- [ ] **Step 2: Add `DebitReal` stubs to the test fakes (mechanical — required for the package to compile)**

In `api/internal/buyin/service_test.go`, add after `CashoutGame`:

```go
func (f *fakeWallet) DebitReal(_ context.Context, userID string, amount int64, key, _ string) error {
	f.feeDebits = append(f.feeDebits, call{userID, amount, key})
	return nil
}
```

and add `feeDebits []call` to the `fakeWallet` struct fields.

In `api/internal/buyin/terms_test.go`, add after its `CashoutGame` stub:

```go
func (w *gateWallet) DebitReal(context.Context, string, int64, string, string) error { return nil }
```

- [ ] **Step 3: Write the failing tests**

Add to `api/internal/buyin/service_test.go`:

```go
func TestBuyInChargesFixedEntryFeeForRealRoomsAfterSeating(t *testing.T) {
	sandbox := &fakeWallet{}
	game := &fakeWallet{}
	mgr := testManager(t)
	rooms := &fakeRoomLookup{room: &roomstore.Room{
		ID: "room-real-fee", CurrencyMode: "real", BigBlind: 20, BuyInMin: 40, BuyInMax: 400, MaxSeats: 9,
		EntryFeeCents: 100,
	}}
	svc := NewServiceWithGame(sandbox, game, mgr, rooms, &fakeActivation{activated: map[string]bool{"user-1": true}})
	ctx := context.Background()

	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	if _, err := mgr.GetOrCreateActor(ctx, "room-real-fee", seed); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	if err := svc.BuyIn(ctx, "room-real-fee", "user-1", 400, false, "nonce-1"); err != nil {
		t.Fatalf("buyin: %v", err)
	}
	if len(game.feeDebits) != 1 || game.feeDebits[0].amount != 100 || game.feeDebits[0].userID != "user-1" {
		t.Fatalf("expected one 100-cent fee debit for user-1, got %+v", game.feeDebits)
	}
	if len(game.holds) != 1 || game.holds[0].amount != 400 {
		t.Fatalf("expected the stake hold to remain 400 (fee is separate), got %+v", game.holds)
	}
}

func TestBuyInChargesFeeAgainOnRebuyAfterLeaving(t *testing.T) {
	sandbox := &fakeWallet{}
	game := &fakeWallet{}
	mgr := testManager(t)
	rooms := &fakeRoomLookup{room: &roomstore.Room{
		ID: "room-real-rebuy", CurrencyMode: "real", BigBlind: 20, BuyInMin: 40, BuyInMax: 400, MaxSeats: 9,
		EntryFeeCents: 100,
	}}
	svc := NewServiceWithGame(sandbox, game, mgr, rooms, &fakeActivation{activated: map[string]bool{"user-1": true}})
	ctx := context.Background()

	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	if _, err := mgr.GetOrCreateActor(ctx, "room-real-rebuy", seed); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	if err := svc.BuyIn(ctx, "room-real-rebuy", "user-1", 400, false, "nonce-1"); err != nil {
		t.Fatalf("first buyin: %v", err)
	}
	if _, err := svc.CashOut(ctx, "room-real-rebuy", "user-1", ""); err != nil {
		t.Fatalf("cashout: %v", err)
	}
	// A fresh nonce, exactly like the UI's rebuy flow (RebuyDialog calls the
	// same joinRoom as a first-time join, with a new crypto.randomUUID()).
	if err := svc.BuyIn(ctx, "room-real-rebuy", "user-1", 400, false, "nonce-2"); err != nil {
		t.Fatalf("rebuy: %v", err)
	}
	if len(game.feeDebits) != 2 {
		t.Fatalf("expected the fee to be charged again on rebuy after leaving, got %+v", game.feeDebits)
	}
}

func TestBuyInSkipsFeeForSandboxRooms(t *testing.T) {
	wallet := &fakeWallet{}
	mgr := testManager(t)
	rooms := testRoomLookup() // sandbox room, EntryFeeCents unset (0)
	svc := NewService(wallet, mgr, rooms)
	ctx := context.Background()

	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	if _, err := mgr.GetOrCreateActor(ctx, "room-sandbox-fee", seed); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := svc.BuyIn(ctx, "room-sandbox-fee", "user-1", 400, false, ""); err != nil {
		t.Fatalf("buyin: %v", err)
	}
	if len(wallet.feeDebits) != 0 {
		t.Fatalf("expected no fee debit for a sandbox room, got %+v", wallet.feeDebits)
	}
}
```

Note: `TestBuyInUsesGameWalletForRealRooms` (existing test, room has no `EntryFeeCents` set, so it defaults to 0) must
keep passing unchanged — it's an implicit regression check that a real room with `EntryFeeCents == 0` never calls
`DebitReal` either, same as sandbox.

- [ ] **Step 4: Run the tests, verify they fail**

Run:
`cd api && go test ./internal/buyin/... -run 'TestBuyInChargesFixedEntryFeeForRealRoomsAfterSeating|TestBuyInChargesFeeAgainOnRebuyAfterLeaving|TestBuyInSkipsFeeForSandboxRooms' -v`
Expected: FAIL — `game.feeDebits` stays empty since nothing calls `DebitReal` yet. (`terms_test.go` and
`service_test.go` must already compile at this point given Steps 1–2; if they don't, fix the fakes first.)

- [ ] **Step 5: Implement the charge in `BuyIn`**

In `api/internal/buyin/service.go`, change the top of `BuyIn` to hoist `room` to function scope:

```go
func (s *Service) BuyIn(ctx context.Context, roomID, playerID string, amount int64, midHand bool, idemKey string) error {
	maxSeats := 0
	var room *roomstore.Room
	if s.rooms != nil {
		var err error
		room, err = s.rooms.Get(ctx, roomID)
		if err != nil {
			return fmt.Errorf("buyin: load room: %w", err)
		}
		if room == nil {
			return fmt.Errorf("buyin: room not found")
		}
		if room.BigBlind <= 0 || amount < room.BuyInMin || amount > room.BuyInMax || amount <= 0 || amount%room.BigBlind != 0 {
			return fmt.Errorf("buyin: amount outside room limits")
		}
		maxSeats = room.MaxSeats
	}
```

Then, at the very end of `BuyIn` — after the existing session-log recording block, replacing the final `return nil`:

```go
	if room != nil && room.CurrencyMode == "real" && room.EntryFeeCents > 0 {
		feeKey := fmt.Sprintf("%s#%s#buyinfee#%s", roomID, playerID, nonce)
		if err := s.game.DebitReal(ctx, playerID, room.EntryFeeCents, feeKey, "poker_table_fee"); err != nil {
			slog.Error("ALARM: poker table entry fee charge failed after seating, needs manual review",
				"player", playerID, "room", roomID, "amount", room.EntryFeeCents, "err", err)
			if s.pending != nil {
				_ = s.pending.Record(ctx, reconcile.PendingCashout{
					ID: feeKey, PlayerID: playerID, Amount: room.EntryFeeCents, CurrencyMode: "real",
					Kind: reconcile.KindFeeDebit, TableRef: roomID, IdempotencyKey: feeKey,
				})
			}
			return fmt.Errorf("buyin: table fee charge failed after seating — reconciliation job will retry: %w", err)
		}
	}

	return nil
}
```

(`room.CurrencyMode == "real"` guarantees `s.game` is non-nil here — `walletFor` already required and used it earlier in
this same call for the stake hold, or this function would have already returned.)

- [ ] **Step 6: Run the tests, verify they pass**

Run: `cd api && go test ./internal/buyin/... -v`
Expected: PASS, full package.

- [ ] **Step 7: Commit**

```bash
git add api/internal/buyin/service.go api/internal/buyin/service_test.go api/internal/buyin/terms_test.go
git commit -m "feat(buyin): charge the fixed real-money table-entry fee on every seat-entry"
```

---

### Task 7: `cmd/reconcile` retries a failed fee debit the same way it retries a failed cash-out credit

**Files:**

- Modify: `api/cmd/reconcile/main.go`

**Interfaces:**

- Consumes: `walletclient.Client.DebitReal` (Task 4), `reconcile.PendingCashout.Kind` / `reconcile.KindFeeDebit` (Task
  5).

- [ ] **Step 1: Write the failing test**

In `api/cmd/reconcile/main_test.go`, add a fake and a test, and update the existing test's call site:

```go
type fakeFeeDebiter struct {
	debits []reconcile.PendingCashout
}

func (f *fakeFeeDebiter) DebitReal(_ context.Context, userID string, amount int64, idempotencyKey, reason string) error {
	f.debits = append(f.debits, reconcile.PendingCashout{PlayerID: userID, Amount: amount, IdempotencyKey: idempotencyKey})
	return nil
}
```

Change `TestRunResolvesUnresolvedCashouts`'s call from `run(context.Background(), pending, game, sandbox)` to
`run(context.Background(), pending, game, sandbox, &fakeFeeDebiter{})`.

Add:

```go
func TestRunRetriesFeeDebit(t *testing.T) {
	pending := &fakePendingLister{
		unresolved: []reconcile.PendingCashout{
			{ID: "fee-1", PlayerID: "user-1", Amount: 100, CurrencyMode: "real", Kind: reconcile.KindFeeDebit, IdempotencyKey: "k1"},
		},
	}
	fee := &fakeFeeDebiter{}

	if err := run(context.Background(), pending, &fakeGameCredit{}, &fakeSandboxCredit{}, fee); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(pending.resolved) != 1 || pending.resolved[0] != "fee-1" {
		t.Fatalf("expected fee-1 resolved, got %v", pending.resolved)
	}
	if len(fee.debits) != 1 || fee.debits[0].PlayerID != "user-1" || fee.debits[0].Amount != 100 {
		t.Fatalf("expected one 100-cent fee retry for user-1, got %+v", fee.debits)
	}
}
```

- [ ] **Step 2: Run the tests, verify they fail**

Run: `cd api && go test ./cmd/reconcile/... -v`
Expected: compile error (`run` still takes 3 args, `fakeFeeDebiter` unused test won't build) — this is expected, fix in
the next step.

- [ ] **Step 3: Update `run` to branch on `Kind`**

In `api/cmd/reconcile/main.go`, add the interface and change `run`'s signature and body:

```go
type feeDebiter interface {
	DebitReal(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
}

func run(ctx context.Context, pending pendingLister, game gameCredit, sandbox sandboxCredit, fee feeDebiter) error {
	entries, err := pending.ListUnresolved(ctx, gracePeriod)
	if err != nil {
		return fmt.Errorf("reconcile: list unresolved: %w", err)
	}
	for _, e := range entries {
		var opErr error
		switch e.Kind {
		case reconcile.KindFeeDebit:
			opErr = fee.DebitReal(ctx, e.PlayerID, e.Amount, e.IdempotencyKey, "poker_table_fee_reconcile")
		default:
			switch e.CurrencyMode {
			case "real":
				tableRef := e.TableRef
				if tableRef == "" {
					tableRef = "unknown"
				}
				opErr = game.CashoutGame(ctx, e.PlayerID, e.Amount, tableRef, e.HoldIDs, e.IdempotencyKey, "poker_cashout_reconcile")
			default:
				if sandbox != nil {
					opErr = sandbox.Credit(ctx, e.PlayerID, e.Amount, e.IdempotencyKey, "poker_cashout_reconcile")
				}
			}
		}

		if opErr != nil {
			slog.Error("ALARM: reconcile operation failed, needs manual review",
				"pending_id", e.ID, "kind", e.Kind, "player", e.PlayerID, "amount", e.Amount, "err", opErr)
			continue
		}
		if err := pending.MarkResolved(ctx, e.ID); err != nil {
			slog.Error("ALARM: reconcile resolved operation but failed to mark pending entry resolved",
				"pending_id", e.ID, "err", err)
		}
	}
	return nil
}
```

Update `handler()`'s call site:

```go
	return run(ctx, pendingStore, wallet, wallet, wallet)
```

(`wallet` is a single `*walletclient.Client` — it already satisfies `gameCredit`, `sandboxCredit`, and now `feeDebiter`
simultaneously, same as today.)

- [ ] **Step 4: Run the tests, verify they pass**

Run: `cd api && go test ./cmd/reconcile/... -v`
Expected: PASS, full package.

- [ ] **Step 5: Commit**

```bash
git add api/cmd/reconcile/main.go api/cmd/reconcile/main_test.go
git commit -m "feat(reconcile): retry a failed real-money table-entry fee debit the same way as a failed cash-out credit"
```

---

### Task 8: UI — disclose the fee before it's charged, correct the rake copy

**Files:**

- Modify: `ui/src/lib/api/rooms.ts`
- Modify: `ui/src/components/lobby/CreateRoomDialog.tsx`
- Modify: `ui/src/components/table/BuyInPanel.tsx`
- Modify: `ui/src/components/table/RebuyDialog.tsx`
- Modify: `ui/src/app/poker-rules/page.tsx`

**Interfaces:**

- Consumes: API's new `fee_cents` (stakes catalog) and `entry_fee_cents` (room) fields — both already flow through
  existing generic JSON decoding, no new endpoint needed.

- [ ] **Step 1: Add the new fields to the TypeScript types**

In `ui/src/lib/api/rooms.ts`, add to `Room`:

```ts
  // Fixed real-money table-entry fee (BRL cents), charged on every seat-entry
  // (join/rebuy) — zero/absent for sandbox rooms. Set once at room creation,
  // never a function of the pot (see docs/plans/2026-07-25-realmoney-fixed-fee-and-sandbox-rake.md).
  entry_fee_cents?: number;
```

and to `Stake`:

```ts
export interface Stake {
  small_blind: number;
  big_blind: number;
  fee_cents?: number;
}
```

- [ ] **Step 2: Show the fee per tier in `CreateRoomDialog`**

In `ui/src/components/lobby/CreateRoomDialog.tsx`, change the stake button's label (inside the `stakes.map(...)` render,
currently just `{formatStake(stake.small_blind, currencyMode)} / {formatStake(stake.big_blind, currencyMode)}`) to:

```tsx
{formatStake(stake.small_blind, currencyMode)} / {formatStake(stake.big_blind, currencyMode)}
{currencyMode === 'real' && stake.fee_cents ? <><br/><small>taxa {formatStake(stake.fee_cents, 'real')}</small></> : null}
```

- [ ] **Step 3: Show the fee in `BuyInPanel`**

In `ui/src/components/table/BuyInPanel.tsx`, change the intro paragraph and add a fee line. Replace:

```tsx
      <p>Escolha {isReal ? 'quanto dinheiro' : 'quantas fichas'} levar. Nada é debitado antes de você confirmar.</p>
```

with:

```tsx
      <p>Escolha {isReal ? 'quanto dinheiro' : 'quantas fichas'} levar. Nada é debitado antes de você confirmar.</p>
      {isReal && !!room.entry_fee_cents &&
        <p className="buyin-fee-notice">Taxa fixa de mesa: {formatBuyIn(room.entry_fee_cents, true)} (cobrada
          junto com o buy-in, não é uma comissão sobre o pote).</p>}
```

- [ ] **Step 4: Show the fee in `RebuyDialog`**

In `ui/src/components/table/RebuyDialog.tsx`, add the same disclosure inside `DialogContent`, right after
`DialogDescription`:

```tsx
      </DialogHeader>
      {isReal && !!room.entry_fee_cents &&
        <p className="buyin-fee-notice">Taxa fixa de mesa: {formatBuyIn(room.entry_fee_cents, true)} (cobrada
          de novo a cada vez que você compra fichas).</p>}
```

- [ ] **Step 5: Correct the rake/fee explanation copy**

In `ui/src/app/poker-rules/page.tsx`, replace the `Rake` article body:

```tsx
        <article id="rake" className="rules-section">
          <h2>Rake</h2>
          <p>O rake é a comissão que a casa retém sobre o pote de cada mão — é assim que a mesa se sustenta. O valor
            fica sempre visível ao lado do pote durante o jogo, nunca embutido ou escondido.</p>
        </article>
```

with:

```tsx
        <article id="rake" className="rules-section">
          <h2>Rake</h2>
          <p>Nas mesas sandbox, o rake é a comissão que a casa retém sobre o pote de cada mão — é assim que a mesa se
            sustenta. O valor fica sempre visível ao lado do pote durante o jogo, nunca embutido ou escondido.</p>
          <p>Nas mesas de dinheiro real, não existe rake: todo o dinheiro do pote é decidido pelos jogadores na mesa.
            Em vez disso, cobramos uma taxa fixa de mesa ao entrar (o "aluguel" da sala), que não depende do
            tamanho do pote nem é um percentual do blind.</p>
        </article>
```

- [ ] **Step 6: Type-check and lint**

Run: `cd ui && npx tsc --noEmit && npx eslint src --max-warnings 0`
Expected: no errors, no warnings (per `ui/CLAUDE.md`'s quality gate — there is no test script for this repo, this is the
gate).

- [ ] **Step 7: Manually verify in the browser**

Start the dev server (`cd ui && npm run dev`), open the lobby, open "Mesa privada", switch to "Dinheiro real", and
confirm each stake button shows its fee. Then create a real-money room and confirm the buy-in screen shows the fixed-fee
disclosure line before confirming. This requires `REAL_MONEY_ENABLED=true` on the API the UI points at — if that's not
available locally, at minimum verify the sandbox path renders unaffected (no fee line appears) and `npx tsc --noEmit` is
the fallback verification per the "if you can't test the UI, say so explicitly" rule.

- [ ] **Step 8: Commit**

```bash
git add ui/src/lib/api/rooms.ts ui/src/components/lobby/CreateRoomDialog.tsx ui/src/components/table/BuyInPanel.tsx ui/src/components/table/RebuyDialog.tsx ui/src/app/poker-rules/page.tsx
git commit -m "feat(ui): disclose the fixed real-money table fee before charging it, fix rake copy"
```

---

## Post-plan cleanup (not a task — do after all 8 land)

- Update `api/CLAUDE.md`'s Phase 5 status paragraph and `ui/CLAUDE.md`'s stale "real-money UI is DESIGNED-ONLY" line (it
  already isn't — `CreateRoomDialog.tsx`/`BuyInPanel.tsx`/`RebuyDialog.tsx` all handle `currency_mode: "real"` today) to
  describe the fixed-fee model instead of the old rake-based one.
- File/track the cross-repo `internal:wallet:debit-real` scope grant for poker's M2M client in `ctech-account` — this
  plan cannot land in prod without it (same blocking category as the existing `internal:wallet:game-status` gap).
