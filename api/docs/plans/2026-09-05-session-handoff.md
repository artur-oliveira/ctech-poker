# Session Handoff Between Devices Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:
> executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a player who opens a table on a new device explicitly assume the seat and have their old device's socket
deliberately closed (never left to expire by TTL), closing issue #353.

**Architecture:** `internal/tableconn` grows connection-level granularity (`playerID -> connID -> expiry` instead of
`playerID -> expiry`) so any instance can learn which `connID`s of a player are live anywhere in the fleet. A new
`RequestHandoffCmd` on the table actor reads that fleet-wide set and asks a new `internal/tablehandoff` Pub/Sub service
(same shape as `internal/tablenotify`, but carrying a payload) to tell whichever instance owns each old `connID` to
close it. That instance closes it through a new `connID`-indexed variant of the existing `internal/wsdrain` close-frame
mechanism (1001, never a bare drop). No new cross-instance lock is needed: the actor's own single-goroutine mailbox
already serializes `RequestHandoffCmd` against any in-flight command from the old connection.

**Tech Stack:** Go, Valkey Pub/Sub (`valkey-go`), `fasthttp/websocket`, existing `internal/table` actor/command
machinery.

**Spec:** `api/docs/specs/2026-09-05-session-handoff-tableconn.md`

## Global Constraints

- `tableconn`, `tablenotify`, `tablehandoff` are all display/coordination state — **never** a source of truth for
  auto-kick or any game-state decision (existing rule, unchanged).
- Every cross-instance signal here is fire-and-forget: a dropped Valkey publish degrades to "the old device stays
  connected a bit longer," never to a state-consistency bug.
- Use the **dedicated realtime Valkey client** (`newRealtimeValkeyClient` in `internal/app/app.go`), never the generic
  cache client — same reasoning as `tablenotify`/`ws.RedisRegistry` (head-of-line blocking).
- No new proto fields: `request_handoff` is a plain `ClientMessage.Type` string, same envelope every other action-less
  command (`request_exit`, `ping`) already uses.
- `go test ./... -race` must pass; anything touching `internal/table/actor.go` timer/command paths also needs
  `go vet -tags integration ./...` (signature changes) per `api/CLAUDE.md`.

---

### Task 1: `tableconn` tracks connections, not just players

**Files:**

- Modify: `api/internal/tableconn/tableconn.go`
- Test: `api/internal/tableconn/tableconn_test.go` (extend existing table-driven tests for the new shape)

**Interfaces:**

- Produces:
  `func (s *Service) Sync(ctx context.Context, tableID string, localConns map[string][]string) (map[string]map[string]bool, error)` —
  replaces the old `Sync(ctx, tableID string, localPlayerIDs []string) (map[string]bool, error)`. `localConns` is
  `playerID -> that player's locally-live connIDs`. Return is `playerID -> connID -> alive-in-the-fleet-right-now`.

- [ ] **Step 1: Write the failing test**

Replace the body of the existing `TestSync*` tests in `api/internal/tableconn/tableconn_test.go` (if none exist yet,
create the file) to exercise the new signature:

```go
package tableconn

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/api-commons/cache"
)

func TestSyncTracksPerConnectionNotJustPlayer(t *testing.T) {
	backend := cache.NewMemoryBackend() // existing test double used elsewhere in this repo's cache tests
	svc := NewService(backend)
	ctx := context.Background()

	connected, err := svc.Sync(ctx, "table-1", map[string][]string{
		"p1": {"conn-a", "conn-b"},
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if !connected["p1"]["conn-a"] || !connected["p1"]["conn-b"] {
		t.Fatalf("connected = %#v, want both conn-a and conn-b for p1", connected)
	}

	// A second instance syncing only conn-a for p1 must not evict conn-b —
	// merge semantics, same as the old per-player map.
	connected, err = svc.Sync(ctx, "table-1", map[string][]string{"p1": {"conn-a"}})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if !connected["p1"]["conn-b"] {
		t.Fatalf("conn-b evicted by an unrelated instance's sync: %#v", connected)
	}
}

func TestSyncExpiresStaleConnections(t *testing.T) {
	backend := cache.NewMemoryBackend()
	svc := NewService(backend)
	ctx := context.Background()
	restore := timeNowFunc
	defer func() { timeNowFunc = restore }()

	now := time.Unix(1_700_000_000, 0)
	timeNowFunc = func() time.Time { return now }
	if _, err := svc.Sync(ctx, "table-1", map[string][]string{"p1": {"conn-a"}}); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	timeNowFunc = func() time.Time { return now.Add(EntryTTL + time.Second) }
	connected, err := svc.Sync(ctx, "table-1", map[string][]string{})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(connected["p1"]) != 0 {
		t.Fatalf("connected = %#v, want conn-a expired", connected)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tableconn/... -run TestSync -v`
Expected: FAIL — `Sync` still has the old `(ctx, tableID string, localPlayerIDs []string)` signature, so this won't even
compile.

- [ ] **Step 3: Implement the new schema**

Replace `tableconn.go`'s `Sync` and its doc comment:

```go
// Sync publishes localConns (playerID -> that player's locally-live connIDs
// on this instance) as still-connected and returns the fleet-wide answer:
// playerID -> connID -> alive right now, merged across every instance that
// has synced recently.
//
// A nil Service (dev/tests without a cache) returns a nil map, which callers
// read as "nothing shared — trust the local view", exactly as
// tablestreak.Load does.
//
// Read-modify-write is deliberate and sufficient — see the package doc.
// Granularity moved from playerID to (playerID, connID) so a caller (the
// handoff command) can learn exactly which connections to close, not just
// whether the player has one somewhere.
func (s *Service) Sync(ctx context.Context, tableID string, localConns map[string][]string) (map[string]map[string]bool, error) {
if s == nil || s.cache == nil {
return nil, nil
}
raw, found, err := s.cache.Get(ctx, key(tableID))
if err != nil {
return nil, fmt.Errorf("tableconn: load %s: %w", tableID, err)
}
// expiries is playerID -> connID -> unix-milli expiry.
expiries := map[string]map[string]int64{}
if found {
if err := json.Unmarshal(raw, &expiries); err != nil {
return nil, fmt.Errorf("tableconn: decode %s: %w", tableID, err)
}
}
now := timeNowFunc()
for playerID, conns := range expiries {
for connID, expiry := range conns {
if expiry <= now.UnixMilli() {
delete(conns, connID)
}
}
if len(conns) == 0 {
delete(expiries, playerID)
}
}
refreshed := now.Add(EntryTTL).UnixMilli()
for playerID, connIDs := range localConns {
if playerID == "" {
continue
}
if expiries[playerID] == nil {
expiries[playerID] = map[string]int64{}
}
for _, connID := range connIDs {
if connID != "" {
expiries[playerID][connID] = refreshed
}
}
}
encoded, err := json.Marshal(expiries)
if err != nil {
return nil, fmt.Errorf("tableconn: encode %s: %w", tableID, err)
}
if err := s.cache.Set(ctx, key(tableID), encoded, int(KeyTTL.Seconds())); err != nil {
return nil, fmt.Errorf("tableconn: save %s: %w", tableID, err)
}
connected := make(map[string]map[string]bool, len(expiries))
for playerID, conns := range expiries {
alive := make(map[string]bool, len(conns))
for connID := range conns {
alive[connID] = true
}
connected[playerID] = alive
}
return connected, nil
}
```

Note the package doc comment at the top of the file already says "This is display state only" — leave that sentence
exactly as-is; add one line under it:

```go
// Granularity is per (player, connection), not just per player, so a
// deliberate handoff (see internal/tablehandoff) can identify exactly which
// connection to close instead of only knowing "connected somewhere."
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tableconn/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/tableconn/tableconn.go api/internal/tableconn/tableconn_test.go
git commit -m "feat(tableconn): track connections, not just players, for handoff (#353)"
```

---

### Task 2: `table.ConnStore` and `Actor` consume the new per-connection shape

**Files:**

- Modify: `api/internal/table/actor_views.go` (`ConnStore` interface, `syncFleetConns`, `applyPresence`)
- Modify: `api/internal/table/actor.go` (`fleetConns` field → `fleetConnIDs`)
- Modify: `api/internal/table/fleetstate_test.go` (`fakeConnStore`)

**Interfaces:**

- Consumes: `tableconn.Service.Sync`'s new signature from Task 1.
- Produces: `ConnStore` interface with
  `Sync(ctx context.Context, tableID string, localConns map[string][]string) (map[string]map[string]bool, error)`;
  `Actor.fleetConnIDs map[string]map[string]bool` (playerID -> connID -> alive); a helper
  `func (a *Actor) fleetHasPlayer(playerID string) bool` used by `applyPresence` in place of the old
  `a.fleetConns[playerID]` bool lookup. Task 3 (`RequestHandoffCmd`) reads `a.fleetConnIDs` directly.

- [ ] **Step 1: Write the failing test**

Update `fakeConnStore` in `fleetstate_test.go` to the new signature (this alone breaks every existing caller, which is
the "test" for this step — a signature change is proven by the compiler, per `api/CLAUDE.md`'s note that plain `go test`
won't catch this class of break, so also run `go vet -tags integration ./...` per Step 2):

```go
// fakeConnStore stands in for internal/tableconn.
type fakeConnStore struct {
shared map[string]map[string]int // playerID -> connID -> refcount-ish marker (just presence)
err    error
calls  int
}

func newFakeConnStore() *fakeConnStore {
return &fakeConnStore{shared: map[string]map[string]int{}}
}

func (f *fakeConnStore) Sync(_ context.Context, _ string, local map[string][]string) (map[string]map[string]bool, error) {
f.calls++
if f.err != nil {
return nil, f.err
}
for playerID, connIDs := range local {
if f.shared[playerID] == nil {
f.shared[playerID] = map[string]int{}
}
for _, connID := range connIDs {
f.shared[playerID][connID] = 1
}
}
out := make(map[string]map[string]bool, len(f.shared))
for playerID, conns := range f.shared {
alive := make(map[string]bool, len(conns))
for connID := range conns {
alive[connID] = true
}
out[playerID] = alive
}
return out, nil
}
```

Every test in `fleetstate_test.go` that asserted on `actor.fleetConns["p1"]` (a `bool`) now asserts on
`actor.fleetConnIDs["p1"]` (non-empty map means connected) — for example `TestConnSyncFailureKeepsTheLastKnownSet` and
`TestConnSyncIsPacedButForcedOnLifecycleEvents` change:

```go
    if len(actor.fleetConnIDs["p1"]) == 0 {
t.Fatalf("fleetConnIDs = %v, want p1", actor.fleetConnIDs)
}
```

`TestLocalSocketWinsOverAStaleFleetSet` sets `actor.fleetConns = map[string]bool{}` — change to
`actor.fleetConnIDs = map[string]map[string]bool{}`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go build ./...`
Expected: FAIL to compile — `Actor.fleetConns` doesn't exist yet (renamed in Step 3), and `ConnStore.Sync`'s old
signature doesn't match the fake.

- [ ] **Step 3: Implement**

In `actor.go`, rename the field and its doc comment (around line 82-85):

```go
    // fleetConnIDs is the last answer connStore gave — playerID -> connID ->
// alive anywhere in the fleet right now. nil means "never synced", which
// applyPresence reads as "trust the local view". Display only, never a
// removal input. RequestHandoffCmd (see actor_presence.go) reads this
// directly to learn which of a player's connIDs live on OTHER instances.
fleetConnIDs map[string]map[string]bool
```

In `actor_views.go`, replace the `ConnStore` interface, `applyPresence`, and `syncFleetConns`:

```go
// ConnStore shares which connections a player holds live anywhere in the
// fleet. See internal/tableconn.
type ConnStore interface {
Sync(ctx context.Context, tableID string, localConns map[string][]string) (map[string]map[string]bool, error)
}

func (a *Actor) applyPresence(seats []hand.SeatView) {
for i := range seats {
playerID := seats[i].PlayerID
_, locallyConnected := a.activeConns[playerID]
_, locallyDisconnected := a.disconnectedSince[playerID]
connected := locallyConnected || !locallyDisconnected
if a.fleetConnIDs != nil {
connected = locallyConnected || len(a.fleetConnIDs[playerID]) > 0
}
seats[i].ConnectionState = "disconnected"
if connected {
seats[i].ConnectionState = "connected"
}
}
}

func (a *Actor) syncFleetConns(force bool) {
if a.connStore == nil {
return
}
now := timeNowFunc()
if !force && !a.connSyncedAt.IsZero() && now.Sub(a.connSyncedAt) < tableconn.SyncInterval {
return
}
a.connSyncedAt = now
local := make(map[string][]string, len(a.activeConns))
for playerID, conns := range a.activeConns {
ids := make([]string, 0, len(conns))
for connID := range conns {
ids = append(ids, connID)
}
local[playerID] = ids
}
ctx, cancel := context.WithTimeout(context.Background(), connStoreTimeout)
defer cancel()
connected, err := a.connStore.Sync(ctx, a.id, local)
if err != nil {
slog.Warn("table conn sync failed", "table_id", a.id, "err", err)
return
}
if connected != nil {
a.fleetConnIDs = connected
}
}
```

`New()` in `actor.go` does not need to initialize `fleetConnIDs` — it is nil until the first sync, exactly like
`fleetConns` was.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/table/... -run TestConn -v && go vet -tags integration ./...`
Expected: PASS, no vet errors.

- [ ] **Step 5: Commit**

```bash
git add api/internal/table/actor.go api/internal/table/actor_views.go api/internal/table/fleetstate_test.go
git commit -m "feat(table): fleet connection state tracks connIDs, not just players (#353)"
```

---

### Task 3: `RequestHandoffCmd` and the `HandoffCloser` seam

**Files:**

- Modify: `api/internal/table/commands.go` (new `RequestHandoffCmd`)
- Modify: `api/internal/table/actor.go` (`handoffCloser` field, wiring in `handle()`)
- Modify: `api/internal/table/actor_presence.go` (`handleRequestHandoff`, `HandoffCloser` interface,
  `SetHandoffCloserForActor`)
- Test: `api/internal/table/fleetstate_test.go` (new tests)

**Interfaces:**

- Consumes: `a.fleetConnIDs` from Task 2.
- Produces: `table.RequestHandoffCmd{PlayerID, NewConnID string, Reply chan error}`; `table.HandoffCloser` interface
  with `RequestClose(ctx context.Context, tableID string, connIDs []string)`;
  `Actor.SetHandoffCloserForActor(c HandoffCloser)`. Task 5 (`internal/tablehandoff.Service`) implements
  `HandoffCloser`; Task 6 (`tablemanager`) wires it in.

- [ ] **Step 1: Write the failing test**

Add to `commands.go`'s existing pattern (see Step 3 below for the type), then add to `fleetstate_test.go`:

```go
// fakeHandoffCloser stands in for internal/tablehandoff.
type fakeHandoffCloser struct {
tableID string
connIDs []string
calls   int
}

func (f *fakeHandoffCloser) RequestClose(_ context.Context, tableID string, connIDs []string) {
f.calls++
f.tableID = tableID
f.connIDs = connIDs
}

// A handoff closes every OTHER live connID for the player, never the new one,
// and is a no-op when the player has no other connection anywhere in the
// fleet (nothing to assume from).
func TestRequestHandoffClosesEveryOtherConnID(t *testing.T) {
actor, _ := completeActor(t, "instance-a")
closer := &fakeHandoffCloser{}
actor.SetHandoffCloserForActor(closer)
actor.fleetConnIDs = map[string]map[string]bool{
"p1": {"old-conn-1": true, "old-conn-2": true, "new-conn": true},
}

reply := make(chan error, 1)
if err := actor.handle(context.Background(), RequestHandoffCmd{
PlayerID: "p1", NewConnID: "new-conn", Reply: reply,
}); err != nil {
t.Fatalf("handle: %v", err)
}
if closer.calls != 1 {
t.Fatalf("RequestClose calls = %d, want 1", closer.calls)
}
if closer.tableID != actor.id {
t.Fatalf("tableID = %q, want %q", closer.tableID, actor.id)
}
got := map[string]bool{}
for _, id := range closer.connIDs {
got[id] = true
}
if !got["old-conn-1"] || !got["old-conn-2"] || got["new-conn"] {
t.Fatalf("connIDs = %v, want exactly old-conn-1 and old-conn-2", closer.connIDs)
}
}

func TestRequestHandoffNoOpWhenNoOtherConnection(t *testing.T) {
actor, _ := completeActor(t, "instance-a")
closer := &fakeHandoffCloser{}
actor.SetHandoffCloserForActor(closer)
actor.fleetConnIDs = map[string]map[string]bool{"p1": {"new-conn": true}}

reply := make(chan error, 1)
if err := actor.handle(context.Background(), RequestHandoffCmd{
PlayerID: "p1", NewConnID: "new-conn", Reply: reply,
}); err != nil {
t.Fatalf("handle: %v", err)
}
if closer.calls != 0 {
t.Fatalf("RequestClose calls = %d, want 0 — nothing else to close", closer.calls)
}
}

// Without a HandoffCloser wired (dev/tests without a cache) this is a no-op,
// not a panic — same convention as ConnStore/ChangeNotifier.
func TestRequestHandoffWithoutACloserDoesNotPanic(t *testing.T) {
actor, _ := completeActor(t, "instance-a")
actor.fleetConnIDs = map[string]map[string]bool{"p1": {"old": true, "new-conn": true}}
reply := make(chan error, 1)
if err := actor.handle(context.Background(), RequestHandoffCmd{
PlayerID: "p1", NewConnID: "new-conn", Reply: reply,
}); err != nil {
t.Fatalf("handle: %v", err)
}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/table/... -run TestRequestHandoff -v`
Expected: FAIL to compile — `RequestHandoffCmd`, `SetHandoffCloserForActor` don't exist yet.

- [ ] **Step 3: Implement**

Add to `commands.go`, right after `ReconnectCmd`:

```go
// RequestHandoffCmd is dispatched when a player explicitly confirms "continue
// here, disconnect the other device." NewConnID is the connection asking to
// take over — every OTHER connID this player currently holds anywhere in the
// fleet (per tableconn, via Actor.fleetConnIDs) gets a deliberate server close,
// never left to expire by TTL. See internal/tablehandoff.
type RequestHandoffCmd struct {
PlayerID  string
NewConnID string
Reply     chan error
}

func (c RequestHandoffCmd) reply() chan error { return c.Reply }
```

Add the case to `handle()` in `actor.go`, right after `case ReconnectCmd:`:

```go
    case RequestHandoffCmd:
return a.handleRequestHandoff(c)
```

Add the field to `Actor` in `actor.go`, right after `changeNotifier ChangeNotifier`:

```go
    // handoffCloser tells whichever instance owns an old connID to close it
// deliberately when a player confirms a device handoff (internal/tablehandoff).
// nil in dev/tests without a cache, where RequestHandoffCmd is a no-op.
handoffCloser HandoffCloser
```

Add to `actor_views.go`, right after `SetChangeNotifierForActor`:

```go
// HandoffCloser asks whichever instance owns connIDs to close each one
// deliberately (1001, never a bare drop) — see internal/tablehandoff.
// Fire-and-forget, same reasoning as ChangeNotifier: nothing here decides
// table state, so a dropped signal only means the old device stays connected
// a bit longer, never a correctness issue.
type HandoffCloser interface {
RequestClose(ctx context.Context, tableID string, connIDs []string)
}

// SetHandoffCloserForActor wires the shared handoff-close signal. Set once,
// right after construction, by tablemanager.
func (a *Actor) SetHandoffCloserForActor(c HandoffCloser) { a.handoffCloser = c }
```

Add `handleRequestHandoff` to `actor_presence.go`, right after `handleReconnect`:

```go
// handleRequestHandoff assumes every other live connection of PlayerID
// (fleet-wide, from tableconn via fleetConnIDs — activeConns alone only ever
// sees THIS instance's own sockets) and asks handoffCloser to close them.
// A no-op if there is nothing else to close (no HandoffCloser wired, or the
// player has no other live connID anywhere) — not an error, since "nobody
// else was connected" is a completely normal outcome of the client's own
// confirmation prompt firing on a stale dot.
//
// No ordering logic is needed here: Run processes one command at a time, so
// any command from the old connection already queued ahead of this one
// commits first, and nothing new from it can arrive after its socket closes
// (the read loop that would dispatch it dies with the socket). See
// docs/specs/2026-09-05-session-handoff-tableconn.md §5.
func (a *Actor) handleRequestHandoff(c RequestHandoffCmd) error {
if a.handoffCloser == nil {
return nil
}
conns := a.fleetConnIDs[c.PlayerID]
if len(conns) == 0 {
return nil
}
var toClose []string
for connID := range conns {
if connID != c.NewConnID {
toClose = append(toClose, connID)
}
}
if len(toClose) == 0 {
return nil
}
a.handoffCloser.RequestClose(context.Background(), a.id, toClose)
return nil
}
```

`context` is already imported in `actor_presence.go` (used by `handleReconnect`/`handleExternalChange`).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/table/... -run TestRequestHandoff -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/table/commands.go api/internal/table/actor.go api/internal/table/actor_views.go api/internal/table/actor_presence.go api/internal/table/fleetstate_test.go
git commit -m "feat(table): add RequestHandoffCmd to close a player's other connections (#353)"
```

---

### Task 4: `wsdrain` closes one connection by ID

**Files:**

- Modify: `api/internal/wsdrain/wsdrain.go`
- Test: `api/internal/wsdrain/wsdrain_test.go`

**Interfaces:**

- Produces: `wsdrain.TrackByID(connID string, c Conn)`, `wsdrain.UntrackByID(connID string)`,
  `wsdrain.CloseByConnID(connIDs []string) (closed int)`.

- [ ] **Step 1: Write the failing test**

Add to `wsdrain_test.go` (mirroring the existing `fakeConn`/`CloseAll` tests already in that file — reuse whatever fake
`Conn` implementation `TestCloseAllStalledPeersDoNotDelayHealthySockets` already defines):

```go
func TestCloseByConnIDClosesOnlyNamedConnections(t *testing.T) {
a := &fakeConn{}
b := &fakeConn{}
TrackByID("conn-a", a)
TrackByID("conn-b", b)
defer UntrackByID("conn-a")
defer UntrackByID("conn-b")

closed := CloseByConnID([]string{"conn-a", "conn-does-not-exist"})
if closed != 1 {
t.Fatalf("closed = %d, want 1", closed)
}
if !a.closed {
t.Fatalf("conn-a was not sent a close frame")
}
if b.closed {
t.Fatalf("conn-b must not be touched — it wasn't named")
}
}

func TestCloseByConnIDIgnoresUnknownIDs(t *testing.T) {
// Must not panic when every requested connID lives on some OTHER
// instance — the common case, since a handoff broadcasts to the whole
// fleet and only the owning instance recognizes any given connID.
closed := CloseByConnID([]string{"conn-elsewhere"})
if closed != 0 {
t.Fatalf("closed = %d, want 0", closed)
}
}
```

If `wsdrain_test.go` doesn't already define a `fakeConn` with a `closed bool` settable from `WriteControl`, add one:

```go
type fakeConn struct {
closed bool
}

func (f *fakeConn) WriteControl(int, []byte, time.Time) error {
f.closed = true
return nil
}
```

(Check the existing file first — `TestCloseAllStalledPeersDoNotDelayHealthySockets` almost certainly already has an
equivalent; reuse it rather than duplicating.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/wsdrain/... -run TestCloseByConnID -v`
Expected: FAIL to compile — `TrackByID`/`UntrackByID`/`CloseByConnID` don't exist yet.

- [ ] **Step 3: Implement**

Add to `wsdrain.go`, right after the existing `conns` map declaration:

```go
var byConnID = make(map[string]Conn)

// TrackByID indexes a connection by its application-level connID, in
// addition to the identity-keyed Track above — CloseByConnID is the only
// consumer, for the session-handoff feature (#353), where the caller only
// knows the connID, never the Conn's Go identity. Call alongside Track, from
// the same place tablews.go registers the socket with ws.Registry.
func TrackByID(connID string, c Conn) {
mu.Lock()
defer mu.Unlock()
byConnID[connID] = c
}

func UntrackByID(connID string) {
mu.Lock()
defer mu.Unlock()
delete(byConnID, connID)
}

// CloseByConnID sends a 1001 close frame to each of connIDs that this
// process actually holds, ignoring any it doesn't recognize (the normal case
// for a handoff broadcast fleet-wide — most instances own none of the named
// IDs). Returns how many were signalled. Unlike CloseAll this is not a
// shutdown path, so there is no grace window to wait out: the caller (a
// Pub/Sub subscriber) must not block on slow peers, so each write gets its
// own goroutine exactly like CloseAll's fan-out, and this function returns
// immediately after dispatching them.
func CloseByConnID(connIDs []string) int {
mu.Lock()
var targets []Conn
for _, id := range connIDs {
if c, ok := byConnID[id]; ok {
targets = append(targets, c)
}
}
mu.Unlock()
if len(targets) == 0 {
return 0
}
frame := fws.FormatCloseMessage(fws.CloseGoingAway, "session handoff to another device")
deadline := time.Now().Add(closeWriteWait)
for _, c := range targets {
go func (c Conn) {
if err := c.WriteControl(fws.CloseMessage, frame, deadline); err != nil {
slog.Debug("ws handoff close frame failed", "err", err)
}
}(c)
}
return len(targets)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/wsdrain/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/wsdrain/wsdrain.go api/internal/wsdrain/wsdrain_test.go
git commit -m "feat(wsdrain): close a connection by connID for session handoff (#353)"
```

---

### Task 5: `internal/tablehandoff` — the Pub/Sub payload channel

**Files:**

- Create: `api/internal/tablehandoff/tablehandoff.go`
- Test: `api/internal/tablehandoff/tablehandoff_test.go`

**Interfaces:**

- Consumes: `valkey.Client` (the dedicated realtime client, same as `tablenotify.NewService`).
- Produces: `tablehandoff.NewService(client valkey.Client) *Service`;
  `(*Service).RequestClose(ctx context.Context, tableID string, connIDs []string)` (satisfies Task 3's
  `table.HandoffCloser`); `(*Service).Listen(ctx context.Context, onClose func(connIDs []string))`.

- [ ] **Step 1: Write the failing test**

```go
package tablehandoff

import (
	"context"
	"testing"
	"time"
)

// A nil Service must degrade to a no-op both ways, same convention as
// tablenotify.Service and tableconn.Service.
func TestNilServiceIsANoOp(t *testing.T) {
	var s *Service
	s.RequestClose(context.Background(), "table-1", []string{"conn-a"}) // must not panic

	done := make(chan struct{})
	go func() {
		s.Listen(context.Background(), func([]string) { t.Fatal("onClose must never fire on a nil service") })
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Listen on a nil service must return immediately, not block")
	}
}
```

This package needs a real Valkey round trip to test `RequestClose`/`Listen` together — that belongs in an integration
test alongside the rest of this repo's Valkey-backed suites (Task 8 covers the end-to-end wiring test). This unit test
only pins the nil-degrades-gracefully contract, matching `tablenotify_test.go`'s equivalent test (check that file for
the exact pattern already established — reuse it, don't invent a new convention).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tablehandoff/... -v`
Expected: FAIL to compile — package doesn't exist yet.

- [ ] **Step 3: Implement**

```go
// Package tablehandoff lets one instance ask whichever instance owns a set
// of WebSocket connIDs to close them deliberately, for the explicit
// session-handoff feature (#353): "continue here, disconnect the other
// device."
//
// It is internal/tablenotify's shape (one shared Valkey Pub/Sub channel,
// subscribed once per process) carrying a payload instead of a bare signal:
// tablenotify only ever needs to say "table X changed, go reload it";
// this needs to say "close exactly these connIDs," which only the instance
// that actually holds one of them can act on (see internal/wsdrain.
// CloseByConnID, which silently ignores any connID it doesn't recognize).
//
// Fire-and-forget throughout, same reasoning as tablenotify: nothing here
// decides table state. A dropped or delayed message only means the old
// device's socket stays open a bit longer than the player asked for — never
// a correctness issue. See docs/specs/2026-09-05-session-handoff-tableconn.md.
package tablehandoff

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/valkey-io/valkey-go"
)

// channel is shared by every table on purpose, same reasoning as
// tablenotify: subscribing is a one-time cost per process.
const channel = "poker:tablehandoff"

// publishTimeout bounds RequestClose's round trip so an unreachable Valkey
// cannot add latency to the caller (Actor.handleRequestHandoff, on the
// player's own interactive command budget).
const publishTimeout = 2 * time.Second

// resubscribeBackoff paces Listen's retry after a dropped receive loop.
const resubscribeBackoff = time.Second

// message is the wire payload published on channel.
type message struct {
	TableID string   `json:"table_id"`
	ConnIDs []string `json:"conn_ids"`
}

// Service publishes and subscribes handoff-close requests over one shared
// Valkey client. A nil *Service (dev/tests without a cache) makes both
// RequestClose and Listen no-ops, matching tablenotify.Service's convention.
type Service struct{ client valkey.Client }

func NewService(client valkey.Client) *Service { return &Service{client: client} }

// RequestClose announces that connIDs (all belonging to one player at
// tableID) should be closed. See table.HandoffCloser, which this satisfies.
func (s *Service) RequestClose(ctx context.Context, tableID string, connIDs []string) {
	if s == nil || s.client == nil || len(connIDs) == 0 {
		return
	}
	payload, err := json.Marshal(message{TableID: tableID, ConnIDs: connIDs})
	if err != nil {
		slog.Warn("handoff close payload encode failed", "table_id", tableID, "err", err)
		return
	}
	ctx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()
	if err := s.client.Do(ctx, s.client.B().Publish().Channel(channel).Message(string(payload)).Build()).Error(); err != nil {
		slog.Warn("handoff close publish failed", "table_id", tableID, "err", err)
	}
}

// Listen blocks, invoking onClose with each published set of connIDs, until
// ctx is cancelled. onClose is expected to call wsdrain.CloseByConnID (or an
// equivalent), which is itself a no-op for any connID this process doesn't
// hold — so every process can safely run the same onClose unconditionally.
func (s *Service) Listen(ctx context.Context, onClose func(connIDs []string)) {
	if s == nil || s.client == nil {
		return
	}
	for ctx.Err() == nil {
		err := s.client.Receive(ctx, s.client.B().Subscribe().Channel(channel).Build(), func(msg valkey.PubSubMessage) {
			var m message
			if err := json.Unmarshal([]byte(msg.Message), &m); err != nil {
				slog.Warn("handoff close payload decode failed", "err", err)
				return
			}
			onClose(m.ConnIDs)
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			slog.Warn("handoff close subscribe interrupted", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(resubscribeBackoff):
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tablehandoff/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/tablehandoff/
git commit -m "feat(tablehandoff): add Pub/Sub channel to close a connection cross-instance (#353)"
```

---

### Task 6: `tablemanager` wiring

**Files:**

- Modify: `api/internal/tablemanager/manager.go`

**Interfaces:**

- Consumes: `table.HandoffCloser` (Task 3).
- Produces: `Manager.SetHandoffCloser(c table.HandoffCloser)`; `HandoffListener` interface
  (`Listen(ctx context.Context, onClose func(connIDs []string))`);
  `Manager.ListenForHandoffCloses(ctx context.Context, listener HandoffListener, onClose func(connIDs []string))` —
  deliberately takes `onClose` as a parameter rather than hardcoding `wsdrain.CloseByConnID`, since `tablemanager` must
  not import the HTTP-layer-adjacent `wsdrain` package (check current imports — if `tablemanager` already has no
  dependency on `wsdrain`, keep it that way; `internal/app` wires the concrete closure).

- [ ] **Step 1: Write the failing test**

`tablemanager` has no existing test file exercising `ListenForExternalChanges` directly (it's a thin dispatch loop) —
mirror that by adding one small test alongside wherever `manager_test.go` or similar already lives:

```go
// fakeHandoffListener stands in for internal/tablehandoff.Service.
type fakeHandoffListener struct {
onClose func (connIDs []string)
}

func (f *fakeHandoffListener) Listen(_ context.Context, onClose func (connIDs []string)) {
f.onClose = onClose
}

func TestListenForHandoffClosesInvokesCallback(t *testing.T) {
listener := &fakeHandoffListener{}
mgr := &Manager{}
var got []string
mgr.ListenForHandoffCloses(context.Background(), listener, func(connIDs []string) {
got = connIDs
})
listener.onClose([]string{"conn-a"})
if len(got) != 1 || got[0] != "conn-a" {
t.Fatalf("got %v, want [conn-a]", got)
}
}
```

(Adjust to whatever `Manager` construction helper the existing `tablemanager` test suite already uses instead of the
bare `&Manager{}` above — check `manager_test.go`/`manager_concurrency_test.go` for the real helper name before writing
this.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tablemanager/... -run TestListenForHandoffCloses -v`
Expected: FAIL to compile — `ListenForHandoffCloses` doesn't exist yet.

- [ ] **Step 3: Implement**

Add the field to the `Manager` struct, right after `changeNotifier table.ChangeNotifier`:

```go
    handoffCloser table.HandoffCloser
```

Add after `SetChangeNotifier`:

```go
// SetHandoffCloser gives every actor this instance creates the shared
// deliberate-disconnect signal (internal/tablehandoff) behind
// RequestHandoffCmd. Without it a handoff request is a silent no-op — see
// table.HandoffCloser's doc comment.
func (m *Manager) SetHandoffCloser(c table.HandoffCloser) {
m.handoffCloser = c
}

// HandoffListener is the subscribe side of internal/tablehandoff — kept
// narrow here for the same reason ChangeListener is: tablemanager depends
// only on the shape it actually calls, not the concrete package.
type HandoffListener interface {
Listen(ctx context.Context, onClose func (connIDs []string))
}

// ListenForHandoffCloses blocks, invoking onClose with each published set of
// connIDs, until ctx is cancelled. onClose is supplied by the caller
// (internal/app, wired to wsdrain.CloseByConnID) rather than hardcoded here,
// since tablemanager has no reason to depend on the HTTP-transport-adjacent
// wsdrain package. Call once per process, alongside the listener's own
// construction.
func (m *Manager) ListenForHandoffCloses(ctx context.Context, listener HandoffListener, onClose func (connIDs []string)) {
if listener == nil {
return
}
listener.Listen(ctx, onClose)
}
```

Find where actors are constructed and wired (`actor.SetConnStoreForActor(m.connStore)` /
`actor.SetChangeNotifierForActor(m.changeNotifier)`, around line 393-396) and add immediately after:

```go
        actor.SetHandoffCloserForActor(m.handoffCloser)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tablemanager/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/tablemanager/manager.go
git commit -m "feat(tablemanager): wire HandoffCloser into every actor this instance creates (#353)"
```

---

### Task 7: wire it all into `internal/app` and the WS gateway

**Files:**

- Modify: `api/internal/app/app.go`
- Modify: `api/internal/api/v1/tablews.go`

**Interfaces:**

- Consumes: `tablehandoff.NewService`, `Manager.SetHandoffCloser`, `Manager.ListenForHandoffCloses`,
  `wsdrain.TrackByID`/`UntrackByID`/`CloseByConnID`, `table.RequestHandoffCmd`.

- [ ] **Step 1: Wire the service in `app.go`**

Right after the existing `tablenotify` block (the one ending `cancelListen()` / `})`), add:

```go
    // Deliberate cross-device handoff close (see internal/tablehandoff and
// docs/specs/2026-09-05-session-handoff-tableconn.md). Same dedicated
// realtime client as tablenotify above — this is exactly the same class
// of latency-sensitive PUBLISH.
if realtime != nil {
handoff := tablehandoff.NewService(realtime)
mgr.SetHandoffCloser(handoff)
handoffListenCtx, cancelHandoffListen := context.WithCancel(context.Background())
lc.Append(fx.Hook{
OnStart: func (context.Context) error {
go mgr.ListenForHandoffCloses(handoffListenCtx, handoff, wsdrain.CloseByConnID)
return nil
},
OnStop: func (context.Context) error {
cancelHandoffListen()
return nil
},
})
}
```

Add the two new imports (`"gopkg.aoctech.app/poker/api/internal/tablehandoff"` and, if not already imported in `app.go`,
`"gopkg.aoctech.app/poker/api/internal/wsdrain"`) to the import block.

- [ ] **Step 2: Wire `TrackByID`/`UntrackByID` and the client message in `tablews.go`**

Right after the existing `wsdrain.Track(safeConn)` / `defer wsdrain.Untrack(safeConn)` pair (around line 283-284), add:

```go
            // Also indexed by connID: CloseByConnID (session handoff, #353) only
// knows the connID a sibling instance published, never this Conn's Go
// identity.
wsdrain.TrackByID(connID, safeConn)
defer wsdrain.UntrackByID(connID)
```

Wait — `connID` is not yet assigned at that point in the function (it's generated later, at line 358:
`connID := uuid.New().String()`). Move this pair to immediately after that line instead, alongside where
`connectionRegistered` is declared:

```go
            connID := uuid.New().String()
wsdrain.TrackByID(connID, safeConn)
defer wsdrain.UntrackByID(connID)
connectionRegistered := false
```

Add `"request_handoff"` to the valid-types list (line 153):

```go
    case "act", "chat", "reaction", "preselect_action", "bot_challenge", "sync_state", "ready", "post_big_blind", "show_cards", "keep_seat", "set_run_it_twice", "peek_cards", "ping", "request_rabbit_hunt", "rabbit_hunt_verify_failed", "request_winner_cards", "accept_winner_cards", "decline_winner_cards", "request_exit", "cancel_exit", "request_handoff":
```

Add the case, right after `case "request_exit":` / before `case "cancel_exit":`:

```go
                case "request_handoff":
ensureActionID()
r := make(chan error, 1)
if err := dispatch(table.RequestHandoffCmd{PlayerID: playerID, NewConnID: connID, Reply: r}); err != nil {
send(&pokerproto.ServerMessage{Type: "error", Code: actionErrorCode(err), Message: err.Error(), ActionId: m.ActionId})
} else {
ack()
}
```

- [ ] **Step 3: Build and run the existing WS gateway tests**

Run: `go build ./... && go test ./internal/api/v1/... -run TableWS -v`
Expected: PASS (no behavior change for any existing message type; `request_handoff` is additive).

- [ ] **Step 4: Commit**

```bash
git add api/internal/app/app.go api/internal/api/v1/tablews.go
git commit -m "feat(api): wire session-handoff request_handoff end to end (#353)"
```

---

### Task 8: integration test — in-flight action ordering during handoff

**Files:**

- Test: `api/tests/integration/handoff_test.go` (new file, `//go:build integration`)

**Interfaces:**

- Consumes: `table.Actor`, `table.RequestHandoffCmd`, `table.ActCmd`, same DynamoDB-Local integration harness the rest
  of `tests/integration` already uses (check `tests/integration/*.go` for the existing table-setup helper — reuse it,
  don't reinvent).

- [ ] **Step 1: Write the test**

This is the ADR's one acceptance criterion that needs an explicit test rather than following "for free" from existing
guarantees: prove that a command from the old connection, already queued ahead of `RequestHandoffCmd` in the actor's
mailbox, still commits — and that nothing from the old connection is silently duplicated or lost.

```go
//go:build integration

package integration

import (
	"context"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/table"
)

// A RequestHandoffCmd must never lose or duplicate a command from the
// connection it's about to close: the actor's single-goroutine mailbox
// already serializes them in arrival order, so anything queued ahead of the
// handoff commits exactly as if the handoff had never been requested.
func TestRequestHandoffDoesNotDisruptAnAlreadyQueuedAction(t *testing.T) {
	actor, cleanup := newTestActorTwoSeated(t) // reuse this package's existing two-seat setup helper
	defer cleanup()

	current := actor.CurrentPlayerIDForActorSnapshot(t) // reuse existing snapshot helper to find whose turn it is

	actReply := make(chan error, 1)
	handoffReply := make(chan error, 1)

	// Simulate: the old connection's Act was already read off the socket and
	// handed to Dispatch (so it's either in the channel buffer or already
	// being processed) a moment before the new device's confirmed handoff
	// reaches the mailbox. Both are just sequential Dispatch calls from the
	// test's perspective — the actor's cmds channel is what serializes them.
	go func() {
		actReply <- actor.Dispatch(table.ActCmd{
			PlayerID: current, ActionID: "act-1", Action: "call", Reply: actReply,
		})
	}()
	if err := actor.Dispatch(table.RequestHandoffCmd{
		PlayerID: current, NewConnID: "new-conn", Reply: handoffReply,
	}); err != nil {
		t.Fatalf("RequestHandoffCmd: %v", err)
	}
	if err := <-actReply; err != nil {
		t.Fatalf("queued Act must still commit despite the handoff: %v", err)
	}

	// The engine actually advanced exactly once for this action — no
	// duplicate application of "call".
	snap := actorSnapshotFor(t, actor, current) // reuse existing snapshot helper
	if snap.CurrentPlayerID == current {
		t.Fatalf("turn did not advance — the queued Act appears to have been lost")
	}
}
```

Note: the exact helper names (`newTestActorTwoSeated`, `actorSnapshotFor`, `CurrentPlayerIDForActorSnapshot`) must be
replaced with whatever this package's existing integration tests actually call — `tests/integration/` already has a
two-seat table + Dispatch harness for the turn-order tests (`TestMultiServerFuzz` and its neighbors); grep that
directory for its setup helper before writing this test for real, rather than inventing new ones.

- [ ] **Step 2: Run it**

Run: `go test -tags integration -race ./tests/integration -run TestRequestHandoffDoesNotDisruptAnAlreadyQueuedAction -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add api/tests/integration/handoff_test.go
git commit -m "test(integration): prove handoff never loses or duplicates a queued action (#353)"
```

---

### Task 9: documentation (mandatory per `api/CLAUDE.md`)

**Files:**

- Modify: `api/CLAUDE.md`

- [ ] **Step 1: Add a Conventions-section bullet**

Add, after the `tablenotify`/cross-process bullets already in `api/CLAUDE.md`:

```markdown
- **Issue #353 fixed: explicit session handoff between devices.** A player confirming "continue here, disconnect the
  other device" sends
  `request_handoff` over the table WS; `table.RequestHandoffCmd` reads
  `internal/tableconn`'s now per-connection fleet-wide set (`map[playerID]map[connID]expiry`, was `map[playerID]expiry`)
  to find every OTHER live `connID` for that player anywhere in the fleet, and
  `internal/tablehandoff` (a payload-carrying Pub/Sub channel, same shape as
  `internal/tablenotify`) tells whichever instance owns each one to close it via `wsdrain.CloseByConnID` — a real 1001
  close frame, never a TTL expiry. No new locking: `Actor.Run`'s existing single-goroutine mailbox already serializes
  `RequestHandoffCmd` against any in-flight command from the old connection, so a queued action from the device being
  replaced always commits before the handoff runs, and nothing new from it can arrive after its socket closes. See
  `docs/specs/2026-09-05-session-handoff-tableconn.md`.
```

- [ ] **Step 2: Commit**

```bash
git add api/CLAUDE.md
git commit -m "docs(api): document session handoff between devices (#353)"
```

## Self-review notes

- **Spec coverage:** every ADR decision (§1 tableconn schema, §2 command, §3 Pub/Sub close, §4 real close frame, §5
  mailbox ordering) maps to Task 1, 3, 5, 4, and 8 respectively; §5's claim is the one thing the plan proves with a
  dedicated integration test rather than asserting by argument alone.
- **Placeholder scan:** the only non-literal names left in the plan are the three integration-test helper names in Task
  8, explicitly flagged as
  "replace with whatever this package already has" rather than invented — everything else is concrete code.
- **Type consistency:** `ConnStore.Sync`, `HandoffCloser.RequestClose`, and
  `RequestHandoffCmd`'s fields are spelled identically everywhere they're used across Tasks 1-7.
- **Scope:** one subsystem (session handoff), nine tasks, each independently testable; no decomposition needed.
