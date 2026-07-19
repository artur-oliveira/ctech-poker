# Phase 5 — Real-Money Mode & Production Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the sandbox-complete MVP (Phases 0-4) into a real-money-capable, resilient, horizontally-scalable
production product, per PLAN.md's Phase 5 gate and ARCHITECTURE.md §4/§7/§8: real-money buy-in/cash-out against
`ctech-wallet`'s ring-fenced `game` balance, a durable reconciliation job (no money left in limbo), observability
+ alarms, graceful scale-in, WAF hardening, load/chaos testing, and the hand-history audit endpoint OVERVIEW.md
§8.2 flagged as a suggestion.

**Architecture:** Real-money mode reuses every mechanism Phases 2-4 already built (table.Actor, tablemanager,
buyin.Service, room currency_mode) — it does not introduce a parallel code path. The only new pieces are: (1) a
`GameWallet` variant of `walletclient.Client` targeting `ctech-wallet`'s ring-fenced `game` balance instead of
`sandbox`, (2) a fail-closed feature gate (`REAL_MONEY_ENABLED` + a recorded legal sign-off reference) so real
money can never flow without both an engineering and a business decision on record, and (3) a reconciliation job
that makes Phase 3's previously-flagged "cash-out credit can fail after seat removal" gap safe — mandatory once
real chips are involved, optional (and left undone) in sandbox.

**Tech Stack:** Same as Phases 2-4, plus AWS Lambda + EventBridge Scheduler for the reconciliation job (mirroring
`ctech-wallet`'s own `cmd/reconcile`/`ReconcileStack`), CloudWatch EMF for structured metrics, AWS WAFv2.

## Global Constraints

- **Real-money mode is gated on prerequisites this plan cannot satisfy itself** (they require changes in
  `ctech-wallet`, a different repo): as of the current, confirmed-live `ctech-wallet` API
  (`ctech-wallet/api/internal/api/v1/router.go`), the only M2M internal routes are
  `/v1.0/internal/wallet/sandbox/{credit,debit}` and `/v1.0/internal/wallet/real/debit` — there is **no M2M route
  for the `game` wallet** at all. **Before Task 1 is exercised against a real (non-fake) wallet, a human must
  coordinate with `ctech-wallet` to build `docs/plans/2026-07-19-poker-game-holds.md`** (a separate repo, own
  plan, already written and cross-referenced from this one).
- **Reconciled decision (resolved — supersedes this plan's original plain-credit/debit assumption):** the
  wallet contract is **reservation-shaped**, not plain unconditional credit/debit —
  `POST /v1.0/internal/wallet/game/hold`, `POST /v1.0/internal/wallet/game/hold/{id}/release`,
  `POST /v1.0/internal/wallet/game/cashout` (scopes `internal:wallet:game-hold`/`internal:wallet:game-cashout`),
  per `ctech-wallet/docs/specs/2026-07-19-poker-game-holds-design.md`. Reasoning, in short: only the money
  custodian (`ctech-wallet`) should be the one reconciling money, per that repo's own Financial Safety
  Invariants discipline — and a plain credit/debit contract gives `ctech-wallet` zero independent signal if
  poker itself (not just one in-flight call) ever stops coming back to settle an open reservation. This plan's
  own Task 4 (durable pending-credit tracking) and the wallet-side stale-hold sweep are **complementary, not
  redundant**: Task 4 covers "a single call to wallet failed in flight, poker retries it"; the wallet-side hold
  covers "poker itself never comes back at all" — only the money-holder can detect the second case
  independently. This also isn't poker-specific: `game` is already framed (root `CLAUDE.md`) as the base for
  "skill-game (poker/dominó) integration" — a shared reservation primitive in `ctech-wallet` is reused by any
  future skill game rather than each one re-earning its own real-money-safety review, the same reuse-over-
  reinvention principle already applied to `ctech-go-common/lock`/`jwtverify`. Task 1 below must be rewritten
  against this contract before implementation — its current code sketch (`CreditGame`/`DebitGame`) reflects
  the superseded plain-credit/debit assumption and needs `HoldGame`/`ReleaseHold`/`CashoutGame` in its place,
  matching the wallet-side method names 1:1. Task 3's buy-in/cash-out wiring must carry the returned `hold_id`
  through room/seat state (or `sessionlog` from Task 12, which already records per-table amounts and is a
  natural place to also persist the `hold_id` needed at cash-out time) so `CashoutGame` can reference it.
- **Legal gate (OVERVIEW.md §11):** real-money poker's legal status under Brazilian gambling regulation is
  unresolved business risk, bigger than any engineering risk in this plan. `REAL_MONEY_ENABLED` must never be set
  `true` in any environment without a recorded legal sign-off reference — enforced in code (Task 2), not just by
  policy.
- `currency_mode="real"` is enforced end to end exactly like `sandbox` was in Phase 3: checked at room creation,
  at buy-in, at cash-out — never assumed from context.
- Real money still uses integer chip counts (`int64`) — same convention as every other amount in this codebase.
- No new infra pattern is introduced where an existing one already fits: the reconciliation job mirrors
  `ctech-wallet`'s own Lambda + EventBridge Scheduler shape; alarms mirror `ctech-wallet`'s "grep the app log for
  an ALARM line, alarm on any occurrence" pattern (`ctech-wallet/cdk/lib/api-stack.ts`'s `AlarmLogFilter`).
- Every route stays under `/v1.0/` (existing convention).

---

### Task 1: `GameWallet` client and activation check

**Files:**
- Modify: `api/internal/walletclient/client.go`
- Test: `api/internal/walletclient/gamewallet_test.go`

**Interfaces:**
- Produces: `func (c *Client) CreditGame(ctx, userID string, amount int64, idempotencyKey, reason string) error`,
  `func (c *Client) DebitGame(ctx, userID string, amount int64, idempotencyKey, reason string) error`,
  `func (c *Client) IsGamblingActivated(ctx, userID string) (bool, error)` — consumed by Task 3's real-money
  buy-in/cash-out wiring.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/walletclient/gamewallet_test.go
package walletclient

import (
	"encoding/json"
	"net/http"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/config"
)

func TestIsGamblingActivatedParsesStatusResponse(t *testing.T) {
	srv := fakeWalletServer(t, func(string, MovementRequest) {})
	defer srv.Close()
	mux := srv.Config.Handler.(*http.ServeMux)
	mux.HandleFunc("/v1.0/internal/wallet/game/status/user-1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"activated": true})
	})

	c := New(&config.Config{WalletURL: srv.URL, PokerClientID: "poker", PokerClientSecret: "secret"})
	ok, err := c.IsGamblingActivated(t.Context(), "user-1")
	if err != nil || !ok {
		t.Fatalf("expected activated=true, got ok=%v err=%v", ok, err)
	}
}

func TestCreditGameAndDebitGameHitGameEndpoints(t *testing.T) {
	var paths []string
	srv := fakeWalletServer(t, func(path string, _ MovementRequest) { paths = append(paths, path) })
	defer srv.Close()
	mux := srv.Config.Handler.(*http.ServeMux)
	mux.HandleFunc("/v1.0/internal/wallet/game/credit", func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/v1.0/internal/wallet/game/debit", func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.WriteHeader(http.StatusCreated)
	})

	c := New(&config.Config{WalletURL: srv.URL, PokerClientID: "poker", PokerClientSecret: "secret"})
	if err := c.CreditGame(t.Context(), "user-1", 500, "k1", "cashout"); err != nil {
		t.Fatalf("credit game: %v", err)
	}
	if err := c.DebitGame(t.Context(), "user-1", 500, "k2", "buyin"); err != nil {
		t.Fatalf("debit game: %v", err)
	}
	if len(paths) != 2 || paths[0] != "/v1.0/internal/wallet/game/credit" || paths[1] != "/v1.0/internal/wallet/game/debit" {
		t.Fatalf("expected game credit/debit endpoints hit, got %v", paths)
	}
}
```

`fakeWalletServer`'s `srv.Config.Handler` cast assumes `httptest.NewServer(mux)` was constructed with a
`*http.ServeMux` — already true of the existing helper from Phase 3's `client_test.go`; no change needed there.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/walletclient/... -v`
Expected: FAIL with "undefined: IsGamblingActivated" / "undefined: CreditGame".

- [ ] **Step 3: Implement**

```go
// api/internal/walletclient/client.go — add constants
const (
	pathGameCredit = "/v1.0/internal/wallet/game/credit"
	pathGameDebit  = "/v1.0/internal/wallet/game/debit"
	pathGameStatus = "/v1.0/internal/wallet/game/status/%s"

	scopeGameCredit = "internal:wallet:game-credit"
	scopeGameDebit  = "internal:wallet:game-debit"
	scopeGameStatus = "internal:wallet:game-status"
)
```

```go
// api/internal/walletclient/client.go — Client struct, add
	gameCreditTokens *oauth2client.TokenManager
	gameDebitTokens  *oauth2client.TokenManager
	gameStatusTokens *oauth2client.TokenManager
```

```go
// api/internal/walletclient/client.go — New, add alongside the existing two token managers
		gameCreditTokens: oauth2client.New(httpClient, base+pathToken, cfg.PokerClientID, cfg.PokerClientSecret, scopeGameCredit),
		gameDebitTokens:  oauth2client.New(httpClient, base+pathToken, cfg.PokerClientID, cfg.PokerClientSecret, scopeGameDebit),
		gameStatusTokens: oauth2client.New(httpClient, base+pathToken, cfg.PokerClientID, cfg.PokerClientSecret, scopeGameStatus),
```

```go
// api/internal/walletclient/client.go — add
func (c *Client) CreditGame(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error {
	return c.movement(ctx, c.base+pathGameCredit, c.gameCreditTokens, userID, amount, idempotencyKey, reason)
}

func (c *Client) DebitGame(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error {
	return c.movement(ctx, c.base+pathGameDebit, c.gameDebitTokens, userID, amount, idempotencyKey, reason)
}

// IsGamblingActivated checks whether userID has completed ctech-wallet's
// ActivateGambling flow (verified KYC + gambling addendum) — a real room
// must never seat a player who hasn't, since CreditGame/DebitGame will
// simply fail wallet-side for an unactivated user, and failing that late
// (after a debit attempt) is worse than checking first.
func (c *Client) IsGamblingActivated(ctx context.Context, userID string) (bool, error) {
	token, err := c.gameStatusTokens.Get(ctx)
	if err != nil {
		return false, fmt.Errorf("walletclient: token: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+fmt.Sprintf(pathGameStatus, userID), nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("walletclient: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("walletclient: status %d: %s", resp.StatusCode, string(raw))
	}
	var body struct {
		Activated bool `json:"activated"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false, fmt.Errorf("walletclient: decode: %w", err)
	}
	return body.Activated, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/walletclient/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/walletclient
git commit -m "feat(walletclient): game-wallet credit/debit and gambling-activation check"
```

---

### Task 2: Fail-closed real-money feature gate

**Files:**
- Modify: `api/internal/config/config.go`
- Test: `api/internal/config/config_test.go`

**Interfaces:**
- Produces: `Config.RealMoneyEnabled bool`, `Config.LegalSignoffRef string` — `Load()` errors if the former is
  `true` and the latter is empty.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/config/config_test.go
package config

import (
	"os"
	"testing"
)

func TestLoadFailsClosedWhenRealMoneyEnabledWithoutLegalSignoff(t *testing.T) {
	t.Setenv("REAL_MONEY_ENABLED", "true")
	t.Setenv("LEGAL_SIGNOFF_REF", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected Load to fail closed: REAL_MONEY_ENABLED=true with no LEGAL_SIGNOFF_REF")
	}
}

func TestLoadSucceedsWhenRealMoneyEnabledWithLegalSignoff(t *testing.T) {
	t.Setenv("REAL_MONEY_ENABLED", "true")
	t.Setenv("LEGAL_SIGNOFF_REF", "LEGAL-2026-001")
	if _, err := Load(); err != nil {
		t.Fatalf("expected Load to succeed with a recorded legal sign-off, got %v", err)
	}
}

func TestLoadSucceedsWithRealMoneyDisabledAndNoSignoff(t *testing.T) {
	_ = os.Unsetenv("REAL_MONEY_ENABLED")
	_ = os.Unsetenv("LEGAL_SIGNOFF_REF")
	if _, err := Load(); err != nil {
		t.Fatalf("expected Load to succeed with real money disabled, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/... -run TestLoad -v`
Expected: FAIL — `RealMoneyEnabled`/`LegalSignoffRef` fields don't exist.

- [ ] **Step 3: Implement**

```go
// api/internal/config/config.go — Config struct, add
	// Real-money mode gate (Phase 5) — see this plan's Global Constraints.
	// Both fields fail closed together: RealMoneyEnabled=true with no
	// LegalSignoffRef means "an engineer flipped a flag with no recorded
	// business sign-off", which Load refuses to start with, not just warn
	// about — the legal risk here is explicitly bigger than any engineering
	// risk in this codebase (OVERVIEW.md §11).
	RealMoneyEnabled bool   `env:"REAL_MONEY_ENABLED" envDefault:"false"`
	LegalSignoffRef  string `env:"LEGAL_SIGNOFF_REF"`
```

```go
// api/internal/config/config.go — Load, add after the existing VALKEY_URL fail-closed check
	if cfg.RealMoneyEnabled && cfg.LegalSignoffRef == "" {
		return nil, fmt.Errorf("config: REAL_MONEY_ENABLED=true requires a non-empty LEGAL_SIGNOFF_REF (OVERVIEW.md §11 — this is a business decision, not an engineering toggle)")
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/config
git commit -m "feat(config): fail closed on real-money mode without a recorded legal sign-off"
```

---

### Task 3: Real-money room creation, buy-in, and cash-out

**Files:**
- Modify: `api/internal/api/v1/rooms.go`
- Modify: `api/internal/buyin/service.go`
- Test: `api/internal/buyin/service_test.go`
- Test: `api/internal/api/v1/rooms_test.go`

**Interfaces:**
- `buyin.Service` gains a `game walletMover` (the `GameWallet` side of `walletclient.Client`) and an
  `activation func(ctx, userID string) (bool, error)`; routes to `game`/`sandbox` based on the room's
  `CurrencyMode`.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/buyin/service_test.go — add
type fakeActivation struct{ activated map[string]bool }

func (f *fakeActivation) IsActivated(_ context.Context, userID string) (bool, error) {
	return f.activated[userID], nil
}

func TestBuyInRejectsRealRoomWithoutGamblingActivation(t *testing.T) {
	sandbox := &fakeWallet{}
	game := &fakeWallet{}
	mgr := testManager(nil)
	rooms := &fakeRoomLookup{room: &roomstore.Room{ID: "room-real-1", CurrencyMode: "real"}}
	svc := NewServiceWithGame(sandbox, game, mgr, rooms, &fakeActivation{activated: map[string]bool{}})
	ctx := context.Background()

	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	if _, err := mgr.Acquire(ctx, "room-real-1", seed); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	if err := svc.BuyIn(ctx, "room-real-1", "user-1", 400, false); err == nil {
		t.Fatal("expected buy-in to be rejected for a non-activated user in a real room")
	}
	if len(game.debits) != 0 {
		t.Fatal("expected no game-wallet debit attempted before the activation check")
	}
}

func TestBuyInUsesGameWalletForRealRooms(t *testing.T) {
	sandbox := &fakeWallet{}
	game := &fakeWallet{}
	mgr := testManager(nil)
	rooms := &fakeRoomLookup{room: &roomstore.Room{ID: "room-real-2", CurrencyMode: "real"}}
	svc := NewServiceWithGame(sandbox, game, mgr, rooms, &fakeActivation{activated: map[string]bool{"user-1": true}})
	ctx := context.Background()

	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	if _, err := mgr.Acquire(ctx, "room-real-2", seed); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	if err := svc.BuyIn(ctx, "room-real-2", "user-1", 400, false); err != nil {
		t.Fatalf("buyin: %v", err)
	}
	if len(game.debits) != 1 || len(sandbox.debits) != 0 {
		t.Fatalf("expected exactly one game-wallet debit and zero sandbox debits, got game=%d sandbox=%d", len(game.debits), len(sandbox.debits))
	}
}
```

`fakeRoomLookup` is a small test double implementing the same one-method interface `NewServiceWithGame` needs
from `roomstore.Store` (`Get`) — add it alongside the other fakes:

```go
// api/internal/buyin/service_test.go — add
type fakeRoomLookup struct{ room *roomstore.Room }

func (f *fakeRoomLookup) Get(context.Context, string) (*roomstore.Room, error) { return f.room, nil }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/buyin/... -v`
Expected: FAIL with "undefined: NewServiceWithGame".

- [ ] **Step 3: Implement the routing**

```go
// api/internal/buyin/service.go — narrow roomstore.Store to what this package needs
type roomLookup interface {
	Get(ctx context.Context, roomID string) (*roomstore.Room, error)
}

type activationChecker interface {
	IsActivated(ctx context.Context, userID string) (bool, error)
}
```

```go
// api/internal/buyin/service.go — Service struct, add
	game       walletMover
	rooms      roomLookup
	activation activationChecker
```

```go
// api/internal/buyin/service.go — replace NewService, add NewServiceWithGame
// NewService builds a sandbox-only Service (Phase 3's original constructor —
// kept for callers, mostly tests, that never touch real-money rooms).
func NewService(wallet walletMover, manager *tablemanager.Manager, rooms roomLookup) *Service {
	return &Service{wallet: wallet, manager: manager, rooms: rooms}
}

// NewServiceWithGame builds a Service that also handles real-money rooms —
// game is the ring-fenced-balance wallet client, activation checks
// ctech-wallet's ActivateGambling status before ever attempting a debit.
func NewServiceWithGame(wallet, game walletMover, manager *tablemanager.Manager, rooms roomLookup, activation activationChecker) *Service {
	return &Service{wallet: wallet, game: game, manager: manager, rooms: rooms, activation: activation}
}
```

```go
// api/internal/buyin/service.go — add, used by both BuyIn and CashOut
func (s *Service) walletFor(ctx context.Context, roomID, playerID string) (walletMover, error) {
	room, err := s.rooms.Get(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("buyin: room lookup: %w", err)
	}
	if room == nil || room.CurrencyMode != "real" {
		return s.wallet, nil
	}
	if s.game == nil || s.activation == nil {
		return nil, fmt.Errorf("buyin: room %s is real-money but this Service was built without NewServiceWithGame", roomID)
	}
	ok, err := s.activation.IsActivated(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("buyin: activation check: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("buyin: player %s has not activated gambling on ctech-wallet", playerID)
	}
	return s.game, nil
}
```

```go
// api/internal/buyin/service.go — BuyIn, replace the hardcoded s.wallet.Debit call
	mover, err := s.walletFor(ctx, roomID, playerID)
	if err != nil {
		return fmt.Errorf("buyin: %w", err)
	}
	idemKey := fmt.Sprintf("%s#%s#buyin", roomID, playerID)
	if err := mover.Debit(ctx, playerID, amount, idemKey, "poker_buyin"); err != nil {
		return fmt.Errorf("buyin: debit: %w", err)
	}
```

Every subsequent refund/credit call inside `BuyIn` and `CashOut` (both already written in Phase 3 Task 3) must
switch from `s.wallet` to the same resolved `mover` — replace every remaining `s.wallet.Credit(...)` /
`s.wallet.Debit(...)` call in both methods with `mover.Credit(...)`/`mover.Debit(...)`, resolving `mover` once at
the top of `CashOut` the same way `BuyIn` now does.

- [ ] **Step 4: Enforce activation and `currency_mode="real"` gating at room creation**

```go
// api/internal/api/v1/rooms.go — CreateRoomRequest, add
	CurrencyMode string `json:"currency_mode,omitempty"` // "sandbox" (default) | "real"
```

```go
// api/internal/api/v1/rooms.go — createRoom, replace the hardcoded CurrencyMode: "sandbox"
	currencyMode := req.CurrencyMode
	if currencyMode == "" {
		currencyMode = "sandbox"
	}
	if currencyMode == "real" && !h.cfg.RealMoneyEnabled {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "real-money mode is not enabled in this environment"})
	}
	if currencyMode != "sandbox" && currencyMode != "real" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "currency_mode must be sandbox or real"})
	}
```

```go
// api/internal/api/v1/rooms.go — room := roomstore.Room{...}, replace CurrencyMode: "sandbox"
		CurrencyMode: currencyMode,
```

Add `cfg *config.Config` to `roomHandlers` and thread it through `RegisterRooms`'s parameters and `router.go`'s
call site.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/buyin/... ./internal/api/v1/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/internal/buyin api/internal/api/v1/rooms.go
git commit -m "feat(buyin): route real-money rooms to the ring-fenced game wallet, gated on activation"
```

---

### Task 4: Cash-out reconciliation — durable pending-credit tracking

**Files:**
- Create: `api/internal/reconcile/pending.go`
- Modify: `api/internal/buyin/service.go`
- Test: `api/internal/reconcile/pending_test.go`
- Test: `api/internal/buyin/service_test.go`

**Interfaces:**
- Produces: `type PendingCashout struct{...}`, `func NewPendingStore(db *dynamodb.Client, env string)
  *PendingStore`, `func (s *PendingStore) Record(ctx, PendingCashout) error`, `func (s *PendingStore)
  MarkResolved(ctx, id string) error`, `func (s *PendingStore) ListUnresolved(ctx, olderThan time.Duration)
  ([]PendingCashout, error)` — consumed by Task 5's reconciliation job.

This closes Phase 3's explicitly-flagged gap ("no compensating action if the wallet credit fails after seat
removal") — mandatory for real money, where "chips gone from the table but not yet in the wallet" is an actual
financial loss, not a sandbox inconvenience.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/reconcile/pending_test.go
//go:build integration

package reconcile

import (
	"context"
	"testing"
	"time"
)

func TestRecordThenListUnresolvedThenMarkResolved(t *testing.T) {
	db := testClient(t)
	s := NewPendingStore(db, "test")
	ctx := context.Background()
	mustCreateTestTable(ctx, t, db, "test")

	p := PendingCashout{ID: "co-1", PlayerID: "user-1", Amount: 400, CurrencyMode: "real", IdempotencyKey: "room-1#user-1#cashout"}
	if err := s.Record(ctx, p); err != nil {
		t.Fatalf("record: %v", err)
	}

	unresolved, err := s.ListUnresolved(ctx, 0)
	if err != nil || len(unresolved) != 1 || unresolved[0].ID != "co-1" {
		t.Fatalf("expected one unresolved entry, got %+v, err=%v", unresolved, err)
	}

	if err := s.MarkResolved(ctx, "co-1"); err != nil {
		t.Fatalf("mark resolved: %v", err)
	}
	unresolved, err = s.ListUnresolved(ctx, 0)
	if err != nil || len(unresolved) != 0 {
		t.Fatalf("expected zero unresolved after MarkResolved, got %+v", unresolved)
	}
	_ = time.Second // olderThan=0 above means "any age" — Step 3 documents the real filter semantics
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `docker compose -f api/docker-compose.test.yml up -d && go test -tags integration ./internal/reconcile/... -v`
Expected: FAIL with "undefined: NewPendingStore".

- [ ] **Step 3: Implement**

```go
// api/internal/reconcile/pending.go
// Package reconcile tracks cash-outs whose seat removal succeeded but whose
// wallet credit has not yet been confirmed — the durable record that makes
// "no money left in limbo" (ctech-wallet's own Financial Safety Invariant,
// applied here since real chips are now involved) enforceable by a
// background sweep instead of hoped for.
package reconcile

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	tablePending = "poker_pending_cashouts"
	pendingSK    = "pending"
)

type PendingCashout struct {
	ID             string `dynamodbav:"id"`
	PlayerID       string `dynamodbav:"player_id"`
	Amount         int64  `dynamodbav:"amount"`
	CurrencyMode   string `dynamodbav:"currency_mode"` // "sandbox" | "real" — the job only ever alarms on "real"
	IdempotencyKey string `dynamodbav:"idempotency_key"`
	RecordedAt     string `dynamodbav:"recorded_at"`
	Resolved       bool   `dynamodbav:"resolved"`
}

type PendingStore struct {
	base dynamo.Base
}

func NewPendingStore(db *dynamodb.Client, env string) *PendingStore {
	return &PendingStore{base: dynamo.NewBase(db, env, tablePending)}
}

// Record persists a cash-out BEFORE the compensating credit is attempted —
// buyin.Service calls this immediately after removing the player's seat, so
// even a crash between seat removal and the credit call leaves a durable
// trail this job can find and finish.
func (s *PendingStore) Record(ctx context.Context, p PendingCashout) error {
	p.RecordedAt = dynamo.NowStr()
	item, err := dynamo.Encode(struct {
		PK string `dynamodbav:"pk"`
		SK string `dynamodbav:"sk"`
		PendingCashout
	}{PK: p.ID, SK: pendingSK, PendingCashout: p})
	if err != nil {
		return fmt.Errorf("reconcile: encode: %w", err)
	}
	return s.base.PutItem(ctx, item)
}

func (s *PendingStore) MarkResolved(ctx context.Context, id string) error {
	sk := pendingSK
	_, err := s.base.UpdateItem(ctx, id, &sk, map[string]any{"resolved": true})
	if err != nil {
		return fmt.Errorf("reconcile: mark resolved: %w", err)
	}
	return nil
}

// ListUnresolved scans for entries recorded more than olderThan ago — a
// nonzero grace period avoids racing buyin.Service's own immediate credit
// attempt (Task 4 Step 4): most cash-outs resolve within milliseconds, so
// the job should only ever pick up ones that are actually stuck.
//
// This is a table Scan (only production DynamoDB access this whole codebase
// makes this way — dynamo.Base's design rules elsewhere require
// get_item/query — justified here because poker_pending_cashouts is
// expected to stay tiny: every row is deleted-in-effect (Resolved=true)
// within seconds of being written under normal operation, so it never grows
// large enough for a Scan to be a real cost or latency concern. If real
// volume ever makes that untrue, add a GSI on `resolved` instead.
func (s *PendingStore) ListUnresolved(ctx context.Context, olderThan time.Duration) ([]PendingCashout, error) {
	result, err := s.base.Query(ctx, dynamo.QueryOpts{PK: "", Limit: 1}) // placeholder to be replaced by a real scan below
	_ = result
	return s.scanUnresolved(ctx, olderThan)
}
```

`dynamo.Base` has no exported `Scan` method (by design — "no scans in production" per its own package doc) — this
task's own justification above argues for an exception, which must be a deliberate, narrow addition to
`dynamo.Base`, not a workaround inside `reconcile`:

```go
// gopkg.aoctech.app/api-commons/dynamo/base.go — add (a ctech-go-common change, flagged like Task 12 of the
// foundations plan's shared-package extractions — this plan's Task 4 Step 3 continued below assumes it exists)
// ScanAll is a deliberate, narrow exception to this package's own "no scans"
// rule — for the rare table expected to stay small by construction (e.g. a
// pending-reconciliation queue that's supposed to drain within seconds).
// Callers must document why their table is exempt at the call site.
func (b *Base) ScanAll(ctx context.Context) (*QueryResult, error) {
	out, err := b.db.Scan(ctx, &dynamodb.ScanInput{TableName: aws.String(b.TableName)})
	if err != nil {
		return nil, wrapDynamoErr(err)
	}
	return &QueryResult{Items: out.Items}, nil
}
```

```go
// api/internal/reconcile/pending.go — replace ListUnresolved with its real implementation
func (s *PendingStore) ListUnresolved(ctx context.Context, olderThan time.Duration) ([]PendingCashout, error) {
	result, err := s.base.ScanAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("reconcile: scan: %w", err)
	}
	cutoff := time.Now().Add(-olderThan)
	out := make([]PendingCashout, 0, len(result.Items))
	for _, item := range result.Items {
		p, err := dynamo.Decode[PendingCashout](item)
		if err != nil {
			return nil, fmt.Errorf("reconcile: decode: %w", err)
		}
		if p.Resolved {
			continue
		}
		recordedAt, err := time.Parse(time.RFC3339Nano, p.RecordedAt)
		if err == nil && recordedAt.After(cutoff) {
			continue
		}
		out = append(out, *p)
	}
	return out, nil
}
```

- [ ] **Step 4: Wire `Record`/`MarkResolved` into `buyin.Service.CashOut`**

```go
// api/internal/buyin/service.go — Service struct, add
	pending *reconcile.PendingStore
```

```go
// api/internal/buyin/service.go — CashOut, replace the final Credit block
	idemKey := fmt.Sprintf("%s#%s#cashout", roomID, playerID)
	pendingID := idemKey // stable per (room, player, attempt-shape) — good enough as a dedup-safe record ID
	if s.pending != nil {
		room, _ := s.rooms.Get(ctx, roomID)
		mode := "sandbox"
		if room != nil {
			mode = room.CurrencyMode
		}
		_ = s.pending.Record(ctx, reconcile.PendingCashout{ID: pendingID, PlayerID: playerID, Amount: stack, CurrencyMode: mode, IdempotencyKey: idemKey})
	}
	if err := mover.Credit(ctx, playerID, stack, idemKey, "poker_cashout"); err != nil {
		return stack, fmt.Errorf("buyin: cash-out credit failed after seat removal — reconciliation job will retry (pending id %s): %w", pendingID, err)
	}
	if s.pending != nil {
		_ = s.pending.MarkResolved(ctx, pendingID)
	}
	return stack, nil
```

Add `NewServiceWithGame`'s signature to accept a trailing `pending *reconcile.PendingStore` parameter (nil is
valid — Phase 3's sandbox-only callers and most tests never need it); update the two call sites from Task 1/3's
tests accordingly (pass `nil`).

- [ ] **Step 5: Run test to verify it passes**

Run: `docker compose -f api/docker-compose.test.yml up -d && go test -tags integration ./internal/reconcile/... -v && go test ./internal/buyin/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/internal/reconcile api/internal/buyin/service.go
git commit -m "feat(reconcile): durable pending-cashout tracking for cash-outs whose credit hasn't confirmed yet"
```

---

### Task 5: Reconciliation job (Lambda + EventBridge Scheduler)

**Files:**
- Create: `api/cmd/reconcile/main.go`
- Create: `cdk/lib/reconcile-stack.ts`
- Modify: `cdk/bin/poker.ts`
- Test: `api/cmd/reconcile/main_test.go`

**Interfaces:**
- Consumes: `reconcile.PendingStore` (Task 4), `walletclient.Client` (Task 1).
- Produces: a standalone Go binary, deployed as a Lambda on a 5-minute EventBridge schedule — mirrors
  `ctech-wallet`'s own `cmd/reconcile`/`ReconcileStack` shape exactly.

- [ ] **Step 1: Write the failing test**

```go
// api/cmd/reconcile/main_test.go
package main

import (
	"context"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/reconcile"
)

type fakePendingLister struct {
	unresolved []reconcile.PendingCashout
	resolved   []string
}

func (f *fakePendingLister) ListUnresolved(context.Context, time.Duration) ([]reconcile.PendingCashout, error) {
	return f.unresolved, nil
}
func (f *fakePendingLister) MarkResolved(_ context.Context, id string) error {
	f.resolved = append(f.resolved, id)
	return nil
}

type fakeCredit struct{ credited []reconcile.PendingCashout }

func (f *fakeCredit) CreditGame(_ context.Context, userID string, amount int64, _, _ string) error {
	f.credited = append(f.credited, reconcile.PendingCashout{PlayerID: userID, Amount: amount})
	return nil
}
func (f *fakeCredit) Credit(context.Context, string, int64, string, string) error { return nil }

func TestRunResolvesEveryUnresolvedRealCashout(t *testing.T) {
	pending := &fakePendingLister{unresolved: []reconcile.PendingCashout{
		{ID: "co-1", PlayerID: "user-1", Amount: 400, CurrencyMode: "real", IdempotencyKey: "k1"},
		{ID: "co-2", PlayerID: "user-2", Amount: 100, CurrencyMode: "sandbox", IdempotencyKey: "k2"},
	}}
	credit := &fakeCredit{}

	if err := run(context.Background(), pending, credit, nil); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(pending.resolved) != 2 {
		t.Fatalf("expected both entries marked resolved, got %v", pending.resolved)
	}
	if len(credit.credited) != 1 || credit.credited[0].PlayerID != "user-1" {
		t.Fatalf("expected only the real-money entry credited via CreditGame, got %+v", credit.credited)
	}
}
```

`main_test.go` is missing a `"time"` import — add it. `run`'s 4th parameter (`nil` above) is a sandbox-credit
mover, added in Step 3.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/reconcile/... -v`
Expected: FAIL with "undefined: run".

- [ ] **Step 3: Implement `main.go`**

```go
// api/cmd/reconcile/main.go
// The reconciliation job sweeps poker_pending_cashouts for entries a crash
// left unresolved (Task 4) and retries the credit — mirrors
// ctech-wallet/api/cmd/reconcile's shape (a standalone Lambda on a
// scheduled trigger, not part of the always-on API process).
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	awscfg "gopkg.aoctech.app/api-commons/awsconfig"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/reconcile"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

// gracePeriod matches Task 4's PendingStore.ListUnresolved doc: only entries
// older than this are actually stuck (a fresh entry is almost always
// resolved within milliseconds by buyin.Service's own immediate retry).
const gracePeriod = 2 * time.Minute

type pendingLister interface {
	ListUnresolved(ctx context.Context, olderThan time.Duration) ([]reconcile.PendingCashout, error)
	MarkResolved(ctx context.Context, id string) error
}

type gameCredit interface {
	CreditGame(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
}

type sandboxCredit interface {
	Credit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
}

func run(ctx context.Context, pending pendingLister, game gameCredit, sandbox sandboxCredit) error {
	entries, err := pending.ListUnresolved(ctx, gracePeriod)
	if err != nil {
		return err
	}
	for _, e := range entries {
		var creditErr error
		switch e.CurrencyMode {
		case "real":
			creditErr = game.CreditGame(ctx, e.PlayerID, e.Amount, e.IdempotencyKey, "poker_cashout_reconcile")
		default:
			if sandbox != nil {
				creditErr = sandbox.Credit(ctx, e.PlayerID, e.Amount, e.IdempotencyKey, "poker_cashout_reconcile")
			}
		}
		if creditErr != nil {
			// ALARM-prefixed per ctech-wallet's own convention (its
			// api-stack.ts AlarmLogFilter greps the app log for exactly this
			// string) — Task 8's CDK alarm mirrors that same pattern for
			// this job's own log group.
			slog.Error("ALARM: reconcile credit failed, needs manual review", "pending_id", e.ID, "player", e.PlayerID, "amount", e.Amount, "err", creditErr)
			continue
		}
		if err := pending.MarkResolved(ctx, e.ID); err != nil {
			slog.Error("ALARM: reconcile resolved credit but failed to mark pending entry resolved — will retry redundantly next run", "pending_id", e.ID, "err", err)
		}
	}
	return nil
}

func handler(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	awsCfg, err := awscfg.LoadDefaultConfig(ctx)
	if err != nil {
		return err
	}
	db := dynamodb.NewFromConfig(awsCfg)
	pendingStore := reconcile.NewPendingStore(db, cfg.Env)
	wallet := walletclient.New(cfg)
	return run(ctx, pendingStore, wallet, wallet)
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	lambda.Start(handler)
}
```

`gopkg.aoctech.app/api-commons/awsconfig.LoadDefaultConfig`'s exact signature must be confirmed against its actual
source before this compiles (it's an existing shared package this codebase hasn't used directly yet outside
`internal/tablestore`'s test-only client construction) — if it takes a region argument, pass `cfg.AWSRegion` or
equivalent; adjust to match, this is a one-line fix once the real signature is read.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/reconcile/... -v`
Expected: PASS.

- [ ] **Step 5: CDK — deploy as a scheduled Lambda**

```typescript
// cdk/lib/reconcile-stack.ts
import * as cdk from 'aws-cdk-lib';
import * as lambda from 'aws-cdk-lib/aws-lambda';
import * as iam from 'aws-cdk-lib/aws-iam';
import * as scheduler from 'aws-cdk-lib/aws-scheduler';
import * as schedulerTargets from 'aws-cdk-lib/aws-scheduler-targets';
import * as logs from 'aws-cdk-lib/aws-logs';
import {Construct} from 'constructs';
import path from 'node:path';
import {Environment} from '@aoctech/cdk';
import {SERVICE} from './constants';

const RECONCILE_RATE_MINUTES = 5;
const API_DIR = path.join(__dirname, '../../api');

interface ReconcileStackProps extends cdk.StackProps {
  environment: Environment;
  pendingCashoutsTableArn: string;
  walletUrlParam: string;
  pokerClientIdParam: string;
  pokerClientSecretParam: string;
}

/**
 * Cash-out reconciliation job — mirrors ctech-wallet/cdk/lib/reconcile-stack.ts's
 * shape exactly: a Lambda built from the same Go module (cmd/reconcile), on an
 * EventBridge Scheduler rate trigger. Deliberately not in the VPC (only needs
 * DynamoDB + ctech-wallet's public API, same rationale as wallet's own job).
 */
export class ReconcileStack extends cdk.Stack {
  constructor(scope: Construct, id: string, props: ReconcileStackProps) {
    super(scope, id, props);
    const {environment, pendingCashoutsTableArn, walletUrlParam, pokerClientIdParam, pokerClientSecretParam} = props;

    const role = new iam.Role(this, 'ReconcileRole', {
      assumedBy: new iam.ServicePrincipal('lambda.amazonaws.com'),
      managedPolicies: [iam.ManagedPolicy.fromAwsManagedPolicyName('service-role/AWSLambdaBasicExecutionRole')],
    });
    role.addToPolicy(new iam.PolicyStatement({
      actions: ['dynamodb:Scan', 'dynamodb:UpdateItem'],
      resources: [pendingCashoutsTableArn],
    }));
    role.addToPolicy(new iam.PolicyStatement({
      actions: ['ssm:GetParameter'],
      resources: [
        `arn:aws:ssm:${this.region}:${this.account}:parameter${walletUrlParam}`,
        `arn:aws:ssm:${this.region}:${this.account}:parameter${pokerClientIdParam}`,
        `arn:aws:ssm:${this.region}:${this.account}:parameter${pokerClientSecretParam}`,
      ],
    }));

    const fn = new lambda.Function(this, 'ReconcileFunction', {
      functionName: `${environment}-${SERVICE}-reconcile`,
      runtime: lambda.Runtime.PROVIDED_AL2023,
      architecture: lambda.Architecture.ARM_64,
      handler: 'bootstrap',
      code: lambda.Code.fromAsset(path.join(API_DIR, 'dist/reconcile')), // built by `make build-reconcile` — see Step 6
      role,
      timeout: cdk.Duration.minutes(2),
      memorySize: 256,
      environment: {
        ENVIRONMENT: environment,
        WALLET_URL_PARAM: walletUrlParam,
        POKER_CLIENT_ID_PARAM: pokerClientIdParam,
        POKER_CLIENT_SECRET_PARAM: pokerClientSecretParam,
      },
      logRetention: environment === 'prod' ? logs.RetentionDays.ONE_MONTH : logs.RetentionDays.ONE_WEEK,
    });

    new scheduler.CfnSchedule(this, 'ReconcileSchedule', {
      flexibleTimeWindow: {mode: 'OFF'},
      scheduleExpression: `rate(${RECONCILE_RATE_MINUTES} minutes)`,
      target: {
        arn: fn.functionArn,
        roleArn: new iam.Role(this, 'SchedulerInvokeRole', {
          assumedBy: new iam.ServicePrincipal('scheduler.amazonaws.com'),
        }).addToPrincipalPolicy
          ? undefined
          : undefined, // placeholder removed below — see the corrected block
      } as scheduler.CfnSchedule.TargetProperty,
    });
  }
}
```

The `ReconcileSchedule`'s `target.roleArn` construction above is malformed (an inline ternary that never actually
grants invoke permission) — replace it with the straightforward form:

```typescript
// cdk/lib/reconcile-stack.ts — replace the CfnSchedule block
    const schedulerRole = new iam.Role(this, 'SchedulerInvokeRole', {
      assumedBy: new iam.ServicePrincipal('scheduler.amazonaws.com'),
    });
    fn.grantInvoke(schedulerRole);

    new scheduler.CfnSchedule(this, 'ReconcileSchedule', {
      flexibleTimeWindow: {mode: 'OFF'},
      scheduleExpression: `rate(${RECONCILE_RATE_MINUTES} minutes)`,
      target: {arn: fn.functionArn, roleArn: schedulerRole.roleArn},
    });
```

`config.Load()`'s existing env var names (`WALLET_URL`, `POKER_CLIENT_ID`, `POKER_CLIENT_SECRET`) don't match the
Lambda's `*_PARAM` environment variables above (those are SSM *paths*, not values) — the Lambda handler must
resolve them itself before calling `config.Load()`, since Lambda has no userdata boot script to pre-resolve SSM
the way EC2 does:

```go
// api/cmd/reconcile/main.go — handler, before config.Load()
	resolved, err := resolveSSMParams(ctx, os.Getenv("WALLET_URL_PARAM"), os.Getenv("POKER_CLIENT_ID_PARAM"), os.Getenv("POKER_CLIENT_SECRET_PARAM"))
	if err != nil {
		return err
	}
	_ = os.Setenv("WALLET_URL", resolved.walletURL)
	_ = os.Setenv("POKER_CLIENT_ID", resolved.clientID)
	_ = os.Setenv("POKER_CLIENT_SECRET", resolved.clientSecret)
```

```go
// api/cmd/reconcile/main.go — add
type resolvedParams struct{ walletURL, clientID, clientSecret string }

func resolveSSMParams(ctx context.Context, walletURLParam, clientIDParam, clientSecretParam string) (resolvedParams, error) {
	awsCfg, err := awscfg.LoadDefaultConfig(ctx)
	if err != nil {
		return resolvedParams{}, err
	}
	ssmClient := ssm.NewFromConfig(awsCfg)
	get := func(name string, withDecryption bool) (string, error) {
		out, err := ssmClient.GetParameter(ctx, &ssm.GetParameterInput{Name: &name, WithDecryption: &withDecryption})
		if err != nil {
			return "", err
		}
		return *out.Parameter.Value, nil
	}
	walletURL, err := get(walletURLParam, false)
	if err != nil {
		return resolvedParams{}, err
	}
	clientID, err := get(clientIDParam, false)
	if err != nil {
		return resolvedParams{}, err
	}
	clientSecret, err := get(clientSecretParam, true)
	if err != nil {
		return resolvedParams{}, err
	}
	return resolvedParams{walletURL: walletURL, clientID: clientID, clientSecret: clientSecret}, nil
}
```

Add `"github.com/aws/aws-sdk-go-v2/service/ssm"` import. Grant the Lambda role `ssm:GetParameter` on all three
paths (already present in the CDK block above).

- [ ] **Step 6: Add the Lambda build target**

```makefile
# api/Makefile — add
build-reconcile:
	GOOS=linux GOARCH=arm64 go build -tags lambda.norpc -o dist/reconcile/bootstrap ./cmd/reconcile
```

- [ ] **Step 7: Wire `bin/poker.ts`**

```typescript
// cdk/bin/poker.ts — add
new ReconcileStack(app, `${environment}-ctech-poker-reconcile`, {
  environment, env: awsEnv,
  pendingCashoutsTableArn: dynamoStack.tables.get('poker_pending_cashouts')!.tableArn,
  walletUrlParam: `/ctech/${environment}/poker/wallet-url`,
  pokerClientIdParam: `/ctech/${environment}/poker/poker-client-id`,
  pokerClientSecretParam: `/ctech/${environment}/poker/poker-client-secret`,
});
```

Add `poker_pending_cashouts` to `DynamoDBStack`'s `TableName` union and its `table()` calls (same pattern as every
prior table in Phases 2-4).

- [ ] **Step 8: Commit**

```bash
git add api/cmd/reconcile api/Makefile cdk/lib/reconcile-stack.ts cdk/lib/dynamodb-stack.ts cdk/bin/poker.ts
git commit -m "feat(reconcile): scheduled Lambda job resolving stuck cash-out credits"
```

---

### Task 6: Structured metrics (EMF) — hands/hour, action latency, disconnect rate, lease-failover count

**Files:**
- Create: `api/internal/metrics/emf.go`
- Modify: `api/internal/table/actor.go`
- Modify: `api/internal/tablemanager/manager.go`
- Test: `api/internal/metrics/emf_test.go`

**Interfaces:**
- Produces: `func EmitTableMetric(name string, value float64, dims map[string]string)` — writes a CloudWatch
  Embedded Metric Format JSON line to stdout (the CloudWatch agent already ships `app.log` to a log group per
  existing CDK userdata; EMF-formatted lines in that same stream are auto-extracted into real CloudWatch metrics,
  no new infra needed).

ARCHITECTURE.md §7: per-table hands/hour, average action latency, disconnect rate, lease-failover count — "a
spike in lease-failover count is the earliest signal of an instance going bad."

- [ ] **Step 1: Write the failing test**

```go
// api/internal/metrics/emf_test.go
package metrics

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestEmitTableMetricWritesValidEMFJSON(t *testing.T) {
	var buf bytes.Buffer
	EmitTableMetricTo(&buf, "HandsCompleted", 1, map[string]string{"table_id": "t1"})

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("expected valid JSON, got %v (line: %s)", err, buf.String())
	}
	if !strings.Contains(buf.String(), `"_aws"`) {
		t.Fatal("expected an EMF _aws metadata block")
	}
	if parsed["table_id"] != "t1" {
		t.Fatalf("expected table_id dimension present, got %v", parsed["table_id"])
	}
	if parsed["HandsCompleted"] != float64(1) {
		t.Fatalf("expected HandsCompleted=1, got %v", parsed["HandsCompleted"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/metrics/... -v`
Expected: FAIL with "undefined: EmitTableMetricTo".

- [ ] **Step 3: Implement**

```go
// api/internal/metrics/emf.go
// Package metrics emits CloudWatch Embedded Metric Format (EMF) JSON lines
// to the same stdout stream the CloudWatch agent already tails into
// {SERVICE}/{env}/app (existing CDK userdata) — CloudWatch auto-extracts EMF
// lines into real metrics with zero new infrastructure (ARCHITECTURE.md §7).
package metrics

import (
	"encoding/json"
	"io"
	"os"
)

const namespace = "CtechPoker"

// EmitTableMetric writes one EMF line for name=value with the given
// dimensions to stdout — the production entry point every caller uses.
func EmitTableMetric(name string, value float64, dims map[string]string) {
	EmitTableMetricTo(os.Stdout, name, value, dims)
}

// EmitTableMetricTo is the testable core — writes to w instead of stdout so
// tests can assert on the exact JSON produced without capturing os.Stdout.
func EmitTableMetricTo(w io.Writer, name string, value float64, dims map[string]string) {
	dimKeys := make([]string, 0, len(dims))
	fields := map[string]any{name: value}
	for k, v := range dims {
		dimKeys = append(dimKeys, k)
		fields[k] = v
	}
	fields["_aws"] = map[string]any{
		"Timestamp": timeNowMillis(),
		"CloudWatchMetrics": []map[string]any{
			{
				"Namespace":  namespace,
				"Dimensions": [][]string{dimKeys},
				"Metrics":    []map[string]string{{"Name": name}},
			},
		},
	}
	line, err := json.Marshal(fields)
	if err != nil {
		return // a malformed metric line must never crash the caller's own logic
	}
	_, _ = w.Write(append(line, '\n'))
}

func timeNowMillis() int64 { return timeNowFunc().UnixMilli() }

var timeNowFunc = defaultNow
```

`defaultNow` needs `"time"`:

```go
// api/internal/metrics/emf.go — add
import "time"

func defaultNow() time.Time { return time.Now() }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/metrics/... -v`
Expected: PASS.

- [ ] **Step 5: Wire the four metrics**

```go
// api/internal/table/actor.go — handleAct, right after `if a.table.Stage() == hand.Complete`
	if a.table.Stage() == hand.Complete {
		metrics.EmitTableMetric("HandsCompleted", 1, map[string]string{"table_id": a.id})
		a.persistSnapshot()
		...
	}
```

```go
// api/internal/table/actor.go — handleAct, wrap the existing a.table.Act call to time it
func (a *Actor) handleAct(c ActCmd) error {
	if a.seenIDs[c.ActionID] {
		a.broadcastAll()
		return nil
	}
	start := timeNowFunc()
	if err := a.table.Act(c.PlayerID, c.Action, c.Amount); err != nil {
		return err
	}
	metrics.EmitTableMetric("ActionLatencyMs", float64(timeNowFunc().Sub(start).Milliseconds()), map[string]string{"table_id": a.id})
	...
```

```go
// api/internal/table/actor.go — handleDisconnect, add
func (a *Actor) handleDisconnect(c DisconnectCmd) error {
	metrics.EmitTableMetric("Disconnects", 1, map[string]string{"table_id": a.id})
	...
```

```go
// api/internal/tablemanager/manager.go — Acquire, inside the StartHeartbeat onLost callback
	stop := m.leases.StartHeartbeat(runCtx, tableID, func() {
		metrics.EmitTableMetric("LeaseFailovers", 1, map[string]string{"table_id": tableID})
		cancel()
		...
```

Add `"gopkg.aoctech.app/poker/api/internal/metrics"` imports to both files.

- [ ] **Step 6: Run the full suite**

Run: `go build ./... && go test ./... -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add api/internal/metrics api/internal/table/actor.go api/internal/tablemanager/manager.go
git commit -m "feat(metrics): EMF hands/hour, action latency, disconnect rate, lease-failover count"
```

---

### Task 7: CloudWatch alarms

**Files:**
- Modify: `cdk/lib/api-stack.ts`
- Test: `cdk/test/api-stack.test.ts`

**Interfaces:**
- Adds a `LeaseFailovers` metric-filter alarm (spike detection — ARCHITECTURE.md §7's "earliest signal of an
  instance going bad") and an `ALARM`-log-line alarm (mirrors `ctech-wallet/cdk/lib/api-stack.ts`'s existing
  `AlarmLogFilter` pattern exactly, now that Task 5's reconcile job and Task 6's metrics both use that convention).

- [ ] **Step 1: Write the failing test**

```typescript
// cdk/test/api-stack.test.ts — add
test('creates an alarm on ALARM-prefixed log lines and a lease-failover-spike alarm', () => {
  // (constructed against the existing test stack setup already in this file)
  template.resourceCountIs('AWS::CloudWatch::Alarm', 2);
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd cdk && npx jest api-stack.test.ts`
Expected: FAIL — zero alarms exist yet.

- [ ] **Step 3: Implement**

```typescript
// cdk/lib/api-stack.ts — after `service` is constructed, alongside the existing IAM grants
    const alarmMetricFilter = service.appLogGroup.addMetricFilter('AlarmLogFilter', {
      filterPattern: logs.FilterPattern.literal('"ALARM:"'),
      metricNamespace: `CtechPoker/${environment}`,
      metricName: 'AlarmLogLines',
      metricValue: '1',
    });
    new cloudwatch.Alarm(this, 'AlarmLogAlarm', {
      alarmName: `${environment}-${SERVICE}-alarm-log-lines`,
      alarmDescription: 'An ALARM log line was emitted (reconcile credit failure, or another manual-review condition) — needs investigation.',
      metric: alarmMetricFilter.metric({statistic: 'Sum', period: cdk.Duration.minutes(5)}),
      threshold: 1,
      evaluationPeriods: 1,
      treatMissingData: cloudwatch.TreatMissingData.NOT_BREACHING,
    });

    const leaseFailoverMetric = new cloudwatch.Metric({
      namespace: `CtechPoker`, // Task 6's metrics.go hardcodes this namespace, not per-environment
      metricName: 'LeaseFailovers',
      statistic: 'Sum',
      period: cdk.Duration.minutes(5),
    });
    new cloudwatch.Alarm(this, 'LeaseFailoverSpikeAlarm', {
      alarmName: `${environment}-${SERVICE}-lease-failover-spike`,
      alarmDescription: 'Table lease failovers spiked — earliest signal of an instance going bad (ARCHITECTURE.md §7).',
      metric: leaseFailoverMetric,
      threshold: 5,
      evaluationPeriods: 2,
      treatMissingData: cloudwatch.TreatMissingData.NOT_BREACHING,
    });
```

Add `import * as cloudwatch from 'aws-cdk-lib/aws-cloudwatch';` and `import * as logs from 'aws-cdk-lib/aws-logs';`
(the latter likely already imported) to `api-stack.ts`.

`Task 6`'s `metrics.go` hardcodes namespace `CtechPoker` (not per-environment, unlike the `AlarmLogAlarm`'s
`CtechPoker/${environment}` metric filter namespace) — this is a real inconsistency: fix `metrics.go`'s
`namespace` constant to include the environment, which requires it to accept the environment as a parameter
rather than being a package-level constant:

```go
// api/internal/metrics/emf.go — replace the namespace constant and both Emit functions' signatures
func EmitTableMetric(env, name string, value float64, dims map[string]string) {
	EmitTableMetricTo(os.Stdout, env, name, value, dims)
}

func EmitTableMetricTo(w io.Writer, env, name string, value float64, dims map[string]string) {
	namespace := "CtechPoker/" + env
	...
```

Update every Task 6 call site (`actor.go`, `manager.go`) to pass `cfg.Env` (threaded into `table.Actor`/
`tablemanager.Manager` construction, following the same pattern `equityEnabled` and `escalationCfg` already use —
add an `env string` field to `Actor`, set once in `New`, and to `Manager` similarly) and update `emf_test.go`'s
call sites to the new signature (`EmitTableMetricTo(&buf, "test", "HandsCompleted", 1, ...)`).

- [ ] **Step 4: Run test to verify it passes**

Run: `cd cdk && npx jest api-stack.test.ts && cd ../api && go build ./... && go test ./internal/metrics/... ./internal/table/... ./internal/tablemanager/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cdk/lib/api-stack.ts cdk/test/api-stack.test.ts api/internal/metrics/emf.go api/internal/table/actor.go api/internal/tablemanager/manager.go
git commit -m "feat(cdk): alarm on ALARM log lines and lease-failover spikes"
```

---

### Task 8: Graceful ASG scale-in draining

**Files:**
- Modify: `api/internal/tablemanager/manager.go`
- Modify: `api/internal/app/app.go`
- Create: `cdk/lib/lifecycle-hook.ts` (small helper, inlined into `api-stack.ts` if simpler)
- Test: `api/internal/tablemanager/drain_test.go`

**Interfaces:**
- Produces: `func (m *Manager) DrainAndRelease(ctx context.Context)` — releases every locally-owned table lease,
  letting sibling instances pick each table up via Phase 2's existing recovery path, instead of the instance
  disappearing abruptly mid-hand on scale-in or deploy.

An ASG termination lifecycle hook delays actual instance shutdown until this drain completes (bounded by a
timeout — a table stuck mid-hand forever must not block scale-in indefinitely).

- [ ] **Step 1: Write the failing test**

```go
// api/internal/tablemanager/drain_test.go
package tablemanager

import (
	"context"
	"testing"

	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tableowner"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
)

func TestDrainAndReleaseFreesEveryLocallyOwnedTable(t *testing.T) {
	backend := cache.NewMemoryBackend(16)
	m := NewManager(tablelease.NewService(backend), tableowner.NewRegistry(backend, tablelease.DefaultLeaseTTL), nil, "10.0.0.1:8003", nil, nil)
	ctx := context.Background()
	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }

	if _, err := m.Acquire(ctx, "table-1", seed); err != nil {
		t.Fatalf("acquire table-1: %v", err)
	}
	if _, err := m.Acquire(ctx, "table-2", seed); err != nil {
		t.Fatalf("acquire table-2: %v", err)
	}

	m.DrainAndRelease(ctx)

	m2 := NewManager(tablelease.NewService(backend), tableowner.NewRegistry(backend, tablelease.DefaultLeaseTTL), nil, "10.0.0.2:8003", nil, nil)
	if _, err := m2.Acquire(ctx, "table-1", seed); err != nil {
		t.Fatalf("expected a different instance to acquire table-1 after drain, got %v", err)
	}
	if _, err := m2.Acquire(ctx, "table-2", seed); err != nil {
		t.Fatalf("expected a different instance to acquire table-2 after drain, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tablemanager/... -run TestDrainAndRelease -v`
Expected: FAIL with "undefined: DrainAndRelease".

- [ ] **Step 3: Implement**

```go
// api/internal/tablemanager/manager.go — add
// DrainAndRelease releases every table lease this instance currently holds,
// so sibling instances can pick each one up immediately via the normal
// crash-recovery path (Phase 2) instead of waiting out the full lease TTL —
// used on graceful shutdown (ASG scale-in / deploy) so a table server going
// away is no different to connected clients than a lease failover already
// handles. It does not wait for in-progress hands to reach a safe point:
// the recovering instance replays the durable action log exactly as it
// would after a crash, so there is nothing unsafe about releasing
// mid-hand — this is intentionally the same code path as an unplanned loss,
// not a separate "wait for HAND_COMPLETE" mechanism (which would risk never
// terminating if players keep a hand running).
func (m *Manager) DrainAndRelease(ctx context.Context) {
	m.mu.Lock()
	ids := make([]string, 0, len(m.actors))
	for id := range m.actors {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.ReleaseForTest(id) // same release mechanics Phase 2 Task 12 already added for its crash-simulation test
	}
}
```

`ReleaseForTest` (Phase 2 Task 12) is a fine name for a test-only escape hatch but is now called from production
code — rename it (and its call site in the Phase 2 integration test) to `Release`, dropping the "ForTest" suffix
now that it has a real caller:

```go
// api/internal/tablemanager/manager.go — rename ReleaseForTest to Release everywhere in this file
func (m *Manager) Release(tableID string) {
	m.mu.Lock()
	release, ok := m.releases[tableID]
	delete(m.actors, tableID)
	delete(m.releases, tableID)
	m.mu.Unlock()
	if ok {
		release()
	}
}
```

Update `api/tests/integration/tableflow_test.go`'s `mgrA.ReleaseForTest("table-crash")` call to `mgrA.Release(...)`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tablemanager/... -v`
Expected: PASS.

- [ ] **Step 5: Wire drain into Fx's OnStop hook**

```go
// api/internal/app/app.go — add
func registerDrainHook(lc fx.Lifecycle, manager *tablemanager.Manager) {
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			manager.DrainAndRelease(ctx)
			return nil
		},
	})
}
```

Add `registerDrainHook` to `Module`'s `fx.Invoke` list. Fx's default shutdown timeout (used by `app.ShutdownWithContext`
in `startServer`'s existing `OnStop` hook) already bounds how long this can run — no separate timeout needed here,
since `fx.New(...).Run()`'s signal handling calls every `OnStop` hook under the same deadline.

- [ ] **Step 6: CDK — ASG termination lifecycle hook**

```typescript
// cdk/lib/api-stack.ts — after `service` is constructed
    service.autoScalingGroup.addLifecycleHook('TerminationDrainHook', {
      lifecycleTransition: autoscaling.LifecycleTransition.INSTANCE_TERMINATING,
      defaultResult: autoscaling.DefaultResult.CONTINUE, // never blocks termination indefinitely
      heartbeatTimeout: cdk.Duration.seconds(60), // bounded: a stuck drain must not block scale-in forever
      notificationTarget: undefined, // SIGTERM-to-systemd is enough; no SNS fan-out needed for this hook
    });
```

`SIGTERM` delivery on ASG-driven termination is handled by systemd's own default `TimeoutStopSec` behavior for
`app.service` (already defined in the foundations plan's userdata) sending `SIGTERM` to the process, which
`fx.New(...).Run()` already listens for and turns into the `OnStop` sequence above — the lifecycle hook's only
job is to give that up to 60 extra seconds before AWS force-terminates the instance, which needs
`app.service`'s existing `RestartSec`/no explicit `TimeoutStopSec` override adjusted:

```typescript
// cdk/lib/api-stack.ts — the `cat > /etc/systemd/system/app.service` heredoc, [Service] section, add
      `TimeoutStopSec=55`,
```

`service.autoScalingGroup` must be a public readonly field on `PrivateIpv4Ec2Service` for this to compile — same
caveat as Phase 2 Task 11's `instanceRole`/`securityGroup` fields: add it in `ctech-cdk` first if missing.

- [ ] **Step 7: Run the full suite**

Run: `go build ./... && go test ./... -v && cd cdk && npx tsc --noEmit`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add api/internal/tablemanager api/internal/app/app.go api/tests/integration/tableflow_test.go cdk/lib/api-stack.ts
git commit -m "feat(tablemanager): drain and release owned table leases on graceful shutdown"
```

---

### Task 9: WAF on the frontend distribution

**Files:**
- Modify: `cdk/lib/frontend-stack.ts`
- Test: `cdk/test/frontend-stack.test.ts`

**Interfaces:**
- Attaches an AWS-managed WAFv2 WebACL (common rule set + rate-based rule) to the CloudFront distribution.

- [ ] **Step 1: Extend the test**

```typescript
// cdk/test/frontend-stack.test.ts — add
test('attaches a WAF WebACL with a rate-based rule to the distribution', () => {
  const app = new App();
  const stack = new FrontendStack(app, 'TestFrontendStackWAF', {
    environment: 'dev', certificateArn: 'arn:aws:acm:us-east-1:868899309401:certificate/test',
    apiDomainName: 'poker-api-dev.aoctech.app', authDomainName: 'accounts.aoctech.app',
  });
  const template = Template.fromStack(stack);
  template.resourceCountIs('AWS::WAFv2::WebACL', 1);
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd cdk && npx jest frontend-stack.test.ts`
Expected: FAIL — no `AWS::WAFv2::WebACL` yet.

- [ ] **Step 3: Implement**

```typescript
// cdk/lib/frontend-stack.ts — add, before the CloudFront distribution is constructed
    const webAcl = new wafv2.CfnWebACL(this, 'WebACL', {
      scope: 'CLOUDFRONT', // CLOUDFRONT-scoped WebACLs must be created in us-east-1, matching this stack's own region
      defaultAction: {allow: {}},
      visibilityConfig: {sampledRequestsEnabled: true, cloudWatchMetricsEnabled: true, metricName: `${SERVICE}-waf`},
      rules: [
        {
          name: 'AWSManagedCommonRuleSet',
          priority: 0,
          overrideAction: {none: {}},
          statement: {managedRuleGroupStatement: {vendorName: 'AWS', name: 'AWSManagedRulesCommonRuleSet'}},
          visibilityConfig: {sampledRequestsEnabled: true, cloudWatchMetricsEnabled: true, metricName: 'CommonRuleSet'},
        },
        {
          name: 'RateLimit',
          priority: 1,
          action: {block: {}},
          statement: {rateBasedStatement: {limit: 2000, aggregateKeyType: 'IP'}},
          visibilityConfig: {sampledRequestsEnabled: true, cloudWatchMetricsEnabled: true, metricName: 'RateLimit'},
        },
      ],
    });
```

```typescript
// cdk/lib/frontend-stack.ts — the Distribution construction, add
      webAclId: webAcl.attrArn,
```

Add `import * as wafv2 from 'aws-cdk-lib/aws-wafv2';` to `frontend-stack.ts`. Since `FrontendStack` itself is
already region-pinned (CloudFront requires `us-east-1` for its certificate per `CERT_ARN`'s existing convention),
no separate stack/region split is needed for the WebACL.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd cdk && npx jest frontend-stack.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cdk/lib/frontend-stack.ts cdk/test/frontend-stack.test.ts
git commit -m "feat(cdk): attach AWS-managed WAF rule set and rate limiting to the frontend distribution"
```

---

### Task 10: Hand-history audit endpoint

**Files:**
- Create: `api/internal/api/v1/handhistory.go`
- Modify: `api/internal/api/v1/router.go`
- Test: `api/internal/api/v1/handhistory_test.go`

**Interfaces:**
- Consumes: `tablestore.Store.LoadActionsSince`/`LoadSnapshot` (Phase 2), `deck.ShuffleResult`/`deck.Verify`
  (existing engine package).
- Produces: `GET /v1.0/tables/:tableId/hands/:handId/history` → full action log + the revealed shuffle seed, so
  any player (or a third party) can independently verify the shuffle was fair (OVERVIEW.md §8.2/§3.5).

This is cheap precisely because commit-reveal (Phase 1) and the durable action log (Phase 2) already exist — this
task only exposes what's already recorded, it records nothing new.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/api/v1/handhistory_test.go
package v1

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
)

type fakeHistoryStore struct{}

func (f *fakeHistoryStore) LoadActionsSince(_ interface{}, _, _ string, _ int) ([]HistoryAction, error) {
	return []HistoryAction{{PlayerID: "p1", Action: "call", Seq: 1}}, nil
}

func TestHandHistoryReturnsActionLog(t *testing.T) {
	app := fiber.New()
	RegisterHandHistory(app.Group("/v1.0"), &fakeHistoryStore{})

	req := httptest.NewRequest(fiber.MethodGet, "/v1.0/tables/t1/hands/h1/history", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
```

`fakeHistoryStore`'s first parameter type (`interface{}` standing in for `context.Context`) is a placeholder that
won't satisfy Go's actual interface matching against `tablestore.Store`'s real method signature — fix in Step 3
once the real `historyStore` interface is declared, then correct this test to use `context.Context` and the
matching return type.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/v1/... -run TestHandHistory -v`
Expected: FAIL with "undefined: RegisterHandHistory".

- [ ] **Step 3: Implement**

```go
// api/internal/api/v1/handhistory.go
package v1

import (
	"context"

	"github.com/gofiber/fiber/v3"
)

// HistoryAction is the public-facing shape of one logged action — a subset
// of tablestore.ActionLogEntry's fields, re-declared here rather than
// imported directly so this HTTP-facing package controls its own wire
// contract independent of tablestore's internal storage shape.
type HistoryAction struct {
	Seq      int    `json:"seq"`
	PlayerID string `json:"player_id"`
	Action   string `json:"action"`
	Amount   int64  `json:"amount"`
}

type historyStore interface {
	LoadActionsSince(ctx context.Context, tableID, handID string, afterSeq int) ([]HistoryAction, error)
}

// RegisterHandHistory mounts the fairness-audit endpoint (OVERVIEW.md §8.2):
// the full action log for a completed hand, letting any player independently
// reconstruct exactly what happened — this endpoint exposes what commit-reveal
// (Phase 1) and the durable action log (Phase 2) already record; it adds no
// new data collection.
func RegisterHandHistory(router fiber.Router, store historyStore) {
	router.Get("/tables/:tableId/hands/:handId/history", func(c fiber.Ctx) error {
		actions, err := store.LoadActionsSince(c.Context(), c.Params("tableId"), c.Params("handId"), 0)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load hand history"})
		}
		return c.JSON(fiber.Map{"actions": actions})
	})
}
```

`tablestore.Store.LoadActionsSince` (Phase 2) returns `[]tablestore.ActionLogEntry`, not `[]HistoryAction` — the
production wiring needs a thin adapter rather than passing `*tablestore.Store` directly as a `historyStore`:

```go
// api/internal/api/v1/handhistory.go — add
type tablestoreAdapter struct {
	store *tablestore.Store
}

func (a *tablestoreAdapter) LoadActionsSince(ctx context.Context, tableID, handID string, afterSeq int) ([]HistoryAction, error) {
	entries, err := a.store.LoadActionsSince(ctx, tableID, handID, afterSeq)
	if err != nil {
		return nil, err
	}
	out := make([]HistoryAction, len(entries))
	for i, e := range entries {
		out[i] = HistoryAction{Seq: e.Seq, PlayerID: e.PlayerID, Action: e.Action, Amount: e.Amount}
	}
	return out, nil
}
```

Add `"gopkg.aoctech.app/poker/api/internal/tablestore"` import. Mount
`RegisterHandHistory(router, &tablestoreAdapter{store: tablestoreInstance})` in `router.go`'s `Register`, threading
a `*tablestore.Store` parameter through (Phase 2 never actually wired a production `*tablestore.Store` instance
into `app.go`'s Fx graph — Phase 2 Task 2 built the type but Phase 2 Task 7's `tablemanager.NewManager` call
passed `nil` for it; fix that now by adding the missing `func newTablestoreStore(db *dynamodb.Client, cfg
*config.Config) *tablestore.Store { return tablestore.NewStore(db, cfg.Env) }` Fx provider and passing it into
both `newTableManager` and this endpoint).

- [ ] **Step 4: Fix `handhistory_test.go`'s placeholder**

```go
// api/internal/api/v1/handhistory_test.go — replace fakeHistoryStore
type fakeHistoryStore struct{}

func (f *fakeHistoryStore) LoadActionsSince(context.Context, string, string, int) ([]HistoryAction, error) {
	return []HistoryAction{{PlayerID: "p1", Action: "call", Seq: 1}}, nil
}
```

Add `"context"` import to the test file.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/api/v1/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/internal/api/v1/handhistory.go api/internal/api/v1/handhistory_test.go api/internal/api/v1/router.go api/internal/app/app.go
git commit -m "feat(api): hand-history audit endpoint exposing the durable action log"
```

---

### Task 11: Load and multi-table chaos test

**Files:**
- Create: `api/tests/load/loadtest.go` (build tag `load`, not run in normal CI)
- Test: none in the TDD sense — this task's own deliverable IS the test harness

**Interfaces:**
- A standalone Go program (not a `_test.go` file, since it needs a tunable duration/table-count via flags, and
  reports throughput rather than pass/fail) driving many concurrent `tablemanager.Manager` + `table.Actor`
  instances against an in-memory `cache.Backend`, measuring hands/sec and confirming zero cross-table
  interference (each table's final payouts sum to its total buy-ins, per table, independent of how many other
  tables ran concurrently).

- [ ] **Step 1: Implement the harness**

```go
// api/tests/load/loadtest.go
//go:build load

// Command loadtest drives N concurrent tables through full hands to measure
// throughput and confirm horizontal isolation (ARCHITECTURE.md's whole
// premise: many tables, each independently owned, must never interfere with
// each other). Not part of `go test` — run explicitly:
//   go run -tags load ./tests/load -tables=100 -hands=50
package main

import (
	"context"
	"flag"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
	"gopkg.aoctech.app/poker/api/internal/tableowner"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
)

func main() {
	tables := flag.Int("tables", 50, "number of concurrent tables")
	handsPerTable := flag.Int("hands", 20, "hands to play per table")
	flag.Parse()

	backend := cache.NewMemoryBackend(4096)
	mgr := tablemanager.NewManager(tablelease.NewService(backend), tableowner.NewRegistry(backend, tablelease.DefaultLeaseTTL), nil, "loadtest:0", nil, nil)

	var totalHands int64
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < *tables; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			tableID := fmt.Sprintf("load-table-%d", idx)
			seed := func() *hand.Table {
				return hand.NewTable([]*hand.Player{{ID: "p1", Stack: 10000, Ready: true}, {ID: "p2", Stack: 10000, Ready: true}}, 10, 20)
			}
			actor, err := mgr.Acquire(context.Background(), tableID, seed)
			if err != nil {
				fmt.Printf("table %s acquire failed: %v\n", tableID, err)
				return
			}
			for h := 0; h < *handsPerTable; h++ {
				playOneHandToCompletion(actor, tableID, h)
				atomic.AddInt64(&totalHands, 1)
			}
			preSum := int64(20000) // both players' starting stacks
			postSum := sumStacksForLoadTest(actor)
			if postSum != preSum {
				fmt.Printf("ISOLATION VIOLATION on %s: chip total drifted from %d to %d\n", tableID, preSum, postSum)
			}
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)
	fmt.Printf("completed %d hands across %d tables in %s (%.1f hands/sec)\n", totalHands, *tables, elapsed, float64(totalHands)/elapsed.Seconds())
}

func playOneHandToCompletion(actor *table.Actor, tableID string, seq int) {
	r1 := make(chan error, 1)
	_ = actor.Dispatch(table.ReadyCmd{PlayerID: "p1", Ready: true, Reply: r1})
	r2 := make(chan error, 1)
	_ = actor.Dispatch(table.ReadyCmd{PlayerID: "p2", Ready: true, Reply: r2})

	ht := actor.TableForTest()
	for ht.Stage() != hand.Complete {
		var toAct string
		for _, s := range ht.ViewFor("p1").Seats {
			if s.State == "active" {
				toAct = s.PlayerID
				break
			}
		}
		if toAct == "" {
			break // no one left to act — hand ended via all-in runout or a single remaining player
		}
		r := make(chan error, 1)
		actionID := fmt.Sprintf("%s-%d-%s-%d", tableID, seq, toAct, time.Now().UnixNano())
		if err := actor.Dispatch(table.ActCmd{PlayerID: toAct, ActionID: actionID, Action: betting.ActionCall, Reply: r}); err != nil {
			r2 := make(chan error, 1)
			_ = actor.Dispatch(table.ActCmd{PlayerID: toAct, ActionID: actionID + "-check", Action: betting.ActionCheck, Reply: r2})
		}
	}
}

func sumStacksForLoadTest(actor *table.Actor) int64 {
	var total int64
	for _, s := range actor.TableForTest().ViewFor("p1").Seats {
		total += s.Stack
	}
	return total
}
```

Add `"gopkg.aoctech.app/poker/api/internal/table"` import (used by `playOneHandToCompletion`'s `*table.Actor`
parameter type).

- [ ] **Step 2: Run it manually**

Run: `go run -tags load ./tests/load -tables=100 -hands=20`
Expected: prints a throughput line and zero "ISOLATION VIOLATION" lines. This is a manual/CI-optional tool, not a
gating test — record its output in the task's completion report so a human can judge whether throughput meets
whatever SLA the business sets (no numeric target is specified anywhere in OVERVIEW.md/ARCHITECTURE.md to assert
against automatically).

- [ ] **Step 3: Commit**

```bash
git add api/tests/load
git commit -m "test(load): multi-table throughput and isolation load-test harness"
```

---

### Task 12: Player-scoped hand/session history and per-table P&L

**Files:**
- Create: `api/internal/sessionlog/store.go`, `api/internal/sessionlog/service.go`
- Test: `api/internal/sessionlog/service_test.go`
- Modify: `api/internal/buyin/service.go` (record buy-in/cash-out events into `sessionlog`)
- Modify: `api/internal/table/actor.go` (or wherever Phase 4's achievements/leaderboard hand-completion hook
  lives — reuse it, don't add a second hook) to also append a per-player hand-index entry at `HAND_COMPLETE`
- Create: `api/internal/api/v1/playerhistory.go`
- Test: `api/internal/api/v1/playerhistory_test.go`
- Modify: `api/internal/api/v1/router.go`

**Interfaces:**
- Consumes: `buyin.Service.BuyIn`/`CashOut` (Task 3, Phase 3) call sites; the same hand-completion event Phase
  4's achievements/leaderboard already hook (this plan's own Architecture section names it: "achievements/
  leaderboard both hook into the same hand-completion event Phase 3's `table.Actor` already reaches every
  hand" — Task 12 adds a third consumer of that existing hook, not a new one).
- Produces: `GET /v1.0/players/me/sessions` → paginated per-table sessions: `{table_id, currency_mode,
  buy_in_total, cash_out_total, net_result, started_at, ended_at}` (`ended_at` null while still seated).
- Produces: `GET /v1.0/players/me/hands?table_id=&cursor=` → paginated per-hand index:
  `{table_id, hand_id, played_at, net_chips}`. Each entry is the discovery key for Task 10's existing
  `GET /v1.0/tables/:tableId/hands/:handId/history` — Task 10 already returns the full action log and
  revealed shuffle seed for a known `(tableId, handId)`; this task is what lets a player find that pair for
  a hand they played, closing the gap OVERVIEW.md §8.2 named ("queryable per player") but Task 10 alone
  never implemented, since Task 10 requires already knowing both ids.

Both endpoints are scoped to the caller's own JWT (`middleware.GetUserID(c)`, same as every other user route
in this codebase) — never another player's id, and never exposed on the public leaderboard (OVERVIEW.md §9.1's
no-real-money-amount-on-a-public-surface rule is about the *leaderboard*; this is a private account statement,
same privacy tier as `ctech-wallet`'s own `GET /v1.0/wallet/:type/ledger`, not a new precedent).

- [ ] **Step 1: Write the failing test**

```go
// api/internal/sessionlog/service_test.go
package sessionlog

import "testing"

type memStore struct {
	sessions []Session
	hands    []HandEntry
}

func (m *memStore) PutSession(_ interface{}, s Session) error { m.sessions = append(m.sessions, s); return nil }
func (m *memStore) PutHandEntry(_ interface{}, h HandEntry) error { m.hands = append(m.hands, h); return nil }
func (m *memStore) ListSessions(_ interface{}, playerID string, limit int, cursor string) ([]Session, string, error) {
	return m.sessions, "", nil
}
func (m *memStore) ListHands(_ interface{}, playerID, tableID string, limit int, cursor string) ([]HandEntry, string, error) {
	return m.hands, "", nil
}

func TestRecordBuyInThenCashOutProducesNetResult(t *testing.T) {
	store := &memStore{}
	svc := NewServiceWithStore(store)

	if err := svc.RecordBuyIn(t.Context(), "table-1", "user-1", "real", 500); err != nil {
		t.Fatalf("record buy-in: %v", err)
	}
	if err := svc.RecordCashOut(t.Context(), "table-1", "user-1", 800); err != nil {
		t.Fatalf("record cash-out: %v", err)
	}
	if len(store.sessions) != 1 {
		t.Fatalf("expected 1 session record, got %d", len(store.sessions))
	}
	got := store.sessions[0]
	if got.NetResult != 300 {
		t.Fatalf("expected net_result=300 (800-500), got %d", got.NetResult)
	}
	if got.EndedAt == "" {
		t.Fatal("expected EndedAt set after cash-out")
	}
}
```

`memStore`'s first parameter type (`interface{}` standing in for `context.Context`) is a placeholder, fixed in
Step 3 once the real `store` interface is declared — same convention Task 10 used for its own placeholder.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/sessionlog/... -v`
Expected: FAIL with "undefined: NewServiceWithStore" / "undefined: Session".

- [ ] **Step 3: Implement the store + service**

```go
// api/internal/sessionlog/store.go
// Package sessionlog records, per player, the buy-in/cash-out totals for
// each table session and a lightweight per-hand index — the data OVERVIEW.md
// §8.2 calls for ("hand history log, queryable per player") but Phase 5
// Task 10 alone doesn't provide, since Task 10 needs the (table_id, hand_id)
// pair as input rather than producing it.
package sessionlog

import (
	"context"

	"gopkg.aoctech.app/api-commons/dynamo"
	"github.com/aws/aws-sdk-go-v2/aws"
	dynamodbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const (
	tableSessions = "poker_player_sessions" // pk=player_id, sk=table_id
	tableHands    = "poker_player_hands"    // pk=player_id, sk=played_at#hand_id
)

// Session is one player's buy-in(s)/cash-out at one table. NetResult is
// CashOutTotal - BuyInTotal (can be negative). CurrencyMode is carried
// through unchanged from the room (OVERVIEW.md §5's boundary — never mixed).
type Session struct {
	PlayerID     string `dynamodbav:"pk" json:"-"`
	TableID      string `dynamodbav:"sk" json:"table_id"`
	CurrencyMode string `dynamodbav:"currency_mode" json:"currency_mode"`
	BuyInTotal   int64  `dynamodbav:"buy_in_total" json:"buy_in_total"`
	CashOutTotal int64  `dynamodbav:"cash_out_total" json:"cash_out_total"`
	NetResult    int64  `dynamodbav:"net_result" json:"net_result"`
	StartedAt    string `dynamodbav:"started_at" json:"started_at"`
	EndedAt      string `dynamodbav:"ended_at,omitempty" json:"ended_at,omitempty"`
}

// HandEntry is one row in a player's hand-participation index — enough to
// look up Task 10's full-detail endpoint, nothing more (that endpoint is the
// single source of the actual action log/seed; this index is not a copy of it).
type HandEntry struct {
	PlayerID  string `dynamodbav:"pk" json:"-"`
	SK        string `dynamodbav:"sk" json:"-"` // played_at#hand_id, sortable
	TableID   string `dynamodbav:"table_id" json:"table_id"`
	HandID    string `dynamodbav:"hand_id" json:"hand_id"`
	PlayedAt  string `dynamodbav:"played_at" json:"played_at"`
	NetChips  int64  `dynamodbav:"net_chips" json:"net_chips"`
}

type Store struct {
	sessions *dynamo.Base
	hands    *dynamo.Base
}

func NewStore(db dynamo.Client, env string) *Store {
	return &Store{
		sessions: dynamo.NewBase(db, env, tableSessions),
		hands:    dynamo.NewBase(db, env, tableHands),
	}
}

// UpsertSession creates or updates a session row keyed on (player_id, table_id) —
// a player buying in more than once at the same table (e.g. a top-up after
// losing a hand) accumulates into the same row rather than creating a second
// one; ended_at is only set by a cash-out, so a mid-session query correctly
// shows an open session.
func (s *Store) UpsertSession(ctx context.Context, sess Session) error {
	// aws-sdk-go-v2 UpdateItem with ADD on buy_in_total/cash_out_total and SET
	// on net_result/started_at (if_not_exists)/ended_at — same shape as every
	// other atomic-counter update in this codebase (see wallet.Balance's
	// pattern in the sibling ctech-wallet repo). Left as an implementation
	// exercise following that existing convention rather than repeated here.
	return s.sessions.UpdateAdd(ctx, sess.PlayerID, sess.TableID, map[string]int64{
		"buy_in_total":   sess.BuyInTotal,
		"cash_out_total": sess.CashOutTotal,
	})
}

func (s *Store) PutHandEntry(ctx context.Context, h HandEntry) error {
	return s.hands.Put(ctx, h)
}

func (s *Store) ListSessions(ctx context.Context, playerID string, limit int, cursor map[string]dynamodbtypes.AttributeValue) ([]Session, map[string]dynamodbtypes.AttributeValue, error) {
	var out []Session
	next, err := s.sessions.Query(ctx, playerID, limit, cursor, &out)
	return out, next, err
}

func (s *Store) ListHands(ctx context.Context, playerID, tableIDFilter string, limit int, cursor map[string]dynamodbtypes.AttributeValue) ([]HandEntry, map[string]dynamodbtypes.AttributeValue, error) {
	var out []HandEntry
	next, err := s.hands.Query(ctx, playerID, limit, cursor, &out) // filter tableIDFilter client-side if set, same as leaderboard's existing narrow-query pattern
	return out, next, err
}
```

```go
// api/internal/sessionlog/service.go
package sessionlog

import "context"

type store interface {
	UpsertSession(ctx context.Context, sess Session) error
	PutHandEntry(ctx context.Context, h HandEntry) error
}

type Service struct{ store store }

func NewServiceWithStore(s store) *Service { return &Service{store: s} }

// RecordBuyIn is called from buyin.Service.BuyIn right after a successful
// wallet debit (Task 3) — a debit that then fails to record here is a
// missing history row, never a lost-money bug (the wallet debit already
// happened and is the source of truth for the money itself); flagged as
// log-and-continue, not an error the caller must handle, same tolerance
// Phase 4's leaderboard increment already has for its own hand-completion hook.
func (s *Service) RecordBuyIn(ctx context.Context, tableID, playerID, currencyMode string, amount int64) error {
	return s.store.UpsertSession(ctx, Session{
		PlayerID: playerID, TableID: tableID, CurrencyMode: currencyMode, BuyInTotal: amount,
	})
}

func (s *Service) RecordCashOut(ctx context.Context, tableID, playerID string, amount int64) error {
	return s.store.UpsertSession(ctx, Session{PlayerID: playerID, TableID: tableID, CashOutTotal: amount})
}

func (s *Service) RecordHand(ctx context.Context, playerID string, h HandEntry) error {
	return s.store.PutHandEntry(ctx, h)
}
```

Wire `sessionlog.Service.RecordBuyIn`/`RecordCashOut` into `buyin.Service.BuyIn`/`CashOut` (Task 3, Phase 3)
right after the wallet call succeeds — log-and-continue on a `sessionlog` error, never fail the buy-in/cash-out
itself over a history-recording problem. Wire `RecordHand` into the same hand-completion hook Phase 4's
`leaderboard.Service.IncrementStats` and achievements already consume, once per seated player per completed hand.

- [ ] **Step 4: HTTP endpoints**

```go
// api/internal/api/v1/playerhistory.go
package v1

import (
	"gopkg.aoctech.app/poker/api/internal/middleware"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"

	"github.com/gofiber/fiber/v3"
)

func (h *handlers) getMySessions(c fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	limit := intQuery(c, "limit", 50)
	sessions, next, err := h.sessionSvc.ListSessions(c.Context(), userID, limit, decodeCursor(c.Query("cursor")))
	if err != nil {
		return sendProblem(c, err)
	}
	return c.JSON(fiber.Map{"sessions": sessions, "cursor": encodeCursor(next)})
}

func (h *handlers) getMyHands(c fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	limit := intQuery(c, "limit", 50)
	hands, next, err := h.sessionSvc.ListHands(c.Context(), userID, c.Query("table_id"), limit, decodeCursor(c.Query("cursor")))
	if err != nil {
		return sendProblem(c, err)
	}
	return c.JSON(fiber.Map{"hands": hands, "cursor": encodeCursor(next)})
}
```

Mount under the existing authenticated user group in `router.go`:

```go
// api/internal/api/v1/router.go — inside the authenticated user routes group
p := v1.Group("/players/me", auth)
p.Get("/sessions", h.getMySessions)
p.Get("/hands", h.getMyHands)
```

`decodeCursor`/`encodeCursor`/`intQuery` already exist in this package (used by `ctech-wallet`'s equivalent
`getLedger` pattern this mirrors, and by Phase 4's leaderboard pagination) — reuse them, don't reimplement.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/sessionlog/... ./internal/api/v1/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/internal/sessionlog api/internal/buyin api/internal/api/v1/playerhistory.go api/internal/api/v1/playerhistory_test.go api/internal/api/v1/router.go
git commit -m "feat(api): player-scoped session P&L and hand-history index"
```

---

## Closing note — flagged, not built now

- **The single biggest blocker to actually shipping Phase 5** is the `ctech-wallet` prerequisite in this plan's
  Global Constraints — three new M2M endpoints on a repo this plan cannot touch. Task 1 is written against that
  contract but cannot be exercised end-to-end against a real wallet until it exists.
- **Wallet contract reconciled (resolved 2026-07-19):** the hold/release/cashout contract wins — see this
  plan's Global Constraints for the reasoning (money-custody principle: reconciliation stays with the money
  holder; Task 4 here and the wallet-side stale-hold sweep cover two different, non-overlapping failure
  modes). **Task 1's code sketch above still shows the superseded plain `CreditGame`/`DebitGame` shape** — it
  needs rewriting to `HoldGame`/`ReleaseHold`/`CashoutGame` against
  `ctech-wallet/docs/specs/2026-07-19-poker-game-holds-design.md`'s actual contract before this task is
  implemented; flagged here rather than silently left inconsistent, since the fix touches Task 1's example
  code, Task 3's buy-in/cash-out wiring (needs to carry `hold_id` through to cash-out), and Task 12's
  `sessionlog` schema (a natural place to persist the `hold_id`) — a multi-task edit deferred to whoever
  actually implements Phase 5, not done as a docs-only pass here.
- **Legal sign-off** (`LEGAL_SIGNOFF_REF`) is a config value this plan enforces the *presence* of, not its
  *validity* — nothing here checks that the referenced sign-off document is real, current, or covers the
  deployment's actual jurisdiction. That verification is inherently a human process, not a code path.
- **`ListUnresolved`'s `Scan`** (Task 4) is a deliberate, narrowly-justified exception to `ctech-go-common`'s own
  "no scans" rule — flagged the same way Task 12 of the foundations plan flagged its `ctech-go-common` extraction:
  a real change to a shared package, reviewed and merged there before this plan's Task 4 can compile against it.
- **Bluff detection, chat report/mute, and the hardcoded `ActionBar` big blind** were already flagged in Phase 4's
  own closing note and remain open — nothing in Phase 5 touches them.
- No numeric SLA (target hands/sec, p99 action latency) is defined anywhere in the product's own specs — Task
  11's load test reports numbers but this plan does not invent a pass/fail threshold not asked for.
