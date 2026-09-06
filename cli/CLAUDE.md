# cli/ — CLAUDE.md

Go terminal client for CTech Poker (`bubbletea` TUI). Separate module from `api/` —
**never import `gopkg.aoctech.app/poker/api`** from anything under `cli/`. Sandbox
(play-money) only; see `docs/specs/2026-09-05-poker-cli.md` and
`docs/plans/2026-09-05-poker-cli.md` at the repo root for the full design.

## Proto

`internal/proto/poker.pb.go` is generated, committed Go — its own copy in package
`proto`, distinct from `api/internal/api/v1/proto` (package `pokerproto`). Regenerate
from the repo root after any `proto/poker.proto` change:

```sh
bash scripts/generate-proto.sh
```

That single script also regenerates the API's Go and the UI's TypeScript — if it
reports an unrelated one-line diff in those files (just a `protoc`/`protoc-gen-go`
patch-version bump in the generated header), revert that file; only commit an actual
content change.

## Provider dependency (not fixed in this repo)

Interactive (non-GET) poker operations require a first-party OAuth client
(`api/internal/api/v1/readscopes.go`'s `firstPartyPokerClientIDs`, which already
includes `poker-cli`). The client itself must still be **registered in
ctech-account** — a config/data change, tracked here rather than in that repo:

- [ ] `client_id=poker-cli`, type `public`, `first_party=true`, PKCE required, no secret.
- [ ] `redirect_uris`: **exactly** `http://127.0.0.1:51789/callback` (the fixed
      `auth.LoopbackPort`). **Confirmed 2026-09-05:** `ctech-account`'s
      `OAuthClient.IsRedirectURIAllowed` (`internal/domain/oauth/client/model.go`) is
      a plain `allowed == uri` string match — no RFC 8252 §7.3 port-agnostic
      loopback comparison — so an ephemeral port is impossible and the exact URL
      above must be registered verbatim. `validateRedirectURIs` allows `http://`
      only for `localhost`/`127.*`, which this is.
- [ ] `allowed_scopes`: `poker:rooms:read poker:players:read poker:sessions:read
      poker:hands:read poker:achievements:read poker:stats:read
      poker:player-notes:read`. The last one gates `rest.Client.Notes` (GET
      `/v1.0/players/me/notes/`), added 2026-09-05 for `/note`; without it
      that one call 403s (the note *save* itself is a POST and is gated by
      client identity, not scope — see `enforceReadOnlyScope` in
      `api/internal/api/v1/readscopes.go` — so it isn't blocked here).
- [ ] Add the grant to every environment's deploy reconciliation (dev/staging/prod) —
      mirrors the still-open real-money M2M scope gap documented in `api/CLAUDE.md`.
- [ ] Confirm `POST /v1.0/token` with `grant_type=api_key` issues a token carrying
      those same scopes for a key created with them.

`rest.Client`'s `/v1.0/social/*` methods (`Friends`, `FriendRequests`, `Blocked`,
`RecentPlayers`, `Inbox`, added 2026-09-05 for `/friends` `/requests` `/blocked`
`/recent` `/inbox`) need **no scope at all** — the whole `/social` group is gated
by `firstPartyOnly` (client identity), not `enforceReadOnlyScope`, per
`api/internal/api/v1/social.go` and `readscopes.go`'s `socialReadPathPrefix`
exemption. They start working the moment the `client_id=poker-cli` registration
above lands, same as every other write-shaped call in this file.

**2026-09-05: `ctech-account/api/cmd/createpublicclient` now exists** —
`ctech-account` PR https://github.com/artur-oliveira/ctech-account/pull/25 (open).
Once merged, run per environment:

```bash
AWS_REGION=us-east-1 TABLE_PREFIX=<env> go run ./cmd/createpublicclient \
  -client-id poker-cli -name "CTech Poker CLI" \
  -redirect-uri http://127.0.0.1:51789/callback \
  -scopes poker:rooms:read,poker:players:read,poker:sessions:read,poker:hands:read,poker:achievements:read,poker:stats:read
```

Idempotent — safe to re-run. `-redirect-uri` must be exactly
`http://127.0.0.1:51789/callback` (matches `auth.LoopbackPort` in this repo) —
`OAuthClient.IsRedirectURIAllowed` there is an exact string match, no RFC 8252
§7.3 port-agnostic loopback comparison.

Until this is done in an environment, the CLI in that environment can only run
GET-only commands (`profile`, `achievements`) — every mutation gets a `403` with a
message pointing back here.

## Architecture: one interactive shell, not subcommands

`poker` with no arguments launches one `bubbletea` program
(`cli/internal/tui.Shell`) for the whole session: a login gate (browser PKCE or
API key), then a `/command` home REPL, then (once Tasks 15-19 land) the same
program transitions into the lobby and table views. There is no `poker login` /
`poker profile` / `poker play` subcommand surface — see
`docs/specs/2026-09-05-poker-cli.md` §10's amendment and
`docs/plans/2026-09-05-poker-cli.md`'s "Amendment" section for why, and what
that changed relative to the original plan.

## Releasing

Tag `vX.Y.Z`. `.github/workflows/cli-release.yml` runs `go test ./... -race`
then `goreleaser release` (`cli/.goreleaser.yaml`), publishing
linux/darwin/windows × amd64/arm64 archives + `checksums.txt` to a GitHub
Release and updating the `artur-oliveira/homebrew-tap` cask. `deploy.yml`'s
`dorny/paths-filter` lists never match `cli/**`, so a CLI-only push deploys
nothing and the API stays unreleased. `cli/install.sh` fetches the newest
stable `vX.Y.Z` release asset for the host.

## Testing

```sh
cd cli && go test ./... -race
```

No integration tests requiring a live server — everything below `internal/` is
tested against `httptest`/fake-transport stand-ins. TUI tests use
`github.com/charmbracelet/x/exp/teatest`.
