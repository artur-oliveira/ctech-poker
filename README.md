# ctech-poker

Online Texas Hold'em poker for the CTech ecosystem, with a real-money mode backed by
`ctech-wallet` and a sandbox (play-money) mode that never touches it.

Layout: `api/` (Go game server), `ui/` (web client), `cdk/` (infra), `proto/`
(shared wire schema), and `cli/` — a Go terminal client (`bubbletea` TUI,
sandbox only) that speaks the same protocol as the web UI. See
`cli/README.md` and `docs/specs/2026-09-05-poker-cli.md`. The CLI has its own
release cadence (`vX.Y.Z` tags → `.github/workflows/cli-release.yml`); the
API is not released.

**Documentação jurídica vigente:** os Termos do CTech Poker publicados pela
Central Jurídica estão na versão **2.1**. `CurrentPokerTermsVersion` em
`api/internal/player/model.go` reexige o aceite quando ela muda; a fonte pública
de verdade é `https://accounts.aoctech.app/products/poker`.

## Read-only OAuth scopes

Poker publishes 11 public `poker:*:read` scopes from
`api/internal/oauthresource/scope-manifest.json`: rooms, player profile,
sessions, hands, achievements, statistics, leaderboard, daily-reward status,
private player notes, and existing sandbox/reaction purchases. There are no
scopes for creating or joining a table, playing, mutating data, claiming a
reward, buying, refunding, or operating either WebSocket.

The daily-reward read permission maps to the existing cooldown endpoint
`GET /v1.0/sandbox-credits/`; its historical route name does not change the
OAuth scope name `poker:daily-reward:read`.

The Poker UI requests all 11 read scopes together with `openid profile`. On
`GET`, a scoped token must carry the exact route-family permission. Mutations
and both WebSockets deliberately have no public scope: they require a user
session (`sub` + `sid`) issued to the first-party `poker` OAuth client
(`azp=poker`). An API key or another OAuth client remains read-only. This
client binding limits delegated clients; it does not make a browser token a
secret. Contract tests keep the manifest, API policy, and UI request
synchronized. Deploy reconciles the public grants in CTech Account between CDK
and API; RFC 9728 metadata is at `GET /.well-known/oauth-protected-resource`.

Status (re-verified against the code, **2026-07-28**): **sandbox mode is live end to end**
(`api/`, `ui/`, `cdk/`), and the **real-money path is reachable** — `POST /v1.0/rooms/`
accepts `currency_mode: "real"` and rejects it unless `REAL_MONEY_ENABLED` is on
(`api/internal/api/v1/rooms.go:58-66`, `:262`), with `REAL_MONEY_ENABLED` /
`LEGAL_SIGNOFF_REF` wired from SSM by the API stack (`cdk/lib/api-stack.ts:251-254`).
Real money remains **off by default** and still blocked on the Brazilian regulatory opinion
below — the gate is a business decision, not a missing feature.

The provably-fair surface is built: the shuffle commit, root commit and (for full showdowns
only) the server seed are published on the table snapshot and persisted with hand history, and
the browser verifies them locally — `api/internal/engine/hand/snapshot.go`,
`ui/src/lib/deckVerify.ts`, `ui/src/components/hands/{DeckReveal,PartialDeckProof}.tsx`.
Hands that end without a showdown never publish the seed; they ship a per-position proof
instead, so the deck is verifiable without exposing mucked cards.

Read [OVERVIEW.md](OVERVIEW.md) for the product and game-rules spec, [ARCHITECTURE.md](ARCHITECTURE.md)
for the technical design (real-time transport, game-server model, wallet integration),
[PLAN.md](PLAN.md) for the phased build history, and [docs/README.md](docs/README.md) for the
implemented-vs-designed index.

## Relationship to other CTech services

- **ctech-account** — OIDC auth for the SPA and the API (`sub` + `sid` claims, `jwtverify`).
  Owns the identity-level display name/avatar; poker keeps its own table nickname.
- **ctech-wallet** — real-money buy-ins/cash-outs and the sandbox play-money ledger, reached
  M2M via `api/internal/walletclient`. Sandbox chips are non-convertible — see OVERVIEW.md § 5.
- **ctech-cdk** (`@aoctech/cdk`) — the `HaproxyEc2Service` construct and `Ec2ScriptRunner`,
  which fetches the shared `assets/ec2/*.sh` bootstrap scripts from
  `{env}-ctech-ec2-scripts` at boot; also owns the VPC, shared edge SG, private zone and
  shared buckets this repo imports. Editing a shared script versions this repo's launch
  template on its next deploy.
- **ctech-go-common** — `jwtverify`, `dynamo`, `cache`, `lock`, `problem`, `ratelimit`, `ws`.
- **Observabilidade de erros** — `api-commons/observability` centraliza logs estruturados; toda resposta de erro HTTP
  é registrada (`WARN` em 4xx, `ERROR` em 5xx) com `request_id`, método, path e causa interna quando disponível.
  `X-Request-ID` é preservado/gerado e exposto por CORS. Logs nunca incluem tokens, payloads ou PII desnecessária.
- **ctech-billing** — not used. Only relevant if the rake/table-fee model changes.

## Read this first

Real-money online poker sits in a legally ambiguous zone under Brazilian gambling regulation —
a **P0 non-technical risk**. See OVERVIEW.md § 11. Get a legal opinion before flipping
`REAL_MONEY_ENABLED` in any environment that faces real users, independent of how the
engineering goes.
