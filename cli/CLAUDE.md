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
      poker:hands:read poker:achievements:read poker:stats:read`.
- [ ] Add the grant to every environment's deploy reconciliation (dev/staging/prod) —
      mirrors the still-open real-money M2M scope gap documented in `api/CLAUDE.md`.
- [ ] Confirm `POST /v1.0/token` with `grant_type=api_key` issues a token carrying
      those same scopes for a key created with them.

**How to actually create it (2026-09-05 investigation): there is no CLI for this
yet.** `ctech-account/api/cmd/createclient` only makes confidential M2M clients;
`cmd/createresource` makes a resource server + its confidential scope publisher;
`OperatorService.EnsureFirstPartyPublicClient` — the only code path that creates a
first-party *public* client — is hardcoded to `SELF_CLIENT_ID` (Account's own
SPA) in `cmd/api/main.go`. Self-service `POST /v1.0/oauth-clients` generates a
random UUID `client_id` and never sets `first_party`. So Task 0b needs **a code
change in ctech-account**: either generalize `EnsureFirstPartyPublicClient` into a
`cmd/createpublicclient` operator command (parallel to `createclient`), or a
one-off provisioning script. Then run it per environment with AWS creds +
`TABLE_PREFIX`/`AWS_REGION`/`VALKEY_URL`.

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

Tag `cli/vX.Y.Z` (Go submodule tag convention). `.github/workflows/cli-release.yml`
runs `go test ./... -race` then `goreleaser release` (`cli/.goreleaser.yaml`),
publishing linux/darwin/windows × amd64/arm64 archives + `checksums.txt` to a
GitHub Release and updating the `aoctech/homebrew-tap` formula. `deploy.yml`'s
`dorny/paths-filter` lists never match `cli/**`, so a CLI-only push deploys
nothing and the API stays unreleased. `cli/install.sh` fetches the newest
`cli/*` release asset for the host.

## Testing

```sh
cd cli && go test ./... -race
```

No integration tests requiring a live server — everything below `internal/` is
tested against `httptest`/fake-transport stand-ins. TUI tests use
`github.com/charmbracelet/x/exp/teatest`.
