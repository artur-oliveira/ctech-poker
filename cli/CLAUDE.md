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

- [ ] `client_id=poker-cli`, type `public` (PKCE required, no secret).
- [ ] `redirect_uris`: `http://127.0.0.1/callback` and `http://localhost/callback`.
      **Open question:** does `ctech-account/api/internal/handler/authorize.go`'s
      `IsRedirectURIAllowed` match a loopback redirect ignoring the port (RFC 8252
      §7.3)? If not, register a fixed port (e.g. `http://127.0.0.1:8765/callback`)
      and set that fixed port in `internal/auth`'s PKCE receiver.
- [ ] `allowed_scopes`: `poker:rooms:read poker:players:read poker:sessions:read
      poker:hands:read poker:achievements:read poker:stats:read`.
- [ ] Add the grant to every environment's deploy reconciliation (dev/staging/prod) —
      mirrors the still-open real-money M2M scope gap documented in `api/CLAUDE.md`.
- [ ] Confirm `POST /v1.0/token` with `grant_type=api_key` issues a token carrying
      those same scopes for a key created with them.

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

## Testing

```sh
cd cli && go test ./... -race
```

No integration tests requiring a live server — everything below `internal/` is
tested against `httptest`/fake-transport stand-ins. TUI tests use
`github.com/charmbracelet/x/exp/teatest`.
