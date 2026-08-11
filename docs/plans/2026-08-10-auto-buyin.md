# Auto Buy-In (Auto Rebuy) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. **Every frontend task in this plan (Tasks 7-10) MUST be executed via the `/impeccable` skill**, per explicit user instruction — do not hand-edit the frontend files directly outside that skill.

**Goal:** Let a player opt into "auto rebuy" when joining a sandbox table; when they bust, the server automatically rebuys them for their original buy-in amount so they keep playing the next hand without manual action, falling back to sitting-out (with an in-place PIX top-up offer if their balance is genuinely zero) when they can't afford it.

**Architecture:** Two new immutable per-seat fields (`AutoRebuy`, `BuyInAmount`) are set once at fresh-seat creation and threaded through the existing join path (HTTP → `buyin.Service` → `table.JoinCmd` → `hand.Table`). A new post-hand hook (`tablemanager.Manager.SetOnAutoRebuySweep`, wired in `app.go` because of the same buyin/tablemanager construction cycle `wirePlayerRemovedHook` already works around) reads every hand participant's post-hand seat state and auto-rebuys anyone who busted with auto-rebuy on and sufficient sandbox balance — entirely in a detached goroutine, because the hook fires synchronously on the table actor's own single-goroutine command loop and both the read (`SeatedSummary`) and the write (`BuyIn`) call `actor.Dispatch`, which would otherwise deadlock the table. The client learns about a bust seat's auto-rebuy status from a new `Seat.auto_rebuy` wire field and shows either nothing (rebuy resolves within a grace window), the existing manual rebuy dialog, or an embedded PIX top-up flow.

**Tech Stack:** Go 1.x (Fiber v3, DynamoDB, `uber-go/fx`), Next.js/React/TypeScript, Protobuf (`protoc` + `protoc-gen-go` + `ts_proto`).

**Design doc:** `docs/specs/2026-08-10-auto-buyin-design.md` — read it first for the product rationale (scope, edge cases, why sandbox-only) this plan doesn't repeat.

## Global Constraints

- **Sandbox rooms only.** The auto-rebuy sweep must check `room.CurrencyMode == roomstore.CurrencyModeSandbox` before doing anything. Real-money rooms are explicitly out of scope (see spec's Scope section — the fixed entry fee would be re-charged on every auto-rebuy).
- **`AutoRebuy` and `BuyInAmount` are set exactly once**, at fresh-seat creation (`hand.Table.AddWaitingPlayer`/`AddMidHandJoiner`'s new-`Player` path). `rebuyExisting` must never touch either field — this is what makes "auto-rebuy amount = original buy-in" hold even after later manual rebuys at a different amount.
- **Never call `buyin.Service.BuyIn`/`SeatedSummary` (or anything that calls `actor.Dispatch`) synchronously from a table-actor hook.** `onHandComplete`-style hooks run inline on the actor's own goroutine; a synchronous `Dispatch` from there deadlocks the table. Always detach with `go func() { ... }()` at the point the hook fires, before doing any actor-touching work.
- **Named constants, no magic strings** (per `api/CLAUDE.md`) — use `roomstore.CurrencyModeSandbox`, not `"sandbox"` literals, in new Go code.
- **Every code change must be documented** (per `api/CLAUDE.md` / `ui/CLAUDE.md`) — doc comments on new exported types/functions are part of "done," not an afterthought.
- **UI must assume real money is off** (per `ui/CLAUDE.md`) — the auto-rebuy checkbox and rebuy-dialog changes must work correctly with `REAL_MONEY_ENABLED` off; do not gate anything on real-money state.
- **Quality gates:** backend — `go build ./... && go test ./... -race` (integration-tagged tests need `docker-compose -f docker-compose.test.yml up -d` per `api/README.md`, run with `go test -tags integration ./...`). Frontend — `npx vitest run`, `npx tsc --noEmit`, `npx eslint src --max-warnings 0`, `npm run build`, all zero-error/zero-warning.

---

## Task 1: Engine data model — `AutoRebuy`/`BuyInAmount` on `hand.Player`, threaded to the snapshot

**Files:**
- Modify: `api/internal/engine/hand/hand.go:49-87` (`Player` struct)
- Modify: `api/internal/engine/hand/snapshot.go:111-136` (`SeatView` struct), `:226-242` (`ViewFor`'s per-seat loop)
- Modify: `api/internal/table/commands.go` (`JoinCmd` struct, around the existing `MidHand`/`HoldID` fields)
- Modify: `api/internal/table/actor.go:1507` (`applyJoinAndCommit`'s new-`Player` literal)
- Test: `api/internal/engine/hand/hand_test.go`, `api/internal/engine/hand/snapshot_test.go`

**Interfaces:**
- Produces: `hand.Player.AutoRebuy bool`, `hand.Player.BuyInAmount int64` — read by `buyin.Service.SeatedSummary` (Task 2) via `hand.SeatView.AutoRebuy`/`BuyInAmount`.
- Produces: `table.JoinCmd.AutoRebuy bool` — set by `buyin.Service.BuyInWithAutoRebuy` (Task 2).

- [ ] **Step 1: Write the failing tests**

In `api/internal/engine/hand/hand_test.go`, add (mirrors the existing `TestAddWaitingPlayerRebuysBustedSeatInsteadOfRejecting` right after it):

```go
// TestAddWaitingPlayerSetsAutoRebuyAndBuyInAmountOnFreshSeat guards the only
// place these two fields are ever written: a brand-new seat.
func TestAddWaitingPlayerSetsAutoRebuyAndBuyInAmountOnFreshSeat(t *testing.T) {
	table := NewTable(nil, 10, 20)
	p := &Player{ID: "p1", Stack: 500, AutoRebuy: true, BuyInAmount: 500}
	if err := table.AddWaitingPlayer(p); err != nil {
		t.Fatalf("AddWaitingPlayer: %v", err)
	}
	seated := table.playerByID("p1")
	if !seated.AutoRebuy || seated.BuyInAmount != 500 {
		t.Fatalf("expected AutoRebuy=true BuyInAmount=500, got AutoRebuy=%v BuyInAmount=%d", seated.AutoRebuy, seated.BuyInAmount)
	}
}

// TestAddWaitingPlayerRebuyKeepsOriginalAutoRebuyAndBuyInAmount is the
// invariant the whole feature depends on: a later rebuy (manual or the
// server's own auto-rebuy) must never change what "the original buy-in" was,
// even if the rebuy amount differs.
func TestAddWaitingPlayerRebuyKeepsOriginalAutoRebuyAndBuyInAmount(t *testing.T) {
	busted := &Player{ID: "p1", Stack: 0, Ready: true, State: SittingOut, AutoRebuy: true, BuyInAmount: 500}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	p3 := &Player{ID: "p3", Stack: 1000, Ready: true}
	p4 := &Player{ID: "p4", Stack: 1000, Ready: true}
	table := NewTable([]*Player{busted, p2, p3, p4}, 10, 20)
	table.dealerDrawn = true

	rebuy := &Player{ID: "p1", Stack: 1000} // a manual rebuy for a DIFFERENT amount, AutoRebuy unset
	if err := table.AddWaitingPlayer(rebuy); err != nil {
		t.Fatalf("AddWaitingPlayer rebuy: %v", err)
	}
	if busted.Stack != 1000 {
		t.Fatalf("expected stack credited to 1000, got %d", busted.Stack)
	}
	if !busted.AutoRebuy || busted.BuyInAmount != 500 {
		t.Fatalf("rebuy must not change AutoRebuy/BuyInAmount, got AutoRebuy=%v BuyInAmount=%d", busted.AutoRebuy, busted.BuyInAmount)
	}
}
```

In `api/internal/engine/hand/snapshot_test.go`, add right after `TestRunItTwicePreferenceIsViewerPrivate` (same file, same pattern):

```go
func TestAutoRebuyPreferenceIsViewerPrivate(t *testing.T) {
	table := NewTable([]*Player{
		{ID: "p1", Stack: 1000, AutoRebuy: true, BuyInAmount: 1000},
		{ID: "p2", Stack: 1000, AutoRebuy: true, BuyInAmount: 1000},
	}, 10, 20)
	view := table.ViewFor("p1")
	for _, seat := range view.Seats {
		switch seat.PlayerID {
		case "p1":
			if !seat.AutoRebuy {
				t.Fatal("viewer must receive their own auto-rebuy preference")
			}
		case "p2":
			if seat.AutoRebuy {
				t.Fatal("opponent's auto-rebuy preference must not be exposed")
			}
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/engine/hand/... -run 'TestAddWaitingPlayerSetsAutoRebuyAndBuyInAmountOnFreshSeat|TestAddWaitingPlayerRebuyKeepsOriginalAutoRebuyAndBuyInAmount|TestAutoRebuyPreferenceIsViewerPrivate' -v` (from `api/`)
Expected: FAIL — `Player`/`SeatView` have no field `AutoRebuy`/`BuyInAmount` (compile error).

- [ ] **Step 3: Add the fields and wire them through**

In `api/internal/engine/hand/hand.go`, in the `Player` struct, right after the `RunItTwice` field (line 54):

```go
	RunItTwice     bool         `dynamodbav:"run_it_twice,omitempty"`
	// AutoRebuy and BuyInAmount are set exactly once, at fresh-seat creation
	// (AddWaitingPlayer/AddMidHandJoiner's new-Player branch) — rebuyExisting
	// never touches either, so a later manual rebuy at a different amount
	// can't change what the server auto-rebuys back to.
	AutoRebuy      bool         `dynamodbav:"auto_rebuy,omitempty"`
	BuyInAmount    int64        `dynamodbav:"buy_in_amount,omitempty"`
```

In `api/internal/engine/hand/snapshot.go`, in `SeatView` (after `RunItTwice` at line 135):

```go
	RunItTwice bool   `json:"run_it_twice,omitempty"`
	// AutoRebuy mirrors Player.AutoRebuy, viewer-private like RunItTwice.
	// BuyInAmount is not wire-facing (never converted to the proto Seat in
	// tablews.go) — it exists on SeatView only so buyin.Service.SeatedSummary
	// can read it off the same snapshot machinery Seated already uses.
	AutoRebuy   bool  `json:"auto_rebuy,omitempty"`
	BuyInAmount int64 `json:"buy_in_amount,omitempty"`
```

In `ViewFor` (snapshot.go:240-242), extend the viewer-only block:

```go
		if p.ID == viewerID {
			sv.RunItTwice = p.RunItTwice
			sv.AutoRebuy = p.AutoRebuy
			sv.BuyInAmount = p.BuyInAmount
		}
```

In `api/internal/table/commands.go`, in `JoinCmd`, add a field next to `HoldID`:

```go
type JoinCmd struct {
	PlayerID string
	Stack    int64
	MaxSeats int
	MidHand          bool
	HoldID           string
	AutoRebuy        bool
	SettlementIntent func() (types.TransactWriteItem, error)
	Reply            chan error
}
```

In `api/internal/table/actor.go:1507`, thread it into the new-`Player` literal:

```go
	p := &hand.Player{ID: c.PlayerID, Stack: c.Stack, HoldID: c.HoldID, LastActionAt: timeNowFunc().UnixMilli(), AutoRebuy: c.AutoRebuy, BuyInAmount: c.Stack}
```

(`BuyInAmount: c.Stack` — the amount just joined with is, by definition, the original buy-in for a fresh seat. This same literal feeds both `AddMidHandJoiner` and `AddWaitingPlayer` below it, and both delegate to `rebuyExisting` for an already-seated player, which never reads `AutoRebuy`/`BuyInAmount` off `incoming` — so this line is the *only* place either field is ever set, satisfying the "exactly once" constraint.)

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/engine/hand/... -run 'TestAddWaitingPlayerSetsAutoRebuyAndBuyInAmountOnFreshSeat|TestAddWaitingPlayerRebuyKeepsOriginalAutoRebuyAndBuyInAmount|TestAutoRebuyPreferenceIsViewerPrivate' -v`
Expected: PASS. Then run the full package to check for regressions: `go test ./internal/engine/hand/... ./internal/table/...`

- [ ] **Step 5: Commit**

```bash
git add api/internal/engine/hand/hand.go api/internal/engine/hand/snapshot.go api/internal/engine/hand/hand_test.go api/internal/engine/hand/snapshot_test.go api/internal/table/commands.go api/internal/table/actor.go
git commit -m "feat: add AutoRebuy/BuyInAmount to seat data model"
```

---

## Task 2: `buyin.Service` — balance check, seat summary, `BuyInWithAutoRebuy`

**Files:**
- Modify: `api/internal/buyin/service.go:26-35` (`walletMover` interface), after `:361` (new methods), `:157-307` (`BuyIn` split)
- Test: `api/internal/buyin/service_test.go`

**Interfaces:**
- Consumes: `hand.SeatView.AutoRebuy`/`BuyInAmount` (Task 1), `table.JoinCmd.AutoRebuy` (Task 1).
- Produces: `buyin.SeatSummary{Seated, Stack, AutoRebuy, BuyInAmount}`, `(*Service).SeatedSummary(ctx, roomID, playerID string) (SeatSummary, error)`, `(*Service).SandboxBalance(ctx, playerID string) (int64, error)`, `(*Service).BuyInWithAutoRebuy(ctx, roomID, playerID string, amount int64, midHand, autoRebuy bool, idemKey string) error` — all consumed by `app.autoRebuySweep` (Task 6) and `roomHandlers.join` (Task 3).

- [ ] **Step 1: Write the failing tests**

In `api/internal/buyin/service_test.go`, add a `balances` field to the existing `fakeWallet` struct (near its other fields, line ~25-30):

```go
type fakeWallet struct {
	credits   []call
	debits    []call
	feeDebits []call
	holds     []holdCall
	cashouts  []cashoutCall
	balances  map[string]int64 // playerID -> sandbox balance, for the auto-rebuy tests
}
```

Add the interface method next to the other `fakeWallet` methods (after `DebitReal`, line ~85):

```go
func (f *fakeWallet) Balances(_ context.Context, userID string) (*walletclient.Balances, error) {
	return &walletclient.Balances{SandboxBalance: f.balances[userID]}, nil
}
```

Add `"gopkg.aoctech.app/poker/api/internal/walletclient"` to this file's import block.

Add the new tests (anywhere after `TestSeatedReportsFalseForNeverJoinedPlayer`):

```go
func TestSandboxBalanceReturnsWalletBalance(t *testing.T) {
	wallet := &fakeWallet{balances: map[string]int64{"player-1": 500}}
	mgr := testManager(t)
	rooms := testRoomLookup()
	svc := NewService(wallet, mgr, rooms)

	balance, err := svc.SandboxBalance(context.Background(), "player-1")
	if err != nil {
		t.Fatalf("SandboxBalance: %v", err)
	}
	if balance != 500 {
		t.Fatalf("expected balance=500, got %d", balance)
	}
}

func TestSeatedSummaryReportsAutoRebuyAndBuyInAmount(t *testing.T) {
	wallet := &fakeWallet{}
	mgr := testManager(t)
	rooms := testRoomLookup()
	svc := NewService(wallet, mgr, rooms)
	ctx := context.Background()

	if err := svc.BuyInWithAutoRebuy(ctx, "test-room", "player-1", 100, false, true, "idem-1"); err != nil {
		t.Fatalf("BuyInWithAutoRebuy: %v", err)
	}

	seat, err := svc.SeatedSummary(ctx, "test-room", "player-1")
	if err != nil {
		t.Fatalf("SeatedSummary: %v", err)
	}
	if !seat.Seated || seat.Stack != 100 || !seat.AutoRebuy || seat.BuyInAmount != 100 {
		t.Fatalf("expected seated=true stack=100 autoRebuy=true buyInAmount=100, got %+v", seat)
	}
}

func TestSeatedSummaryReportsFalseAutoRebuyWhenNotOptedIn(t *testing.T) {
	wallet := &fakeWallet{}
	mgr := testManager(t)
	rooms := testRoomLookup()
	svc := NewService(wallet, mgr, rooms)
	ctx := context.Background()

	if err := svc.BuyIn(ctx, "test-room", "player-1", 100, false, "idem-1"); err != nil {
		t.Fatalf("BuyIn: %v", err)
	}

	seat, err := svc.SeatedSummary(ctx, "test-room", "player-1")
	if err != nil {
		t.Fatalf("SeatedSummary: %v", err)
	}
	if seat.AutoRebuy {
		t.Fatalf("expected AutoRebuy=false for a plain BuyIn, got %+v", seat)
	}
}

// TestSeatedSummaryKeepsOriginalBuyInAmountAcrossManualRebuy exercises the
// same invariant as hand_test.go's
// TestAddWaitingPlayerRebuyKeepsOriginalAutoRebuyAndBuyInAmount, but through
// the buyin.Service seam the auto-rebuy sweep actually calls.
func TestSeatedSummaryKeepsOriginalBuyInAmountAcrossManualRebuy(t *testing.T) {
	wallet := &fakeWallet{}
	mgr := testManager(t)
	rooms := &fakeRoomLookup{room: &roomstore.Room{
		ID: "room-rebuy-amount", CurrencyMode: "sandbox", BigBlind: 20, BuyInMin: 40, BuyInMax: 400, MaxSeats: 9,
	}}
	svc := NewService(wallet, mgr, rooms)
	ctx := context.Background()

	seed := func() *hand.Table {
		return hand.NewTable([]*hand.Player{{
			ID: "player-1", Stack: 0, State: hand.SittingOut, AutoRebuy: true, BuyInAmount: 100,
		}}, 10, 20)
	}
	if _, err := mgr.GetOrCreateActor(ctx, "room-rebuy-amount", seed); err != nil {
		t.Fatalf("seed busted seat: %v", err)
	}

	if err := svc.BuyIn(ctx, "room-rebuy-amount", "player-1", 200, false, "rebuy"); err != nil {
		t.Fatalf("manual rebuy: %v", err)
	}

	seat, err := svc.SeatedSummary(ctx, "room-rebuy-amount", "player-1")
	if err != nil {
		t.Fatalf("SeatedSummary: %v", err)
	}
	if seat.Stack != 200 {
		t.Fatalf("expected post-rebuy stack=200, got %d", seat.Stack)
	}
	if !seat.AutoRebuy || seat.BuyInAmount != 100 {
		t.Fatalf("expected AutoRebuy/BuyInAmount pinned to original join (true/100), got auto_rebuy=%v buy_in_amount=%d", seat.AutoRebuy, seat.BuyInAmount)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -tags integration ./internal/buyin/... -run 'TestSandboxBalance|TestSeatedSummary' -v` (from `api/`; requires `docker-compose -f docker-compose.test.yml up -d` first, per `api/README.md`)
Expected: FAIL — compile error (`fakeWallet` doesn't implement `walletMover` yet, `SandboxBalance`/`SeatedSummary`/`BuyInWithAutoRebuy` don't exist).

- [ ] **Step 3: Implement**

In `api/internal/buyin/service.go`, add the import and extend `walletMover` (lines 26-35):

```go
import (
	...
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

type walletMover interface {
	Credit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
	Debit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
	HoldGame(ctx context.Context, userID string, amount int64, tableRef, idempotencyKey, reason string) (string, error)
	ReleaseHold(ctx context.Context, holdID string) error
	CashoutGame(ctx context.Context, userID string, amount int64, tableRef string, holdIDs []string, idempotencyKey, reason string) error
	DebitReal(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
	Balances(ctx context.Context, userID string) (*walletclient.Balances, error)
}
```

Split `BuyIn` (lines 153-307) into a thin public wrapper, a new `BuyInWithAutoRebuy`, and a private `buyIn` carrying the original body:

```go
// BuyIn debits amount from playerID's sandbox wallet, then seats them into
// roomID's live table. If seating fails, the debit is immediately reversed
// with a distinct idempotency key (":refund" suffix) so the reversal can
// never collide with — or be mistaken as a retry of — the original debit.
func (s *Service) BuyIn(ctx context.Context, roomID, playerID string, amount int64, midHand bool, idemKey string) error {
	return s.buyIn(ctx, roomID, playerID, amount, midHand, false, idemKey)
}

// BuyInWithAutoRebuy is BuyIn plus the one-time auto-rebuy opt-in. Only
// meaningful for a brand-new seat: hand.Table.rebuyExisting ignores the
// incoming Player's AutoRebuy field entirely, so calling this on an
// already-seated player's rebuy is a harmless no-op, never a way to
// retroactively flip auto-rebuy on for an existing seat.
func (s *Service) BuyInWithAutoRebuy(ctx context.Context, roomID, playerID string, amount int64, midHand, autoRebuy bool, idemKey string) error {
	return s.buyIn(ctx, roomID, playerID, amount, midHand, autoRebuy, idemKey)
}

func (s *Service) buyIn(ctx context.Context, roomID, playerID string, amount int64, midHand, autoRebuy bool, idemKey string) error {
	// ... existing BuyIn body, unchanged, EXCEPT the actor.Dispatch call below ...
}
```

Inside that body, change the one line that builds the `JoinCmd` (previously line 240):

```go
	joinErr := actor.Dispatch(table.JoinCmd{PlayerID: playerID, Stack: amount, MaxSeats: maxSeats, MidHand: midHand, HoldID: holdID, AutoRebuy: autoRebuy, SettlementIntent: feeIntent, Reply: reply})
```

Add the two new methods after `Seated` (after line 361):

```go
// SandboxBalance reports playerID's current sandbox wallet balance. Always
// reads the sandbox wallet (s.wallet), never the real-money one (s.game) —
// its only caller, the post-hand auto-rebuy sweep, is sandbox-only by design
// (see docs/specs/2026-08-10-auto-buyin-design.md's Scope section).
func (s *Service) SandboxBalance(ctx context.Context, playerID string) (int64, error) {
	balances, err := s.wallet.Balances(ctx, playerID)
	if err != nil {
		return 0, fmt.Errorf("buyin: balances: %w", err)
	}
	return balances.SandboxBalance, nil
}

// SeatSummary is deliberately narrower than a full hand.SeatView — only what
// app.autoRebuySweep needs to decide whether a busted seat should
// self-resolve.
type SeatSummary struct {
	Seated      bool
	Stack       int64
	AutoRebuy   bool
	BuyInAmount int64
}

// SeatedSummary is Seated plus the seat's auto-rebuy configuration. Kept
// separate from Seated (the read path for GET /rooms/:id/seated) so that
// endpoint's response shape never has to grow fields meant only for the
// internal sweep.
func (s *Service) SeatedSummary(ctx context.Context, roomID, playerID string) (SeatSummary, error) {
	actor, err := s.manager.GetOrCreateActor(ctx, roomID, s.seedFor(ctx, roomID))
	if err != nil || actor == nil {
		return SeatSummary{}, fmt.Errorf("buyin: table unavailable: %w", err)
	}

	snapCh := make(chan hand.Snapshot, 1)
	reply := make(chan error, 1)
	if err := actor.Dispatch(table.SnapshotCmd{PlayerID: playerID, Snapshot: snapCh, Reply: reply}); err != nil {
		return SeatSummary{}, err
	}
	select {
	case snap := <-snapCh:
		for _, seat := range snap.Seats {
			if seat.PlayerID == playerID {
				return SeatSummary{Seated: true, Stack: seat.Stack, AutoRebuy: seat.AutoRebuy, BuyInAmount: seat.BuyInAmount}, nil
			}
		}
		return SeatSummary{}, nil
	default:
		return SeatSummary{}, nil
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test -tags integration ./internal/buyin/... -v` (full package, to catch any other call site broken by the `walletMover` interface change)
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/buyin/service.go api/internal/buyin/service_test.go
git commit -m "feat: add buyin.Service.SeatedSummary/SandboxBalance/BuyInWithAutoRebuy"
```

---

## Task 3: HTTP join endpoint accepts `auto_rebuy`

**Files:**
- Modify: `api/internal/api/v1/roomdto.go:19-23` (`JoinRoomRequest`)
- Modify: `api/internal/api/v1/rooms.go:277` (`join` handler)
- Test: `api/internal/api/v1/rooms_test.go`

**Interfaces:**
- Consumes: `(*buyin.Service).BuyInWithAutoRebuy` (Task 2).

- [ ] **Step 1: Write the failing test**

First, check how the existing join tests in `api/internal/api/v1/rooms_test.go` fake `*buyin.Service`'s call (grep `func TestJoin` in that file) and mirror that exact harness. Add:

```go
func TestJoinPassesAutoRebuyThrough(t *testing.T) {
	// Follow the same app/fiber/fake-buyin-backend harness as the existing
	// TestJoin* tests in this file. Assert that POSTing
	// {"amount": <valid>, "auto_rebuy": true} to /rooms/:id/join results in
	// the seated player's hand.Player.AutoRebuy being true — read it back via
	// the same table.Actor/tablemanager.Manager the test harness already
	// exposes (e.g. actor.TableForTest().ViewFor(playerID).Seats), the same
	// way TestBuyInDebitsThenSeats (buyin/service_test.go) asserts on stack.
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/api/v1/... -run TestJoinPassesAutoRebuyThrough -v` (from `api/`)
Expected: FAIL — `auto_rebuy: true` is silently dropped (unknown JSON field, no `AutoRebuy` on `JoinRoomRequest`), so the seated player's `AutoRebuy` reads back `false`.

- [ ] **Step 3: Implement**

In `api/internal/api/v1/roomdto.go`, extend `JoinRoomRequest`:

```go
type JoinRoomRequest struct {
	Amount         int64  `json:"amount"`
	ShareCode      string `json:"share_code,omitempty"`
	IdempotencyKey string `json:"idem_key,omitempty"`
	AutoRebuy      bool   `json:"auto_rebuy,omitempty"`
}
```

In `api/internal/api/v1/rooms.go:277`, switch to the new service method:

```go
	if err := h.buyin.BuyInWithAutoRebuy(c.Context(), room.ID, userID, req.Amount, room.Status == "active", req.AutoRebuy, req.IdempotencyKey); err != nil {
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/api/v1/... -run TestJoinPassesAutoRebuyThrough -v`
Expected: PASS. Then: `go test ./internal/api/v1/...` (full package) to confirm the other `join` tests still pass unchanged.

- [ ] **Step 5: Commit**

```bash
git add api/internal/api/v1/roomdto.go api/internal/api/v1/rooms.go api/internal/api/v1/rooms_test.go
git commit -m "feat: accept auto_rebuy on POST /rooms/:id/join"
```

---

## Task 4: Wire — `Seat.auto_rebuy` proto field

**Files:**
- Modify: `proto/poker.proto:13-46` (`Seat` message)
- Modify: `api/internal/api/v1/tablews.go:833-864` (`ConvertSnapshot`)
- Generated (via script, not hand-edited): `api/internal/api/v1/proto/poker.pb.go`, `ui/src/lib/api/proto/poker.ts`
- Test: `api/internal/api/v1/tablews_conversion_test.go`

**Interfaces:**
- Consumes: `hand.SeatView.AutoRebuy` (Task 1).
- Produces: `pokerproto.Seat.AutoRebuy *bool` (Go), `Seat.auto_rebuy?: boolean` (TS) — consumed by `ui/src/app/table/page.tsx` and `RebuyDialog.tsx` (Task 10).

- [ ] **Step 1: Write the failing test**

In `api/internal/api/v1/tablews_conversion_test.go`, find the existing `run_it_twice` conversion test (grep `RunItTwice` in that file) and add an analogous one right after it:

```go
func TestConvertSnapshotMapsAutoRebuyToOptionalBool(t *testing.T) {
	snap := hand.Snapshot{Seats: []hand.SeatView{
		{PlayerID: "p1", AutoRebuy: true},
		{PlayerID: "p2", AutoRebuy: false},
	}}
	proto := ConvertSnapshot(snap)
	if proto.Seats[0].AutoRebuy == nil || !*proto.Seats[0].AutoRebuy {
		t.Fatal("expected p1's auto_rebuy to be set true")
	}
	if proto.Seats[1].AutoRebuy != nil {
		t.Fatal("expected p2's auto_rebuy to be nil (unset), matching run_it_twice's false-is-absent convention")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/api/v1/... -run TestConvertSnapshotMapsAutoRebuyToOptionalBool -v` (from `api/`)
Expected: FAIL — compile error, `pokerproto.Seat` has no field `AutoRebuy` yet.

- [ ] **Step 3: Implement**

In `proto/poker.proto`, add a field to `Seat` after `run_it_twice` (line 45):

```protobuf
  optional bool run_it_twice = 18;
  optional bool auto_rebuy = 19;
```

Regenerate (requires `protoc`, `protoc-gen-go`, and `ui/node_modules/.bin/protoc-gen-ts_proto` — see prerequisites in `scripts/generate-proto.sh`):

```bash
./scripts/generate-proto.sh
```

This regenerates `api/internal/api/v1/proto/poker.pb.go` and `ui/src/lib/api/proto/poker.ts`. Do not hand-edit either.

In `api/internal/api/v1/tablews.go`, in `ConvertSnapshot` (after the `runItTwice` block, ~line 851):

```go
		var runItTwice *bool
		if s.RunItTwice {
			enabled := true
			runItTwice = &enabled
		}
		var autoRebuy *bool
		if s.AutoRebuy {
			enabled := true
			autoRebuy = &enabled
		}
		protoSeats[i] = &pokerproto.Seat{
			PlayerId:          s.PlayerID,
			Name:              s.Name,
			AvatarUrl:         avatarURL,
			PlaystyleBadge:    playstyleBadge,
			RunItTwice:        runItTwice,
			AutoRebuy:         autoRebuy,
			ConnectionState:   s.ConnectionState,
			...
```

(Insert the `AutoRebuy: autoRebuy,` line into the existing `protoSeats[i] = &pokerproto.Seat{...}` literal, right after `RunItTwice`, leaving every other field untouched.)

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/api/v1/... -run TestConvertSnapshotMapsAutoRebuyToOptionalBool -v`
Expected: PASS. Then `go build ./...` from `api/` and `npx tsc --noEmit` from `ui/` to confirm both generated stubs compile.

- [ ] **Step 5: Commit**

```bash
git add proto/poker.proto api/internal/api/v1/proto/poker.pb.go ui/src/lib/api/proto/poker.ts api/internal/api/v1/tablews.go api/internal/api/v1/tablews_conversion_test.go
git commit -m "feat: add Seat.auto_rebuy to the wire protocol"
```

---

## Task 5: `tablemanager.Manager` — post-hand auto-rebuy sweep hook

**Files:**
- Modify: `api/internal/tablemanager/manager.go:39-49` (`Manager` struct), `:80-90` (setters), `:161-165` (`GetOrCreateActor`'s `onHandComplete` wrapper)
- Test: `api/internal/tablemanager/manager_test.go`

**Interfaces:**
- Produces: `(*Manager).SetOnAutoRebuySweep(fn func(tableID, handID string, outcome hand.HandOutcome))` — consumed by `app.wireAutoRebuyHook` (Task 6).

Note: this is a plain fan-out from the *existing* `onHandComplete` actor hook — no change to `table/actor.go` is needed. `Manager.GetOrCreateActor` already wraps the actor's single `onHandComplete` callback into one manager-level call; this task makes that wrapper also invoke a second, independently-installable callback.

- [ ] **Step 1: Write the failing test**

Check `api/internal/tablemanager/manager_test.go` for its actor-completing-a-hand test harness (likely absent — `onHandComplete`/`onPlayerRemoved` currently have no manager-level test, only the actor-level `TestOnHandCompleteReceivesNonEmptyHandID` in `api/internal/table/actor_test.go`). Add a manager-level test using that same hand-driving pattern (`ReadyCmd` x2 then `ActCmd` fold), via `mgr.GetOrCreateActor` instead of `table.New` directly:

```go
func TestOnAutoRebuySweepFiresAfterHandCompletes(t *testing.T) {
	db := testClient(t)
	env := fmt.Sprintf("tablemanager_autorebuy_test_%d", time.Now().UnixNano())
	mustCreateTestTables(t, db, env)
	store := tablestore.NewStore(db, env)
	mgr := NewManager(tablelease.NewService(cache.NewMemoryBackend(16)), store, nil, nil, nil)

	var gotTableID, gotHandID string
	var gotOutcome hand.HandOutcome
	mgr.SetOnAutoRebuySweep(func(tableID, handID string, outcome hand.HandOutcome) {
		gotTableID, gotHandID, gotOutcome = tableID, handID, outcome
	})

	seed := func() *hand.Table {
		return hand.NewTable([]*hand.Player{
			{ID: "p1", Stack: 1000, Ready: true},
			{ID: "p2", Stack: 1000, Ready: true},
		}, 10, 20)
	}
	actor, err := mgr.GetOrCreateActor(context.Background(), "table-1", seed)
	if err != nil {
		t.Fatalf("get or create actor: %v", err)
	}

	toAct := actor.TableForTest().CurrentPlayerIDForActor()
	reply := make(chan error, 1)
	if err := actor.Dispatch(table.ActCmd{PlayerID: toAct, ActionID: "a1", Action: betting.ActionFold, Reply: reply}); err != nil {
		t.Fatalf("fold: %v", err)
	}

	if gotTableID != "table-1" || gotHandID == "" {
		t.Fatalf("expected sweep to fire with tableID=table-1 non-empty handID, got tableID=%q handID=%q", gotTableID, gotHandID)
	}
	if len(gotOutcome.Participants) == 0 {
		t.Fatal("expected a non-empty outcome.Participants")
	}
}
```

(Match this test's imports/helpers — `testClient`, `mustCreateTestTables`, `betting.ActionFold` — to whatever `manager_test.go` already imports; it's the same integration harness `buyin/service_test.go` and `table/actor_test.go` use.)

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -tags integration ./internal/tablemanager/... -run TestOnAutoRebuySweepFiresAfterHandCompletes -v` (from `api/`)
Expected: FAIL — compile error, `Manager` has no method `SetOnAutoRebuySweep`.

- [ ] **Step 3: Implement**

In `api/internal/tablemanager/manager.go`, add a field to `Manager` (next to `onPlayerRemoved`, line ~45):

```go
	onPlayerRemoved        func(tableID, playerID, reason string, stack int64, holdID string)
	autoRebuySweep         func(tableID, handID string, outcome hand.HandOutcome)
```

Add a setter (next to `SetOnPlayerRemoved`):

```go
// SetOnAutoRebuySweep installs the post-hand auto-rebuy hook, invoked with
// (tableID, handID, outcome) right after the achievements/history
// onHandComplete hook, for every actor this manager creates (including ones
// created before this call). The callback fires synchronously on the table
// actor's own goroutine — same as onHandComplete — so it must never call
// anything that dispatches back into the actor without detaching first (see
// app.wireAutoRebuyHook).
func (m *Manager) SetOnAutoRebuySweep(fn func(tableID, handID string, outcome hand.HandOutcome)) {
	m.autoRebuySweep = fn
}
```

In `GetOrCreateActor`, extend the existing `onHandComplete` wrapper (line ~161-165):

```go
	actor.SetOnHandCompleteForActor(func(handID string, outcome hand.HandOutcome, names map[string]string) {
		if m.onHandComplete != nil {
			m.onHandComplete(tableID, handID, outcome, names)
		}
		if m.autoRebuySweep != nil {
			m.autoRebuySweep(tableID, handID, outcome)
		}
	})
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -tags integration ./internal/tablemanager/... -v` (full package)
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/tablemanager/manager.go api/internal/tablemanager/manager_test.go
git commit -m "feat: add tablemanager auto-rebuy sweep hook"
```

---

## Task 6: `app.go` — wire the sweep, detached from the actor's goroutine

**Files:**
- Modify: `api/internal/app/app.go` (new `autoRebuySweep`/`wireAutoRebuyHook` functions, `fx.Invoke` registration near line 86)
- Test: `api/internal/app/app_test.go`

**Interfaces:**
- Consumes: `buyin.SeatSummary`, `(*buyin.Service).SeatedSummary`/`SandboxBalance`/`BuyIn` (Task 2), `(*tablemanager.Manager).SetOnAutoRebuySweep` (Task 5).

- [ ] **Step 1: Write the failing tests**

In `api/internal/app/app_test.go`, add fakes matching this file's existing style (see `fakeRoomModeReader` at the top) and tests:

```go
type fakeAutoRebuyRoomLookup struct{ room *roomstore.Room }

func (f fakeAutoRebuyRoomLookup) Get(context.Context, string) (*roomstore.Room, error) {
	return f.room, nil
}

type autoRebuyBuyInCall struct {
	roomID, playerID string
	amount           int64
}

type fakeAutoRebuyBuyin struct {
	seats    map[string]buyin.SeatSummary
	balances map[string]int64
	buyIns   []autoRebuyBuyInCall
}

func (f *fakeAutoRebuyBuyin) SeatedSummary(_ context.Context, _, playerID string) (buyin.SeatSummary, error) {
	return f.seats[playerID], nil
}
func (f *fakeAutoRebuyBuyin) SandboxBalance(_ context.Context, playerID string) (int64, error) {
	return f.balances[playerID], nil
}
func (f *fakeAutoRebuyBuyin) BuyIn(_ context.Context, roomID, playerID string, amount int64, _ bool, _ string) error {
	f.buyIns = append(f.buyIns, autoRebuyBuyInCall{roomID, playerID, amount})
	return nil
}

func TestAutoRebuySweepRebuysBustedAutoRebuySeatWithSufficientBalance(t *testing.T) {
	buyinSvc := &fakeAutoRebuyBuyin{
		seats:    map[string]buyin.SeatSummary{"player-1": {Seated: true, Stack: 0, AutoRebuy: true, BuyInAmount: 100}},
		balances: map[string]int64{"player-1": 500},
	}
	rooms := fakeAutoRebuyRoomLookup{room: &roomstore.Room{CurrencyMode: roomstore.CurrencyModeSandbox}}

	autoRebuySweep(context.Background(), buyinSvc, rooms, "table-1", "hand-1", hand.HandOutcome{Participants: []string{"player-1"}})

	if len(buyinSvc.buyIns) != 1 || buyinSvc.buyIns[0].amount != 100 {
		t.Fatalf("expected one 100-chip auto-rebuy, got %+v", buyinSvc.buyIns)
	}
}

func TestAutoRebuySweepSkipsInsufficientBalance(t *testing.T) {
	buyinSvc := &fakeAutoRebuyBuyin{
		seats:    map[string]buyin.SeatSummary{"player-1": {Seated: true, Stack: 0, AutoRebuy: true, BuyInAmount: 100}},
		balances: map[string]int64{"player-1": 50},
	}
	rooms := fakeAutoRebuyRoomLookup{room: &roomstore.Room{CurrencyMode: roomstore.CurrencyModeSandbox}}

	autoRebuySweep(context.Background(), buyinSvc, rooms, "table-1", "hand-1", hand.HandOutcome{Participants: []string{"player-1"}})

	if len(buyinSvc.buyIns) != 0 {
		t.Fatalf("expected no auto-rebuy for insufficient balance, got %+v", buyinSvc.buyIns)
	}
}

func TestAutoRebuySweepSkipsRealMoneyRooms(t *testing.T) {
	buyinSvc := &fakeAutoRebuyBuyin{
		seats:    map[string]buyin.SeatSummary{"player-1": {Seated: true, Stack: 0, AutoRebuy: true, BuyInAmount: 100}},
		balances: map[string]int64{"player-1": 500},
	}
	rooms := fakeAutoRebuyRoomLookup{room: &roomstore.Room{CurrencyMode: roomstore.CurrencyModeReal}}

	autoRebuySweep(context.Background(), buyinSvc, rooms, "table-1", "hand-1", hand.HandOutcome{Participants: []string{"player-1"}})

	if len(buyinSvc.buyIns) != 0 {
		t.Fatalf("expected no auto-rebuy sweep for real-money rooms, got %+v", buyinSvc.buyIns)
	}
}

func TestAutoRebuySweepSkipsSeatWithoutAutoRebuy(t *testing.T) {
	buyinSvc := &fakeAutoRebuyBuyin{
		seats:    map[string]buyin.SeatSummary{"player-1": {Seated: true, Stack: 0, AutoRebuy: false, BuyInAmount: 100}},
		balances: map[string]int64{"player-1": 500},
	}
	rooms := fakeAutoRebuyRoomLookup{room: &roomstore.Room{CurrencyMode: roomstore.CurrencyModeSandbox}}

	autoRebuySweep(context.Background(), buyinSvc, rooms, "table-1", "hand-1", hand.HandOutcome{Participants: []string{"player-1"}})

	if len(buyinSvc.buyIns) != 0 {
		t.Fatalf("expected no auto-rebuy when AutoRebuy is false, got %+v", buyinSvc.buyIns)
	}
}

func TestAutoRebuySweepSkipsSeatThatDidNotBust(t *testing.T) {
	buyinSvc := &fakeAutoRebuyBuyin{
		seats:    map[string]buyin.SeatSummary{"player-1": {Seated: true, Stack: 300, AutoRebuy: true, BuyInAmount: 100}},
		balances: map[string]int64{"player-1": 500},
	}
	rooms := fakeAutoRebuyRoomLookup{room: &roomstore.Room{CurrencyMode: roomstore.CurrencyModeSandbox}}

	autoRebuySweep(context.Background(), buyinSvc, rooms, "table-1", "hand-1", hand.HandOutcome{Participants: []string{"player-1"}})

	if len(buyinSvc.buyIns) != 0 {
		t.Fatalf("expected no auto-rebuy for a seat that still has chips, got %+v", buyinSvc.buyIns)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/app/... -run TestAutoRebuySweep -v` (from `api/`)
Expected: FAIL — compile error, `autoRebuySweep` doesn't exist.

- [ ] **Step 3: Implement**

In `api/internal/app/app.go`, add (near `wirePlayerRemovedHook`, after it):

```go
// autoRebuyRoomLookup and autoRebuyBuyinService narrow *roomstore.Store and
// *buyin.Service down to exactly what autoRebuySweep needs, so it's testable
// with plain fakes instead of the DynamoDB-backed integration harness
// buyin's own tests require.
type autoRebuyRoomLookup interface {
	Get(ctx context.Context, roomID string) (*roomstore.Room, error)
}

type autoRebuyBuyinService interface {
	SeatedSummary(ctx context.Context, roomID, playerID string) (buyin.SeatSummary, error)
	SandboxBalance(ctx context.Context, playerID string) (int64, error)
	BuyIn(ctx context.Context, roomID, playerID string, amount int64, midHand bool, idemKey string) error
}

// autoRebuySweep checks every one of a just-completed hand's participants
// and auto-rebuys anyone who busted (Stack==0) with auto-rebuy on and enough
// sandbox balance to cover their original buy-in. Sandbox rooms only — see
// docs/specs/2026-08-10-auto-buyin-design.md's Scope section for why
// real-money is excluded. Errors are logged and skipped per player, never
// retried: a skipped player just stays sitting out, same as insufficient
// balance.
func autoRebuySweep(ctx context.Context, buyinSvc autoRebuyBuyinService, rooms autoRebuyRoomLookup, tableID, handID string, outcome hand.HandOutcome) {
	room, err := rooms.Get(ctx, tableID)
	if err != nil {
		slog.Error("auto-rebuy: load room failed", "table", tableID, "err", err)
		return
	}
	if room == nil || room.CurrencyMode != roomstore.CurrencyModeSandbox {
		return
	}
	for _, playerID := range outcome.Participants {
		seat, err := buyinSvc.SeatedSummary(ctx, tableID, playerID)
		if err != nil {
			slog.Error("auto-rebuy: seat lookup failed", "table", tableID, "player", playerID, "err", err)
			continue
		}
		if !seat.Seated || seat.Stack != 0 || !seat.AutoRebuy || seat.BuyInAmount <= 0 {
			continue
		}
		balance, err := buyinSvc.SandboxBalance(ctx, playerID)
		if err != nil {
			slog.Error("auto-rebuy: balance check failed", "table", tableID, "player", playerID, "err", err)
			continue
		}
		if balance < seat.BuyInAmount {
			continue
		}
		nonce := handID + "-auto-" + playerID
		if err := buyinSvc.BuyIn(ctx, tableID, playerID, seat.BuyInAmount, false, nonce); err != nil {
			slog.Error("auto-rebuy: buy-in failed", "table", tableID, "player", playerID, "err", err)
		}
	}
}

// wireAutoRebuyHook installs the post-hand auto-rebuy sweep. Same
// construction-cycle reason as wirePlayerRemovedHook above: buyin.Service
// depends on *tablemanager.Manager, so this can only be wired after Fx
// builds both.
//
// autoRebuySweep is dispatched in a detached goroutine, never called inline:
// this callback fires synchronously from inside the table actor's own
// single-goroutine command loop (table/actor.go's notifyHandComplete, called
// from broadcastAll before the actor's Run loop reads its next command), and
// both SeatedSummary and BuyIn dispatch back into that same loop. Calling
// either synchronously here would deadlock the whole table.
func wireAutoRebuyHook(mgr *tablemanager.Manager, buyinSvc *buyin.Service, rooms *roomstore.Store) {
	mgr.SetOnAutoRebuySweep(func(tableID, handID string, outcome hand.HandOutcome) {
		go autoRebuySweep(context.Background(), buyinSvc, rooms, tableID, handID, outcome)
	})
}
```

Register it in the `fx.Invoke` list (next to `wirePlayerRemovedHook`, line ~86):

```go
	fx.Invoke(wirePlayerRemovedHook),
	fx.Invoke(wireAutoRebuyHook),
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/app/... -v` (full package)
Expected: PASS. Then `go build ./...` from `api/` to confirm the `fx.Invoke` wiring type-checks.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/app.go api/internal/app/app_test.go
git commit -m "feat: wire post-hand auto-rebuy sweep"
```

---

## Task 7: Frontend — `joinRoom` accepts `autoRebuy`

> Execute via the `/impeccable` skill.

**Files:**
- Modify: `ui/src/lib/api/rooms.ts:56-64` (`joinRoom`)
- Test: `ui/src/lib/api/rooms.test.ts` (create if it doesn't already cover `joinRoom`; check first)

**Interfaces:**
- Produces: `joinRoom(id: string, amount: number, shareCode?: string, autoRebuy?: boolean): Promise<void>` — consumed by `BuyInPanel.tsx` (Task 8) and `RebuyDialog.tsx` (Task 10).

- [ ] **Step 1: Write the failing test**

```ts
it('sends auto_rebuy when requested', async () => {
  const post = vi.spyOn(apiClient, 'post').mockResolvedValue({data: undefined});
  await joinRoom('room-1', 100, undefined, true);
  expect(post).toHaveBeenCalledWith(
    '/v1.0/rooms/room-1/join',
    expect.objectContaining({amount: 100, auto_rebuy: true}),
    expect.anything(),
  );
});
```

(Match this repo's existing mocking convention for `apiClient` — check `ui/src/lib/api/*.test.ts` for the established pattern, e.g. `vi.mock('./client', ...)`, before writing this literally.)

- [ ] **Step 2: Run the test to verify it fails**

Run: `npx vitest run rooms.test.ts` (from `ui/`)
Expected: FAIL — `joinRoom` doesn't send `auto_rebuy` (TS also errors: too many arguments).

- [ ] **Step 3: Implement**

```ts
export async function joinRoom(id: string, amount: number, shareCode?: string, autoRebuy?: boolean) {
  // idem_key must be fresh per buy-in click (a rejoin/rebuy is a distinct
  // debit) but stable across a single click's own network retries. The
  // server derives its wallet idempotency key from this, so leaving it out
  // makes every buy-in for this player+room collide on the same key.
  await apiClient.post(
    `/v1.0/rooms/${id}/join`,
    {amount, share_code: shareCode || undefined, auto_rebuy: autoRebuy || undefined, idem_key: crypto.randomUUID()},
    {silentError: true},
  );
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `npx vitest run rooms.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ui/src/lib/api/rooms.ts ui/src/lib/api/rooms.test.ts
git commit -m "feat(ui): thread auto_rebuy through joinRoom"
```

---

## Task 8: Frontend — `BuyInPanel` auto-rebuy checkbox

> Execute via the `/impeccable` skill.

**Files:**
- Modify: `ui/src/components/table/BuyInPanel.tsx`
- Test: `ui/src/components/table/BuyInPanel.test.tsx` (check for an existing file first — extend it if present)

**Interfaces:**
- Consumes: `joinRoom(id, amount, shareCode, autoRebuy)` (Task 7).

- [ ] **Step 1: Write the failing test**

```tsx
it('sends auto_rebuy=true when the checkbox is checked before confirming', async () => {
  const user = userEvent.setup();
  // ... render BuyInPanel with the existing test harness (mock getRoom/joinRoom) ...
  await user.click(screen.getByLabelText(/auto.?rebuy|recompra automática/i));
  await user.click(screen.getByRole('button', {name: /entrar com/i}));
  expect(joinRoom).toHaveBeenCalledWith(expect.any(String), expect.any(Number), undefined, true);
});
```

(Mirror whatever mocking/rendering setup this component's existing tests use — check for `BuyInPanel.test.tsx` first; if none exists, follow `RebuyDialog`'s or a sibling table component's test file as the template.)

- [ ] **Step 2: Run the test to verify it fails**

Run: `npx vitest run BuyInPanel.test.tsx` (from `ui/`)
Expected: FAIL — no such checkbox exists yet.

- [ ] **Step 3: Implement**

Add state and the checkbox to `BuyInPanel` (`ui/src/components/table/BuyInPanel.tsx`):

```tsx
  const [autoRebuy, setAutoRebuy] = useState(false);
  const autoRebuyId = useId();
```

```tsx
  async function confirm() {
    setJoining(true);
    setError('');
    try {
      await joinRoom(roomId, value, shareCode, autoRebuy);
      onSeatedAction();
    } catch (err) {
      ...
```

In the JSX, right before the confirm `Button`:

```tsx
      <div className="buyin-auto-rebuy">
        <input id={autoRebuyId} type="checkbox" checked={autoRebuy} disabled={joining}
               onChange={event => setAutoRebuy(event.target.checked)}/>
        <label htmlFor={autoRebuyId}>Comprar fichas automaticamente se eu ficar sem, para continuar jogando sem
          esperar.</label>
      </div>
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `npx vitest run BuyInPanel.test.tsx`
Expected: PASS. Then `npx tsc --noEmit && npx eslint src --max-warnings 0` from `ui/`.

- [ ] **Step 5: Commit**

```bash
git add ui/src/components/table/BuyInPanel.tsx ui/src/components/table/BuyInPanel.test.tsx
git commit -m "feat(ui): add auto-rebuy checkbox to BuyInPanel"
```

---

## Task 9: Frontend — extract `PixPaymentView` out of `PurchaseModal`

> Execute via the `/impeccable` skill.

**Files:**
- Create: `ui/src/components/store/PixPaymentView.tsx`
- Modify: `ui/src/components/store/PurchaseModal.tsx:104-128` (use the extracted component)
- Test: `ui/src/components/store/PixPaymentView.test.tsx`

Pure refactor — no behavior change. Needed so Task 10's `RebuyDialog` can embed the same QR/copy-paste/countdown PIX view `PurchaseModal` uses (the store's SKU-purchase flow) without nesting a second `<Dialog>` inside `RebuyDialog`'s own `<Dialog>`.

**Interfaces:**
- Produces: `<PixPaymentView purchase={SandboxPurchase} />` — renders the QR code, copy-paste field, countdown, and sandbox-only disclaimer. No props beyond `purchase`; it owns its own copy-to-clipboard state internally (unlike `PurchaseModal`, it does not need `onCloseAction`/`onUpdateAction`/`onRegenerateAction` — those stay on `PurchaseModal` for the expired/regenerate flow, which `RebuyDialog` does not need per the design spec's grace-window story). Consumed by `PurchaseModal.tsx` and `RebuyDialog.tsx` (Task 10).

- [ ] **Step 1: Write the failing test**

```tsx
import {render, screen} from '@testing-library/react';
import {PixPaymentView} from './PixPaymentView';

it('renders the QR code and pix copia-e-cola field for a pending purchase', () => {
  render(<PixPaymentView purchase={{
    purchase_id: 'p1', sku: 'sku-1', status: 'pending',
    qr_code_base64: 'aGVsbG8=', pix_copia_e_cola: '00020126...',
  } as never}/>);
  expect(screen.getByLabelText(/pix copia e cola/i)).toHaveValue('00020126...');
  expect(screen.getByAltText(/qr code pix/i)).toBeInTheDocument();
});
```

(Match the exact test-utils import path and `SandboxPurchase` mock shape this repo's existing `PurchaseModal.test.tsx` — if any — already uses.)

- [ ] **Step 2: Run the test to verify it fails**

Run: `npx vitest run PixPaymentView.test.tsx` (from `ui/`)
Expected: FAIL — module doesn't exist.

- [ ] **Step 3: Implement**

Create `ui/src/components/store/PixPaymentView.tsx`, moving the QR/copy-paste/countdown/disclaimer block (current `PurchaseModal.tsx` lines 104-128) into its own component with its own `copied`/`copyFailed` state (currently owned by `PurchaseModal`):

```tsx
'use client';
import {useState} from 'react';
import Image from 'next/image';
import {Check, Copy, ShieldCheck} from 'lucide-react';
import {Button} from '@/components/ui/button';
import type {SandboxPurchase} from '@/lib/api/wallet';
import {formatDuration, useCountdownMs} from './useCountdown';

export function PixPaymentView({purchase}: { purchase: SandboxPurchase }) {
  const [copied, setCopied] = useState(false);
  const [copyFailed, setCopyFailed] = useState(false);
  const expiresMs = purchase.expires_at ? new Date(purchase.expires_at).getTime() : null;
  const qrImageType = purchase.qr_code_base64?.startsWith('PHN2Zy') ? 'image/svg+xml' : 'image/png';
  const remainingMs = useCountdownMs(expiresMs);
  const expired = expiresMs !== null && remainingMs <= 0;

  async function copy() {
    if (!purchase.pix_copia_e_cola) return;
    try {
      if (!navigator.clipboard?.writeText) throw new Error('clipboard unavailable');
      await navigator.clipboard.writeText(purchase.pix_copia_e_cola);
      setCopied(true);
      setCopyFailed(false);
    } catch {
      setCopyFailed(true);
    }
  }

  return <>
    {purchase.qr_code_base64 && <div className="store-qr">
      <Image src={`data:${qrImageType};base64,${purchase.qr_code_base64}`} alt="QR code Pix para pagamento"
             width={200} height={200} unoptimized/>
    </div>}
    <div className="buyin-control store-pix-control">
      <label htmlFor="pix-copia-e-cola">Pix copia e cola</label>
      <div className="store-pix-field">
        <input id="pix-copia-e-cola" value={purchase.pix_copia_e_cola ?? ''} readOnly
               onClick={event => event.currentTarget.select()}/>
        <Button type="button" variant="ghost" size="icon"
                aria-label={copied ? 'Código Pix copiado' : expired ? 'Código Pix expirado' : 'Copiar código Pix'}
                title={copied ? 'Copiado' : expired ? 'Código expirado' : 'Copiar código Pix'}
                disabled={expired || !purchase.pix_copia_e_cola} onClick={() => void copy()}>
          {copied ? <Check aria-hidden="true"/> : <Copy aria-hidden="true"/>}
        </Button>
      </div>
      <span className="sr-only" aria-live="polite">{copied ? 'Código Pix copiado.' : ''}</span>
    </div>
    {copyFailed && <p className="buyin-error" role="alert">Não foi possível copiar automaticamente. Selecione o código acima e copie manualmente.</p>}
    {expiresMs !== null && <p className={`store-countdown${expired ? ' is-expiring' : ''}`}>
      {expired ? 'Código expirado' : `Expira em ${formatDuration(remainingMs)}`}
    </p>}
    <p className="store-payment-note"><ShieldCheck aria-hidden="true"/> As fichas são apenas do modo sandbox e não têm valor em dinheiro.</p>
  </>;
}
```

In `PurchaseModal.tsx`, replace lines 104-128 (the block this was extracted from) with:

```tsx
          : <PixPaymentView purchase={purchase!}/>}
```

(`purchase!` is safe here: this branch is only reached when `purchase?.status && purchase.status !== 'pending'` is false and `purchase?.status !== 'confirmed'`, both of which already require `purchase` to be non-null in the surrounding ternary — same as the existing code's implicit assumption.) Remove `copied`/`copyFailed`/`copy`/`expiresMs`/`qrImageType`/`remainingMs`/`expired`/`recoverableExpired`'s now-duplicated pieces from `PurchaseModal` that moved into `PixPaymentView` — keep `recoverableExpired` (still used by the regenerate-Pix footer) and `expired`/`expiresMs`, recomputed locally in `PurchaseModal` only if still referenced elsewhere in that file (check before deleting — `recoverableExpired = expired || purchase?.status === 'expired'` at line 29 still needs `expired`).

- [ ] **Step 4: Run the test to verify it passes**

Run: `npx vitest run PixPaymentView.test.tsx PurchaseModal.test.tsx` (the latter if it exists — confirm no regression)
Expected: PASS. Then `npx tsc --noEmit && npx eslint src --max-warnings 0 && npm run build` from `ui/`.

- [ ] **Step 5: Commit**

```bash
git add ui/src/components/store/PixPaymentView.tsx ui/src/components/store/PixPaymentView.test.tsx ui/src/components/store/PurchaseModal.tsx
git commit -m "refactor(ui): extract PixPaymentView from PurchaseModal"
```

---

## Task 10: Frontend — `RebuyDialog` grace window + embedded PIX top-up

> Execute via the `/impeccable` skill.

**Files:**
- Modify: `ui/src/components/table/RebuyDialog.tsx` (full rewrite of the dialog body)
- Modify: `ui/src/app/table/page.tsx:429-431` (pass the seat's `auto_rebuy` flag through)
- Test: `ui/src/components/table/RebuyDialog.test.tsx` (extend or create)

**Interfaces:**
- Consumes: `Seat.auto_rebuy` (Task 4, via the generated proto TS type), `joinRoom(id, amount, shareCode, autoRebuy)` (Task 7), `PixPaymentView` (Task 9), `listSkus`/`createPurchase`/`getPurchase` (`@/lib/api/wallet`), `getMe` (`@/lib/api/player`, for `sandbox_balance`), `SkuGrid` (`@/components/store/SkuGrid`).

- [ ] **Step 1: Write the failing tests**

```tsx
// RebuyDialog.test.tsx — add alongside whatever existing tests already cover
// the manual-rebuy path (mock timers with vi.useFakeTimers() for the grace
// window).

it('shows nothing during the grace window when auto_rebuy is on', () => {
  render(<RebuyDialog roomId="r1" room={mockRoom} autoRebuy onRebuyAction={vi.fn()}/>);
  expect(screen.queryByText(/você ficou sem fichas/i)).not.toBeInTheDocument();
});

it('falls back to the manual rebuy dialog after the grace window if still busted with balance', async () => {
  vi.useFakeTimers();
  mockGetMe.mockResolvedValue({sandbox_balance: 300});
  render(<RebuyDialog roomId="r1" room={mockRoom} autoRebuy onRebuyAction={vi.fn()}/>);
  await vi.advanceTimersByTimeAsync(1600);
  expect(await screen.findByText(/você ficou sem fichas/i)).toBeInTheDocument();
  expect(screen.getByRole('slider')).toBeInTheDocument();
  vi.useRealTimers();
});

it('offers an embedded PIX top-up after the grace window when balance is exactly zero', async () => {
  vi.useFakeTimers();
  mockGetMe.mockResolvedValue({sandbox_balance: 0});
  render(<RebuyDialog roomId="r1" room={mockRoom} autoRebuy onRebuyAction={vi.fn()}/>);
  await vi.advanceTimersByTimeAsync(1600);
  expect(await screen.findByText(/comprar fichas/i)).toBeInTheDocument();
  expect(screen.queryByRole('slider')).not.toBeInTheDocument();
  vi.useRealTimers();
});

it('renders the manual dialog immediately (no grace window) when auto_rebuy is off', () => {
  render(<RebuyDialog roomId="r1" room={mockRoom} onRebuyAction={vi.fn()}/>);
  expect(screen.getByText(/você ficou sem fichas/i)).toBeInTheDocument();
});
```

(Match this repo's established mocking convention for `@/lib/api/wallet` and `@/lib/api/player` — check other table-component tests for how they mock query hooks, e.g. wrapping in the same `QueryClientProvider` test harness `page.tsx` tests use.)

- [ ] **Step 2: Run the tests to verify they fail**

Run: `npx vitest run RebuyDialog.test.tsx` (from `ui/`)
Expected: FAIL — `RebuyDialog` has no `autoRebuy` prop yet and always renders the manual dialog immediately.

- [ ] **Step 3: Implement**

Rewrite `ui/src/components/table/RebuyDialog.tsx`:

```tsx
'use client';
import {useEffect, useId, useState} from 'react';
import {Wallet} from 'lucide-react';
import {useQuery} from '@tanstack/react-query';
import {Button} from '@/components/ui/button';
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger
} from '@/components/ui/dialog';
import type {Room} from '@/lib/api/rooms';
import {joinRoom} from '@/lib/api/rooms';
import {getMe} from '@/lib/api/player';
import {createPurchase, listSkus, type SandboxPurchase} from '@/lib/api/wallet';
import {SkuGrid} from '@/components/store/SkuGrid';
import {PixPaymentView} from '@/components/store/PixPaymentView';
import {formatBuyIn, midBuyIn} from './BuyInPanel';

const GENERIC_ERROR = 'Não foi possível comprar mais fichas agora. Tente novamente.';
// ponytail: fixed grace window, not an explicit server "auto-rebuy failed"
// push — good enough for one local wallet HTTP round trip. If wallet latency
// ever gets unpredictable enough to make this flaky, replace with an
// explicit failure event from the server instead of guessing a timeout.
const AUTO_REBUY_GRACE_MS = 1500;

/** Shown once a seated player's stack hits zero. With auto-rebuy off this is
 * the same buy-in ceremony as first sitting down. With auto-rebuy on, it
 * waits out a short grace window for the server's own auto-rebuy to land
 * (the stack going positive unmounts this dialog on its own) before falling
 * back to the manual flow — an embedded PIX top-up if the balance is truly
 * zero, the plain manual rebuy slider otherwise. */
export function RebuyDialog({roomId, room, autoRebuy = false, onRebuyAction}: {
  roomId: string;
  room: Room;
  autoRebuy?: boolean;
  onRebuyAction: () => void
}) {
  const sliderId = useId();
  const [open, setOpen] = useState(true);
  const [amount, setAmount] = useState<number | null>(null);
  const [joining, setJoining] = useState(false);
  const [error, setError] = useState('');
  const [graceElapsed, setGraceElapsed] = useState(!autoRebuy);
  const [pixPurchase, setPixPurchase] = useState<SandboxPurchase | null>(null);

  useEffect(() => {
    if (!autoRebuy) return undefined;
    const id = window.setTimeout(() => setGraceElapsed(true), AUTO_REBUY_GRACE_MS);
    return () => window.clearTimeout(id);
  }, [autoRebuy]);

  const player = useQuery({queryKey: ['player', 'me'], queryFn: getMe, enabled: graceElapsed});
  const skus = useQuery({queryKey: ['wallet', 'skus'], queryFn: listSkus, enabled: graceElapsed && player.data?.sandbox_balance === 0});
  const balanceIsZero = player.data?.sandbox_balance === 0;

  const step = room.big_blind > 0 ? room.big_blind : 1;
  const value = amount ?? midBuyIn(room.buy_in_min, room.buy_in_max, room.big_blind);
  const isReal = room.currency_mode === 'real';
  const unit = isReal ? 'reais' : 'fichas';
  const fmt = (n: number) => formatBuyIn(n, isReal);

  async function confirm() {
    setJoining(true);
    setError('');
    try {
      await joinRoom(roomId, value);
      setOpen(false);
      onRebuyAction();
    } catch {
      setError(GENERIC_ERROR);
      setJoining(false);
    }
  }

  async function selectSku(sku: { id: string }) {
    try {
      setPixPurchase(await createPurchase(sku.id));
    } catch {
      setError(GENERIC_ERROR);
    }
  }

  if (!graceElapsed) return null;

  return <Dialog open={open} onOpenChangeAction={next => {
    setOpen(next);
    if (!next) setError('');
  }}>
    <DialogTrigger render={<Button type="button" variant="ghost" size="icon" aria-label="Comprar mais fichas"/>}>
      <Wallet/>
    </DialogTrigger>
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Você ficou sem fichas</DialogTitle>
        <DialogDescription>{balanceIsZero && !isReal
          ? 'Seu saldo sandbox zerou. Compre mais fichas com Pix para continuar nesta mesa.'
          : `Compre mais ${unit} para continuar jogando nesta mesa.`}</DialogDescription>
      </DialogHeader>
      {isReal && !!room.entry_fee_cents &&
          <p className="buyin-fee-notice">Taxa fixa de mesa: {formatBuyIn(room.entry_fee_cents, true)} (cobrada
              de novo a cada vez que você compra fichas).</p>}
      {error && <p className="buyin-error" role="alert">{error}</p>}
      {balanceIsZero && !isReal
        ? pixPurchase
          ? <PixPaymentView purchase={pixPurchase}/>
          : <SkuGrid skus={skus.data ?? []} isLoading={skus.isLoading} isError={skus.isError}
                     onRetryAction={() => void skus.refetch()} onSelectAction={sku => void selectSku(sku)}
                     pendingSku={null}/>
        : <>
          <div className="buyin-control">
            <label htmlFor={sliderId}>Recompra</label>
            <input id={sliderId} type="range" min={room.buy_in_min} max={room.buy_in_max} step={step} value={value}
                   disabled={joining} onChange={event => setAmount(Number(event.target.value))}
                   aria-valuetext={`${fmt(value)} ${unit}`}/>
            <output htmlFor={sliderId}>{fmt(value)} <span>{unit}</span></output>
            <small>mín. {fmt(room.buy_in_min)} · máx. {fmt(room.buy_in_max)}</small>
          </div>
          <DialogFooter>
            <Button type="button" variant="ghost" disabled={joining} onClick={() => setOpen(false)}>Agora não</Button>
            <Button type="button" disabled={joining} onClick={confirm}>
              {joining ? 'Comprando…' : `Comprar ${fmt(value)}`}
            </Button>
          </DialogFooter>
        </>}
    </DialogContent>
  </Dialog>;
}
```

Skipped: closing `pixPurchase`'s dialog state back to the SKU grid on expiry/refund (the "Voltar aos pacotes" / regenerate flow `PurchaseModal` has) — `RebuyDialog`'s PIX embed is a one-shot top-up, not the full store; add if players report getting stuck on an expired code without regenerating.

In `ui/src/app/table/page.tsx:429-431`, pass the flag through:

```tsx
            {viewerSeat && isPaused && viewerSeat.stack === 0 && room &&
              s.stage !== 'showdown' && s.stage !== 'complete' &&
                <RebuyDialog roomId={id} room={room} autoRebuy={Boolean(viewerSeat.auto_rebuy)}
                             onRebuyAction={() => rt.ready(true)}/>}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `npx vitest run RebuyDialog.test.tsx page.test.tsx` (from `ui/`, the latter if a `table/page.test.tsx` exists — confirm no regression from the new prop)
Expected: PASS. Then the full frontend quality gate: `npx vitest run && npx tsc --noEmit && npx eslint src --max-warnings 0 && npm run build`.

- [ ] **Step 5: Commit**

```bash
git add ui/src/components/table/RebuyDialog.tsx ui/src/components/table/RebuyDialog.test.tsx ui/src/app/table/page.tsx
git commit -m "feat(ui): auto-rebuy grace window and embedded PIX top-up in RebuyDialog"
```

---

## Documentation

Per both `api/CLAUDE.md`'s and `ui/CLAUDE.md`'s "every code change MUST be documented" policy: after Task 6, add a short section to `api/README.md` (or wherever the buy-in/rebuy flow is currently documented) describing the auto-rebuy sweep and its sandbox-only scope; after Task 10, do the same for the UI's join/rebuy flow if it's documented anywhere in `ui/`. Locate the existing buy-in documentation first (`grep -rn "rebuy\|buy-in" api/README.md ui/README.md`) and extend it in place rather than creating a new doc file.
