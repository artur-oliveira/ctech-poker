# ctech-poker

Online Texas Hold'em poker for the CTech ecosystem, with a real-money mode backed by
`ctech-wallet` and a sandbox (play-money) mode that never touches it.

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
- **ctech-cdk** (`@aoctech/cdk`) — shared `PrivateIpv4Ec2Service` construct and the EC2
  userdata helpers; also owns the VPC, ALB, wildcard cert and shared buckets this repo imports.
- **ctech-go-common** — `jwtverify`, `dynamo`, `cache`, `lock`, `problem`, `ratelimit`, `ws`.
- **ctech-billing** — not used. Only relevant if the rake/table-fee model changes.

## Read this first

Real-money online poker sits in a legally ambiguous zone under Brazilian gambling regulation —
a **P0 non-technical risk**. See OVERVIEW.md § 11. Get a legal opinion before flipping
`REAL_MONEY_ENABLED` in any environment that faces real users, independent of how the
engineering goes.
