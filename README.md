# ctech-poker

Online Texas Hold'em poker for the CTech ecosystem, with a real-money mode backed by
`ctech-wallet` and a sandbox (play-money) mode that never touches it.

Status: **planning — no implementation yet.** See [OVERVIEW.md](OVERVIEW.md) for the product
and game-rules spec, [ARCHITECTURE.md](ARCHITECTURE.md) for the technical design (real-time
transport, game-server model, wallet integration), and [PLAN.md](PLAN.md) for the phased build
plan.

## Relationship to other CTech services

- **ctech-account** — user auth for the SPA and lobby.
- **ctech-wallet** — real-money mode buy-ins/cash-outs. Sandbox mode uses an entirely separate,
  non-convertible play-money ledger owned by this repo — see OVERVIEW.md § Sandbox Isolation.
- **ctech-billing** — not required for MVP. Only relevant if/when a rake or table-fee monetization
  model is adopted (see OVERVIEW.md § Monetization — currently an open question, not a decision).

## Read this first

This spec flags a **P0 non-technical risk**: real-money online poker sits in a legally
ambiguous zone under Brazilian gambling regulation. See OVERVIEW.md § 10. Get a legal opinion
before real-money mode goes anywhere near production, independent of how the engineering goes.
