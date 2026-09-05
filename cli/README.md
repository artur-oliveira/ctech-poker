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

## Usage

Run `poker` with no arguments. It launches an interactive shell — there is no
`poker login` / `poker play` subcommand surface; everything happens inside
the running program, `/command`-style (like Claude Code's own shell):

1. **Login gate.** On first run (or after `/logout`), pick a login method:
   a browser (OAuth PKCE — opens automatically) or an API key (paste it and
   press enter). Credentials are stored in
   `~/.config/ctech-poker/credentials.json` (mode `0600`) and reused on the
   next run.
2. **Home REPL.** Once logged in, type a command and press enter:

   ```
   /profile        show your poker profile
   /achievements   show your achievement progress
   /play           join a table (coming soon)
   /enter <id>     join a table by room id (coming soon)
   /logout         forget stored credentials and log in again
   /help           list commands
   /exit           quit
   ```

Override the config file, API/account URLs, or card rendering with
`--config`, `--api-url`, `--account-url`, `--cards ascii|color`, or the
`CTECH_POKER_API_URL` / `CTECH_POKER_ACCOUNT_URL` / `CTECH_POKER_CLIENT_ID` /
`NO_COLOR` environment variables.

Install instructions and release downloads land here as the remaining
implementation-plan tasks complete.
