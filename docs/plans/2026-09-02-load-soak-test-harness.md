# Load / soak test harness — runbook (issue #123)

Validate the poker system at a target concurrency profile before wider release / real-money
enablement. Two complementary harnesses ship in `api/`:

| Harness | What it drives | Where it runs | Needs |
| --- | --- | --- | --- |
| `api/tests/load/soak_test.go` (`TestSoak`, `-tags load`) | The real `tablemanager` → `table.Actor` → `tablestore` command path, in-process, across a **simulated fleet** of M managers sharing one store + lease backend | A laptop | DynamoDB Local (`:8555`) only — no server, wallet, or JWKS |
| `api/cmd/loadtest` | **Synthetic binary-protobuf WebSocket clients** against a running API (table gateway `GET /v1.0/tables/:id/ws`) | A laptop or one throwaway instance, pointed at a deployed **prod-like** stack | Reachable API URL + one player access token (JWT) per synthetic seat |

Both are dependency-light (stdlib + existing repo deps + the generated protobuf types). Neither
provisions infra. The only non-$0 item in this effort is a prod-like environment for the duration
of a `cmd/loadtest` run — **tear it down afterwards.**

---

## Target concurrency profile

Agreed with the product owner (revisit before real-money enablement):

| Metric | Target (1×) | Stress (2×) |
| --- | --- | --- |
| Concurrent tables | 25 | 50 |
| Concurrent sockets / seated players | 200 | 400 |
| Players per table | 6–8 | 6–8 |
| Hands / sec (fleet-wide) | ≈ 3–5 | ≈ 6–10 |
| Soak duration | 2 h | 30–60 min |

Run **1×** for the full soak duration, then **2×** for a shorter burst. Also run the two
resilience scenarios below at 1×.

---

## Pass / fail thresholds

Applied automatically by both harnesses (exit 1 / `t.Errorf` on breach):

| Signal | Pass |
| --- | --- |
| p99 action-commit / action-ack latency | **< 750 ms** |
| p95 action-commit / action-ack latency | < 300 ms (advisory) |
| Error rate (rejected actions / actions sent) | **< 1 %** |
| `unavailable` / `actor_stopped` errors | < 0.1 % (any sustained rate is a fail — it means the store or an actor is falling over, not a player mistake) |
| DynamoDB throttled requests (deployed runs) | 0 sustained; brief single-digit blips during a deploy are acceptable |
| Hands completed | grows linearly for the whole run (a plateau = the fleet stalled) |
| Chip conservation | exact (the engine's own invariant; a soak that trips it is a hard fail) |

`version_conflict` and `stale_state` errors are **expected** under multi-instance load (two
instances racing the same conditional write) — they are counted but only fail the run if they
dominate the error budget.

---

## Running the in-process soak harness (local, $0)

```bash
cd api

# 1. DynamoDB Local on :8555
podman compose -f docker-compose.test.yml up -d
#   or, without compose:
#   podman run -d --name ddb -p 8555:8000 amazon/dynamodb-local:latest \
#     -jar DynamoDBLocal.jar -inMemory -sharedDb

# 2. Run. Skips unless LOADTEST_DURATION is set.
LOADTEST_DURATION=2m \
LOADTEST_TABLES=25 \
LOADTEST_PLAYERS=6 \
LOADTEST_SERVERS=3 \
  go test -tags load -run TestSoak -timeout 30m -v ./tests/load

# 3. Tear down
podman rm -f ddb   # or: podman compose -f docker-compose.test.yml down
```

Env knobs (all optional): `LOADTEST_TABLES` (20), `LOADTEST_PLAYERS` (6), `LOADTEST_SERVERS` (3),
`LOADTEST_THINK_MIN_MS` (20), `LOADTEST_THINK_MAX_MS` (120), `LOADTEST_CHURN_PCT` (2).

What it exercises: `ReadyCmd` hand start, `ActCmd` play with a weighted fold/check/call/min-raise
bot, `JoinCmd` rebuys for busted seats, and `Connect`/`Disconnect` churn — each player pinned to
one simulated instance, every command routed through it, so cross-instance conditional writes are
under genuine concurrent load (run with `-race` to add the detector).

This harness is the fast feedback loop: it catches actor-goroutine contention, command-queue
depth blowups, conditional-write conflict storms, and latency regressions **without** an AWS bill.
It does **not** exercise the WebSocket transport, fan-out (Valkey pub/sub), the HTTP buy-in /
wallet path, JWKS verification, or real DynamoDB capacity.

### Sample local run (2 tables × 4 players × 30s, 3 simulated instances, DynamoDB Local)

```
==================== soak results ====================
elapsed:            30s
tables x players:   2 x 4  (8 seats) across 3 servers
actions committed:  89
hands completed:    8
throughput:         3.0 actions/s, 0.27 hands/s
churn events:       14
rebuys:             0
commit latency:     p50=5.99ms p95=12.28ms p99=15.83ms max=15.94ms
errors:             0 total (0.000%)
=====================================================
--- PASS: TestSoak (30.04s)
```

---

## Running the WebSocket harness (deployed, prod-like)

Stand up a prod-like stack (same instance size, DynamoDB capacity mode, and Valkey as prod).
Provision one throwaway box (or run from a laptop) — **do not** add WAF/NAT/multi-AZ for the test.

```bash
cd api
go build -o /tmp/loadtest ./cmd/loadtest

# tokens.txt: one player access token (JWT) per line — one per synthetic seat.
# Mint them from ctech-account for dedicated load-test users (RS256/ES256; the
# harness does not and cannot forge them).

/tmp/loadtest \
  -url https://poker-loadtest.example.internal \
  -table-ids room_aaa,room_bbb,room_ccc,...          # 25 rooms, pre-created via POST /v1.0/rooms
  -players-per-table 8 \
  -token-file tokens.txt \
  -auto-buyin -buyin-amount 1000 \
  -ramp 2m -duration 2h \
  -churn-every 5m \
  -think-min 800ms -think-max 4s
```

Flags: `-ramp` spreads client startup; `-duration` is the soak length; `-churn-every` makes each
client drop and re-open its socket on that interval (jittered) to exercise reconnect + presence
under load; `-weight-fold` / `-weight-check-call` tune the bot; `-auto-buyin` performs
`POST /v1.0/rooms/:id/join` before connecting (rooms and their tier/blinds must already exist).
Prints a progress line every `-report-every` and a final report; **exit code 1 on a threshold
breach.**

Rooms must be created up front (`POST /v1.0/rooms`) and, for real-money tiers, the entitlement /
fee path must be funded — out of scope for the harness. For a pure-sandbox prod-like run this is
just N `POST /v1.0/rooms` calls plus the token list.

---

## Resilience scenarios (run at 1×)

1. **Rolling deploy during load.** Start the WS harness at 1×, then trigger a normal rolling
   deploy of the API. Expect: a brief spike in `unavailable` + WS reconnects as `wsdrain` sends
   1001 closes and instances cycle, then recovery to baseline within ~1 deploy cycle. No chip
   conservation break, no stuck tables, hands-completed keeps climbing. Cross-check the
   graceful-WS-shutdown path (`internal/wsdrain`, `OnStop` → `DrainAndRelease`).
2. **Simulated spot kill mid-test.** While the harness runs at 1×, terminate one instance
   abruptly (`aws ec2 terminate-instances`, no lifecycle-hook drain — simulating the
   rebalance-storm gap documented in `cdk/CLAUDE.md`). Expect: that instance's sockets reconnect
   elsewhere, tables it served are picked up by other instances on the next command (leases are
   advisory — correctness is DynamoDB conditional writes), `duplicate-seat` commit guard holds
   (`docs/specs/2026-09-01-duplicate-seat-commit-guard.md`). Watch for stranded turn timers or a
   table that never advances.

---

## Reading results alongside CloudWatch (deployed runs only)

During and after a `cmd/loadtest` run, correlate the harness report with:

- **DynamoDB throttles** — the per-table `ReadThrottleEvents` / `WriteThrottleEvents` and the
  throttle alarms (added in the PRs for the alarm/throttle work): any sustained throttling on
  `*_poker_table_state` / `*_poker_action_log` / `*_poker_action_guards` means capacity is the
  ceiling, not the app. Feeds the DynamoDB sizing decision.
- **Instance RSS + CPU credits** — if a burstable instance drains CPU credits during the soak, or
  RSS climbs monotonically (leak), the instance class / count is wrong. Feeds the instance sizing
  decision.
- **Actor command-queue depth** — the `table.Actor` mailbox is deliberately blocking; sustained
  high depth (or rising `Dispatch` latency in the harness with flat DynamoDB latency) means the
  actor goroutine is the bottleneck.
- **Valkey latency / CPU** — fan-out is Valkey pub/sub; a p99 broadcast latency regression in the
  harness with flat commit latency points here.
- **Broadcast / commit latency alarms** — the harness p50/p95/p99 should track the server-side
  latency metrics; a large gap means transport or fan-out overhead.

Record the harness report + the CloudWatch panels for 1× and 2× in the issue. Those numbers feed
the sizing decisions and are the **re-run gate before real-money enablement.**
