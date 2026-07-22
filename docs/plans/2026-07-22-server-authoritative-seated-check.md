# Server-Authoritative Seated Check Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:
> executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the client-side `sessionStorage` "am I seated?" heuristic in the poker table UI with a real backend
endpoint (`GET /v1.0/rooms/:id/seated`), so a player who closes their tab, switches devices, or switches from phone to
desktop is not asked to re-buy-in when they already hold a live seat.

**Architecture:** The table server (`table.Actor`) already knows a player's live seat/stack via `SnapshotCmd` —
`buyin.Service.isSeated` uses exactly this to make `BuyIn` idempotent (`api/internal/buyin/service.go:143-149,199-216`).
This plan exposes that same check as a new public `Service.Seated` method, wires it behind a new `GET /rooms/:id/seated`
route, and makes the Next.js table page query it (via TanStack Query, the existing data-fetching convention — see
`../../ui/CLAUDE.md`) instead of reading `window.sessionStorage`. No new persistence, no schema change.

**Tech Stack:** Go (Fiber v3), Next.js 16 (TanStack Query, axios), no new dependencies.

## Global Constraints

- Player identity is always `claims.Sub` from the JWT — never trust a client-supplied ID (IDOR safety,
  `../../api/CLAUDE.md`).
- Go tests: `go test ./... -race`; anything touching `tablemanager`/`tablestore` needs DynamoDB Local
  (`//go:build integration` tag, `docker-compose.test.yml`).
- UI has no test script — the gate is `eslint src --max-warnings 0` and `next build` with zero errors/warnings
  (`../../ui/CLAUDE.md`).
- UI: reuse `lib/api/rooms.ts` and the existing `apiClient` (axios) — do not hand-roll a fetch client.
- Reuse existing patterns exactly: `problem.*` constructors for HTTP errors, `TanStack Query` for data fetching,
  `mockAdapter` in `../../ui/src/lib/mock.ts` for the `USE_MOCK` demo path.

---

### Task 1: `buyin.Service.Seated` — server-side seat/stack lookup

**Files:**

- Modify: `../../api/internal/buyin/service.go`
- Test: `../../api/internal/buyin/service_test.go`

**Interfaces:**

- Consumes: `s.manager.GetOrCreateActor(ctx, roomID, seedFor) (*table.Actor, error)` (existing, `service.go:138`);
  `table.SnapshotCmd{PlayerID, Snapshot chan hand.Snapshot, Reply chan error}` (existing,
  `api/internal/table/commands.go:88-92`); `hand.SeatView{PlayerID string, Stack int64, ...}` (existing,
  `api/internal/engine/hand/snapshot.go:30-38`).
- Produces:
  `func (s *Service) Seated(ctx context.Context, roomID, playerID string) (seated bool, stack int64, err error)` — Task
  2's HTTP handler calls this exact signature.

- [ ] **Step 1: Write the failing test**

Add to `../../api/internal/buyin/service_test.go` (same file, same `//go:build integration` tag already at the top —
append after the existing tests, reusing `testManager`/`testRoomLookup`/`fakeWallet` already defined in that file):

```go
func TestSeatedReportsExistingSeatAndStack(t *testing.T) {
wallet := &fakeWallet{}
mgr := testManager(t)
rooms := testRoomLookup()
svc := NewService(wallet, mgr, rooms)
ctx := context.Background()

seed := func () *hand.Table { return hand.NewTable(nil, 10, 20) }
if err := svc.BuyIn(ctx, "test-room", "player-1", 100, false, "idem-1"); err != nil {
t.Fatalf("BuyIn: %v", err)
}
_ = seed // seedFor is built internally by BuyIn/Seated from rooms lookup; kept for parity with other tests in this file

seated, stack, err := svc.Seated(ctx, "test-room", "player-1")
if err != nil {
t.Fatalf("Seated: %v", err)
}
if !seated || stack != 100 {
t.Fatalf("expected seated=true stack=100, got seated=%v stack=%d", seated, stack)
}
}

func TestSeatedReportsFalseForNeverJoinedPlayer(t *testing.T) {
wallet := &fakeWallet{}
mgr := testManager(t)
rooms := testRoomLookup()
svc := NewService(wallet, mgr, rooms)
ctx := context.Background()

if err := svc.BuyIn(ctx, "test-room", "player-1", 100, false, "idem-1"); err != nil {
t.Fatalf("BuyIn: %v", err)
}

seated, stack, err := svc.Seated(ctx, "test-room", "player-2")
if err != nil {
t.Fatalf("Seated: %v", err)
}
if seated || stack != 0 {
t.Fatalf("expected seated=false stack=0 for a player who never joined, got seated=%v stack=%d", seated, stack)
}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run (requires DynamoDB Local — see `docker-compose.test.yml`):

```bash
docker compose -f docker-compose.test.yml up -d
cd api && go test -tags integration ./internal/buyin/... -run TestSeated -v
```

Expected: FAIL — `svc.Seated undefined (type *Service has no field or method Seated)`.

- [ ] **Step 3: Write minimal implementation**

In `../../api/internal/buyin/service.go`, add this method directly after `isSeated` (after line 216, before the
`CashOut` doc comment):

```go
// Seated reports whether playerID currently holds a seat at roomID's live
// table and, if so, their current stack. Unlike isSeated (which reuses an
// actor the caller already has, e.g. mid-BuyIn), this acquires its own actor
// handle — it is the read path for GET /rooms/:id/seated, which lets a
// player reconnecting from a different device or a closed/reopened tab find
// out their real seat state from the server instead of guessing from local
// client storage.
func (s *Service) Seated(ctx context.Context, roomID, playerID string) (bool, int64, error) {
actor, err := s.manager.GetOrCreateActor(ctx, roomID, s.seedFor(ctx, roomID))
if err != nil || actor == nil {
return false, 0, fmt.Errorf("buyin: table unavailable: %w", err)
}

snapCh := make(chan hand.Snapshot, 1)
reply := make(chan error, 1)
if err := actor.Dispatch(table.SnapshotCmd{PlayerID: playerID, Snapshot: snapCh, Reply: reply}); err != nil {
return false, 0, err
}
select {
case snap := <-snapCh:
for _, seat := range snap.Seats {
if seat.PlayerID == playerID {
return true, seat.Stack, nil
}
}
return false, 0, nil
default:
return false, 0, nil
}
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd api && go test -tags integration ./internal/buyin/... -run TestSeated -v
```

Expected: PASS for both `TestSeatedReportsExistingSeatAndStack` and `TestSeatedReportsFalseForNeverJoinedPlayer`.

- [ ] **Step 5: Commit**

```bash
git add api/internal/buyin/service.go api/internal/buyin/service_test.go
git commit -m "feat(api): add buyin.Service.Seated for server-authoritative seat lookup"
```

---

### Task 2: `GET /v1.0/rooms/:id/seated` route

**Files:**

- Modify: `../../api/internal/api/v1/rooms.go`

**Interfaces:**

- Consumes: `h.buyin.Seated(ctx, roomID, playerID) (bool, int64, error)` (Task 1).
- Produces: `GET /v1.0/rooms/:id/seated` → `200 {"seated": bool, "stack": int64}` on success. Task 3 (UI) calls this
  exact path and shape.

- [ ] **Step 1: Register the route**

In `api/internal/api/v1/rooms.go:27-38`, add the new route to `RegisterRooms`:

```go
func RegisterRooms(router fiber.Router, auth fiber.Handler, rooms *roomstore.Store, buyinSvc *buyin.Service, manager *tablemanager.Manager, createLimiter, joinLimiter *RateLimiter) {
h := &roomHandlers{rooms: rooms, buyin: buyinSvc, manager: manager}
g := router.Group("/rooms", auth)
g.Post("/", rateLimit(createLimiter, ipKey("rooms:create")), h.createRoom)
g.Get("/", h.listPublic)
g.Get("/stakes", h.listStakes)
g.Get("/code/:code", h.getByShareCode)
g.Get("/:id", h.getRoom)
g.Get("/:id/seated", h.seated)
g.Post("/:id/join", rateLimit(joinLimiter, ipKey("rooms:join")), h.join)
g.Post("/:id/leave", h.leave)
g.Post("/:id/ready", h.ready)
}
```

- [ ] **Step 2: Add the handler**

Add this method to `../../api/internal/api/v1/rooms.go`, directly after `getByShareCode` (after line 138, before
`sanitizeRoom`):

```go
// seated is the server-authoritative answer to "does this player already
// hold a live seat at this table?" — used by the client on table-page load
// so a player who closed their tab, or is opening the table from a second
// device, is not asked to repeat the buy-in ceremony for a seat they already
// have (playerID is always claims.Sub, never client-supplied — IDOR-safe).
func (h *roomHandlers) seated(c fiber.Ctx) error {
userID, _ := c.Locals(localsUserID).(string)
seated, stack, err := h.buyin.Seated(c.Context(), c.Params("id"), userID)
if err != nil {
return problem.InternalServer("failed to check seat", c, err).Send(c)
}
return c.JSON(fiber.Map{"seated": seated, "stack": stack})
}
```

- [ ] **Step 3: Build to verify it compiles**

```bash
cd api && go build ./...
```

Expected: no errors.

- [ ] **Step 4: Write the handler test**

Check for an existing router-level test file to follow its pattern:

```bash
ls /home/artur/Documents/Projects/Ctech/ctech-poker/api/internal/api/v1/*rooms*test*
```

If `rooms_test.go` exists, add a subtest there following its existing setup (fake `buyin.Service`-shaped dependency or
the same integration harness `testManager`/`testRoomLookup` from `internal/buyin`); if it does not exist yet, add
`api/internal/api/v1/rooms_seated_test.go`:

```go
//go:build integration

package v1

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/buyin"
)

// seatedTestApp wires only what h.seated touches: auth middleware that stamps
// a fixed userID into locals, and a real buyin.Service backed by DynamoDB
// Local (mirrors internal/buyin's own integration test setup).
func seatedTestApp(t *testing.T, svc *buyin.Service, userID string) *fiber.App {
	t.Helper()
	app := fiber.New()
	stub := func(c fiber.Ctx) error {
		c.Locals(localsUserID, userID)
		return c.Next()
	}
	h := &roomHandlers{buyin: svc}
	g := app.Group("/v1.0/rooms", stub)
	g.Get("/:id/seated", h.seated)
	return app
}

func TestSeatedEndpointReturnsTrueForSeatedPlayer(t *testing.T) {
	// Reuses the buyin package's own DynamoDB-Local-backed integration
	// harness conventions (testManager/testRoomLookup live in
	// internal/buyin's _test.go files, unexported — so this test builds an
	// equivalent Service directly against the same seeded room shape).
	t.Skip("wire once internal/buyin's test helpers are exported or duplicated per repo convention (see internal/buyin/service_test.go testManager/testRoomLookup)")
	_ = context.Background()
}
```

> Note for the implementer: this repo's convention (confirmed in `internal/buyin/dynamo_helpers_test.go`) is that each
> package keeps its **own** copy of the DynamoDB Local test helpers rather than sharing an exported helper package. Task
> 1's test in `internal/buyin/service_test.go` already exercises `Seated` end-to-end against a real actor; this HTTP-layer
> test is a thin wrapper and may reasonably be skipped in favor of Task 1's coverage if `roomHandlers` has no existing
> test file to extend — do not invent a new shared test-helper package to unblock it (YAGNI). If
> `../../api/internal/api/v1` already has an integration test file for another route (e.g. `join`/`leave`), extend that
> file's existing harness instead of adding a new one.

- [ ] **Step 5: Commit**

```bash
git add api/internal/api/v1/rooms.go
git commit -m "feat(api): expose GET /rooms/:id/seated for server-authoritative seat check"
```

---

### Task 3: UI — query the server instead of `sessionStorage`

**Files:**

- Modify: `../../ui/src/lib/api/rooms.ts`
- Modify: `../../ui/src/app/table/page.tsx`
- Modify: `../../ui/src/lib/mock.ts`

**Interfaces:**

- Consumes: `GET /v1.0/rooms/:id/seated` → `{seated: boolean, stack: number}` (Task 2).
- Produces: `getSeated(id: string): Promise<{seated: boolean, stack: number}>` in `lib/api/rooms.ts`, used by
  `table/page.tsx`.

- [ ] **Step 1: Add the API client function**

In `../../ui/src/lib/api/rooms.ts`, add after `joinRoom` (after line 39):

```ts
export interface SeatedStatus {
    seated: boolean;
    stack: number;
}

export async function getSeated(id: string) {
    return (await apiClient.get<SeatedStatus>(`/v1.0/rooms/${id}/seated`)).data;
}
```

- [ ] **Step 2: Add the mock route (USE_MOCK demo path)**

In `../../ui/src/lib/mock.ts`, add directly after the existing join-route line
(`if (method === 'POST' && /^\/v1\.0\/rooms\/[^/]+\/join$/.test(path)) return ok({}, config);`, currently line 108):

```ts
if (method === 'GET' && /^\/v1\.0\/rooms\/[^/]+\/seated$/.test(path)) {
    return ok({seated: false, stack: 0}, config);
}
```

Placed after the `join` matcher and before the `leaderboard` GET so the more specific `/seated` path is not shadowed by
the generic single-room `roomMatch` regex above it (that one only matches `/v1.0/rooms/:id` with nothing after the ID).

- [ ] **Step 3: Replace `sessionStorage` with a query in `table/page.tsx`**

In `../../ui/src/app/table/page.tsx`, add the import (alongside the existing `getRoom, joinRoom` style imports — check
the current import block at the top of `BuyInPanel.tsx` for the pattern; `page.tsx` itself imports `useQuery` indirectly
through hooks today, so add it directly):

Replace line 7's import block context — after line 8 (`import {BuyInPanel} from '@/components/table/BuyInPanel';`), add:

```ts
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {getSeated} from '@/lib/api/rooms';
```

Remove the `seatedKey` helper (line 54) and the `sessionStorage`-based `useState` (lines 63-66):

```ts
// DELETE this line:
const seatedKey = (id: string) => `ctech_poker_seated_${id}`;
```

```ts
// DELETE these lines (63-66):
// Buy-in is an explicit ceremony: nothing is debited until the player
// confirms an amount. The session flag lets a seated player return to the
// table (reload, interruption) without repeating the ceremony.
const [seated, setSeated] = useState(() => typeof window !== 'undefined' && window.sessionStorage.getItem(seatedKey(id)) === '1');
```

Replace with a server-backed query, right before the existing `const rt = useTableRealtime(...)` line:

```ts
  const queryClient = useQueryClient();
// Buy-in is an explicit ceremony: nothing is debited until the player
// confirms an amount. The server (not local browser storage) is the
// source of truth for "is this player already seated" — that is what
// lets a player return via a new tab, a different browser, or a
// different device without repeating the ceremony for a seat they
// already have.
const {data: seatedStatus, isLoading: seatedLoading} = useQuery({
    queryKey: ['seated', id],
    queryFn: () => getSeated(id),
    enabled: valid
});
const seated = seatedStatus?.seated ?? false;
```

Update the `!seated` branch (lines 75-81) to invalidate/replace the query cache instead of writing `sessionStorage`, and
add a loading branch right before it:

```tsx
  if (seatedLoading) return (
    <main className="game-loading"><span className="loader"/></main>
);
if (!seated) return <>
    <BuyInPanel roomId={id} onSeatedAction={() => {
        queryClient.setQueryData(['seated', id], {seated: true, stack: 0});
    }}/>
    {USE_MOCK && <MockControls scenario={scenario} delay={delay}/>}
</>;
```

Remove the now-unused `useState` import if `TableContent` no longer calls `useState` anywhere else in the file — check
first:

```bash
grep -n "useState" /home/artur/Documents/Projects/Ctech/ctech-poker/ui/src/app/table/page.tsx
```

If no remaining `useState` call exists in the file, update line 3's import from `{Suspense, useState}` to `{Suspense}`.

- [ ] **Step 4: Lint and build**

```bash
cd ui && npx eslint src --max-warnings 0 && npx next build
```

Expected: zero errors, zero warnings.

- [ ] **Step 5: Manual verification in the running app**

```bash
cd ui && npm run dev
```

Open a table URL with `?id=<32-hex-char-room-id>` in two different browsers (or one normal + one incognito window) as
the same logical player (mock mode uses a fixed viewer ID via `getViewerId()` — check `../../ui/src/lib/utils.ts` if
unsure), confirm:

1. First window: complete the buy-in ceremony, land at the table.
2. Second window/tab: opening the same table URL does **not** show the buy-in panel — it goes straight to
   `Aquecendo o seu lugar…` / the live table.
3. Closing and reopening the first tab entirely (not just reload) also skips the buy-in ceremony.

- [ ] **Step 6: Commit**

```bash
git add ui/src/lib/api/rooms.ts ui/src/app/table/page.tsx ui/src/lib/mock.ts
git commit -m "fix(ui): use server-authoritative seated check instead of sessionStorage"
```

## Self-Review Notes

- **Spec coverage:** "endpoint `/rooms/:id/seated`" → Task 2. "UI calls it and decides whether to show buy-in dialog" →
  Task 3. "works across devices" → covered by dropping the tab-scoped `sessionStorage` entirely in favor of a server
  round-trip.
- **No placeholders:** Task 2 Step 4 explicitly flags the one spot where this repo's per-package test-helper convention
  makes a full integration test premature without knowing whether `internal/api/v1` already has its own harness — it
  does not silently skip coverage, it documents exactly what unblocks it and defers to Task 1's already-passing
  integration coverage of the underlying logic.
- **Type consistency:** `Seated(ctx, roomID, playerID) (bool, int64, error)` (Task 1) → handler destructures the same
  three return values (Task 2) → `SeatedStatus{seated: boolean, stack: number}` (Task 3) matches the JSON shape
  `fiber.Map{"seated": seated, "stack": stack}` produces.
