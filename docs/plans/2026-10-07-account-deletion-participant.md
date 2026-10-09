# Account Deletion — Poker Participant Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make ctech-poker a participant of the LGPD account-deletion saga. Poker will:
- answer ctech-account's eligibility check;
- refuse every write for a locked or erased user;
- cut revoked tokens off at once;
- consume `user.locked` / `user.unlocked` / `user.erase` from its own SQS queue;
- erase or anonymize everything it keeps about the user;
- ack the result to ctech-account.

**Architecture:**
- `gopkg.aoctech.app/api-commons` v1.14.0 provides the contract.
  - `erasure.Store` keeps the lock/tombstone in `{env}_poker_erasure_state`.
  - `erasure.Consumer` runs the SQS loop and lock handling.
  - `erasure.AckClient` posts the acks.
  - `jwtverify.Verifier.WithRevocation` handles token cut-off.
- Poker adds three pieces:
  - the lock check at its choke points: `authMiddleware`, both WebSocket gateways, and the wallet webhook;
  - an eligibility endpoint;
  - a new package `internal/userpurge` whose `Purger.Purge` is the consumer's `PurgeFunc`. It implements the data inventory §7;
  - an `OnLock` hook that force-closes the locked user's open sockets on every instance through `ws.Registry`.
- Infra: one table, one queue + DLQ subscribed to the account topic, and IAM, all in the existing CDK stacks.

**Tech Stack:** Go 1.27, Fiber v3, uber-fx, DynamoDB (aws-sdk-go-v2), S3, SQS, Valkey, AWS CDK (TypeScript, jest).

**Spec:** ctech-account repo:
- `docs/specs/2026-10-06-account-deletion-overview.md` (D1–D14)
- `docs/specs/2026-10-06-account-deletion-data-inventory.md` §2, §7
- `docs/specs/2026-10-06-account-deletion-saga-protocol.md` §3–§5, §7–§10

The wire contract is fixed by ctech-account `docs/plans/2026-10-07-account-deletion-phase3-participants.md` (Global Constraints, Tasks 3 and 5).

## Global Constraints

- Branch `feat/account-deletion-participant` from `main`. Conventional Commits. **No `Co-Authored-By` or any Claude/Anthropic attribution** in commits or PRs.
- `gopkg.aoctech.app/api-commons` **v1.14.0** (poker pins v1.11.0 today).
- Service id: `poker`. Every name below is literal.
  - Eligibility:
    - Route: `GET /v1.0/internal/erasure/eligibility/:sub`.
    - Token: minted by ctech-account with `aud` = poker's `SERVICE_AUDIENCE`, `azp` = account's `SELF_CLIENT_ID` (config `ERASURE_ACCOUNT_CLIENT_ID`, default `accounts`), no `sid`, scope `internal:poker:erasure-eligibility`.
    - Body: `erasure.Eligibility`.
    - Any failure is a `503`, never "eligible".
  - Ack:
    - `POST {CTECH_URL}/v1.0/internal/erasure/ack`, body `erasure.Ack`.
    - Client-credentials token from `{CTECH_URL}/v1.0/token`, scope `internal:account:erasure-ack`, using a **dedicated** confidential client: `ERASURE_ACK_CLIENT_ID` / `ERASURE_ACK_CLIENT_SECRET`, from SSM `/ctech/{env}/poker/erasure-ack-client-id` and `/ctech/{env}/poker/erasure-ack-client-secret`. It is **not** `POKER_CLIENT_ID`.
  - Lock fan-out: `erasure.Consumer.OnLock` (api-commons v1.14.0) force-closes every open socket of the locked sub on every instance, both table sockets (seated or spectating) and gateway sockets. It goes through the `ws.Registry` key `erasure-lock#<sub>`. The connect-time check stays.
  - SNS topic `{env}-account-user-erasure`. Its ARN is in SSM `/ctech/{env}/account/erasure-topic-arn`.
    - Subscribe queue `{env}-poker-user-erasure` (DLQ `{env}-poker-user-erasure-dlq`, `maxReceiveCount` 5).
    - Raw message delivery on.
    - `FilterPolicy {"services":["poker"]}` on the **message attribute** (default scope).
  - State table `{env}_poker_erasure_state`: `pk` String, TTL `ttl`, no sort key (prefix passed to `erasure.NewStore` is `{env}_poker`).
- Blocker codes (stable, translated by the account UI):
  - `poker.seated_at_table`
  - `poker.chips_held`
  - `poker.pending_cashout`
  - `poker.pending_fee_debit`
- The pseudonym that replaces an erased player is random per purge run (`anon_` + `crypto/rand.Text()` lowercased). It is held in memory only and never written next to the sub.
- Reports filed by or against the user are retained anonymized for **1 year** (`ttl` capped at `now + 365 d`).
- After every Go task, run from `api/`: `go vet ./... && go vet -tags integration ./... && go test ./...`. All must be green (`api/CLAUDE.md`: vet the integration tag after any signature change).
- Integration tests need DynamoDB Local: `docker compose -f api/docker-compose.test.yml up -d` (port 8555).
- Mandatory documentation policy (root, `api/`, `cdk/` CLAUDE.md): every behavior/config/infra change is documented in the same branch (Task 11).

## Rulings

Rulings are decisions where the spec was silent or conflicting. Each is followed by its cost if wrong.

- **R1 State table is `{env}_poker_erasure_state`**, not `{env}_erasure_state`.
  - Every participant shares the AWS account, and poker's table names all carry `poker_` (`cdk/lib/dynamodb-stack.ts`).
  - The existing IAM wildcard `{env}_poker*` covers it.
  - *Cost:* a rename (nobody outside poker reads it).
- **R2 (decided by the user 2026-10-08) Acks use a dedicated confidential client.**
  - Config: `ERASURE_ACK_CLIENT_ID` / `ERASURE_ACK_CLIENT_SECRET`, from SSM `/ctech/{env}/poker/erasure-ack-client-id` and `-secret`.
  - The operator creates the client in ctech-account, grants it only `internal:account:erasure-ack`, and lists its id as poker's `client_id` in account's `ERASURE_PARTICIPANTS`.
  - Both values are required in prod.
- **R3 Every non-GET route verifies with `VerifyClaimsStrict`**, not only money routes. If the revocation list is unreachable, the route fails closed with `503`.
  - Valkey is already mandatory in prod (ws registry), so a Valkey outage already breaks writes.
  - *Cost:* none beyond that.
- **R4 The local lock is checked on every authenticated request, every method.**
  - Reason: `GET /players/me` lazily creates the profile and would resurrect an erased user.
  - On a store error, writes return `503` and reads fail open.
  - *Cost:* one strongly consistent `GetItem` per authenticated request (~1 RRU).
- **R5 A locked user may still `POST /v1.0/rooms/:id/leave`.** Money only flows back to the player on that path.
- **R6 (decided by the user 2026-10-08) Sockets open at lock time are force-closed, and connect is still checked (fail closed).**
  - The consumer's `OnLock` hook (api-commons v1.14.0) fires when a `user.locked`, or the synthetic lock applied on `user.erase`, moves the sub to locked. No-op redeliveries do not fire it.
  - The hook broadcasts on `ws.Registry` key `erasure-lock#<sub>`. Valkey pub/sub fans that out to every instance.
  - Every table socket (seated or spectating) and every gateway socket registers a closer under that key at connect. The closer closes the socket, and the gateway's read loop then runs its normal cleanup.
  - *Cost if a broadcast is lost* (Valkey blip): the socket lives until it disconnects. The connect-time check and the HTTP lock still hold.
- **R7 (legal approved 2026-10-08) The user's own rows are erased, not anonymized**, where only the user could ever read them:
  - their own `poker_player_hands` partition;
  - `poker_player_matchups`, which is readable only with both real ids, and re-keying would swap the pair's low/high sides.

  Leaderboard rows are **re-keyed** to the pseudonym with `player_name` dropped, so other players' ranks do not move.
  - *Cost:* if legal reads "anonymize" literally, head-to-head aggregates must be re-keyed instead (side swap logic in `matchup`).
- **R8 One pseudonym per purge *run*.** A crash plus redelivery can give the user two pseudonyms across hands, never two inside one row.
  - *Cost:* minor inconsistency in others' histories after a crash.
- **R9 (legal approved 2026-10-08) Report retention** (the inventory said `TODO`):
  - reports filed by or against the user are kept anonymized for 1 year;
  - text the erased user wrote is dropped: `details` on reports they filed, and `evidence_message` (their copied chat) on reports against them.
  - *Cost:* needs legal sign-off (open question 1).
- **R10 Tombstones never expire** (`erasedTTL` 0). They hold only the opaque sub, and they stop resurrection forever.
  - *Cost:* one tiny row per erased user.
- **R11 Deliberately untouched:**
  - `poker_table_state`: live, actor-owned, 7-day TTL; the user cannot be seated.
  - `poker_rooms`: `created_by` and invite grants hold the opaque sub only, reaped by TTL.
  - table-keyed Valkey keys, rate-limit windows and the leaderboard rank mirror: TTL of 5 min or less.
  - events the user caused in inboxes of people who are not friends: TTL 90 d.
  - free text written by *other* players that mentions the user's name.
  - *Cost:* residual opaque ids for at most the TTL. Names inside others' free text survive.
- **R12 The eligibility route lives under `/v1.0`.** Account's `ERASURE_PARTICIPANTS` entry for poker is `{"service":"poker","url":"https://poker-api[-env].aoctech.app/v1.0","audience":"<poker SERVICE_AUDIENCE>","client_id":"<ERASURE_ACK_CLIENT_ID>"}`. ctech-account's prod `SELF_CLIENT_ID` is `accounts` with no override (confirmed 2026-10-08), so `ERASURE_ACCOUNT_CLIENT_ID` keeps its default.
- **R13 No tournament blocker.** No tournament concept exists in code (grep: none). "Chips held in a game":
  - a seat at a real-money table is covered by `poker.seated_at_table`;
  - an unresolved settlement still holding wallet holds becomes `poker.chips_held`.
- **R14 Queue visibility timeout is 12 h** (the SQS maximum). The consumer has no heartbeat, and a heavy player's purge rewrites every hand they played.
  - *Cost:* a crashed purge waits up to 12 h before redelivery. Account's backoff is 15 min to 24 h anyway.
- **R15 The wallet webhook drops (200) product purchases of a locked or erased user.** Granting would resurrect erased rows (saga §4.5). The payment stays in the wallet ledger.
  - *Cost:* a user who paid a PIX before the lock and then cancels deletion needs support to get the item.
- **R16 `internal:poker:erasure-eligibility` is declared in poker's scope manifest** with `"visibility": "internal"`, the same convention as account's internal scopes. It is not exposed by `PublicActiveScopes`.
- **R17 Settlement rows written before `gsi_player_settlements` existed (#333) are invisible to the blocker check.**
  - *Cost:* such a row would not block; the reconcile sweeper still pays it to the player id.
- **R18 `erasure.Store.Clear` (service-scope re-consent) is not wired.** Single-service unlink is out of scope on the account side too (phase 3 plan).

## Review Focus

1. **A locked or erased user hits `GET /players/me`.** It must not recreate the profile; it returns `403`. Test: Task 2 `TestAuthMiddlewareErasureLock/locked_user_cannot_read`.
2. **The purge crashes halfway and the message is redelivered.** The second run finishes the work, does not fail, and leaves no trace. Test: Tasks 6–7 run `Purge` twice and scan every table for the sub and the display name.
3. **Valkey is down.** Reads still work; writes return `503` and are never served with a revoked token. Test: Task 1 `TestAuthMiddlewareRevocation`.
4. **Poker's blocker lookup fails during eligibility.** Account must see a `503`, never `eligible: true`. Test: Task 3 `TestErasureEligibility/failure_is_not_eligible`.
5. **A PIX product purchase confirms after the user was erased.** Nothing is re-granted and nothing is broadcast. Test: Task 2 `TestWalletWebhookDropsPurchaseOfErasedUser`.

---

## File map

| File | Responsibility |
|---|---|
| `api/internal/api/v1/auth.go` | strict verify on writes, `BlockedFunc`, `erasureLock`, `wsLocked` |
| `api/internal/api/v1/tablews.go` | lock check on both WS gateways |
| `api/internal/api/v1/walletwebhook.go` | drop purchases of locked/erased users |
| `api/internal/api/v1/router.go` | thread `blocked` through |
| `api/internal/api/v1/erasure.go` | eligibility endpoint |
| `api/internal/api/v1/erasurews.go` | force-close a locked user's sockets fleet-wide (`OnLock` hook) |
| `api/internal/userpurge/eligibility.go` | blockers |
| `api/internal/userpurge/anonymize.go` | pseudonymization of DynamoDB items and archive JSON lines |
| `api/internal/userpurge/inventory.go` | table names + treatment per table |
| `api/internal/userpurge/dynamo.go` | query/scan/delete/rewrite/re-key helpers |
| `api/internal/userpurge/s3.go` | archive rewrite, avatar version purge |
| `api/internal/userpurge/purger.go` | `Purger.Purge` (the `PurgeFunc`) and its steps |
| `api/internal/matchup/store.go` | export `PairKey` |
| `api/internal/app/erasure.go` | Fx wiring: store, eligibility route, consumer |
| `api/internal/app/app.go` | `newVerifier` revocation, `Module` entries, route seam |
| `api/internal/config/config.go` | `ERASURE_QUEUE_URL`, `ERASURE_ACCOUNT_CLIENT_ID`, `ACTION_LOG_ARCHIVE_BUCKET` |
| `api/internal/oauthresource/scope-manifest.json` | internal eligibility scope |
| `cdk/lib/{constants,dynamodb-stack,api-stack,archiver-stack}.ts` | table, queue, DLQ, subscription, IAM, env |

---

### Task 1: api-commons v1.14.0, revocation, strict writes

**Files:**
- Modify: `api/go.mod`, `api/go.sum`
- Modify: `api/internal/app/app.go` (`newVerifier`)
- Modify: `api/internal/api/v1/auth.go`
- Test: `api/internal/api/v1/auth_test.go`

**Interfaces:**
- Consumes: `jwtverify.Verifier.WithRevocation(cache.Backend) *Verifier`, `VerifyClaimsStrict`, `jwtverify.Revoke`, `jwtverify.ErrRevocationUnavailable` (api-commons v1.14.0).
- Produces: `func unavailable(detail string) *problem.Problem` in package `v1` (a 503 problem), used by Tasks 2–3.

- [ ] **Step 1: Upgrade the dependency**

```bash
cd api && go get gopkg.aoctech.app/api-commons@v1.14.0 && go mod tidy && go build ./...
```
Expected: no output from `go build`. (v1.12–v1.14 only add `erasure` (incl. `Consumer.OnLock`, v1.14.0), `jwtverify` revocation and an optional `ws.Publisher`, with no breaking change. v1.14.0 is released by a separate ctech-go-common plan: this task is blocked until the tag exists — `go list -m gopkg.aoctech.app/api-commons@v1.14.0` must resolve.)

- [ ] **Step 2: Write the failing test** — append to `api/internal/api/v1/auth_test.go` and add `"context"` and `"errors"` to its imports:

```go
// downCache is a revocation backend that is unreachable.
type downCache struct{ cache.Backend }

func (downCache) Get(context.Context, string) ([]byte, bool, error) {
	return nil, false, errors.New("valkey down")
}

// A revoked user is cut off at once; a revocation list outage fails open on
// reads and closed on writes (saga protocol §5).
func TestAuthMiddlewareRevocation(t *testing.T) {
	key, srv := newJWKSServer(t)
	revoked := cache.NewMemoryBackend(10)
	if err := jwtverify.Revoke(context.Background(), revoked, "user_1", time.Now(), jwtverify.RevocationTTL); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	exp := time.Now().Add(15 * time.Minute).Unix()
	iat := time.Now().Add(-time.Minute).Unix()
	userTok := signToken(t, key, jwt.MapClaims{"sub": "user_1", "sid": "s1", "azp": "poker", "token_use": "access", "exp": exp, "iat": iat})
	otherTok := signToken(t, key, jwt.MapClaims{"sub": "user_2", "sid": "s2", "azp": "poker", "token_use": "access", "exp": exp, "iat": iat})

	newApp := func(revocation cache.Backend) *fiber.App {
		verifier := jwtverify.NewVerifier(srv.URL, "", "", cache.NewMemoryBackend(10)).WithRevocation(revocation)
		app := fiber.New()
		ok := func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }
		auth := authMiddleware(verifier)
		app.Get("/protected", auth, ok)
		app.Post("/protected", auth, ok)
		return app
	}
	status := func(t *testing.T, app *fiber.App, method, token string) int {
		t.Helper()
		req := httptest.NewRequest(method, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		return resp.StatusCode
	}

	app := newApp(revoked)
	if got := status(t, app, fiber.MethodGet, userTok); got != fiber.StatusUnauthorized {
		t.Fatalf("revoked user: want 401, got %d", got)
	}
	if got := status(t, app, fiber.MethodGet, otherTok); got != fiber.StatusOK {
		t.Fatalf("other user: want 200, got %d", got)
	}
	down := newApp(downCache{})
	if got := status(t, down, fiber.MethodGet, userTok); got != fiber.StatusOK {
		t.Fatalf("read with revocation list down: want 200 (fail open), got %d", got)
	}
	if got := status(t, down, fiber.MethodPost, userTok); got != fiber.StatusServiceUnavailable {
		t.Fatalf("write with revocation list down: want 503 (fail closed), got %d", got)
	}
}
```

- [ ] **Step 3: Run it and watch it fail**

Run: `cd api && go test ./internal/api/v1/ -run TestAuthMiddlewareRevocation -count=1`
Expected: FAIL. The last assertion fails with `write with revocation list down: want 503 (fail closed), got 200`, because `authMiddleware` always uses `VerifyClaims`.

- [ ] **Step 4: Implement**

In `api/internal/api/v1/auth.go`:
- add the import `"errors"` (`"log/slog"` comes in Task 2, where it is first used);
- replace the `claims, err := verifier.VerifyClaims(...)` line with:

```go
		// Writes fail closed when the revocation list is unreachable (saga
		// protocol §5: money-moving routes are strict); reads fail open.
		verify := verifier.VerifyClaims
		if c.Method() != fiber.MethodGet {
			verify = verifier.VerifyClaimsStrict
		}
		claims, err := verify(c.Context(), strings.TrimPrefix(authHeader, "Bearer "))
		if errors.Is(err, jwtverify.ErrRevocationUnavailable) {
			return unavailable("authorization is temporarily unavailable, retry shortly").Send(c)
		}
```

At the end of `auth.go`, add:

```go
// unavailable is a 503: the request may succeed if retried.
func unavailable(detail string) *problem.Problem {
	return problem.New(fiber.StatusServiceUnavailable, "/problems/unavailable", "Service Unavailable", detail)
}
```

In `api/internal/app/app.go`, replace `newVerifier`:

```go
func newVerifier(c cache.Backend, cfg *config.Config) *jwtverify.Verifier {
	// Account-deletion revocation entries live in Valkey DB 0, the shared base
	// URL. VALKEY_URL is that base URL (no DB suffix, cdk/lib/constants.ts
	// SSM_SHARED), so the main backend reads them; no second backend needed.
	return jwtverify.NewVerifier(cfg.CtechJWKSURL, cfg.ServiceAudience, cfg.CtechIssuerURL, c).WithRevocation(c)
}
```

- [ ] **Step 5: Run the tests**

Run: `cd api && go test ./internal/api/v1/ -run 'TestAuthMiddleware' -count=1 && go vet ./... && go vet -tags integration ./...`
Expected: `ok  	gopkg.aoctech.app/poker/api/internal/api/v1`, with vet silent.

- [ ] **Step 6: Commit**

```bash
git add api/go.mod api/go.sum api/internal/app/app.go api/internal/api/v1/auth.go api/internal/api/v1/auth_test.go
git commit -m "feat(api): api-commons v1.14.0, JWT revocation, strict verify on writes"
```

---

### Task 2: The erasure lock at every write choke point

**Files:**
- Create: `api/internal/app/erasure.go`
- Create: `api/internal/api/v1/walletwebhook_erasure_test.go`
- Modify: `api/internal/api/v1/auth.go`, `api/internal/api/v1/tablews.go`, `api/internal/api/v1/walletwebhook.go`, `api/internal/api/v1/router.go`, `api/internal/app/app.go`
- Test: `api/internal/api/v1/auth_test.go`, plus `, nil` added to the existing `RegisterWalletWebhook` calls in three tests

**Interfaces:**
- Consumes: `erasure.NewStore(db, tablePrefix string, erasedTTL time.Duration) *erasure.Store` and `(*erasure.Store).Blocked(ctx, sub string) (bool, error)`; `unavailable` (Task 1).
- Produces (later tasks rely on these):
  - `type BlockedFunc func(ctx context.Context, sub string) (bool, error)` in package `v1`.
  - `authMiddleware(verifier *jwtverify.Verifier, blocked BlockedFunc) fiber.Handler`.
  - `v1.Register(..., cosmeticLoadoutSvc *cosmeticloadout.Service, blocked BlockedFunc)`.
  - `RegisterTableWS(..., reactionLimiter *RateLimiter, blocked BlockedFunc)`.
  - `RegisterGeneralWS(..., presenceSvc *presence.Service, blocked BlockedFunc)`.
  - `RegisterWalletWebhook(..., reg ws.Registry, blocked BlockedFunc)`.
  - `func newErasureStore(db *dynamodb.Client, cfg *config.Config) *erasure.Store` in package `app`.

**Where writes enter (the choke points):**
- every authenticated HTTP route goes through `authMiddleware` (`router.go`: `auth := authMiddleware(...)`);
- the two WS gateways verify their own first frame;
- the wallet webhook is unauthenticated (HMAC) and grants entitlements for `purchase.UserID`.

These background writers need no check:
- the hand pipeline and auto-rebuy only act on seated players, and seated is a blocker;
- `cmd/reconcile` and `cmd/tablecleanup` only pay money back to the player.

- [ ] **Step 1: Write the failing tests**

Point the existing tests at the new signatures:

```bash
cd api
sed -i 's/authMiddleware(verifier)/authMiddleware(verifier, nil)/' internal/api/v1/auth_test.go
sed -i -E 's/^(\s*RegisterWalletWebhook\(.*)\)$/\1, nil)/' internal/api/v1/walletwebhook_test.go internal/api/v1/walletwebhook_cosmetic_test.go internal/api/v1/walletwebhook_reaction_test.go
```

Append to `api/internal/api/v1/auth_test.go`:

```go
// A user the deletion saga locked or erased is refused everywhere except
// leaving a table (saga protocol §4.2, plan rulings R4/R5).
func TestAuthMiddlewareErasureLock(t *testing.T) {
	key, srv := newJWKSServer(t)
	verifier := jwtverify.NewVerifier(srv.URL, "", "", cache.NewMemoryBackend(10))
	exp := time.Now().Add(15 * time.Minute).Unix()
	tok := signToken(t, key, jwt.MapClaims{"sub": "user_1", "sid": "s1", "azp": "poker", "token_use": "access", "exp": exp})

	var locked bool
	var lockErr error
	blocked := func(_ context.Context, sub string) (bool, error) { return locked && sub == "user_1", lockErr }
	app := fiber.New()
	auth := authMiddleware(verifier, blocked)
	ok := func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }
	app.Get("/v1.0/players/me", auth, ok)
	app.Post("/v1.0/rooms", auth, ok)
	app.Post("/v1.0/rooms/:id/leave", auth, ok)

	cases := []struct {
		name         string
		locked       bool
		err          error
		method, path string
		want         int
	}{
		{"active user writes", false, nil, fiber.MethodPost, "/v1.0/rooms", fiber.StatusOK},
		{"locked user cannot write", true, nil, fiber.MethodPost, "/v1.0/rooms", fiber.StatusForbidden},
		{"locked user cannot read", true, nil, fiber.MethodGet, "/v1.0/players/me", fiber.StatusForbidden},
		{"locked user may leave a table", true, nil, fiber.MethodPost, "/v1.0/rooms/r1/leave", fiber.StatusOK},
		{"store down fails closed on writes", false, errors.New("dynamo down"), fiber.MethodPost, "/v1.0/rooms", fiber.StatusServiceUnavailable},
		{"store down fails open on reads", false, errors.New("dynamo down"), fiber.MethodGet, "/v1.0/players/me", fiber.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			locked, lockErr = tc.locked, tc.err
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("Authorization", "Bearer "+tok)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			if resp.StatusCode != tc.want {
				t.Fatalf("want %d, got %d", tc.want, resp.StatusCode)
			}
		})
	}
}

// A socket outlives any later check, so the WS gateways refuse on a store failure.
func TestWSLockedFailsClosed(t *testing.T) {
	ctx := context.Background()
	if wsLocked(ctx, nil, "u") {
		t.Fatal("no check configured must allow")
	}
	if !wsLocked(ctx, func(context.Context, string) (bool, error) { return false, errors.New("down") }, "u") {
		t.Fatal("store failure must refuse")
	}
	if !wsLocked(ctx, func(context.Context, string) (bool, error) { return true, nil }, "u") {
		t.Fatal("locked user must be refused")
	}
	if wsLocked(ctx, func(context.Context, string) (bool, error) { return false, nil }, "u") {
		t.Fatal("active user must pass")
	}
}
```

Create `api/internal/api/v1/walletwebhook_erasure_test.go`:

```go
//go:build integration

package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/api-commons/ws"
	"gopkg.aoctech.app/poker/api/internal/reactionpurchase"
	"gopkg.aoctech.app/poker/api/internal/sandboxpurchase"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

// A PIX product purchase confirmed after the buyer was erased must not
// re-create their rows (saga protocol §4.5, plan ruling R15).
func TestWalletWebhookDropsPurchaseOfErasedUser(t *testing.T) {
	db := webhookTestDynamoClient(t)
	env := fmt.Sprintf("webhook_erasure_test_%d", time.Now().UnixNano())
	webhookCreateTestTable(t, db, dynamo.TableName(env, "poker_reaction_entitlements"))
	webhookCreateTestTable(t, db, dynamo.TableName(env, "poker_reaction_purchases"))
	entitlements := reactionpurchase.NewEntitlementStore(db, env)
	wallet := &fakeReactionWebhookWallet{
		getResult: &walletclient.ProductPurchase{PurchaseID: "prdp-erased-1", UserID: "player-1", SKU: "poker_reaction_cold", Amount: 100, Status: "confirmed"},
	}
	svc := reactionpurchase.NewService(wallet, entitlements, reactionpurchase.NewStore(db, env))
	sandboxSvc := sandboxpurchase.NewService(&fakeSandboxWallet{}, newFakeStoreForWebhookTest())
	reg := ws.NewMemoryRegistry()
	conn := &recordingConn{}
	reg.Register("user#player-1", "conn-1", conn)
	blocked := func(_ context.Context, sub string) (bool, error) { return sub == "player-1", nil }

	app := fiber.New()
	RegisterWalletWebhook(app.Group("/v1.0"), "secret", sandboxSvc, svc, noopCosmeticSvc(), reg, blocked)
	body, _ := json.Marshal(map[string]string{"purchase_id": "prdp-erased-1"})
	req := httptest.NewRequest(http.MethodPost, "/v1.0/webhooks/wallet", bytes.NewReader(body))
	req.Header.Set("X-Wallet-Signature", sign("secret", body))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 (drop, do not make the wallet retry), got %d", resp.StatusCode)
	}
	if len(conn.messages) != 0 {
		t.Fatalf("an erased user must not be notified, got %d messages", len(conn.messages))
	}
	got, err := entitlements.Get(context.Background(), "player-1", "cold")
	if err != nil || got != nil {
		t.Fatalf("erased user's entitlement re-granted: %+v (err=%v)", got, err)
	}
}
```

- [ ] **Step 2: Run them and watch them fail**

Run: `cd api && go vet ./internal/api/v1/`
Expected: FAIL with `too many arguments in call to authMiddleware` and `undefined: wsLocked`.

- [ ] **Step 3: Implement the lock in `auth.go`**

Add `"context"` to the imports. Change the signature to `func authMiddleware(verifier *jwtverify.Verifier, blocked BlockedFunc) fiber.Handler`. After the `enforceReadOnlyScope` block, and before `c.Locals(localsUserID, ...)`, insert:

```go
		if denied := erasureLock(c, blocked, claims.Sub); denied != nil {
			return denied.Send(c)
		}
```

Append to `auth.go`:

```go
// BlockedFunc reports whether the account-deletion saga has locked or erased
// sub (erasure.Store.Blocked). nil disables the check; only the narrow test
// seam in internal/app passes nil.
type BlockedFunc func(ctx context.Context, sub string) (bool, error)

// erasureLock refuses a user the deletion saga locked or erased (saga
// protocol §4.2) on every method, because some GETs write (GET /players/me
// lazily creates the profile). Leaving a table is the one exception: money
// only flows back to the player there. A store failure fails closed on
// writes and open on reads.
func erasureLock(c fiber.Ctx, blocked BlockedFunc, sub string) *problem.Problem {
	if blocked == nil {
		return nil
	}
	locked, err := blocked(c.Context(), sub)
	switch {
	case err != nil && c.Method() != fiber.MethodGet:
		slog.ErrorContext(c.Context(), "erasure lock check failed", "error", err)
		return unavailable("account state is temporarily unavailable, retry shortly")
	case err != nil:
		slog.WarnContext(c.Context(), "erasure lock check failed; serving the read", "error", err)
		return nil
	case locked && !isTableLeave(c):
		return problem.Forbidden("this account is being deleted")
	}
	return nil
}

func isTableLeave(c fiber.Ctx) bool {
	return c.Method() == fiber.MethodPost &&
		strings.HasPrefix(c.Path(), "/v1.0/rooms/") && strings.HasSuffix(c.Path(), "/leave")
}

// wsLocked is erasureLock for the WebSocket gateways. A socket outlives any
// later check, so a store failure refuses the connection.
func wsLocked(ctx context.Context, blocked BlockedFunc, sub string) bool {
	if blocked == nil {
		return false
	}
	locked, err := blocked(ctx, sub)
	if err != nil {
		slog.ErrorContext(ctx, "erasure lock check failed; refusing websocket", "error", err)
		return true
	}
	return locked
}
```

- [ ] **Step 4: Gate the WebSockets in `tablews.go`**

Add `blocked BlockedFunc,` as the last parameter of both `RegisterTableWS` (after `reactionLimiter *RateLimiter,`) and `RegisterGeneralWS` (after `presenceSvc *presence.Service,`).

In `RegisterTableWS`, replace:

```go
			playerID := claims.Sub

			// Private rooms are invite-only end to end: the WS gate mirrors
```

with:

```go
			playerID := claims.Sub
			if wsLocked(ctx, blocked, playerID) {
				send(&pokerproto.ServerMessage{Type: "error", Code: "unauthorized"})
				closePokerWS(ctx, conn, "connection shutdown")
				return
			}

			// Private rooms are invite-only end to end: the WS gate mirrors
```

In `RegisterGeneralWS`, replace:

```go
			playerID := claims.Sub
			connID := uuid.New().String()
```

with:

```go
			playerID := claims.Sub
			if wsLocked(ctx, blocked, playerID) {
				send(&pokerproto.ServerMessage{Type: "error", Code: "unauthorized"})
				closePokerWS(ctx, conn, "connection shutdown")
				return
			}
			connID := uuid.New().String()
```

- [ ] **Step 5: Gate the wallet webhook in `walletwebhook.go`**

Change both signatures to take `blocked BlockedFunc` as the last parameter:
- `RegisterWalletWebhook(router fiber.Router, hmacSecret string, sandboxSvc *sandboxpurchase.Service, reactionSvc *reactionpurchase.Service, cosmeticSvc *cosmeticpurchase.Service, reg ws.Registry, blocked BlockedFunc)`
- `walletWebhookHandler(..., reg ws.Registry, blocked BlockedFunc)`

`RegisterWalletWebhook` passes `blocked` through. In the product branch, right after the `if purchase == nil || purchase.PurchaseID != payload.PurchaseID { ... }` block, insert:

```go
			if blocked != nil {
				locked, err := blocked(c.Context(), purchase.UserID)
				if err != nil {
					slog.Error("wallet webhook: erasure lock check failed", "purchase_id", payload.PurchaseID, "err", err)
					return c.SendStatus(fiber.StatusInternalServerError)
				}
				if locked {
					// Granting would resurrect a locked or erased user's rows
					// (saga protocol §4.5). The payment stays in the wallet ledger.
					slog.Warn("wallet webhook: dropped purchase of a locked or erased user", "purchase_id", payload.PurchaseID)
					return c.SendStatus(fiber.StatusOK)
				}
			}
```

(Sandbox purchases need no check: `ConfirmFromWebhook` returns early when the local row is missing, and the purge erases it.)

- [ ] **Step 6: Thread `blocked` through `router.go`**

Add `blocked BlockedFunc,` as the last parameter of `Register`, after `cosmeticLoadoutSvc *cosmeticloadout.Service,`. Then:
- change `RegisterTableWS(router, verifier, manager, reg, cfg.CorsAllowedOrigins, seed, rooms, cfg, players, pokerStatsStore, wsActionLimiter, wsReactionLimiter)` to end with `, wsReactionLimiter, blocked)`;
- change `RegisterGeneralWS(router, verifier, reg, cfg.CorsAllowedOrigins, presenceSvc)` to `RegisterGeneralWS(router, verifier, reg, cfg.CorsAllowedOrigins, presenceSvc, blocked)`;
- change `auth := authMiddleware(verifier)` to `auth := authMiddleware(verifier, blocked)`;
- change `RegisterWalletWebhook(router, cfg.WalletWebhookHMACSecret, sandboxPurchaseSvc, reactionPurchaseSvc, cosmeticPurchaseSvc, reg)` to end with `, reg, blocked)`.

- [ ] **Step 7: Wire the store in the app**

Create `api/internal/app/erasure.go`:

```go
package app

import (
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/api-commons/erasure"
	"gopkg.aoctech.app/poker/api/internal/config"
)

// Account-deletion saga participant wiring
// (docs/plans/2026-10-07-account-deletion-participant.md).

// newErasureStore binds {env}_poker_erasure_state. The poker_ segment keeps it
// from colliding with another participant's state table in the same AWS
// account (plan ruling R1). Tombstones never expire (R10).
func newErasureStore(db *dynamodb.Client, cfg *config.Config) *erasure.Store {
	return erasure.NewStore(db, cfg.Env+"_poker", 0)
}
```

In `api/internal/app/app.go`:
- add `newErasureStore,` to `fx.Provide` right after `newDynamoClient,`;
- add `"gopkg.aoctech.app/api-commons/erasure"` to the imports;
- give `registerRoutesWithSocialRuntime` a last parameter `erasureStore *erasure.Store,` (after `cosmeticLoadoutSvc *cosmeticloadout.Service,`);
- end its `v1.Register(...)` call with `..., cosmeticLoadoutSvc, erasureStore.Blocked)`;
- in `registerRoutes`, end its `v1.Register(...)` call with `..., nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)` (one more `nil`, the `blocked` argument).

- [ ] **Step 8: Run the tests**

Run: `cd api && go vet ./... && go vet -tags integration ./... && go test ./internal/api/v1/ ./internal/app/ -count=1`
Expected: `ok` for both packages.

Run (DynamoDB Local up): `cd api && go test -tags integration ./internal/api/v1/ -run 'TestWalletWebhook' -count=1`
Expected: `ok`.

- [ ] **Step 9: Commit**

```bash
git add api/internal/api/v1 api/internal/app
git commit -m "feat(api): refuse locked or erased users on HTTP, WebSocket and wallet webhook"
```

---

### Task 3: Eligibility endpoint and blockers

**Files:**
- Create: `api/internal/userpurge/eligibility.go`, `api/internal/userpurge/eligibility_test.go`
- Create: `api/internal/api/v1/erasure.go`, `api/internal/api/v1/erasure_test.go`
- Modify: `api/internal/app/erasure.go`, `api/internal/app/app.go`, `api/internal/config/config.go`, `api/internal/oauthresource/scope-manifest.json`

**Interfaces:**
- Consumes:
  - `(*sessionlog.Store).FindLatestOpenSession(ctx, playerID string) (string, error)`;
  - `(*reconcile.PendingStore).ListForPlayer(ctx, playerID string, limit int, startKey map[string]types.AttributeValue) ([]reconcile.PendingCashout, map[string]types.AttributeValue, error)`;
  - `erasure.Blocker`, `erasure.NewEligibility`;
  - `unavailable` (Task 1).
- Produces:
  - `userpurge.NewEligibility(seats seatFinder, pending settlementLister) *userpurge.Eligibility`, with `(*Eligibility).Blockers(ctx, sub string) ([]erasure.Blocker, error)`;
  - constants `userpurge.BlockerSeatedAtTable`, `BlockerChipsHeld`, `BlockerPendingCashout`, `BlockerPendingFeeDebit`;
  - `v1.RegisterErasure(router fiber.Router, verifier *jwtverify.Verifier, accountClientID string, blockers BlockersFunc)`;
  - `type v1.BlockersFunc func(ctx context.Context, sub string) ([]erasure.Blocker, error)`;
  - `v1.ScopeErasureEligibility`;
  - `config.Config.ErasureAccountClientID`.

- [ ] **Step 1: Write the failing blocker test** — `api/internal/userpurge/eligibility_test.go`:

```go
package userpurge

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/poker/api/internal/reconcile"
)

type fakeSeats struct {
	table string
	err   error
}

func (f fakeSeats) FindLatestOpenSession(context.Context, string) (string, error) { return f.table, f.err }

// fakeSettlements serves one page per ListForPlayer call, chained by a fake startKey.
type fakeSettlements struct{ pages [][]reconcile.PendingCashout }

func (f fakeSettlements) ListForPlayer(_ context.Context, _ string, _ int, start map[string]types.AttributeValue) ([]reconcile.PendingCashout, map[string]types.AttributeValue, error) {
	if len(f.pages) == 0 {
		return nil, nil, nil
	}
	i := 0
	if start != nil {
		i, _ = strconv.Atoi(start["page"].(*types.AttributeValueMemberS).Value)
	}
	var next map[string]types.AttributeValue
	if i+1 < len(f.pages) {
		next = map[string]types.AttributeValue{"page": &types.AttributeValueMemberS{Value: strconv.Itoa(i + 1)}}
	}
	return f.pages[i], next, nil
}

func TestBlockers(t *testing.T) {
	ctx := context.Background()
	t.Run("unknown user has none", func(t *testing.T) {
		got, err := NewEligibility(fakeSeats{}, fakeSettlements{}).Blockers(ctx, "u")
		if err != nil || len(got) != 0 {
			t.Fatalf("want no blockers, got %+v (err=%v)", got, err)
		}
	})
	t.Run("seat and unresolved settlements across pages", func(t *testing.T) {
		settlements := fakeSettlements{pages: [][]reconcile.PendingCashout{
			{{HoldIDs: []string{"h1"}}, {Resolved: true, HoldIDs: []string{"h0"}}},
			{{Kind: reconcile.KindFeeDebit}, {Kind: ""}, {Kind: reconcile.KindCashout}},
		}}
		got, err := NewEligibility(fakeSeats{table: "t1"}, settlements).Blockers(ctx, "u")
		if err != nil {
			t.Fatal(err)
		}
		want := []struct {
			code   string
			detail string
		}{
			{BlockerSeatedAtTable, "t1"}, {BlockerChipsHeld, "1"}, {BlockerPendingCashout, "2"}, {BlockerPendingFeeDebit, "1"},
		}
		if len(got) != len(want) {
			t.Fatalf("want %d blockers, got %+v", len(want), got)
		}
		for i, w := range want {
			if got[i].Code != w.code {
				t.Fatalf("blocker %d: want %s, got %s", i, w.code, got[i].Code)
			}
		}
		if got[0].Detail["table_id"] != "t1" || got[2].Detail["count"] != 2 {
			t.Fatalf("details: %+v", got)
		}
	})
	t.Run("a lookup failure is an error, never eligible", func(t *testing.T) {
		if _, err := NewEligibility(fakeSeats{err: errors.New("down")}, fakeSettlements{}).Blockers(ctx, "u"); err == nil {
			t.Fatal("want error")
		}
	})
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `cd api && go test ./internal/userpurge/ -count=1`
Expected: FAIL with `undefined: NewEligibility`.

- [ ] **Step 3: Implement** — `api/internal/userpurge/eligibility.go`:

```go
// Package userpurge is poker's side of the LGPD account-deletion saga
// (ctech-account docs/specs/2026-10-06-account-deletion-saga-protocol.md):
// blockers for the eligibility check and the purge that implements the data
// inventory §7. The protocol itself lives in api-commons/erasure.
package userpurge

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/erasure"
	"gopkg.aoctech.app/poker/api/internal/reconcile"
)

// Blocker codes. Stable: ctech-account's UI translates them.
const (
	BlockerSeatedAtTable   = "poker.seated_at_table"
	BlockerChipsHeld       = "poker.chips_held"
	BlockerPendingCashout  = "poker.pending_cashout"
	BlockerPendingFeeDebit = "poker.pending_fee_debit"
)

type seatFinder interface {
	FindLatestOpenSession(ctx context.Context, playerID string) (string, error)
}

type settlementLister interface {
	ListForPlayer(ctx context.Context, playerID string, limit int, startKey map[string]types.AttributeValue) ([]reconcile.PendingCashout, map[string]types.AttributeValue, error)
}

// Eligibility computes what stops a user's erasure right now.
type Eligibility struct {
	seats   seatFinder
	pending settlementLister
}

func NewEligibility(seats seatFinder, pending settlementLister) *Eligibility {
	return &Eligibility{seats: seats, pending: pending}
}

// Blockers lists poker's blockers for sub: a seat (real-money seats hold
// chips in the wallet), and every unresolved settlement. There is no
// tournament concept in poker (plan ruling R13).
func (e *Eligibility) Blockers(ctx context.Context, sub string) ([]erasure.Blocker, error) {
	var out []erasure.Blocker
	tableID, err := e.seats.FindLatestOpenSession(ctx, sub)
	if err != nil {
		return nil, fmt.Errorf("userpurge: open session of %s: %w", sub, err)
	}
	if tableID != "" {
		out = append(out, erasure.Blocker{Code: BlockerSeatedAtTable, Detail: map[string]any{"table_id": tableID}})
	}
	counts := map[string]int{}
	var start map[string]types.AttributeValue
	for {
		rows, next, err := e.pending.ListForPlayer(ctx, sub, 100, start)
		if err != nil {
			return nil, fmt.Errorf("userpurge: settlements of %s: %w", sub, err)
		}
		for _, p := range rows {
			switch {
			case p.Resolved:
			case len(p.HoldIDs) > 0:
				counts[BlockerChipsHeld]++
			case p.Kind == reconcile.KindFeeDebit:
				counts[BlockerPendingFeeDebit]++
			default:
				counts[BlockerPendingCashout]++
			}
		}
		if len(next) == 0 {
			break
		}
		start = next
	}
	for _, code := range []string{BlockerChipsHeld, BlockerPendingCashout, BlockerPendingFeeDebit} {
		if n := counts[code]; n > 0 {
			out = append(out, erasure.Blocker{Code: code, Detail: map[string]any{"count": n}})
		}
	}
	return out, nil
}
```

- [ ] **Step 4: Run it**

Run: `cd api && go test ./internal/userpurge/ -count=1`
Expected: `ok  	gopkg.aoctech.app/poker/api/internal/userpurge`.

- [ ] **Step 5: Write the failing endpoint test** — `api/internal/api/v1/erasure_test.go`:

```go
package v1

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/erasure"
	"gopkg.aoctech.app/api-commons/jwtverify"
)

func TestErasureEligibility(t *testing.T) {
	key, srv := newJWKSServer(t)
	verifier := jwtverify.NewVerifier(srv.URL, "", "", cache.NewMemoryBackend(10))
	exp := time.Now().Add(5 * time.Minute).Unix()
	var fail error
	blockers := func(_ context.Context, sub string) ([]erasure.Blocker, error) {
		if fail != nil {
			return nil, fail
		}
		if sub == "seated" {
			return []erasure.Blocker{{Code: "poker.seated_at_table"}}, nil
		}
		return nil, nil
	}
	app := fiber.New()
	RegisterErasure(app.Group("/v1.0"), verifier, "accounts", blockers)
	tok := func(claims jwt.MapClaims) string {
		claims["token_use"], claims["exp"] = "access", exp
		return signToken(t, key, claims)
	}
	account := tok(jwt.MapClaims{"sub": "ctech-account", "azp": "accounts", "scope": ScopeErasureEligibility})

	get := func(t *testing.T, sub, token string) (int, erasure.Eligibility) {
		t.Helper()
		req := httptest.NewRequest(fiber.MethodGet, "/v1.0/internal/erasure/eligibility/"+sub, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		var el erasure.Eligibility
		if resp.StatusCode == fiber.StatusOK {
			if err := json.NewDecoder(resp.Body).Decode(&el); err != nil {
				t.Fatalf("decode: %v", err)
			}
		}
		return resp.StatusCode, el
	}

	t.Run("blockers are listed", func(t *testing.T) {
		code, el := get(t, "seated", account)
		if code != fiber.StatusOK || el.Eligible || len(el.Blockers) != 1 || el.Blockers[0].Code != "poker.seated_at_table" {
			t.Fatalf("got %d %+v", code, el)
		}
	})
	t.Run("unknown user is eligible", func(t *testing.T) {
		code, el := get(t, "nobody", account)
		if code != fiber.StatusOK || !el.Eligible || el.Blockers == nil {
			t.Fatalf("got %d %+v", code, el)
		}
	})
	for name, token := range map[string]string{
		"user session refused":  tok(jwt.MapClaims{"sub": "u", "sid": "s", "azp": "accounts", "scope": ScopeErasureEligibility}),
		"other client refused":  tok(jwt.MapClaims{"sub": "poker", "azp": "poker", "scope": ScopeErasureEligibility}),
		"missing scope refused": tok(jwt.MapClaims{"sub": "ctech-account", "azp": "accounts"}),
	} {
		t.Run(name, func(t *testing.T) {
			if code, _ := get(t, "u", token); code != fiber.StatusForbidden {
				t.Fatalf("want 403, got %d", code)
			}
		})
	}
	t.Run("no token", func(t *testing.T) {
		if code, _ := get(t, "u", ""); code != fiber.StatusUnauthorized {
			t.Fatalf("want 401, got %d", code)
		}
	})
	t.Run("failure is not eligible", func(t *testing.T) {
		fail = errors.New("dynamo down")
		defer func() { fail = nil }()
		if code, _ := get(t, "u", account); code != fiber.StatusServiceUnavailable {
			t.Fatalf("want 503, got %d", code)
		}
	})
}
```

- [ ] **Step 6: Run it and watch it fail**

Run: `cd api && go test ./internal/api/v1/ -run TestErasureEligibility -count=1`
Expected: FAIL with `undefined: RegisterErasure`.

- [ ] **Step 7: Implement the endpoint** — `api/internal/api/v1/erasure.go`:

```go
package v1

import (
	"context"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/erasure"
	"gopkg.aoctech.app/api-commons/jwtverify"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

// ScopeErasureEligibility is carried by the service token ctech-account
// mints to ask poker for a user's deletion blockers (saga protocol §4.1).
const ScopeErasureEligibility = "internal:poker:erasure-eligibility"

// BlockersFunc lists what stops a user's erasure right now.
type BlockersFunc func(ctx context.Context, sub string) ([]erasure.Blocker, error)

// RegisterErasure mounts GET /internal/erasure/eligibility/:sub. Only
// ctech-account's own service token may call it: azp = accountClientID, no
// session, and the eligibility scope. An unknown sub is eligible; any
// failure is a 503, never "eligible".
func RegisterErasure(router fiber.Router, verifier *jwtverify.Verifier, accountClientID string, blockers BlockersFunc) {
	router.Get("/internal/erasure/eligibility/:sub", func(c fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			return problem.Unauthorized("missing bearer token").Send(c)
		}
		claims, err := verifier.VerifyClaims(c.Context(), strings.TrimPrefix(authHeader, "Bearer "))
		if err != nil || claims == nil {
			return problem.Unauthorized("invalid credentials").Send(c)
		}
		if accountClientID == "" || claims.SID != "" || claims.AZP != accountClientID || !claims.HasScope(ScopeErasureEligibility) {
			return problem.Forbidden("erasure eligibility is reserved to ctech-account").Send(c)
		}
		found, err := blockers(c.Context(), c.Params("sub"))
		if err != nil {
			slog.ErrorContext(c.Context(), "erasure eligibility failed", "error", err)
			return unavailable("eligibility is temporarily unavailable").Send(c)
		}
		return c.JSON(erasure.NewEligibility(found...))
	})
}
```

- [ ] **Step 8: Config, manifest and wiring**

In `api/internal/config/config.go`, add to `Config` after `WalletWebhookHMACSecret`:

```go
	// Account-deletion saga (docs/plans/2026-10-07-account-deletion-participant.md).
	// ErasureAccountClientID is ctech-account's own client id (its SELF_CLIENT_ID):
	// the azp of the token it mints for GET /internal/erasure/eligibility.
	ErasureAccountClientID string `env:"ERASURE_ACCOUNT_CLIENT_ID" envDefault:"accounts"`
```

In `api/internal/oauthresource/scope-manifest.json`, append to `scopes` (after the `poker:cosmetic-purchases:read` entry, adding the comma):

```json
    { "name": "internal:poker:erasure-eligibility", "descriptions": { "pt-BR": "Consultar bloqueios de exclusao de conta (somente ctech-account).", "en": "Read account-deletion blockers (ctech-account only)." }, "visibility": "internal", "status": "active" }
```

Replace `api/internal/app/erasure.go` with:

```go
package app

import (
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/erasure"
	"gopkg.aoctech.app/api-commons/jwtverify"
	v1 "gopkg.aoctech.app/poker/api/internal/api/v1"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/reconcile"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
	"gopkg.aoctech.app/poker/api/internal/userpurge"
)

// Account-deletion saga participant wiring
// (docs/plans/2026-10-07-account-deletion-participant.md).

// newErasureStore binds {env}_poker_erasure_state. The poker_ segment keeps it
// from colliding with another participant's state table in the same AWS
// account (plan ruling R1). Tombstones never expire (R10).
func newErasureStore(db *dynamodb.Client, cfg *config.Config) *erasure.Store {
	return erasure.NewStore(db, cfg.Env+"_poker", 0)
}

func newEligibility(sessions *sessionlog.Store, pending *reconcile.PendingStore) *userpurge.Eligibility {
	return userpurge.NewEligibility(sessions, pending)
}

func registerErasureRoutes(app *fiber.App, cfg *config.Config, verifier *jwtverify.Verifier, elig *userpurge.Eligibility) {
	v1.RegisterErasure(app.Group("/v1.0"), verifier, cfg.ErasureAccountClientID, elig.Blockers)
}
```

Check how `app.go` imports package `v1` (`grep -n 'api/v1"' api/internal/app/app.go`) and use the same alias.

In `app.go`'s `Module`:
- add `newEligibility,` after `newErasureStore,`;
- add `fx.Invoke(registerErasureRoutes),` right before `fx.Invoke(startServer),`.

- [ ] **Step 9: Run the tests**

Run: `cd api && go vet ./... && go vet -tags integration ./... && go test ./internal/api/v1/ ./internal/userpurge/ ./internal/app/ ./internal/oauthresource/ ./internal/config/ -count=1`
Expected: `ok` for each package. `oauthresource`'s test still sees 12 public scopes.

- [ ] **Step 10: Commit**

```bash
git add api/internal/userpurge api/internal/api/v1/erasure.go api/internal/api/v1/erasure_test.go api/internal/app api/internal/config/config.go api/internal/oauthresource/scope-manifest.json
git commit -m "feat(api): erasure eligibility endpoint with poker blockers"
```

---

### Task 4: Anonymizer

**Files:**
- Create: `api/internal/userpurge/anonymize.go`, `api/internal/userpurge/anonymize_test.go`

**Interfaces:**
- Produces:
  - `newPseudonym() string`;
  - `anonymizeMap(m map[string]types.AttributeValue, sub, pseudo string) (map[string]types.AttributeValue, bool)`;
  - `anonymizeItem(item map[string]types.AttributeValue, sub, pseudo string) (map[string]types.AttributeValue, bool)` (keeps `pk`/`sk`);
  - `anonymizeJSONLines(body []byte, sub, pseudo string) ([]byte, bool, error)`;
  - `str(v string) types.AttributeValue`.

The rules are one generic walk, so no per-table field list can go stale:
1. Every string, string-set element, list element, map value **and map key** that contains the sub gets it replaced by the pseudonym. Map keys matter because `Payouts`, `RoundIdx` and `player_hands` are keyed by player id.
2. A map whose `id` or `player_id` equals the sub loses `name`, `avatar_url`, `playstyle_badge` and `message`. This covers:
   - `hand.Player` (`id`, `name`, `avatar_url`, `playstyle_badge`);
   - `OpponentSummary`, `ReplaySeat` and highlight winners (`player_id`, `name`);
   - chat entries and action-log rows (`player_id`, `message`).

- [ ] **Step 1: Write the failing test** — `api/internal/userpurge/anonymize_test.go`:

```go
package userpurge

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const (
	testSub    = "11111111-1111-4111-8111-111111111111"
	testPseudo = "anon_test"
)

func TestAnonymizeItemDropsPIIOfTheErasedPlayerOnly(t *testing.T) {
	item := map[string]types.AttributeValue{
		"pk": str("other"), "sk": str("sandbox#h1"),
		"opponents": &types.AttributeValueMemberL{Value: []types.AttributeValue{
			&types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
				"player_id": str(testSub), "name": str("Alice"),
				"avatar_url": str("https://x/v1.0/avatars/" + testSub + "/1.jpg"),
				"won":        &types.AttributeValueMemberBOOL{Value: true},
			}},
			&types.AttributeValueMemberM{Value: map[string]types.AttributeValue{"player_id": str("bob"), "name": str("Bob")}},
		}},
		"payouts":   &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{testSub: &types.AttributeValueMemberN{Value: "10"}}},
		"reporters": &types.AttributeValueMemberSS{Value: []string{testSub, "bob"}},
	}
	out, changed := anonymizeItem(item, testSub, testPseudo)
	if !changed {
		t.Fatal("expected a change")
	}
	opps := out["opponents"].(*types.AttributeValueMemberL).Value
	alice := opps[0].(*types.AttributeValueMemberM).Value
	if alice["player_id"].(*types.AttributeValueMemberS).Value != testPseudo {
		t.Fatalf("player_id not pseudonymized: %+v", alice["player_id"])
	}
	for _, k := range []string{"name", "avatar_url"} {
		if _, ok := alice[k]; ok {
			t.Fatalf("%s of the erased player kept", k)
		}
	}
	if _, ok := alice["won"]; !ok {
		t.Fatal("game data must stay")
	}
	if opps[1].(*types.AttributeValueMemberM).Value["name"].(*types.AttributeValueMemberS).Value != "Bob" {
		t.Fatal("another player's name must stay")
	}
	if _, ok := out["payouts"].(*types.AttributeValueMemberM).Value[testPseudo]; !ok {
		t.Fatal("map key not pseudonymized")
	}
	if got := out["reporters"].(*types.AttributeValueMemberSS).Value; got[0] != testPseudo || got[1] != "bob" {
		t.Fatalf("string set: %v", got)
	}
	if out["pk"].(*types.AttributeValueMemberS).Value != "other" || out["sk"].(*types.AttributeValueMemberS).Value != "sandbox#h1" {
		t.Fatal("keys must not move")
	}
	if _, again := anonymizeItem(out, testSub, testPseudo); again {
		t.Fatal("a second pass must be a no-op")
	}
}

func TestAnonymizeItemKeepsKeysAndDropsOwnChat(t *testing.T) {
	item := map[string]types.AttributeValue{"pk": str(testSub), "player_id": str(testSub), "message": str("oi")}
	out, changed := anonymizeItem(item, testSub, testPseudo)
	if !changed || out["pk"].(*types.AttributeValueMemberS).Value != testSub {
		t.Fatal("pk must stay; rows keyed by the sub are re-keyed, not rewritten")
	}
	if _, ok := out["message"]; ok {
		t.Fatal("chat text of the erased player kept")
	}
}

func TestAnonymizeJSONLinesKeepsNumbersExact(t *testing.T) {
	body := []byte(`{"player_id":"` + testSub + `","message":"oi","amount":12345678901234567890,"frame":{"seats":[{"player_id":"` + testSub + `","name":"Alice"},{"player_id":"bob","name":"Bob"}]}}` + "\n" +
		`{"player_id":"bob","amount":1}` + "\n")
	out, changed, err := anonymizeJSONLines(body, testSub, testPseudo)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	text := string(out)
	for _, gone := range []string{testSub, "Alice", `"message"`} {
		if strings.Contains(text, gone) {
			t.Fatalf("%q survived: %s", gone, text)
		}
	}
	for _, kept := range []string{"12345678901234567890", "Bob", testPseudo} {
		if !strings.Contains(text, kept) {
			t.Fatalf("%q lost: %s", kept, text)
		}
	}
	if _, again, _ := anonymizeJSONLines(out, testSub, testPseudo); again {
		t.Fatal("a second pass must be a no-op")
	}
}

func TestNewPseudonymIsRandom(t *testing.T) {
	a, b := newPseudonym(), newPseudonym()
	if a == b || !strings.HasPrefix(a, "anon_") {
		t.Fatalf("got %q and %q", a, b)
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `cd api && go test ./internal/userpurge/ -run 'TestAnonymize|TestNewPseudonym' -count=1`
Expected: FAIL with `undefined: anonymizeItem`.

- [ ] **Step 3: Implement** — `api/internal/userpurge/anonymize.go`:

```go
package userpurge

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// idKeys name the attribute that says whose a nested record is
// (hand.Player.id, OpponentSummary.player_id, ReplaySeat.player_id, chat
// entries, action-log rows).
var idKeys = []string{"id", "player_id"}

// piiKeys are dropped from any record whose idKeys name the erased player:
// display name, avatar URL, playstyle badge and chat text.
var piiKeys = []string{"name", "avatar_url", "playstyle_badge", "message"}

// newPseudonym is the random id that replaces one erased player in other
// players' rows. It lives in memory for one purge run only and is never
// written next to the sub (data inventory §7, plan ruling R8).
func newPseudonym() string { return "anon_" + strings.ToLower(rand.Text()) }

func str(v string) types.AttributeValue { return &types.AttributeValueMemberS{Value: v} }

// anonymizeAV replaces sub with pseudo in every string, string-set element,
// list element, map value and map key of v, and drops piiKeys from every map
// that identifies sub. It reports whether anything changed.
func anonymizeAV(v types.AttributeValue, sub, pseudo string) (types.AttributeValue, bool) {
	switch t := v.(type) {
	case *types.AttributeValueMemberS:
		if strings.Contains(t.Value, sub) {
			return str(strings.ReplaceAll(t.Value, sub, pseudo)), true
		}
	case *types.AttributeValueMemberSS:
		out, changed := make([]string, len(t.Value)), false
		for i, s := range t.Value {
			out[i] = strings.ReplaceAll(s, sub, pseudo)
			changed = changed || out[i] != s
		}
		if changed {
			return &types.AttributeValueMemberSS{Value: out}, true
		}
	case *types.AttributeValueMemberL:
		out, changed := make([]types.AttributeValue, len(t.Value)), false
		for i, e := range t.Value {
			var c bool
			out[i], c = anonymizeAV(e, sub, pseudo)
			changed = changed || c
		}
		if changed {
			return &types.AttributeValueMemberL{Value: out}, true
		}
	case *types.AttributeValueMemberM:
		if out, changed := anonymizeMap(t.Value, sub, pseudo); changed {
			return &types.AttributeValueMemberM{Value: out}, true
		}
	}
	return v, false
}

// anonymizeMap is anonymizeAV for a map, which is also a whole item (keys included).
func anonymizeMap(m map[string]types.AttributeValue, sub, pseudo string) (map[string]types.AttributeValue, bool) {
	identifies := false
	for _, k := range idKeys {
		if s, ok := m[k].(*types.AttributeValueMemberS); ok && s.Value == sub {
			identifies = true
		}
	}
	out, changed := make(map[string]types.AttributeValue, len(m)), false
	for k, v := range m {
		if identifies && slices.Contains(piiKeys, k) {
			changed = true
			continue
		}
		nv, c := anonymizeAV(v, sub, pseudo)
		nk := strings.ReplaceAll(k, sub, pseudo)
		changed = changed || c || nk != k
		out[nk] = nv
	}
	return out, changed
}

// anonymizeItem rewrites a stored item in place: pk and sk keep their
// values, so a Put lands on the same item. Rows whose key holds the sub are
// re-keyed instead (run.rekey).
func anonymizeItem(item map[string]types.AttributeValue, sub, pseudo string) (map[string]types.AttributeValue, bool) {
	body := maps.Clone(item)
	delete(body, "pk")
	delete(body, "sk")
	out, changed := anonymizeMap(body, sub, pseudo)
	for _, k := range []string{"pk", "sk"} {
		if v, ok := item[k]; ok {
			out[k] = v
		}
	}
	return out, changed
}

// anonymizeJSON applies the same rules to decoded JSON.
func anonymizeJSON(v any, sub, pseudo string) (any, bool) {
	switch t := v.(type) {
	case string:
		if strings.Contains(t, sub) {
			return strings.ReplaceAll(t, sub, pseudo), true
		}
	case []any:
		out, changed := make([]any, len(t)), false
		for i, e := range t {
			var c bool
			out[i], c = anonymizeJSON(e, sub, pseudo)
			changed = changed || c
		}
		if changed {
			return out, true
		}
	case map[string]any:
		identifies := false
		for _, k := range idKeys {
			if s, ok := t[k].(string); ok && s == sub {
				identifies = true
			}
		}
		out, changed := make(map[string]any, len(t)), false
		for k, e := range t {
			if identifies && slices.Contains(piiKeys, k) {
				changed = true
				continue
			}
			ne, c := anonymizeJSON(e, sub, pseudo)
			nk := strings.ReplaceAll(k, sub, pseudo)
			changed = changed || c || nk != k
			out[nk] = ne
		}
		if changed {
			return out, true
		}
	}
	return v, false
}

// anonymizeJSONLines rewrites one action-log archive object (JSON Lines, see
// cmd/archiver). Numbers stay json.Number: the archive is the audit trail of
// every chip amount and must not lose precision past 2^53.
func anonymizeJSONLines(body []byte, sub, pseudo string) ([]byte, bool, error) {
	var out bytes.Buffer
	changed := false
	for _, line := range bytes.Split(body, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err != nil {
			return nil, false, err
		}
		nv, c := anonymizeJSON(v, sub, pseudo)
		changed = changed || c
		enc, err := json.Marshal(nv)
		if err != nil {
			return nil, false, err
		}
		out.Write(enc)
		out.WriteByte('\n')
	}
	return out.Bytes(), changed, nil
}
```

- [ ] **Step 4: Run it**

Run: `cd api && go test ./internal/userpurge/ -count=1`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add api/internal/userpurge/anonymize.go api/internal/userpurge/anonymize_test.go
git commit -m "feat(api): pseudonymize an erased player in DynamoDB items and archive lines"
```

---

### Task 5: Inventory and DynamoDB helpers

**Files:**
- Create: `api/internal/userpurge/inventory.go`, `api/internal/userpurge/inventory_test.go`, `api/internal/userpurge/dynamo.go`, `api/internal/userpurge/purger.go`, `api/internal/userpurge/s3.go`

**Interfaces:**
- Produces:
  - every `Table*` constant listed below, and `inventory map[string]string`;
  - the `Purger` struct and `run` struct;
  - `NewPurger(db *dynamodb.Client, objects S3API, c cache.Backend, cfg Config, blockers BlockersFunc) *Purger`;
  - `type Config struct{ Env, AvatarBucket, ArchiveBucket string }`, `type BlockersFunc`, `type S3API`;
  - helpers `key`, `keyOf`, `stringAttr`, `(*Purger).table`, `(*Purger).partition`, `(*Purger).eachItem`, `(*Purger).eachScan`, `(*Purger).deleteKeys`;
  - run methods `getItem`, `rewrite`, `rewriteKey`, `rekey`, `deleteItem`, `erasePartition`, `count`.

  Tasks 6–7 use all of them.

- [ ] **Step 1: Write the failing test** — `api/internal/userpurge/inventory_test.go`:

```go
package userpurge

import (
	"os"
	"regexp"
	"testing"
)

// Every poker_* table the CDK declares must have a decided treatment here.
// A new table that holds player data and is not added to Purge is a silent
// LGPD gap; this makes it a red build instead.
func TestInventoryClassifiesEveryTable(t *testing.T) {
	re := regexp.MustCompile(`'(poker_[a-z_]+)'`)
	for _, file := range []string{"../../../cdk/lib/dynamodb-stack.ts", "../../../cdk/lib/constants.ts"} {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			if _, ok := inventory[m[1]]; !ok {
				t.Errorf("%s declares %s, which userpurge does not classify: decide erase/anonymize/retain (data inventory §7) and handle it in Purge", file, m[1])
			}
		}
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `cd api && go test ./internal/userpurge/ -run TestInventory -count=1`
Expected: FAIL with `undefined: inventory`.

- [ ] **Step 3: Implement `inventory.go`**

```go
package userpurge

// Unprefixed table names. Each owning package keeps its own unexported
// constant; this file mirrors them so the purge can address every table, and
// TestInventoryClassifiesEveryTable fails when cdk/lib declares a poker_*
// table that inventory does not classify.
const (
	TableTableState           = "poker_table_state"
	TableTableStateHistory    = "poker_table_state_history"
	TableActionLog            = "poker_action_log"
	TableActionGuards         = "poker_action_guards"
	TableRooms                = "poker_rooms"
	TablePlayerProfiles       = "poker_player_profiles"
	TableAchievementProgress  = "poker_achievement_progress"
	TableLeaderboardStats     = "poker_leaderboard_stats"
	TableDailyReward          = "poker_daily_reward"
	TablePendingCashouts      = "poker_pending_cashouts"
	TablePlayerSessions       = "poker_player_sessions"
	TablePlayerHands          = "poker_player_hands"
	TablePlayerNotes          = "poker_player_notes"
	TableHandMeta             = "poker_hand_meta"
	TableHandShares           = "poker_hand_shares"
	TablePokerStats           = "poker_player_poker_stats"
	TableMatchups             = "poker_player_matchups"
	TableSandboxPurchases     = "poker_sandbox_purchases"
	TableReactionEntitlements = "poker_reaction_entitlements"
	TableReactionPurchases    = "poker_reaction_purchases"
	TableCosmeticEntitlements = "poker_cosmetic_entitlements"
	TableCosmeticPurchases    = "poker_cosmetic_purchases"
	TableCosmeticLoadouts     = "poker_cosmetic_loadouts"
	TableChatPrefs            = "poker_chat_prefs"
	TableBotCheckContests     = "poker_botcheck_contests"
	TableWalletAlertPrefs     = "poker_wallet_alert_prefs"
	TablePromoCodes           = "poker_promo_codes"
	TablePromoRedemptions     = "poker_promo_redemptions"
	TableTableEntitlements    = "poker_table_entitlements"
	TableTableHighlights      = "poker_table_highlights"
	TableHandReveals          = "poker_hand_reveals"
	TableHandRevealPayments   = "poker_hand_reveal_payments"
	TableSocialEdges          = "poker_social_edges"
	TableRecentPlayers        = "poker_recent_players"
	TableSocialEvents         = "poker_social_events"
	TablePlayerReports        = "poker_player_reports"
	TableErasureState         = "poker_erasure_state"
)

// Index names the purge queries (cdk/lib/dynamodb-stack.ts).
const (
	gsiReportReporter    = "gsi_reporter"           // reports/store.go
	gsiHandShareOwner    = "gsi_owner"              // handshare/store.go
	gsiPlayerSettlements = "gsi_player_settlements" // reconcile/pending.go
)

// inventory is the data inventory §7 as built: the treatment of every table
// (plan rulings R7, R9, R11).
var inventory = map[string]string{
	TableTableState:           "untouched: live actor-owned item; the user cannot be seated (blocker), the chat ring rolls over, 7-day TTL",
	TableTableStateHistory:    "anonymize: every snapshot of every table the user played",
	TableActionLog:            "anonymize: every action of every hand the user played, plus the S3 archive",
	TableActionGuards:         "untouched: no player data",
	TableRooms:                "untouched: created_by / invite grants hold the opaque sub only, reaped by TTL",
	TablePlayerProfiles:       "erase: own row, avatar-report guard rows by/about the user, the user in avatar_reporters sets",
	TableAchievementProgress:  "erase",
	TableLeaderboardStats:     "anonymize: re-keyed to the pseudonym, player_name dropped",
	TableDailyReward:          "erase",
	TablePendingCashouts:      "erase: resolved rows only; an unresolved row is a blocker",
	TablePlayerSessions:       "erase: sessions and buyinguard# rows",
	TablePlayerHands:          "anonymize other players' rows of shared hands; erase the user's own rows",
	TablePlayerNotes:          "erase: notes written by and about the user",
	TableHandMeta:             "erase",
	TableHandShares:           "erase",
	TablePokerStats:           "erase",
	TableMatchups:             "erase: a pair aggregate is only readable with both real ids",
	TableSandboxPurchases:     "erase (the payment is retained in the wallet ledger)",
	TableReactionEntitlements: "erase",
	TableReactionPurchases:    "erase (the payment is retained in the wallet ledger)",
	TableCosmeticEntitlements: "erase",
	TableCosmeticPurchases:    "erase (the payment is retained in the wallet ledger)",
	TableCosmeticLoadouts:     "erase",
	TableChatPrefs:            "erase",
	TableBotCheckContests:     "erase",
	TableWalletAlertPrefs:     "erase",
	TablePromoCodes:           "untouched: counters only",
	TablePromoRedemptions:     "erase",
	TableTableEntitlements:    "erase",
	TableTableHighlights:      "anonymize: every day of every table the user played",
	TableHandReveals:          "anonymize: every hand the user played",
	TableHandRevealPayments:   "erase (the payment is retained in the wallet ledger)",
	TableSocialEdges:          "erase: both directions",
	TableRecentPlayers:        "erase the user's rows; anonymize opponents' rows of shared hands",
	TableSocialEvents:         "erase: own inbox, and events/invite guards the user caused in friends' inboxes",
	TablePlayerReports:        "retain anonymized 1 year: re-keyed to the pseudonym, the user's own text dropped",
	TableErasureState:         "untouched: the saga's own lock/tombstone (api-commons erasure.Store)",
}
```

- [ ] **Step 4: Implement the purger shell** — `purger.go`:

```go
package userpurge

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/erasure"
)

// reportRetention bounds how long an anonymized report outlives the erasure
// (anti-abuse; plan ruling R9).
const reportRetention = 365 * 24 * time.Hour

// Config names what the purge touches. An empty bucket skips that step (dev).
type Config struct {
	Env           string
	AvatarBucket  string
	ArchiveBucket string
}

// BlockersFunc lists what stops a user's erasure right now (Eligibility.Blockers).
type BlockersFunc func(ctx context.Context, sub string) ([]erasure.Blocker, error)

// Purger is poker's erasure.PurgeFunc. Every step is idempotent and re-reads
// from scratch, so a crash or a redelivered message resumes. Rows that drive
// discovery (the user's own hand rows, their social edges) are deleted only
// after what they point to.
type Purger struct {
	db       *dynamodb.Client
	objects  S3API
	cache    cache.Backend
	cfg      Config
	blockers BlockersFunc
	now      func() time.Time
}

func NewPurger(db *dynamodb.Client, objects S3API, c cache.Backend, cfg Config, blockers BlockersFunc) *Purger {
	return &Purger{db: db, objects: objects, cache: c, cfg: cfg, blockers: blockers, now: time.Now}
}

// run is one Purge invocation. pseudo is never persisted next to sub.
type run struct {
	p      *Purger
	sub    string
	pseudo string
	counts map[string]int
	tables map[string]bool // table ids whose table-keyed rows this run already anonymized
}

func (r *run) count(key string, n int) {
	if n > 0 {
		r.counts[key] += n
	}
}

// Purge erases or anonymizes m.Sub (data inventory §7). It re-checks the
// blockers first and never half-purges a blocked user. m.Organizations is
// ignored: poker holds no organization-scoped data.
func (p *Purger) Purge(ctx context.Context, m erasure.Message) (erasure.Ack, error) {
	blockers, err := p.blockers(ctx, m.Sub)
	if err != nil {
		return erasure.Ack{}, fmt.Errorf("userpurge: eligibility: %w", err)
	}
	if len(blockers) > 0 {
		return erasure.Ack{Result: erasure.ResultBlocked, Blockers: blockers}, nil
	}
	r := &run{p: p, sub: m.Sub, pseudo: newPseudonym(), counts: map[string]int{}, tables: map[string]bool{}}
	for _, step := range r.steps() {
		if err := step(ctx); err != nil {
			return erasure.Ack{}, err
		}
	}
	return erasure.Ack{Result: erasure.ResultDone, Counts: r.counts}, nil
}

// steps is the purge in order. Tasks 6 and 7 add to it.
func (r *run) steps() []func(context.Context) error {
	return nil
}
```

- [ ] **Step 5: Implement the S3 surface** — `s3.go`:

```go
package userpurge

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3API is the slice of *s3.Client the purge uses.
type S3API interface {
	ListObjectsV2(ctx context.Context, in *s3.ListObjectsV2Input, opts ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	GetObject(ctx context.Context, in *s3.GetObjectInput, opts ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObject(ctx context.Context, in *s3.PutObjectInput, opts ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	ListObjectVersions(ctx context.Context, in *s3.ListObjectVersionsInput, opts ...func(*s3.Options)) (*s3.ListObjectVersionsOutput, error)
	DeleteObjects(ctx context.Context, in *s3.DeleteObjectsInput, opts ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error)
}
```

- [ ] **Step 6: Implement the DynamoDB helpers** — `dynamo.go`:

```go
package userpurge

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	batchWriteMax      = 25
	batchWriteAttempts = 5
)

type item = map[string]types.AttributeValue

func key(pk, sk string) item {
	k := item{"pk": str(pk)}
	if sk != "" {
		k["sk"] = str(sk)
	}
	return k
}

func keyOf(it item) item {
	k := item{"pk": it["pk"]}
	if sk, ok := it["sk"]; ok {
		k["sk"] = sk
	}
	return k
}

func stringAttr(it item, name string) string {
	if s, ok := it[name].(*types.AttributeValueMemberS); ok {
		return s.Value
	}
	return ""
}

func (p *Purger) table(name string) string { return dynamo.TableName(p.cfg.Env, name) }

// partition is a strongly consistent query of one base-table partition.
func (p *Purger) partition(table, pk string) *dynamodb.QueryInput {
	return &dynamodb.QueryInput{
		TableName:                 aws.String(p.table(table)),
		KeyConditionExpression:    aws.String("pk = :pk"),
		ExpressionAttributeValues: item{":pk": str(pk)},
		ConsistentRead:            aws.Bool(true),
	}
}

// eachItem pages in and calls fn per item. Deleting the items already seen
// is safe: ExclusiveStartKey resumes after a key even if it is gone.
func (p *Purger) eachItem(ctx context.Context, in *dynamodb.QueryInput, fn func(item) error) error {
	for {
		out, err := p.db.Query(ctx, in)
		if err != nil {
			return fmt.Errorf("userpurge: query %s: %w", aws.ToString(in.TableName), err)
		}
		for _, it := range out.Items {
			if err := fn(it); err != nil {
				return err
			}
		}
		if len(out.LastEvaluatedKey) == 0 {
			return nil
		}
		in.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

// eachScan is eachItem for a Scan. Used only where no index exists; erasures
// are rare, so a full read per erasure is the price of not adding a GSI.
func (p *Purger) eachScan(ctx context.Context, in *dynamodb.ScanInput, fn func(item) error) error {
	for {
		out, err := p.db.Scan(ctx, in)
		if err != nil {
			return fmt.Errorf("userpurge: scan %s: %w", aws.ToString(in.TableName), err)
		}
		for _, it := range out.Items {
			if err := fn(it); err != nil {
				return err
			}
		}
		if len(out.LastEvaluatedKey) == 0 {
			return nil
		}
		in.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

// deleteKeys batch-deletes keys (25 per call), retrying unprocessed items.
func (p *Purger) deleteKeys(ctx context.Context, table string, keys []item) error {
	name := p.table(table)
	for len(keys) > 0 {
		n := min(len(keys), batchWriteMax)
		reqs := make([]types.WriteRequest, n)
		for i, k := range keys[:n] {
			reqs[i] = types.WriteRequest{DeleteRequest: &types.DeleteRequest{Key: k}}
		}
		keys = keys[n:]
		pending := map[string][]types.WriteRequest{name: reqs}
		for attempt := 0; len(pending) > 0; attempt++ {
			if attempt == batchWriteAttempts {
				return fmt.Errorf("userpurge: batch delete %s: unprocessed items after %d attempts", name, attempt)
			}
			if attempt > 0 {
				time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
			}
			out, err := p.db.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{RequestItems: pending})
			if err != nil {
				return fmt.Errorf("userpurge: batch delete %s: %w", name, err)
			}
			pending = out.UnprocessedItems
		}
	}
	return nil
}

func (r *run) getItem(ctx context.Context, table string, k item) (item, error) {
	out, err := r.p.db.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(r.p.table(table)), Key: k, ConsistentRead: aws.Bool(true)})
	if err != nil {
		return nil, fmt.Errorf("userpurge: get %s: %w", table, err)
	}
	return out.Item, nil
}

// rewrite pseudonymizes it in place. A row reaped by TTL meanwhile is fine.
func (r *run) rewrite(ctx context.Context, table string, it item) error {
	out, changed := anonymizeItem(it, r.sub, r.pseudo)
	if !changed {
		return nil
	}
	_, err := r.p.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(r.p.table(table)),
		Item:                out,
		ConditionExpression: aws.String("attribute_exists(pk)"),
	})
	if dynamo.IsConditionFailed(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("userpurge: rewrite %s: %w", table, err)
	}
	r.count(table+"_anonymized", 1)
	return nil
}

func (r *run) rewriteKey(ctx context.Context, table string, k item) error {
	it, err := r.getItem(ctx, table, k)
	if err != nil || len(it) == 0 {
		return err
	}
	return r.rewrite(ctx, table, it)
}

// rekey moves old to next's key in one transaction (rows whose key holds the sub).
func (r *run) rekey(ctx context.Context, table string, old, next item) error {
	name := aws.String(r.p.table(table))
	_, err := r.p.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: []types.TransactWriteItem{
		{Delete: &types.Delete{TableName: name, Key: keyOf(old)}},
		{Put: &types.Put{TableName: name, Item: next}},
	}})
	if err != nil {
		return fmt.Errorf("userpurge: rekey %s: %w", table, err)
	}
	r.count(table+"_anonymized", 1)
	return nil
}

// deleteItem deletes one item and counts it only if it existed.
func (r *run) deleteItem(ctx context.Context, table string, k item) error {
	out, err := r.p.db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(r.p.table(table)), Key: k, ReturnValues: types.ReturnValueAllOld,
	})
	if err != nil {
		return fmt.Errorf("userpurge: delete %s: %w", table, err)
	}
	if len(out.Attributes) > 0 {
		r.count(table+"_erased", 1)
	}
	return nil
}

// erasePartition deletes every item whose pk is pk.
func (r *run) erasePartition(ctx context.Context, table, pk string) error {
	var keys []item
	if err := r.p.eachItem(ctx, r.p.partition(table, pk), func(it item) error {
		keys = append(keys, keyOf(it))
		return nil
	}); err != nil {
		return err
	}
	r.count(table+"_erased", len(keys))
	return r.p.deleteKeys(ctx, table, keys)
}
```

Because of the `type item = ...` alias, the `map[string]types.AttributeValue` signatures in `anonymize.go` stay valid unchanged.

- [ ] **Step 7: Run the tests**

Run: `cd api && go vet ./internal/userpurge/ && go test ./internal/userpurge/ -count=1`
Expected: `ok`. The inventory test passes: all 36 CDK tables are classified, and `poker_erasure_state` is pre-classified for Task 10. `go vet` may report unused helpers only as lint, not as vet errors; they are used in Tasks 6–7.

- [ ] **Step 8: Commit**

```bash
git add api/internal/userpurge
git commit -m "feat(api): userpurge inventory of every poker table and DynamoDB helpers"
```

---

### Task 6: Purge — hands shared with other players

**Files:**
- Modify: `api/internal/userpurge/purger.go`, `api/internal/userpurge/s3.go`, `api/internal/matchup/store.go`
- Create: `api/internal/userpurge/helpers_integration_test.go`, `api/internal/userpurge/purger_integration_test.go`

**Interfaces:**
- Consumes: `sessionlog.HandItem` (`SK`, `CurrencyMode`, `TableID`, `HandID`, `Opponents []sessionlog.OpponentSummary{PlayerID}`) and everything from Task 5.
- Produces:
  - `matchup.PairKey(mode, a, b string) string`;
  - run methods `sharedHands`, `tableRows`, `handRows`, `archive`, `archiveObject`;
  - test helpers `testClient`, `createPurgeTables`, `put`, `putAV`, `scanTable`, `mentions`, `newFakeS3`, `testU`, `testO`.

Key formats come from the owning packages:
- action log pk is `tableID#handID` (`tablestore/dynamo.go`);
- the archive key is that pk with `#` replaced by `/`, plus `/<ns>-<event>.jsonl` (`cmd/archiver`);
- recent players sk is `hand#<handID>` (`recentplayers/dynamo.go`);
- reveal payments pk is `handID#viewerID` (`handreveal/payments.go`);
- every participant's `poker_player_hands` row of one hand shares sk `mode#handID`.

- [ ] **Step 1: Export the matchup key** — in `api/internal/matchup/store.go`, below `pairKey`:

```go
// PairKey is the item key of the unordered pair (a, b) in mode. Exported for
// userpurge, which deletes an erased player's pairs.
func PairKey(mode, a, b string) string {
	k, _, _ := pairKey(mode, a, b)
	return k
}
```

- [ ] **Step 2: Write the test helpers** — `api/internal/userpurge/helpers_integration_test.go`:

```go
//go:build integration

package userpurge

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/api-commons/erasure"
)

const (
	testU = "11111111-1111-4111-8111-111111111111" // the erased user ("Alice")
	testO = "22222222-2222-4222-8222-222222222222" // an opponent ("Bob")
)

// testClient mirrors the per-package DynamoDB Local helper (buyin/dynamo_helpers_test.go).
func testClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"), config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")))
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String("http://localhost:8555") })
}

type gsi struct {
	name, pk, sk string
	skNumber     bool
}

// purgeTables mirrors cdk/lib/dynamodb-stack.ts: table -> has a sort key.
var purgeTables = map[string]bool{
	TableTableStateHistory: true, TableActionLog: true, TablePlayerProfiles: false, TableAchievementProgress: true,
	TableLeaderboardStats: true, TableDailyReward: true, TablePlayerSessions: true, TablePlayerHands: true,
	TablePlayerNotes: true, TableHandMeta: true, TablePokerStats: false, TableMatchups: false,
	TableSandboxPurchases: true, TableReactionEntitlements: true, TableReactionPurchases: true,
	TableCosmeticEntitlements: true, TableCosmeticPurchases: true, TableCosmeticLoadouts: true,
	TableChatPrefs: false, TableBotCheckContests: true, TableWalletAlertPrefs: false, TablePromoRedemptions: true,
	TableTableEntitlements: true, TableTableHighlights: true, TableHandReveals: false, TableHandRevealPayments: false,
	TableSocialEdges: true, TableRecentPlayers: true, TableSocialEvents: true,
	TablePlayerReports: true, TableHandShares: false, TablePendingCashouts: true,
}

var purgeIndexes = map[string]gsi{
	TablePlayerReports:   {name: gsiReportReporter, pk: "gsi_reporter_pk", sk: "gsi_reporter_sk"},
	TableHandShares:      {name: gsiHandShareOwner, pk: "owner_id", sk: "created_at", skNumber: true},
	TablePendingCashouts: {name: gsiPlayerSettlements, pk: "player_id", sk: "recorded_at"},
}

func createPurgeTables(t *testing.T, db *dynamodb.Client, env string) {
	t.Helper()
	for table, withSK := range purgeTables {
		attrs := map[string]types.ScalarAttributeType{"pk": types.ScalarAttributeTypeS}
		keys := []types.KeySchemaElement{{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash}}
		if withSK {
			attrs["sk"] = types.ScalarAttributeTypeS
			keys = append(keys, types.KeySchemaElement{AttributeName: aws.String("sk"), KeyType: types.KeyTypeRange})
		}
		var indexes []types.GlobalSecondaryIndex
		if ix, ok := purgeIndexes[table]; ok {
			attrs[ix.pk] = types.ScalarAttributeTypeS
			skType := types.ScalarAttributeTypeS
			if ix.skNumber {
				skType = types.ScalarAttributeTypeN
			}
			attrs[ix.sk] = skType
			indexes = append(indexes, types.GlobalSecondaryIndex{
				IndexName: aws.String(ix.name),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String(ix.pk), KeyType: types.KeyTypeHash},
					{AttributeName: aws.String(ix.sk), KeyType: types.KeyTypeRange},
				},
				Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
			})
		}
		var defs []types.AttributeDefinition
		for name, typ := range attrs {
			defs = append(defs, types.AttributeDefinition{AttributeName: aws.String(name), AttributeType: typ})
		}
		if _, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
			TableName: aws.String(dynamo.TableName(env, table)), AttributeDefinitions: defs, KeySchema: keys,
			GlobalSecondaryIndexes: indexes, BillingMode: types.BillingModePayPerRequest,
		}); err != nil {
			t.Fatalf("create %s: %v", table, err)
		}
	}
}

func put(t *testing.T, db *dynamodb.Client, env, table string, fields map[string]any) {
	t.Helper()
	av, err := attributevalue.MarshalMap(fields)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	putAV(t, db, env, table, av)
}

func putAV(t *testing.T, db *dynamodb.Client, env, table string, av map[string]types.AttributeValue) {
	t.Helper()
	if _, err := db.PutItem(context.Background(), &dynamodb.PutItemInput{TableName: aws.String(dynamo.TableName(env, table)), Item: av}); err != nil {
		t.Fatalf("put %s: %v", table, err)
	}
}

func scanTable(t *testing.T, db *dynamodb.Client, env, table string) []map[string]types.AttributeValue {
	t.Helper()
	out, err := db.Scan(context.Background(), &dynamodb.ScanInput{TableName: aws.String(dynamo.TableName(env, table)), ConsistentRead: aws.Bool(true)})
	if err != nil {
		t.Fatalf("scan %s: %v", table, err)
	}
	return out.Items
}

// mentions reports whether needle appears anywhere in it, keys included.
func mentions(it map[string]types.AttributeValue, needle string) bool {
	_, found := anonymizeMap(it, needle, "\x00")
	return found
}

func noBlockers(context.Context, string) ([]erasure.Blocker, error) { return nil, nil }

func newTestPurger(t *testing.T, db *dynamodb.Client, objects S3API, c cache.Backend, blockers BlockersFunc) (*Purger, string) {
	t.Helper()
	env := fmt.Sprintf("userpurge_test_%d", time.Now().UnixNano())
	createPurgeTables(t, db, env)
	return NewPurger(db, objects, c, Config{Env: env, AvatarBucket: "avatars", ArchiveBucket: "archive"}, blockers), env
}

func purgeMsg() erasure.Message {
	return erasure.Message{Version: 1, Type: erasure.TypeErase, RequestID: "req-1", Sub: testU,
		Scope: erasure.ScopeAccount, Services: []string{"poker"}, IssuedAt: time.Now()}
}

// fakeS3 is an in-memory bucket store with one version per key.
type fakeS3 struct {
	mu      sync.Mutex
	objects map[string][]byte // bucket + "/" + key
}

func newFakeS3() *fakeS3 { return &fakeS3{objects: map[string][]byte{}} }

func (f *fakeS3) put(bucket, key string, body []byte) { f.objects[bucket+"/"+key] = body }

func (f *fakeS3) keys(bucket, prefix string) []string {
	var out []string
	for k := range f.objects {
		if rest, ok := strings.CutPrefix(k, bucket+"/"); ok && strings.HasPrefix(rest, prefix) {
			out = append(out, rest)
		}
	}
	sort.Strings(out)
	return out
}

func (f *fakeS3) ListObjectsV2(_ context.Context, in *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := &s3.ListObjectsV2Output{IsTruncated: aws.Bool(false)}
	for _, k := range f.keys(aws.ToString(in.Bucket), aws.ToString(in.Prefix)) {
		out.Contents = append(out.Contents, s3types.Object{Key: aws.String(k)})
	}
	return out, nil
}

func (f *fakeS3) GetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, ok := f.objects[aws.ToString(in.Bucket)+"/"+aws.ToString(in.Key)]
	if !ok {
		return nil, fmt.Errorf("no such key %s", aws.ToString(in.Key))
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(body))}, nil
}

func (f *fakeS3) PutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	body, err := io.ReadAll(in.Body)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.put(aws.ToString(in.Bucket), aws.ToString(in.Key), body)
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeS3) ListObjectVersions(_ context.Context, in *s3.ListObjectVersionsInput, _ ...func(*s3.Options)) (*s3.ListObjectVersionsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := &s3.ListObjectVersionsOutput{IsTruncated: aws.Bool(false)}
	for _, k := range f.keys(aws.ToString(in.Bucket), aws.ToString(in.Prefix)) {
		out.Versions = append(out.Versions, s3types.ObjectVersion{Key: aws.String(k), VersionId: aws.String("v1")})
	}
	return out, nil
}

func (f *fakeS3) DeleteObjects(_ context.Context, in *s3.DeleteObjectsInput, _ ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, o := range in.Delete.Objects {
		delete(f.objects, aws.ToString(in.Bucket)+"/"+aws.ToString(o.Key))
	}
	return &s3.DeleteObjectsOutput{}, nil
}

// seedSharedHand writes one sandbox hand h1 at table t1 between U and O.
func seedSharedHand(t *testing.T, db *dynamodb.Client, env string, objects *fakeS3) {
	t.Helper()
	avatar := "https://poker-api.aoctech.app/v1.0/avatars/" + testU + "/1.jpg"
	put(t, db, env, TablePlayerHands, map[string]any{"pk": testU, "sk": "sandbox#h1", "currency_mode": "sandbox", "table_id": "t1", "hand_id": "h1",
		"opponents": []map[string]any{{"player_id": testO, "name": "Bob"}}})
	put(t, db, env, TablePlayerHands, map[string]any{"pk": testO, "sk": "sandbox#h1", "currency_mode": "sandbox", "table_id": "t1", "hand_id": "h1",
		"opponents": []map[string]any{{"player_id": testU, "name": "Alice", "avatar_url": avatar, "hole_cards": []string{"As", "Ad"}}}})
	put(t, db, env, TableActionLog, map[string]any{"pk": "t1#h1", "sk": "0000000001", "player_id": testU, "message": "oi, sou a Alice",
		"frame": map[string]any{"seats": []map[string]any{{"player_id": testU, "name": "Alice"}, {"player_id": testO, "name": "Bob"}}}})
	put(t, db, env, TableTableStateHistory, map[string]any{"pk": "t1", "sk": "100",
		"state": map[string]any{"Players": []map[string]any{{"id": testU, "name": "Alice", "avatar_url": avatar}, {"id": testO, "name": "Bob"}},
			"Payouts": map[string]any{testU: 10}}})
	put(t, db, env, TableTableHighlights, map[string]any{"pk": "t1", "sk": "2026-10-07",
		"winners": []map[string]any{{"player_id": testU, "name": "Alice", "payout": 10}}})
	put(t, db, env, TableHandReveals, map[string]any{"pk": "h1", "winner_id": testU,
		"player_hands": map[string]any{testU: map[string]any{"cards": []string{"As", "Ad"}}, testO: map[string]any{"cards": []string{"2c", "7d"}}}})
	put(t, db, env, TableHandRevealPayments, map[string]any{"pk": "h1#" + testU})
	put(t, db, env, TableRecentPlayers, map[string]any{"pk": testO, "sk": "hand#h1", "opponents": []string{testU}})
	put(t, db, env, TableMatchups, map[string]any{"pk": "pair#sandbox#" + testU + "#" + testO, "hands_together": 1})
	objects.put("archive", "t1/h1/1-e1.jsonl",
		[]byte(`{"player_id":"`+testU+`","message":"oi, sou a Alice","amount":5,"frame":{"seats":[{"player_id":"`+testU+`","name":"Alice"},{"player_id":"`+testO+`","name":"Bob"}]}}`+"\n"))
}
```

- [ ] **Step 3: Write the failing test** — `api/internal/userpurge/purger_integration_test.go`:

```go
//go:build integration

package userpurge

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/erasure"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
)

// Other players keep their history of a hand they shared with the erased
// user, under one pseudonym and without the user's name, avatar or chat.
// Purge runs twice: a redelivered message must finish cleanly.
func TestPurgeAnonymizesSharedHands(t *testing.T) {
	db := testClient(t)
	objects := newFakeS3()
	p, env := newTestPurger(t, db, objects, cache.NewMemoryBackend(10), noBlockers)
	seedSharedHand(t, db, env, objects)

	for i := range 2 {
		ack, err := p.Purge(context.Background(), purgeMsg())
		if err != nil || ack.Result != erasure.ResultDone {
			t.Fatalf("run %d: %+v %v", i, ack, err)
		}
		if i == 0 && ack.Counts[TablePlayerHands+"_erased"] != 1 {
			t.Fatalf("counts: %+v", ack.Counts)
		}
	}

	hands := scanTable(t, db, env, TablePlayerHands)
	if len(hands) != 1 {
		t.Fatalf("want only the opponent's hand row, got %d", len(hands))
	}
	var bobs sessionlog.HandItem
	if err := attributevalue.UnmarshalMap(hands[0], &bobs); err != nil {
		t.Fatal(err)
	}
	pseudo := bobs.Opponents[0].PlayerID
	if !strings.HasPrefix(pseudo, "anon_") || bobs.Opponents[0].Name != "" || bobs.Opponents[0].AvatarURL != "" || len(bobs.Opponents[0].HoleCards) != 2 {
		t.Fatalf("opponent summary: %+v", bobs.Opponents[0])
	}
	logRow := scanTable(t, db, env, TableActionLog)[0]
	if stringAttr(logRow, "player_id") != pseudo {
		t.Fatalf("one pseudonym per run: hand row %s, action log %s", pseudo, stringAttr(logRow, "player_id"))
	}
	if _, ok := logRow["message"]; ok {
		t.Fatal("the erased user's chat survived")
	}
	if !mentions(logRow, "Bob") {
		t.Fatal("the opponent's seat must stay")
	}
	for _, table := range []string{TablePlayerHands, TableActionLog, TableTableStateHistory, TableTableHighlights,
		TableHandReveals, TableHandRevealPayments, TableRecentPlayers, TableMatchups} {
		for _, it := range scanTable(t, db, env, table) {
			if mentions(it, testU) || mentions(it, "Alice") {
				t.Fatalf("%s still mentions the user: %v", table, it)
			}
		}
	}
	for _, table := range []string{TableHandRevealPayments, TableMatchups} {
		if n := len(scanTable(t, db, env, table)); n != 0 {
			t.Fatalf("%s: want erased, %d rows left", table, n)
		}
	}
	archived := string(objects.objects["archive/t1/h1/1-e1.jsonl"])
	if strings.Contains(archived, testU) || strings.Contains(archived, "Alice") || !strings.Contains(archived, pseudo) || !strings.Contains(archived, "Bob") {
		t.Fatalf("archive: %s", archived)
	}
}
```

- [ ] **Step 4: Run it and watch it fail**

Run (DynamoDB Local up): `cd api && go test -tags integration ./internal/userpurge/ -run TestPurgeAnonymizesSharedHands -count=1`
Expected: FAIL with `run 0: ... counts: map[]` (no steps yet).

- [ ] **Step 5: Implement the step** — in `purger.go`, set `steps` to:

```go
func (r *run) steps() []func(context.Context) error {
	return []func(context.Context) error{r.sharedHands}
}
```

Append to `purger.go`, adding imports `"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"`, `"gopkg.aoctech.app/poker/api/internal/matchup"` and `"gopkg.aoctech.app/poker/api/internal/sessionlog"`:

```go
// sharedHands walks the user's own hand rows, the only index of the hands
// they played. Per hand, it anonymizes everything others keep about it and
// then deletes the user's row, so a retry resumes at the first hand left.
func (r *run) sharedHands(ctx context.Context) error {
	return r.p.eachItem(ctx, r.p.partition(TablePlayerHands, r.sub), func(it item) error {
		var h sessionlog.HandItem
		if err := attributevalue.UnmarshalMap(it, &h); err != nil {
			return fmt.Errorf("userpurge: decode hand %s: %w", stringAttr(it, "sk"), err)
		}
		if err := r.tableRows(ctx, h.TableID); err != nil {
			return err
		}
		if err := r.handRows(ctx, h); err != nil {
			return err
		}
		return r.deleteItem(ctx, TablePlayerHands, keyOf(it))
	})
}

// tableRows anonymizes the table-keyed rows (state snapshots, daily
// highlights) once per table per run.
func (r *run) tableRows(ctx context.Context, tableID string) error {
	if tableID == "" || r.tables[tableID] {
		return nil
	}
	for _, table := range []string{TableTableStateHistory, TableTableHighlights} {
		if err := r.p.eachItem(ctx, r.p.partition(table, tableID), func(it item) error {
			return r.rewrite(ctx, table, it)
		}); err != nil {
			return err
		}
	}
	r.tables[tableID] = true
	return nil
}

// handRows anonymizes one hand everywhere other players see it.
func (r *run) handRows(ctx context.Context, h sessionlog.HandItem) error {
	if err := r.p.eachItem(ctx, r.p.partition(TableActionLog, h.TableID+"#"+h.HandID), func(it item) error {
		return r.rewrite(ctx, TableActionLog, it)
	}); err != nil {
		return err
	}
	if err := r.archive(ctx, h.TableID, h.HandID); err != nil {
		return err
	}
	if err := r.rewriteKey(ctx, TableHandReveals, key(h.HandID, "")); err != nil {
		return err
	}
	if err := r.deleteItem(ctx, TableHandRevealPayments, key(h.HandID+"#"+r.sub, "")); err != nil {
		return err
	}
	for _, o := range h.Opponents {
		if o.PlayerID == "" || o.PlayerID == r.sub {
			continue
		}
		if err := r.rewriteKey(ctx, TablePlayerHands, key(o.PlayerID, h.SK)); err != nil {
			return err
		}
		if err := r.rewriteKey(ctx, TableRecentPlayers, key(o.PlayerID, "hand#"+h.HandID)); err != nil {
			return err
		}
		if err := r.deleteItem(ctx, TableMatchups, key(matchup.PairKey(h.CurrencyMode, r.sub, o.PlayerID), "")); err != nil {
			return err
		}
	}
	return nil
}
```

Append to `s3.go`, adding imports `"bytes"`, `"fmt"`, `"io"`, `"github.com/aws/aws-sdk-go-v2/aws"`:

```go
// archive rewrites every action-log archive object of one hand. The archiver
// keys objects by the action-log pk with '#' replaced by '/'. It archives
// only INSERT stream records, so the purge's own rewrites (MODIFY) are not
// archived again.
func (r *run) archive(ctx context.Context, tableID, handID string) error {
	if r.p.cfg.ArchiveBucket == "" {
		return nil
	}
	in := &s3.ListObjectsV2Input{Bucket: aws.String(r.p.cfg.ArchiveBucket), Prefix: aws.String(tableID + "/" + handID + "/")}
	for {
		out, err := r.p.objects.ListObjectsV2(ctx, in)
		if err != nil {
			return fmt.Errorf("userpurge: list archive %s/%s: %w", tableID, handID, err)
		}
		for _, obj := range out.Contents {
			if err := r.archiveObject(ctx, aws.ToString(obj.Key)); err != nil {
				return err
			}
		}
		if !aws.ToBool(out.IsTruncated) {
			return nil
		}
		in.ContinuationToken = out.NextContinuationToken
	}
}

func (r *run) archiveObject(ctx context.Context, objectKey string) error {
	bucket := aws.String(r.p.cfg.ArchiveBucket)
	got, err := r.p.objects.GetObject(ctx, &s3.GetObjectInput{Bucket: bucket, Key: aws.String(objectKey)})
	if err != nil {
		return fmt.Errorf("userpurge: get archive %s: %w", objectKey, err)
	}
	body, err := io.ReadAll(got.Body)
	_ = got.Body.Close()
	if err != nil {
		return fmt.Errorf("userpurge: read archive %s: %w", objectKey, err)
	}
	out, changed, err := anonymizeJSONLines(body, r.sub, r.pseudo)
	if err != nil {
		return fmt.Errorf("userpurge: parse archive %s: %w", objectKey, err)
	}
	if !changed {
		return nil
	}
	if _, err := r.p.objects.PutObject(ctx, &s3.PutObjectInput{Bucket: bucket, Key: aws.String(objectKey), Body: bytes.NewReader(out)}); err != nil {
		return fmt.Errorf("userpurge: put archive %s: %w", objectKey, err)
	}
	r.count("action_log_archive_objects_anonymized", 1)
	return nil
}
```

- [ ] **Step 6: Run the tests**

Run: `cd api && go vet -tags integration ./... && go test -tags integration ./internal/userpurge/ -count=1 && go test ./internal/userpurge/ ./internal/matchup/ -count=1`
Expected: `ok` three times.

- [ ] **Step 7: Commit**

```bash
git add api/internal/userpurge api/internal/matchup/store.go
git commit -m "feat(api): purge anonymizes hands the erased user shared with others"
```

---

### Task 7: Purge — everything else

**Files:**
- Modify: `api/internal/userpurge/purger.go`, `api/internal/userpurge/s3.go`
- Test: `api/internal/userpurge/purger_integration_test.go`

**Interfaces:**
- Consumes: Tasks 5–6, `roomstore.CurrencyModeSandbox` / `roomstore.CurrencyModeReal`, `cache.Backend.Delete` / `DeletePrefix`.
- Produces: run methods `socialGraph`, `actorEvents`, `notesAboutUser`, `avatarReports`, `reports`, `leaderboard`, `ownRows`, `ownByIndex`, `cacheKeys`, `avatars`. `steps()` becomes final.

Key formats, each from its owning package:

| Data | Format | Source |
|---|---|---|
| Session guard pk | `buyinguard#<sub>` | `sessionlog` |
| Stats pk | `stats#<mode>#<sub>` | `pokerstats` |
| Avatar-report guard pk | `avreport#<target>#<reporter>`, with `target_id` / `reporter_id` | `player/store.go` |
| Legacy avatar reporters | `avatar_reporters` String Set | `player/store.go` |
| Invite guard sk | `invite_guard#<actor>#<room>` | `social/event_store.go` |
| Presence keys | `poker:presence:connections:<sub>`, `poker:presence:table:<sub>` | `presence/valkey.go` |
| Chat prefs key | `chatprefs:extra:<sub>` | `chatprefs/cache.go` |
| Pocket-pair key | `poker:achv:lastpocketpair:<mode>#<sub>` | `achievements/service.go` |
| Reaction ownership keys | `reaction-owned:<sub>:<id>` | `reactionpurchase/ownershipcache.go` |
| Avatar objects | `av/<sub>/<v>.jpg`, `up/<sub>/<v>.jpg` | `avatar` |

- [ ] **Step 1: Write the failing tests** — append to `purger_integration_test.go`:

```go
// After a purge, no table holds the user's sub or display name, and what is
// kept (other players' rows, reports, leaderboard) is anonymized.
func TestPurgeLeavesNoTraceOfTheUser(t *testing.T) {
	ctx := context.Background()
	db := testClient(t)
	objects := newFakeS3()
	valkey := cache.NewMemoryBackend(100)
	p, env := newTestPurger(t, db, objects, valkey, noBlockers)
	seedSharedHand(t, db, env, objects)
	third := "33333333-3333-4333-8333-333333333333"

	// Rows the user owns outright.
	for table, sk := range map[string]string{
		TableAchievementProgress: "sandbox#_progress", TableDailyReward: "streak", TablePlayerSessions: "1",
		TableHandMeta: "hand#h1", TableSandboxPurchases: "p1", TableReactionEntitlements: "fire", TableReactionPurchases: "p2",
		TableCosmeticEntitlements: "deck#casino", TableCosmeticPurchases: "p3", TableCosmeticLoadouts: "l1",
		TableBotCheckContests: "c1", TablePromoRedemptions: "CODE", TableTableEntitlements: "ent#t1",
		TableRecentPlayers: "hand#h1", TableSocialEvents: "e2",
	} {
		put(t, db, env, table, map[string]any{"pk": testU, "sk": sk, "name": "Alice's"})
	}
	for _, table := range []string{TableChatPrefs, TableWalletAlertPrefs} {
		put(t, db, env, table, map[string]any{"pk": testU})
	}
	put(t, db, env, TablePlayerProfiles, map[string]any{"pk": testU, "name": "Alice", "friend_code": "PKR-ALICE"})
	put(t, db, env, TablePlayerSessions, map[string]any{"pk": "buyinguard#" + testU, "sk": "k1"})
	put(t, db, env, TablePokerStats, map[string]any{"pk": "stats#sandbox#" + testU})
	put(t, db, env, TablePokerStats, map[string]any{"pk": "stats#real#" + testU})
	put(t, db, env, TableHandShares, map[string]any{"pk": "tok1", "owner_id": testU, "created_at": 1})
	put(t, db, env, TablePendingCashouts, map[string]any{"pk": "t1#" + testU + "#cashout", "sk": "pending", "player_id": testU,
		"resolved": true, "recorded_at": "2026-10-01T00:00:00Z"})
	// Rows about the user in other players' partitions.
	putAV(t, db, env, TablePlayerProfiles, map[string]types.AttributeValue{"pk": str(testO), "name": str("Bob"),
		"avatar_reporters": &types.AttributeValueMemberSS{Value: []string{testU, third}}})
	put(t, db, env, TablePlayerProfiles, map[string]any{"pk": "avreport#" + testO + "#" + testU, "target_id": testO, "reporter_id": testU})
	put(t, db, env, TablePlayerNotes, map[string]any{"pk": testU, "sk": testO, "note": "Bob tilts"})
	put(t, db, env, TablePlayerNotes, map[string]any{"pk": testO, "sk": testU, "note": "Alice bluffs"})
	put(t, db, env, TableSocialEdges, map[string]any{"pk": testU, "sk": testO, "relationship": "friends"})
	put(t, db, env, TableSocialEdges, map[string]any{"pk": testO, "sk": testU, "relationship": "friends"})
	put(t, db, env, TableSocialEvents, map[string]any{"pk": testO, "sk": "e1", "actor_id": testU})
	put(t, db, env, TableSocialEvents, map[string]any{"pk": testO, "sk": "invite_guard#" + testU + "#r1"})
	put(t, db, env, TableSocialEvents, map[string]any{"pk": testO, "sk": "e3", "actor_id": third})
	put(t, db, env, TablePlayerReports, map[string]any{"pk": testU, "sk": testO + "#abc", "reporter_id": testO,
		"evidence_message": "Alice xingou", "gsi_reporter_pk": testO, "gsi_reporter_sk": "1#abc"})
	put(t, db, env, TablePlayerReports, map[string]any{"pk": testO, "sk": testU + "#def", "reporter_id": testU,
		"details": "Bob trapaceia, diz Alice", "gsi_reporter_pk": testU, "gsi_reporter_sk": "1#def"})
	put(t, db, env, TableLeaderboardStats, map[string]any{"pk": testU, "sk": "stats#sandbox", "player_name": "Alice", "hands_played": 3})
	objects.put("avatars", "av/"+testU+"/1.jpg", []byte("jpg"))
	objects.put("avatars", "up/"+testU+"/2.jpg", []byte("jpg"))
	objects.put("avatars", "av/"+testO+"/1.jpg", []byte("jpg"))
	for _, k := range []string{"poker:presence:table:" + testU, "reaction-owned:" + testU + ":fire", "chatprefs:extra:" + testU} {
		if err := valkey.Set(ctx, k, []byte("1"), 60); err != nil {
			t.Fatal(err)
		}
	}

	for i := range 2 {
		if ack, err := p.Purge(ctx, purgeMsg()); err != nil || ack.Result != erasure.ResultDone {
			t.Fatalf("run %d: %+v %v", i, ack, err)
		}
	}

	for table := range purgeTables {
		for _, it := range scanTable(t, db, env, table) {
			if mentions(it, testU) || mentions(it, "Alice") {
				t.Fatalf("%s still mentions the user: %v", table, it)
			}
		}
	}
	reports := scanTable(t, db, env, TablePlayerReports)
	if len(reports) != 2 {
		t.Fatalf("reports are retained anonymized: want 2, got %d", len(reports))
	}
	for _, it := range reports {
		if _, ok := it["ttl"]; !ok {
			t.Fatalf("a retained report must expire (1 year): %v", it)
		}
	}
	board := scanTable(t, db, env, TableLeaderboardStats)
	if len(board) != 1 || !strings.HasPrefix(stringAttr(board[0], "pk"), "anon_") {
		t.Fatalf("leaderboard row must be re-keyed: %v", board)
	}
	profiles := scanTable(t, db, env, TablePlayerProfiles)
	if len(profiles) != 1 || stringAttr(profiles[0], "name") != "Bob" {
		t.Fatalf("only Bob's profile must remain: %v", profiles)
	}
	if set := profiles[0]["avatar_reporters"].(*types.AttributeValueMemberSS).Value; len(set) != 1 || set[0] != third {
		t.Fatalf("avatar_reporters: %v", set)
	}
	events := scanTable(t, db, env, TableSocialEvents)
	if len(events) != 1 || stringAttr(events[0], "sk") != "e3" {
		t.Fatalf("only the third party's event must remain: %v", events)
	}
	if n := len(scanTable(t, db, env, TableSocialEdges)); n != 0 {
		t.Fatalf("social edges: %d left", n)
	}
	if left := objects.keys("avatars", ""); len(left) != 1 || left[0] != "av/"+testO+"/1.jpg" {
		t.Fatalf("avatars left: %v", left)
	}
	for _, k := range []string{"poker:presence:table:" + testU, "reaction-owned:" + testU + ":fire", "chatprefs:extra:" + testU} {
		if _, found, _ := valkey.Get(ctx, k); found {
			t.Fatalf("valkey key %s survived", k)
		}
	}
}

// A blocker that appeared since the lock stops the purge before anything is erased.
func TestPurgeBlockedErasesNothing(t *testing.T) {
	db := testClient(t)
	blocked := func(context.Context, string) ([]erasure.Blocker, error) {
		return []erasure.Blocker{{Code: BlockerSeatedAtTable}}, nil
	}
	p, env := newTestPurger(t, db, newFakeS3(), cache.NewMemoryBackend(10), blocked)
	put(t, db, env, TablePlayerProfiles, map[string]any{"pk": testU, "name": "Alice"})
	ack, err := p.Purge(context.Background(), purgeMsg())
	if err != nil || ack.Result != erasure.ResultBlocked || len(ack.Blockers) != 1 {
		t.Fatalf("got %+v %v", ack, err)
	}
	if n := len(scanTable(t, db, env, TablePlayerProfiles)); n != 1 {
		t.Fatal("a blocked purge must not erase anything")
	}
}
```

Add `"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"` to this file's imports (used by `putAV` and the `avatar_reporters` assertion).

- [ ] **Step 2: Run them and watch them fail**

Run: `cd api && go test -tags integration ./internal/userpurge/ -run 'TestPurgeLeavesNoTrace|TestPurgeBlocked' -count=1`
Expected: `TestPurgeLeavesNoTraceOfTheUser` FAILs with `... still mentions the user`. `TestPurgeBlockedErasesNothing` passes already (the blocker check exists since Task 5).

- [ ] **Step 3: Implement** — in `purger.go`, set the final `steps`:

```go
func (r *run) steps() []func(context.Context) error {
	return []func(context.Context) error{
		r.sharedHands, r.socialGraph, r.notesAboutUser, r.avatarReports,
		r.reports, r.leaderboard, r.ownRows, r.cacheKeys, r.avatars,
	}
}
```

Append to `purger.go`, adding imports `"strconv"`, `"strings"`, `"github.com/aws/aws-sdk-go-v2/aws"`, `"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"` and `"gopkg.aoctech.app/poker/api/internal/roomstore"`:

```go
// socialGraph erases both directions of every edge, and first the events and
// invite guards the user caused in each counterpart's inbox (found only
// through the edge, which is why the edge goes last).
func (r *run) socialGraph(ctx context.Context) error {
	return r.p.eachItem(ctx, r.p.partition(TableSocialEdges, r.sub), func(edge item) error {
		other := stringAttr(edge, "sk")
		if err := r.actorEvents(ctx, other); err != nil {
			return err
		}
		if err := r.deleteItem(ctx, TableSocialEdges, key(other, r.sub)); err != nil {
			return err
		}
		return r.deleteItem(ctx, TableSocialEdges, keyOf(edge))
	})
}

func (r *run) actorEvents(ctx context.Context, recipient string) error {
	in := r.p.partition(TableSocialEvents, recipient)
	in.FilterExpression = aws.String("actor_id = :sub OR begins_with(sk, :guard)")
	in.ExpressionAttributeValues[":sub"] = str(r.sub)
	in.ExpressionAttributeValues[":guard"] = str("invite_guard#" + r.sub + "#")
	var keys []item
	if err := r.p.eachItem(ctx, in, func(it item) error {
		keys = append(keys, keyOf(it))
		return nil
	}); err != nil {
		return err
	}
	r.count(TableSocialEvents+"_erased", len(keys))
	return r.p.deleteKeys(ctx, TableSocialEvents, keys)
}

// notesAboutUser erases notes others wrote about the user (sk = sub, no index).
func (r *run) notesAboutUser(ctx context.Context) error {
	var keys []item
	if err := r.p.eachScan(ctx, &dynamodb.ScanInput{
		TableName:                 aws.String(r.p.table(TablePlayerNotes)),
		FilterExpression:          aws.String("sk = :sub"),
		ExpressionAttributeValues: item{":sub": str(r.sub)},
		ConsistentRead:            aws.Bool(true),
	}, func(it item) error {
		keys = append(keys, keyOf(it))
		return nil
	}); err != nil {
		return err
	}
	r.count(TablePlayerNotes+"_erased", len(keys))
	return r.p.deleteKeys(ctx, TablePlayerNotes, keys)
}

// avatarReports erases the avatar-report guard rows by or about the user and
// removes the user from the legacy avatar_reporters sets (no index: scan).
func (r *run) avatarReports(ctx context.Context) error {
	return r.p.eachScan(ctx, &dynamodb.ScanInput{
		TableName:                 aws.String(r.p.table(TablePlayerProfiles)),
		FilterExpression:          aws.String("(begins_with(pk, :guard) AND (target_id = :sub OR reporter_id = :sub)) OR contains(avatar_reporters, :sub)"),
		ExpressionAttributeValues: item{":guard": str("avreport#"), ":sub": str(r.sub)},
		ConsistentRead:            aws.Bool(true),
	}, func(it item) error {
		if strings.HasPrefix(stringAttr(it, "pk"), "avreport#") {
			return r.deleteItem(ctx, TablePlayerProfiles, keyOf(it))
		}
		if _, err := r.p.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
			TableName:                 aws.String(r.p.table(TablePlayerProfiles)),
			Key:                       keyOf(it),
			UpdateExpression:          aws.String("DELETE avatar_reporters :sub"),
			ExpressionAttributeValues: item{":sub": &types.AttributeValueMemberSS{Value: []string{r.sub}}},
		}); err != nil {
			return fmt.Errorf("userpurge: avatar_reporters: %w", err)
		}
		r.count(TablePlayerProfiles+"_anonymized", 1)
		return nil
	})
}

// reports keeps reports for 1 year under the pseudonym (plan ruling R9) and
// drops the text the erased user wrote: their copied chat on reports against
// them, their details on reports they filed.
func (r *run) reports(ctx context.Context) error {
	expires := r.p.now().Add(reportRetention).Unix()
	against := func(it item) error { return r.retainReport(ctx, it, "evidence_message", expires) }
	if err := r.p.eachItem(ctx, r.p.partition(TablePlayerReports, r.sub), against); err != nil {
		return err
	}
	return r.p.eachItem(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(r.p.table(TablePlayerReports)),
		IndexName:                 aws.String(gsiReportReporter),
		KeyConditionExpression:    aws.String("gsi_reporter_pk = :sub"),
		ExpressionAttributeValues: item{":sub": str(r.sub)},
	}, func(it item) error {
		// The index lags the table: re-read, so a retry cannot re-key twice.
		cur, err := r.getItem(ctx, TablePlayerReports, keyOf(it))
		if err != nil || len(cur) == 0 {
			return err
		}
		return r.retainReport(ctx, cur, "details", expires)
	})
}

func (r *run) retainReport(ctx context.Context, it item, dropText string, expires int64) error {
	next, _ := anonymizeMap(it, r.sub, r.pseudo)
	delete(next, dropText)
	if cur, ok := next["ttl"].(*types.AttributeValueMemberN); !ok || mustInt(cur.Value) > expires {
		next["ttl"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(expires, 10)}
	}
	return r.rekey(ctx, TablePlayerReports, it, next)
}

func mustInt(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 1<<63 - 1
	}
	return n
}

// leaderboard re-keys the user's board rows to the pseudonym so other
// players' ranks do not move, and drops the display name.
func (r *run) leaderboard(ctx context.Context) error {
	return r.p.eachItem(ctx, r.p.partition(TableLeaderboardStats, r.sub), func(it item) error {
		next, _ := anonymizeMap(it, r.sub, r.pseudo)
		delete(next, "player_name")
		return r.rekey(ctx, TableLeaderboardStats, it, next)
	})
}

// erasedBySub are the tables partitioned by the user's sub: the whole
// partition is the user's own data.
var erasedBySub = []string{
	TableAchievementProgress, TableDailyReward, TablePlayerSessions, TablePlayerNotes, TableHandMeta,
	TableSandboxPurchases, TableReactionEntitlements, TableReactionPurchases, TableCosmeticEntitlements,
	TableCosmeticPurchases, TableCosmeticLoadouts, TableChatPrefs, TableBotCheckContests,
	TableWalletAlertPrefs, TablePromoRedemptions, TableTableEntitlements, TableSocialEvents,
	TableRecentPlayers, TablePlayerHands,
}

func (r *run) ownRows(ctx context.Context) error {
	for _, table := range erasedBySub {
		if err := r.erasePartition(ctx, table, r.sub); err != nil {
			return err
		}
	}
	for _, part := range []struct{ table, pk string }{
		{TablePlayerSessions, "buyinguard#" + r.sub},
		{TablePokerStats, "stats#" + roomstore.CurrencyModeSandbox + "#" + r.sub},
		{TablePokerStats, "stats#" + roomstore.CurrencyModeReal + "#" + r.sub},
	} {
		if err := r.erasePartition(ctx, part.table, part.pk); err != nil {
			return err
		}
	}
	if err := r.ownByIndex(ctx, TableHandShares, gsiHandShareOwner, "owner_id", false); err != nil {
		return err
	}
	// Unresolved settlements are a blocker; resolvedOnly makes sure a money
	// row is never deleted even if one raced the eligibility check.
	if err := r.ownByIndex(ctx, TablePendingCashouts, gsiPlayerSettlements, "player_id", true); err != nil {
		return err
	}
	// The profile goes last: nothing above reads it, so any earlier crash
	// leaves a row a retry still finds.
	return r.erasePartition(ctx, TablePlayerProfiles, r.sub)
}

func (r *run) ownByIndex(ctx context.Context, table, index, attr string, resolvedOnly bool) error {
	in := &dynamodb.QueryInput{
		TableName:                 aws.String(r.p.table(table)),
		IndexName:                 aws.String(index),
		KeyConditionExpression:    aws.String("#a = :sub"),
		ExpressionAttributeNames:  map[string]string{"#a": attr},
		ExpressionAttributeValues: item{":sub": str(r.sub)},
	}
	if resolvedOnly {
		in.FilterExpression = aws.String("resolved = :true")
		in.ExpressionAttributeValues[":true"] = &types.AttributeValueMemberBOOL{Value: true}
	}
	var keys []item
	if err := r.p.eachItem(ctx, in, func(it item) error {
		keys = append(keys, keyOf(it))
		return nil
	}); err != nil {
		return err
	}
	r.count(table+"_erased", len(keys))
	return r.p.deleteKeys(ctx, table, keys)
}

// cacheKeys deletes the user's Valkey keys. Rate-limit windows, the
// leaderboard rank mirror and table-keyed keys expire within minutes (R11).
func (r *run) cacheKeys(ctx context.Context) error {
	for _, k := range []string{
		"poker:presence:connections:" + r.sub,
		"poker:presence:table:" + r.sub,
		"chatprefs:extra:" + r.sub,
		"poker:achv:lastpocketpair:" + roomstore.CurrencyModeSandbox + "#" + r.sub,
		"poker:achv:lastpocketpair:" + roomstore.CurrencyModeReal + "#" + r.sub,
	} {
		if err := r.p.cache.Delete(ctx, k); err != nil {
			return fmt.Errorf("userpurge: cache delete: %w", err)
		}
	}
	if err := r.p.cache.DeletePrefix(ctx, "reaction-owned:"+r.sub+":"); err != nil {
		return fmt.Errorf("userpurge: cache delete prefix: %w", err)
	}
	return nil
}
```

Append to `s3.go`, adding the import `s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"`:

```go
// avatars deletes every version and delete marker of the user's avatars
// (both prefixes; the bucket is versioned in prod, inventory §2).
func (r *run) avatars(ctx context.Context) error {
	if r.p.cfg.AvatarBucket == "" {
		return nil
	}
	bucket := aws.String(r.p.cfg.AvatarBucket)
	for _, prefix := range []string{"av/" + r.sub + "/", "up/" + r.sub + "/"} {
		in := &s3.ListObjectVersionsInput{Bucket: bucket, Prefix: aws.String(prefix)}
		for {
			out, err := r.p.objects.ListObjectVersions(ctx, in)
			if err != nil {
				return fmt.Errorf("userpurge: list avatar versions: %w", err)
			}
			var ids []s3types.ObjectIdentifier
			for _, v := range out.Versions {
				ids = append(ids, s3types.ObjectIdentifier{Key: v.Key, VersionId: v.VersionId})
			}
			for _, d := range out.DeleteMarkers {
				ids = append(ids, s3types.ObjectIdentifier{Key: d.Key, VersionId: d.VersionId})
			}
			if len(ids) > 0 {
				res, err := r.p.objects.DeleteObjects(ctx, &s3.DeleteObjectsInput{Bucket: bucket, Delete: &s3types.Delete{Objects: ids, Quiet: aws.Bool(true)}})
				if err != nil {
					return fmt.Errorf("userpurge: delete avatars: %w", err)
				}
				if len(res.Errors) > 0 {
					return fmt.Errorf("userpurge: delete avatar %s: %s", aws.ToString(res.Errors[0].Key), aws.ToString(res.Errors[0].Message))
				}
				r.count("avatar_objects_erased", len(ids))
			}
			if !aws.ToBool(out.IsTruncated) {
				break
			}
			in.KeyMarker, in.VersionIdMarker = out.NextKeyMarker, out.NextVersionIdMarker
		}
	}
	return nil
}
```

- [ ] **Step 4: Run the tests**

Run: `cd api && go vet ./... && go vet -tags integration ./... && go test -tags integration ./internal/userpurge/ -count=1 && go test ./internal/userpurge/ -count=1`
Expected: `ok` for both runs.

- [ ] **Step 5: Commit**

```bash
git add api/internal/userpurge
git commit -m "feat(api): purge erases the user's own rows, social graph, reports, cache and avatars"
```

---

### Task 8: Consumer and ack wiring

**Files:**
- Modify: `api/go.mod`, `api/go.sum`, `api/internal/config/config.go`, `api/internal/app/erasure.go`, `api/internal/app/app.go`
- Test: `api/internal/config/config_test.go`

**Interfaces:**
- Consumes:
  - `erasure.NewConsumer(sqsClient, queueURL, service string, store *erasure.Store, purge erasure.PurgeFunc, acks *erasure.AckClient) *erasure.Consumer` and its `Run(ctx) error`;
  - `erasure.NewAckClient(httpClient *http.Client, ackURL string, tokens *oauth2client.TokenManager)`;
  - `oauth2client.New(httpClient, cache, tokenURL, clientID, clientSecret, scope string)`;
  - `userpurge.NewPurger`, `newErasureStore`, `newEligibility`.
- Produces:
  - `config.Config.ErasureQueueURL` (`ERASURE_QUEUE_URL`), `config.Config.ActionLogArchiveBucket` (`ACTION_LOG_ARCHIVE_BUCKET`), `config.Config.ErasureAckClientID` (`ERASURE_ACK_CLIENT_ID`) and `config.Config.ErasureAckClientSecret` (`ERASURE_ACK_CLIENT_SECRET`), all required in prod (the ack client is dedicated, ruling R2);
  - `func startErasureConsumer(lc fx.Lifecycle, cfg *config.Config, db *dynamodb.Client, c cache.Backend, store *erasure.Store, elig *userpurge.Eligibility) error`.

- [ ] **Step 1: Write the failing test** — append to `api/internal/config/config_test.go`, adding `"strings"` to its imports if absent:

```go
// Prod must never run without the deletion consumer: account would wait for
// poker's ack forever (saga protocol §7).
func TestLoadRequiresErasureWiringInProd(t *testing.T) {
	t.Setenv("ENVIRONMENT", "prod")
	t.Setenv("VALKEY_URL", "redis://localhost:6379")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://poker.aoctech.app")
	t.Setenv("WALLET_WEBHOOK_HMAC_SECRET", "s")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "ERASURE_QUEUE_URL") {
		t.Fatalf("want ERASURE_QUEUE_URL error, got %v", err)
	}
	t.Setenv("ERASURE_QUEUE_URL", "https://sqs.us-east-1.amazonaws.com/1/prod-poker-user-erasure")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "ACTION_LOG_ARCHIVE_BUCKET") {
		t.Fatalf("want ACTION_LOG_ARCHIVE_BUCKET error, got %v", err)
	}
	t.Setenv("ACTION_LOG_ARCHIVE_BUCKET", "poker-action-log-archive-prod")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "ERASURE_ACK_CLIENT_ID") {
		t.Fatalf("want ERASURE_ACK_CLIENT_ID error, got %v", err)
	}
	t.Setenv("ERASURE_ACK_CLIENT_ID", "poker-erasure")
	t.Setenv("ERASURE_ACK_CLIENT_SECRET", "secret")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("complete prod config rejected: %v", err)
	}
	if cfg.ErasureAccountClientID != "accounts" {
		t.Fatalf("ERASURE_ACCOUNT_CLIENT_ID default: %q", cfg.ErasureAccountClientID)
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `cd api && go test ./internal/config/ -run TestLoadRequiresErasureWiringInProd -count=1`
Expected: FAIL with `want ERASURE_QUEUE_URL error, got <nil>`.

- [ ] **Step 3: Implement the config** — in `config.go`, below `ErasureAccountClientID` (Task 3), add:

```go
	// ErasureQueueURL is poker's SQS queue on ctech-account's user-erasure
	// topic; ActionLogArchiveBucket is the action-log S3 archive the purge
	// rewrites. Both come from /etc/app-static.env (cdk/lib/api-stack.ts).
	ErasureQueueURL        string `env:"ERASURE_QUEUE_URL"`
	ActionLogArchiveBucket string `env:"ACTION_LOG_ARCHIVE_BUCKET"`
	// ErasureAckClientID/Secret are poker's dedicated confidential client for
	// acks to ctech-account (scope internal:account:erasure-ack only). Kept
	// apart from POKER_CLIENT_ID so the wallet credentials never carry it.
	// From SSM /ctech/<env>/poker/erasure-ack-client-{id,secret}.
	ErasureAckClientID     string `env:"ERASURE_ACK_CLIENT_ID"`
	ErasureAckClientSecret string `env:"ERASURE_ACK_CLIENT_SECRET"`
```

In `Load()`, before the `if cfg.CtechJWKSURL == "" ...` block, add:

```go
	if cfg.ErasureQueueURL == "" && cfg.Env == "prod" {
		return nil, fmt.Errorf("config: ERASURE_QUEUE_URL must be set in production — without it account deletions are never purged or acked")
	}
	if cfg.ActionLogArchiveBucket == "" && cfg.Env == "prod" {
		return nil, fmt.Errorf("config: ACTION_LOG_ARCHIVE_BUCKET must be set in production — the purge must anonymize the action-log archive")
	}
	if (cfg.ErasureAckClientID == "" || cfg.ErasureAckClientSecret == "") && cfg.Env == "prod" {
		return nil, fmt.Errorf("config: ERASURE_ACK_CLIENT_ID and ERASURE_ACK_CLIENT_SECRET must be set in production — without them no erasure is ever acked")
	}
```

- [ ] **Step 4: Add the SQS client and the consumer**

```bash
cd api && go get github.com/aws/aws-sdk-go-v2/service/sqs && go mod tidy
```

Replace `api/internal/app/erasure.go` with the final version:

```go
package app

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/gofiber/fiber/v3"
	"go.uber.org/fx"
	"gopkg.aoctech.app/api-commons/awsconfig"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/erasure"
	"gopkg.aoctech.app/api-commons/jwtverify"
	"gopkg.aoctech.app/api-commons/oauth2client"
	v1 "gopkg.aoctech.app/poker/api/internal/api/v1"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/reconcile"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
	"gopkg.aoctech.app/poker/api/internal/userpurge"
)

// Account-deletion saga participant wiring
// (docs/plans/2026-10-07-account-deletion-participant.md).

const (
	// erasureService is poker's participant id in erasure messages and the SNS filter.
	erasureService = "poker"
	// erasureAckScope is the only scope of poker's dedicated ack client
	// (ERASURE_ACK_CLIENT_ID), granted by an operator in ctech-account.
	erasureAckScope = "internal:account:erasure-ack"
)

// newErasureStore binds {env}_poker_erasure_state. The poker_ segment keeps it
// from colliding with another participant's state table in the same AWS
// account (plan ruling R1). Tombstones never expire (R10).
func newErasureStore(db *dynamodb.Client, cfg *config.Config) *erasure.Store {
	return erasure.NewStore(db, cfg.Env+"_poker", 0)
}

func newEligibility(sessions *sessionlog.Store, pending *reconcile.PendingStore) *userpurge.Eligibility {
	return userpurge.NewEligibility(sessions, pending)
}

func registerErasureRoutes(app *fiber.App, cfg *config.Config, verifier *jwtverify.Verifier, elig *userpurge.Eligibility) {
	v1.RegisterErasure(app.Group("/v1.0"), verifier, cfg.ErasureAccountClientID, elig.Blockers)
}

// startErasureConsumer runs the saga consumer in the background: it applies
// user.locked/user.unlocked itself and calls Purger.Purge on user.erase, then
// acks to ctech-account. Every replica runs one; SQS hands each message to
// one of them, and the purge is idempotent.
func startErasureConsumer(lc fx.Lifecycle, cfg *config.Config, db *dynamodb.Client, c cache.Backend, store *erasure.Store, elig *userpurge.Eligibility) error {
	if cfg.ErasureQueueURL == "" {
		slog.Warn("ERASURE_QUEUE_URL empty: account-deletion consumer disabled (dev only; config.Load refuses this in prod)")
		return nil
	}
	awsCfg, err := awsconfig.Load(context.Background(), cfg.AWSRegion)
	if err != nil {
		return err
	}
	// Poker instances have IPv6 egress only (see newAvatarService).
	objects := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.EndpointOptions.UseDualStackEndpoint = aws.DualStackEndpointStateEnabled
	})
	queue := sqs.NewFromConfig(awsCfg, func(o *sqs.Options) {
		o.EndpointOptions.UseDualStackEndpoint = aws.DualStackEndpointStateEnabled
	})
	purger := userpurge.NewPurger(db, objects, c, userpurge.Config{
		Env: cfg.Env, AvatarBucket: cfg.AvatarBucket, ArchiveBucket: cfg.ActionLogArchiveBucket,
	}, elig.Blockers)
	httpClient := &http.Client{Timeout: 10 * time.Second}
	account := strings.TrimRight(cfg.CtechURL, "/")
	tokens := oauth2client.New(httpClient, c, account+"/v1.0/token", cfg.ErasureAckClientID, cfg.ErasureAckClientSecret, erasureAckScope)
	acks := erasure.NewAckClient(httpClient, account+"/v1.0/internal/erasure/ack", tokens)
	consumer := erasure.NewConsumer(queue, cfg.ErasureQueueURL, erasureService, store, purger.Purge, acks)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				defer close(done)
				_ = consumer.Run(ctx)
			}()
			return nil
		},
		// A purge cut short here is redelivered after the visibility timeout
		// and resumes: every step re-reads from scratch.
		OnStop: func(stopCtx context.Context) error {
			cancel()
			select {
			case <-done:
			case <-stopCtx.Done():
			}
			return nil
		},
	})
	return nil
}
```

Before relying on the `v1` alias, confirm with `grep -n 'internal/api/v1"' api/internal/app/app.go`. In `app.go`'s `Module`, add `fx.Invoke(startErasureConsumer),` right before `fx.Invoke(startServer),`.

- [ ] **Step 5: Run the tests**

Run: `cd api && go vet ./... && go vet -tags integration ./... && go test ./... -count=1`
Expected: every package `ok`.

- [ ] **Step 6: Commit**

```bash
git add api/go.mod api/go.sum api/internal/config api/internal/app/erasure.go api/internal/app/app.go
git commit -m "feat(api): run the account-deletion consumer and ack to ctech-account"
```

---

### Task 9: Force-close open sockets on lock

**Files:**
- Create: `api/internal/api/v1/erasurews.go`, `api/internal/api/v1/erasurews_test.go`
- Modify: `api/internal/api/v1/tablews.go`, `api/internal/app/erasure.go`

**Interfaces:**
- Consumes:
  - `(*erasure.Consumer).OnLock(fn func(ctx context.Context, sub string)) *erasure.Consumer` (api-commons v1.14.0). It is called when a `user.locked`, or the synthetic lock applied on `user.erase`, changes the sub's state to locked. It is not called on no-op redeliveries.
  - `ws.Registry` (`Register`, `Unregister`, `Broadcast`).
  - `startErasureConsumer` (Task 8).
- Produces:
  - `func CloseLockedSockets(reg ws.Registry) func(ctx context.Context, sub string)` in package `v1`;
  - `watchErasureLock(reg ws.Registry, sub, connID string, conn io.Closer) func()`;
  - `startErasureConsumer` gains a `reg ws.Registry` parameter.

**Why the registry:** the consumer runs on whichever replica SQS hands the message to, but the user's sockets can be on any instance.
- `ws.Registry` (Valkey pub/sub) already fans `Broadcast` out to every instance.
- Each socket registers a closer under `erasure-lock#<sub>`. One broadcast closes all of them.
- A closed `*fws.Conn` makes the gateway's read loop return. Its existing defers then run the normal disconnect: unregister, presence close, `table.DisconnectCmd`.

Table sockets cover seated players and spectators alike: every viewer registers `tableID#playerID`, and the closer is registered in the same block. The connect-time `wsLocked` check (Task 2) stays.

- [ ] **Step 1: Write the failing test** — `api/internal/api/v1/erasurews_test.go`:

```go
package v1

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"gopkg.aoctech.app/api-commons/ws"
)

type fakeCloser struct{ closed atomic.Bool }

func (f *fakeCloser) Close() error {
	f.closed.Store(true)
	return nil
}

// A lock closes every socket of that player (table + gateway), and only those.
func TestCloseLockedSocketsClosesOnlyThatPlayersSockets(t *testing.T) {
	reg := ws.NewMemoryRegistry()
	table, gateway, other, gone := &fakeCloser{}, &fakeCloser{}, &fakeCloser{}, &fakeCloser{}
	defer watchErasureLock(reg, "u1", "table-conn", table)()
	defer watchErasureLock(reg, "u1", "gateway-conn", gateway)()
	defer watchErasureLock(reg, "u2", "other-conn", other)()
	watchErasureLock(reg, "u1", "closed-conn", gone)() // disconnected before the lock: unregistered

	CloseLockedSockets(reg)(context.Background(), "u1")

	deadline := time.Now().Add(time.Second)
	for !(table.closed.Load() && gateway.closed.Load()) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !table.closed.Load() || !gateway.closed.Load() {
		t.Fatal("every socket of the locked player must be closed")
	}
	if other.closed.Load() {
		t.Fatal("another player's socket was closed")
	}
	if gone.closed.Load() {
		t.Fatal("an unregistered socket was touched")
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `cd api && go test ./internal/api/v1/ -run TestCloseLockedSockets -count=1`
Expected: FAIL with `undefined: watchErasureLock`.

- [ ] **Step 3: Implement** — `api/internal/api/v1/erasurews.go`:

```go
package v1

import (
	"context"
	"io"

	"gopkg.aoctech.app/api-commons/ws"
)

// erasureLockKey is the ws.Registry key every socket of a player also listens
// on. A broadcast there reaches the player's sockets on every instance
// (Valkey pub/sub fan-out), which is what lets the replica that consumed the
// lock close sockets held by its siblings (plan ruling R6).
func erasureLockKey(sub string) string { return "erasure-lock#" + sub }

// lockCloser is the ws.Conn registered under erasureLockKey: anything
// broadcast there closes its socket. The gateway's read loop then fails and
// runs its usual cleanup (unregister, presence, table disconnect).
type lockCloser struct{ conn io.Closer }

func (l lockCloser) WriteMessage(int, []byte) error { return l.conn.Close() }

// watchErasureLock registers conn to be closed when sub is locked and
// returns the matching unregister.
func watchErasureLock(reg ws.Registry, sub, connID string, conn io.Closer) func() {
	key := erasureLockKey(sub)
	reg.Register(key, connID, lockCloser{conn: conn})
	return func() { reg.Unregister(key, connID) }
}

// CloseLockedSockets is the erasure.Consumer OnLock hook: it closes every
// table socket (seated or spectating) and gateway socket of sub, fleet-wide.
func CloseLockedSockets(reg ws.Registry) func(ctx context.Context, sub string) {
	return func(ctx context.Context, sub string) {
		reg.Broadcast(ctx, erasureLockKey(sub), []byte("account_locked"))
	}
}
```

In `tablews.go`, `RegisterTableWS`, replace:

```go
			wsdrain.TrackByID(connID, safeConn)
			defer wsdrain.UntrackByID(connID)
```

with:

```go
			wsdrain.TrackByID(connID, safeConn)
			defer wsdrain.UntrackByID(connID)
			// Account deletion: closed fleet-wide when the player is locked.
			defer watchErasureLock(reg, playerID, connID, conn)()
```

In `RegisterGeneralWS`, replace (the block Task 2 left):

```go
			connID := uuid.New().String()
```

with:

```go
			connID := uuid.New().String()
			// Account deletion: closed fleet-wide when the player is locked.
			defer watchErasureLock(reg, playerID, connID, conn)()
```

Only one `connID := uuid.New().String()` sits in `RegisterGeneralWS`; the table WS one is followed by the `wsdrain.TrackByID` comment, so edit by context.

In `api/internal/app/erasure.go`:
- add the import `"gopkg.aoctech.app/api-commons/ws"`;
- add the parameter `reg ws.Registry` to `startErasureConsumer`, after `elig *userpurge.Eligibility`;
- replace `consumer := erasure.NewConsumer(queue, cfg.ErasureQueueURL, erasureService, store, purger.Purge, acks)` with:

```go
	// OnLock fires only when a message really moves the sub to locked (not on
	// redeliveries); it closes the user's sockets on every instance (R6).
	consumer := erasure.NewConsumer(queue, cfg.ErasureQueueURL, erasureService, store, purger.Purge, acks).
		OnLock(v1.CloseLockedSockets(reg))
```

Fx already provides `ws.Registry` (`newWsRegistry`), so the invoke needs no other change.

- [ ] **Step 4: Run the tests**

Run: `cd api && go vet ./... && go vet -tags integration ./... && go test ./internal/api/v1/ ./internal/app/ -count=1`
Expected: `ok` for both packages.

- [ ] **Step 5: Commit**

```bash
git add api/internal/api/v1/erasurews.go api/internal/api/v1/erasurews_test.go api/internal/api/v1/tablews.go api/internal/app/erasure.go
git commit -m "feat(api): close a locked user's sockets fleet-wide on the erasure lock"
```

---

### Task 10: Infrastructure

**Files:**
- Modify: `cdk/lib/constants.ts`, `cdk/lib/dynamodb-stack.ts`, `cdk/lib/api-stack.ts`, `cdk/lib/archiver-stack.ts`
- Test: `cdk/test/dynamodb-stack.test.ts`, `cdk/test/api-stack.test.ts`

**Interfaces:**
- Produces:
  - `actionLogArchiveBucketName(env: Environment): string`;
  - `SSM_ACCOUNT(env).erasureTopicArn`;
  - table `{env}_poker_erasure_state`;
  - queue `{env}-poker-user-erasure` and DLQ `{env}-poker-user-erasure-dlq`;
  - env lines `ERASURE_QUEUE_URL=` and `ACTION_LOG_ARCHIVE_BUCKET=` in `/etc/app-static.env`;
  - `SSM_POKER(env).erasureAckClientId` / `.erasureAckClientSecret`, and `ERASURE_ACK_CLIENT_ID` / `ERASURE_ACK_CLIENT_SECRET` in `ssmEnvArgs` (dedicated ack client, ruling R2).

- [ ] **Step 1: Write the failing tests**

In `cdk/test/dynamodb-stack.test.ts`, change `template.resourceCountIs('AWS::DynamoDB::GlobalTable', 36);` to `37` and append:

```ts
test('poker_erasure_state holds the account-deletion lock/tombstone (pk only, TTL)', () => {
  const template = Template.fromStack(new DynamoDBStack(new App(), 'TestErasureState', {environment: 'dev', cloudwatchAlarmsEnabled: false}));
  template.hasResourceProperties('AWS::DynamoDB::GlobalTable', {
    TableName: 'dev_poker_erasure_state',
    KeySchema: [{AttributeName: 'pk', KeyType: 'HASH'}],
    TimeToLiveSpecification: {AttributeName: 'ttl', Enabled: true},
  });
});
```

Append to `cdk/test/api-stack.test.ts`:

```ts
test('subscribes a poker queue to the account user-erasure topic (account deletion)', () => {
  const template = Template.fromStack(synthStack());
  template.hasResourceProperties('AWS::SQS::Queue', {
    QueueName: 'dev-poker-user-erasure',
    VisibilityTimeout: 43200,
    RedrivePolicy: Match.objectLike({maxReceiveCount: 5}),
  });
  template.hasResourceProperties('AWS::SQS::Queue', {QueueName: 'dev-poker-user-erasure-dlq'});
  template.hasResourceProperties('AWS::SNS::Subscription', {
    Protocol: 'sqs',
    RawMessageDelivery: true,
    FilterPolicy: {services: ['poker']},
  });
  template.hasResourceProperties('AWS::IAM::Policy', {
    PolicyDocument: {Statement: Match.arrayWith([Match.objectLike({
      Action: Match.arrayWith(['sqs:ReceiveMessage', 'sqs:DeleteMessage']),
    })])},
  });
  template.hasResourceProperties('AWS::IAM::Policy', {
    PolicyDocument: {Statement: Match.arrayWith([Match.objectLike({Action: 's3:ListBucketVersions'})])},
  });
  const text = userDataText(template);
  expect(text).toContain('ERASURE_QUEUE_URL=');
  expect(text).toContain('ACTION_LOG_ARCHIVE_BUCKET=poker-action-log-archive-dev');
  expect(text).toContain('ERASURE_ACK_CLIENT_ID=/ctech/dev/poker/erasure-ack-client-id');
  expect(text).toContain('ERASURE_ACK_CLIENT_SECRET=/ctech/dev/poker/erasure-ack-client-secret');
});

test('erasure DLQ alarm exists only when prod + cloudwatchAlarmsEnabled', () => {
  const on = Template.fromStack(synthStack({environment: 'prod', cloudwatchAlarmsEnabled: true}));
  on.hasResourceProperties('AWS::CloudWatch::Alarm', {AlarmName: 'prod-poker-user-erasure-dlq-depth'});
});
```

- [ ] **Step 2: Run them and watch them fail**

Run: `cd cdk && npx jest test/dynamodb-stack.test.ts test/api-stack.test.ts`
Expected: FAIL. The count is `Expected 37 but got 36`, and the queue tests fail with `Template has 0 resources with type AWS::SQS::Queue`.

- [ ] **Step 3: Implement**

`cdk/lib/constants.ts`:
- add to `SSM_ACCOUNT` the entry `erasureTopicArn: \`/ctech/${env}/account/erasure-topic-arn\`,` (published by ctech-account's iam-stack);
- add to `SSM_POKER` (operator-provisioned, like `clientId` / `clientSecret`):

```ts
  // Dedicated confidential client for account-deletion acks to ctech-account
  // (scope internal:account:erasure-ack only), kept apart from the wallet M2M client.
  erasureAckClientId: `/ctech/${env}/poker/erasure-ack-client-id`,
  erasureAckClientSecret: `/ctech/${env}/poker/erasure-ack-client-secret`,
```
- below `SSM_ACCOUNT`, add:

```ts
/** Action-log S3 archive (archiver-stack.ts), rewritten by the account-deletion purge. */
export const actionLogArchiveBucketName = (env: Environment) => `poker-action-log-archive-${env}`;
```

`cdk/lib/archiver-stack.ts`: import `actionLogArchiveBucketName` from `./constants`, and replace `` bucketName: `poker-action-log-archive-${environment}`, `` with `bucketName: actionLogArchiveBucketName(environment),`.

`cdk/lib/dynamodb-stack.ts`:
- add `'poker_erasure_state' |` to the `TableName` union (on the `'poker_hand_reveals' | ...` line);
- after the `poker_hand_reveal_payments` table, add:

```ts
    // poker_erasure_state: the account-deletion saga's per-user lock and
    // tombstone (pk "SUB#<sub>", TTL "ttl"). Schema owned by api-commons
    // erasure.Store. PITR stays on: a lost tombstone lets a late message
    // resurrect an erased user.
    table('poker_erasure_state', false, true);
```

`cdk/lib/api-stack.ts`:
- imports: add `import * as sqs from 'aws-cdk-lib/aws-sqs';` and `import * as snsSubscriptions from 'aws-cdk-lib/aws-sns-subscriptions';`, and add `actionLogArchiveBucketName,` to the `./constants` import list;
- right after the `s3:PutObject/DeleteObject/GetObject` statement on `${avatarsBucketName}/av/*`, add:

```ts
    // ── Account-deletion saga participant (docs/plans/2026-10-07-account-deletion-participant.md)
    // ctech-account publishes user.locked/unlocked/erase on its topic with a
    // `services` message attribute; this queue receives only poker's.
    const erasureDlq = new sqs.Queue(this, 'ErasureDLQ', {
      queueName: `${environment}-poker-user-erasure-dlq`,
      retentionPeriod: cdk.Duration.days(14),
      encryption: sqs.QueueEncryption.SQS_MANAGED,
      enforceSSL: true,
    });
    const erasureQueue = new sqs.Queue(this, 'ErasureQueue', {
      queueName: `${environment}-poker-user-erasure`,
      // The consumer has no visibility heartbeat and one purge rewrites every
      // hand the user played: the SQS maximum (plan ruling R14).
      visibilityTimeout: cdk.Duration.hours(12),
      retentionPeriod: cdk.Duration.days(14),
      receiveMessageWaitTime: cdk.Duration.seconds(20),
      encryption: sqs.QueueEncryption.SQS_MANAGED,
      enforceSSL: true,
      deadLetterQueue: {queue: erasureDlq, maxReceiveCount: 5},
    });
    sns.Topic.fromTopicArn(this, 'AccountUserErasureTopic',
      ssm.StringParameter.valueForStringParameter(this, account.erasureTopicArn),
    ).addSubscription(new snsSubscriptions.SqsSubscription(erasureQueue, {
      rawMessageDelivery: true,
      filterPolicy: {services: sns.SubscriptionFilter.stringFilter({allowlist: ['poker']})},
    }));
    erasureQueue.grantConsumeMessages(instanceRole);
    // The purge deletes every version of the user's avatars (bucket versioned in prod).
    instanceRole.addToPolicy(new iam.PolicyStatement({
      actions: ['s3:ListBucketVersions'],
      resources: [`arn:${cdk.Aws.PARTITION}:s3:::${avatarsBucketName}`],
    }));
    instanceRole.addToPolicy(new iam.PolicyStatement({
      actions: ['s3:DeleteObjectVersion'],
      resources: [
        `arn:${cdk.Aws.PARTITION}:s3:::${avatarsBucketName}/av/*`,
        `arn:${cdk.Aws.PARTITION}:s3:::${avatarsBucketName}/up/*`,
      ],
    }));
    // The dedicated ack client's credentials, read at service start (ruling R2).
    instanceRole.addToPolicy(new iam.PolicyStatement({
      actions: ['ssm:GetParameter'],
      resources: [poker.erasureAckClientId, poker.erasureAckClientSecret].map(
        (path) => `arn:${cdk.Aws.PARTITION}:ssm:${this.region}:${this.account}:parameter${path}`,
      ),
    }));
    // ...and rewrites the action-log archive of every hand the user played.
    const archiveBucket = actionLogArchiveBucketName(environment);
    instanceRole.addToPolicy(new iam.PolicyStatement({
      actions: ['s3:ListBucket'],
      resources: [`arn:${cdk.Aws.PARTITION}:s3:::${archiveBucket}`],
    }));
    instanceRole.addToPolicy(new iam.PolicyStatement({
      actions: ['s3:GetObject', 's3:PutObject'],
      resources: [`arn:${cdk.Aws.PARTITION}:s3:::${archiveBucket}/*`],
    }));
```

- in the `/etc/app-static.env` block, after `` `AVATAR_BUCKET=${avatarsBucketName}`, ``, add:

```ts
      `ERASURE_QUEUE_URL=${erasureQueue.queueUrl}`,
      `ACTION_LOG_ARCHIVE_BUCKET=${archiveBucket}`,
```

- after the `MemoryPressureAlarm` `if (isProd && cloudwatchAlarmsEnabled) { ... }` block, add:

```ts
    if (isProd && cloudwatchAlarmsEnabled) {
      new cloudwatch.Alarm(this, 'ErasureDlqDepthAlarm', {
        alarmName: `${environment}-poker-user-erasure-dlq-depth`,
        alarmDescription: 'An account-deletion message failed 5 times: a user is stuck in PURGING until it is redriven.',
        metric: erasureDlq.metricApproximateNumberOfMessagesVisible({period: cdk.Duration.minutes(5), statistic: 'max'}),
        threshold: 1,
        evaluationPeriods: 1,
        comparisonOperator: cloudwatch.ComparisonOperator.GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
        treatMissingData: cloudwatch.TreatMissingData.NOT_BREACHING,
      }).addAlarmAction(new cloudwatchActions.SnsAction(
        sns.Topic.fromTopicArn(this, 'ErasureAlertsTopic', ALERTS_TOPIC_ARN),
      ));
    }
```

- in `ssmEnvArgs`, after `` `AVATAR_BASE_URL=${avatarBaseUrlParam}`, ``, add:

```ts
      `ERASURE_ACK_CLIENT_ID=${poker.erasureAckClientId}`,
      `ERASURE_ACK_CLIENT_SECRET=${poker.erasureAckClientSecret}`,
```

(The table needs no new IAM: the existing `table/${environment}_poker*` wildcard covers it.)

- [ ] **Step 4: Run the tests**

Run: `cd cdk && npx tsc --noEmit && npx jest`
Expected: all suites pass. The user-data size test (16384 bytes) still passes, since the change adds two short lines.

Run: `cd api && go test ./internal/userpurge/ -run TestInventory -count=1`
Expected: `ok`, because `poker_erasure_state` was classified in Task 5.

- [ ] **Step 5: Commit**

```bash
git add cdk/lib cdk/test
git commit -m "feat(cdk): erasure state table, user-erasure queue + DLQ, purge IAM"
```

Do **not** commit the generated `cdk/lib/*.js` / `*.d.ts` files unless the repo already tracks them for the files you changed. Check with `git ls-files cdk/lib | grep '\.js$'` and follow what is tracked.

---

### Task 11: Documentation

**Files:**
- Modify: `api/README.md`, `api/CLAUDE.md`, `cdk/CLAUDE.md`, `docs/README.md`
- Create: `docs/runbooks/account-deletion.md`

- [ ] **Step 1: `api/README.md`**

In "Configuration (environment variables)", add three rows to the table:

```markdown
| `ERASURE_QUEUE_URL` *                             | —                                 | poker's SQS queue on ctech-account's `{env}-account-user-erasure` topic; empty disables the deletion consumer (dev only) |
| `ACTION_LOG_ARCHIVE_BUCKET` *                     | —                                 | action-log S3 archive the deletion purge rewrites; empty skips that step (dev only)                                  |
| `ERASURE_ACCOUNT_CLIENT_ID`                       | `accounts`                        | ctech-account's own client id (`SELF_CLIENT_ID`): the only `azp` allowed on the eligibility endpoint                |
| `ERASURE_ACK_CLIENT_ID` / `ERASURE_ACK_CLIENT_SECRET` * | —                           | dedicated confidential client for erasure acks (`internal:account:erasure-ack` only), SSM `/ctech/<env>/poker/erasure-ack-client-{id,secret}` |
```

In "Authentication & authorization", add these bullets:

```markdown
- **Token revocation (account deletion).** The verifier checks ctech-account's revocation list (`jwtverify.WithRevocation`,
  entries in Valkey DB 0 = `VALKEY_URL`). Reads fail open if Valkey is down; every non-GET route uses
  `VerifyClaimsStrict` and returns **503** instead.
- **Erasure lock.** `authMiddleware` refuses (**403**) any request from a user the deletion saga locked or erased
  (`{env}_poker_erasure_state`, `erasure.Store.Blocked`), on every method, since `GET /players/me` creates the profile.
  The one exception is `POST /rooms/:id/leave`. A store error is **503** on writes and fails open on reads. Both
  WebSocket gateways refuse a locked user at connect (fail closed); the wallet webhook drops product purchases of a
  locked or erased user.
```

Add a section "## Account deletion (LGPD participant)" before "## Known issues":

```markdown
## Account deletion (LGPD participant)

Poker is a participant of ctech-account's erasure saga (ctech-account `docs/specs/2026-10-06-account-deletion-*`,
plan `docs/plans/2026-10-07-account-deletion-participant.md`).

- `GET /v1.0/internal/erasure/eligibility/:sub` — ctech-account's service token only (`azp` =
  `ERASURE_ACCOUNT_CLIENT_ID`, no `sid`, scope `internal:poker:erasure-eligibility`). Returns `erasure.Eligibility`;
  any failure is 503. Blocker codes: `poker.seated_at_table`, `poker.chips_held`, `poker.pending_cashout`,
  `poker.pending_fee_debit`.
- The consumer (`internal/app/erasure.go`, api-commons `erasure.Consumer`) reads `ERASURE_QUEUE_URL`, applies
  lock/unlock to `{env}_poker_erasure_state`, runs `userpurge.Purger.Purge` on `user.erase`, then acks
  `POST {CTECH_URL}/v1.0/internal/erasure/ack` with the dedicated `ERASURE_ACK_CLIENT_ID`'s client-credentials
  token (scope `internal:account:erasure-ack`). When a message locks the user, its `OnLock` hook broadcasts on
  `ws.Registry` key `erasure-lock#<sub>` and every instance closes that user's table and gateway sockets
  (`api/v1/erasurews.go`).
- What the purge does per table is `internal/userpurge/inventory.go`. A new `poker_*` table fails
  `TestInventoryClassifiesEveryTable` until it is classified there and handled in `Purge`.
- Others' hand histories keep the erased player as a random pseudonym (`anon_…`, never stored next to the sub),
  without name, avatar or chat. Reports are kept anonymized for 1 year.
```

- [ ] **Step 2: `api/CLAUDE.md`** — add to "Conventions (follow these)":

```markdown
- **Every new user write path goes through the erasure lock.** HTTP routes get it from `authMiddleware`; anything that
  writes per-user rows outside it (a webhook, a background consumer) must call the `BlockedFunc`
  (`erasure.Store.Blocked`) and drop work for a locked or erased user, or the account-deletion purge is undone
  (saga protocol §4.5). **Every new `poker_*` table must be classified in `internal/userpurge/inventory.go` and
  handled by `Purger.Purge`** — `TestInventoryClassifiesEveryTable` enforces the first half.
```

- [ ] **Step 3: `cdk/CLAUDE.md`** — change the table count line from `**36 DynamoDB tables**` to `**37 DynamoDB tables**`, then add to "Architecture facts":

```markdown
- **Account-deletion participant** (`api-stack.ts`): queue `<env>-poker-user-erasure` (+ DLQ, `maxReceiveCount` 5,
  12 h visibility) subscribed to ctech-account's `<env>-account-user-erasure` topic (ARN from SSM
  `/ctech/<env>/account/erasure-topic-arn`, so ctech-account's stack must deploy first), raw delivery,
  `FilterPolicy {"services":["poker"]}` on the message attribute. `poker_erasure_state` (`dynamodb-stack.ts`) holds
  the lock/tombstone. The instance role may list/delete avatar versions and read/write the action-log archive for the
  purge. DLQ depth alarm: prod + `CLOUDWATCH_ALARMS_ENABLED` only.
```

- [ ] **Step 4: `docs/runbooks/account-deletion.md`**:

```markdown
# Runbook — account deletion (poker participant)

## One-time enablement (per environment)
1. Deploy ctech-account's stack first (it creates `<env>-account-user-erasure` and its SSM ARN), then poker's.
2. In ctech-account, create a dedicated confidential client for poker's acks, grant it only
   `internal:account:erasure-ack`, and store its id/secret in SSM `/ctech/<env>/poker/erasure-ack-client-id` and
   `/ctech/<env>/poker/erasure-ack-client-secret` (SecureString). Instance refresh to load them.
3. Add poker to ctech-account's `/ctech-account/<env>/erasure-participants` JSON:
   `{"service":"poker","url":"https://poker-api[-<env>].aoctech.app/v1.0","audience":"<poker SERVICE_AUDIENCE>","client_id":"<ERASURE_ACK_CLIENT_ID>"}`.
4. Check poker's `ERASURE_ACCOUNT_CLIENT_ID` equals ctech-account's `SELF_CLIENT_ID` (default `accounts`).

## DLQ alarm (`<env>-poker-user-erasure-dlq-depth`)
A message failed 5 times. Read the app log for `erasure: message left for redelivery` with its error, fix the cause,
then redrive the DLQ to the queue (`aws sqs start-message-move-task --source-arn <dlq-arn> --profile ctech`). The
purge is idempotent: redriving is always safe.

## Restoring a DynamoDB table from PITR/backup (mandatory, saga protocol §8)
A restore brings erased users back. After restoring any `<env>_poker_*` table, list purged requests from
ctech-account (see its README "Account deletion" restore runbook) whose `purged_at` is after the restore point, and
have ctech-account re-publish `user.erase` for each (admin redrive). Do not restore `<env>_poker_erasure_state` to an
older point: it would drop tombstones.
```

- [ ] **Step 5: `docs/README.md`** — add a row to "Feature status":

```markdown
| Account deletion participant (LGPD): eligibility, lock, purge, ack       | **BUILT, needs operator enablement** | `api/internal/userpurge`, `api/internal/app/erasure.go`, `docs/runbooks/account-deletion.md` |
```

- [ ] **Step 6: Verify and commit**

Run: `cd api && go test ./... -count=1 && cd ../cdk && npx jest`
Expected: all green, since docs do not affect tests.

```bash
git add api/README.md api/CLAUDE.md cdk/CLAUDE.md docs/README.md docs/runbooks/account-deletion.md
git commit -m "docs: account deletion participant (config, auth, purge, runbook)"
```

---

## Cross-project impact

- **ctech-account (api, cdk):** no code change, but enabling poker needs operator steps on the account side:
  - create poker's dedicated ack client and grant it `internal:account:erasure-ack` (R2);
  - add poker to `ERASURE_PARTICIPANTS`, using poker's `/v1.0` base URL and its `SERVICE_AUDIENCE` as `audience`;
  - deploy account's stack before poker's (the SSM topic ARN must exist at poker deploy time).

  Poker relies on two account behaviors:
  - the eligibility token carries `azp` = `SELF_CLIENT_ID` and no `sid`;
  - the ack route is `/v1.0/internal/erasure/ack`.

  If account changes either, poker's endpoint returns 403 or its acks fail into the DLQ.
- **ctech-account ui:** must translate the four `poker.*` blocker codes.
- **ctech-go-common:** poker needs **v1.14.0**, which adds `erasure.Consumer.OnLock` (separate go-common plan). Task 1 is blocked until that tag exists. Its README "Account erasure" still says filter scope `MessageBody` and scope `account:erasure:ack`; account's phase 3 plan (Task 8) owns that fix. The `erasure.AckClient` doc comment has the same stale scope name.
- **ctech-wallet:** none directly. Poker's money trail stays in the wallet ledger (D5). A product purchase dropped by the webhook during a deletion stays paid in the wallet, so support refunds it (R15).
- **ctech-dfe, ctech-billing:** none. The same participant pattern applies; `userpurge`'s anonymizer and the `BlockedFunc` choke-point approach are candidates for `ctech-go-common/erasure` if a second service needs them.
- **Shared Valkey:** read-only use of DB 0 revocation keys (one GET per verification). Shared Valkey latency now also gates poker writes (R3).

## Decisions (user, 2026-10-08)

1. Legal approves the 1-year anonymized report retention (R9). Legal also approves erasing, rather than anonymizing, the user's own hand rows and the matchups (R7).
2. ctech-account's prod `SELF_CLIENT_ID` is `accounts`, with no override (R12).
3. Acks use a dedicated confidential client: `ERASURE_ACK_CLIENT_ID` / `_SECRET` from SSM (R2, Tasks 8 and 10).
4. Sockets already open at lock time are force-closed through the v1.14.0 `OnLock` hook (R6, Task 9). The connect-time check stays.

No open questions remain.
