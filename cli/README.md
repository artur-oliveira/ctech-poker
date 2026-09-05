# CTech Poker CLI

A terminal client for CTech Poker — sandbox (play-money) tables only for now.

> Work in progress. See `docs/specs/2026-09-05-poker-cli.md` and
> `docs/plans/2026-09-05-poker-cli.md` at the repo root for the full design and
> implementation plan.

## Install

**Script (Linux / macOS):**

```sh
curl -fsSL https://raw.githubusercontent.com/aoctech/ctech-poker/main/cli/install.sh | sh
```

Downloads the latest release binary for your OS/arch into `~/.local/bin/poker`
(override with `PREFIX=/usr/local`).

**Homebrew (macOS / Linux):**

```sh
brew install aoctech/tap/poker
```

**Windows:** download `poker_<version>_windows_<arch>.zip` from the
[releases page](https://github.com/aoctech/ctech-poker/releases) (tags prefixed
`cli/`), unzip, and put `poker.exe` on your `PATH`.

**From source:**

```sh
cd cli
go build -o poker ./cmd/poker
```

Releases are cut from `cli/vX.Y.Z` git tags and cover linux, macOS and Windows
on amd64 and arm64. The API itself has no release — only this CLI does.

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
   /play           join a table (pick size, stake, buy-in)
   /enter <id>     join a table by room id
   /clear          clear the screen (Ctrl+L works too)
   /logout         forget stored credentials and log in again
   /help           list commands with descriptions
   /exit           quit
   ```

   Typing `/` opens a Claude-Code-style suggestion menu: `↑`/`↓` to move,
   `Tab`/`Enter` to accept, `Esc` to dismiss. Long output (like the
   achievements list) scrolls — `PgUp`/`PgDn`/`Home`/`End`.

3. **At a table** the same conventions apply — a `/` prompt with menu +
   Tab-complete, `PgUp`/`PgDn` scrollback, `Ctrl+L`/`/clear`:

   ```
   /check /call /raise <v> /pot /allin /fold
   /talk <msg>  /react <code> [player]  /peek [all|1|2]
   /sitout /ready /summary /last-winners /share
   /exit /exit! /clear /help
   ```

   Hotkeys `f`/`c`/`r`/`p`/`k` map to fold/call/raise/pot/peek on your turn.

Override the config file, API/account URLs, or card rendering with
`--config`, `--api-url`, `--account-url`, `--cards ascii|color`, or the
`CTECH_POKER_API_URL` / `CTECH_POKER_ACCOUNT_URL` / `CTECH_POKER_CLIENT_ID` /
`NO_COLOR` environment variables.
