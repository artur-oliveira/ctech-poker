# CTech Poker CLI

A terminal client for CTech Poker — sandbox (play-money) tables only for now.

> Work in progress. See `docs/specs/2026-09-05-poker-cli.md` and
> `docs/plans/2026-09-05-poker-cli.md` at the repo root for the full design and
> implementation plan.

## Building from source

```sh
cd cli
go build -o poker ./cmd/poker
```

## Login

```sh
poker login              # opens your browser (PKCE)
poker login --api-key K  # or use a long-lived API key instead
poker logout
```

Credentials are stored in `~/.config/ctech-poker/credentials.json` (mode `0600`).
Override the config file, API/account URLs, or card rendering with
`--config`, `--api-url`, `--account-url`, `--cards ascii|color`, or the
`CTECH_POKER_API_URL` / `CTECH_POKER_ACCOUNT_URL` / `CTECH_POKER_CLIENT_ID` /
`NO_COLOR` environment variables.

Install instructions, the full command reference, and release downloads land
here as the remaining implementation-plan tasks complete.
