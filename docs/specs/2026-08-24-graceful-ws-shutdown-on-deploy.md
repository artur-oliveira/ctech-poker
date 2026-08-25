# Graceful WebSocket shutdown on rolling deploy — Design

## Problem

Players intermittently see "Desconectado" in production. Root-caused (not fixed here) to the
rolling-deploy restart mechanism added in `1242bf1` ("front the API with nginx and enable
zero-downtime rolling deploy"), not to the alpine base image, nginx WS proxying config, or the
multi-process/lease architecture — all three were checked and are correct:

- nginx sets `Upgrade`/`Connection $connection_upgrade` and long (3600s) read/send timeouts on
  the WS locations (`ctech-cdk/assets/ec2-alpine/setup-nginx.sh`).
- Table state is durable in DynamoDB (`tablestore`) and `tablelease` is explicitly
  latency-only — any process can serve any table's actor and load correct state, so a reconnect
  landing on the other process (`app`/`app2`) is not the problem.

The actual mechanism: `ctech-cdk/assets/ec2-alpine/setup-deploy.sh`'s `restart_and_wait` restarts
`app` then `app2` one at a time via `rc-service restart`. On `OnStop`
(`api/internal/app/app.go`), `tablemanager.DrainAndRelease(ctx)` only releases table leases; it
does nothing for already-open client sockets. The subsequent `app.ShutdownWithContext` with a 5s
timeout force-closes every still-open connection once that window elapses — including every live
WebSocket pinned to the unit being restarted. Each deploy therefore drops roughly half of all
connected players (whichever unit's turn it is), twice (once per unit). The client's own
reconnect logic (already correct — bounded backoff, gives up cleanly after
`MAX_RECONNECT_ATTEMPTS`) then does its job, but the drop is real and shows as "Desconectado"
until it completes.

This is zero-downtime only for *new* connections/health checks; it is not zero-downtime for
sessions already open at deploy time. Fixing that is what this spec is for.

## Goal

Reduce (does not have to fully eliminate — see "Non-goals") the number of players who see an
abrupt drop during a routine deploy, without giving up the current deploy mechanism's simplicity
(SSM RunCommand → `rc-service restart` per unit).

## Approach: send clients a clean close before force-closing

On `OnStop`, before (or in place of) waiting out the 5s `ShutdownWithContext` window:

1. Iterate every table actor's live WS connections registered to this process (the
   `ws.Registry` from `api-commons`, already used for connection bookkeeping — see
   `api/CLAUDE.md`'s "Reuse `gopkg.aoctech.app/api-commons`" convention) and send each a clean
   WebSocket close frame with code 1001 ("going away") immediately.
2. Give the client a brief window (bounded well under the existing 5s shutdown budget, e.g. 1–2s)
   to process the close and start its own reconnect *before* the transport is torn out from under
   it — a clean 1001 close reaches the client's `onclose` handler the same way an abrupt drop
   does today, but sooner and without waiting on a dead read/write to time out first, so the
   client-side reconnect attempt starts measurably earlier.
3. Proceed with `DrainAndRelease` + `ShutdownWithContext` as today; nothing else about the
   sequence changes.

This does not eliminate the drop (the client is still disconnected momentarily — there is no way
around that with a process restart), but it shortens the gap between "the client's socket dies"
and "the client starts reconnecting," and gives the client an unambiguous, immediate signal
instead of relying on TCP-level failure detection which can lag.

## Alternatives considered, not chosen

- **Increase the 5s shutdown timeout.** Delays the drop, doesn't remove it — explicitly called
  out as a non-fix in the original investigation. Rejected.
- **Stagger/rate-limit deploys to low-traffic windows.** Reduces exposure but doesn't fix the
  mechanism, and constrains how often the team can ship. Worth doing operationally regardless of
  whether the technical fix above lands, but out of scope for this spec.
- **Sticky sessions / single-process deploys.** Investigated and ruled out as the root cause
  already (state is durable, leases are latency-only) — would be solving a problem that doesn't
  exist here at real architectural cost.

## Non-goals

- True zero-downtime WS continuity (e.g. connection draining to a still-running sibling process
  before the old one exits, session migration mid-socket) — meaningfully more infrastructure
  (a real drain/rebalance step, likely an ASG lifecycle hook, which `api/CLAUDE.md` already notes
  doesn't exist today) for a UX papercut, not a correctness bug. Revisit only if the lighter fix
  above proves insufficient in practice.
- Changing deploy frequency/cadence — an operational decision, not this spec's call.

## Status

**Implemented 2026-08-24.** `api/internal/wsdrain` is a process-local registry of live sockets
(`Track`/`Untrack` from both WS handlers in `tablews.go`); `startServer`'s `OnStop`
(`internal/app/app.go`) calls `wsdrain.CloseAll(ctx, wsDrainGrace)` — a 1001 close frame to every
tracked connection, then a 1.5s grace — before `DrainAndRelease` and the existing 5s
`ShutdownWithContext`. The registry is a package-level singleton rather than an injected
dependency: it is process-wide transport state with exactly one consumer, and threading it through
`v1.Register`'s 30-argument signature would buy nothing.

## Rollout / verification

- Manually verify against a table with an open WS during a `setup-deploy.sh` run: confirm the
  close frame arrives (client `onclose` fires) before the process actually exits, and that the
  client's existing reconnect lands within one backoff cycle.
- No schema/API-visible change — this is purely `OnStop` sequencing, so no client version gate is
  needed.
