# CTech Poker CLI

A terminal client for CTech Poker — sandbox (play-money) tables only for now.

> Work in progress. See `docs/specs/2026-09-05-poker-cli.md` and
> `docs/plans/2026-09-05-poker-cli.md` at the repo root for the full design and
> implementation plan.

## Install

**Script (Linux / macOS):**

```sh
curl -fsSL https://raw.githubusercontent.com/artur-oliveira/ctech-poker/main/cli/install.sh | sh
```

Downloads the latest release binary for your OS/arch into `~/.local/bin/poker`
(override with `PREFIX=/usr/local`).

**Homebrew (macOS / Linux):**

```sh
brew install --cask artur-oliveira/tap/poker
```

**Windows:** download `poker_<version>_windows_<arch>.zip` from the
[releases page](https://github.com/artur-oliveira/ctech-poker/releases), unzip,
and put `poker.exe` on your `PATH`.

**From source:**

```sh
cd cli
go build -o poker ./cmd/poker
```

Releases are cut from `vX.Y.Z` git tags and cover linux, macOS and Windows
on amd64 and arm64. The API itself has no release — only this CLI does.

## Usage

Run `poker` with no arguments. It launches an interactive shell — there is no
`poker login` / `poker play` subcommand surface; everything happens inside
the running program, `/command`-style (like Claude Code's own shell):

1. **Login gate.** On first run (or after `/logout`), pick a login method:
   a browser (OAuth PKCE) or an API key. Picking the browser locks the
   screen on a waiting spinner with the authorize URL shown — press `C` to
   copy it (in case the browser didn't open on its own) or `Esc` to cancel
   and pick a different method. Credentials are stored in
   `~/.config/ctech-poker/credentials.json` (mode `0600`) and reused on the
   next run.
2. **Home REPL.** Once logged in, type a command and press enter:

   ```
   /profile        show your poker profile
   /achievements   show your achievement progress
   /hands          browse paginated hand history and open hand details
   /friends        browse friends and presence (`next` / `prev` paginate)
   /requests       browse received or sent friend requests
   /recent         browse opponents from the last 90 days
   /blocked        browse blocked players
   /inbox          browse social activity and table invites
   /play           join a table (pick size, stake, buy-in, auto-rebuy)
   /enter <id>     join a table by room id
   /clear          clear the screen (Ctrl+L works too)
   /logout         forget stored credentials and log in again
   /help           list commands with descriptions
   /exit           quit
   ```

   Typing `/` opens a Claude-Code-style suggestion menu: `↑`/`↓` to move,
   `Tab`/`Enter` to accept, `Esc` to dismiss. Long output (like the
   achievements list) scrolls — `PgUp`/`PgDn`/`Home`/`End`.

   `/hands` opens a dedicated history archive instead of printing into the
   home scrollback. It summarizes the current page's result, groups hands by
   day, and keeps the active row visible. Use `↑`/`↓` (or `j`/`k`) to choose a
   hand, `Enter` to open its cards, board, opponents, action timeline, and
   available shuffle proof, `N`/`P` to move between cursor pages, and `Esc` to
   go back. Inside a detail, `↑`/`↓` and `PgUp`/`PgDn` scroll; `Esc` returns to
   the list and `Q` returns home.

   Social lists are cursor-paginated too: append `next` or `prev` to
   `/friends`, `/recent`, `/blocked`, or `/inbox`; requests use
   `/requests [sent] [next|prev]`.

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

See [`WEB_PARITY.md`](WEB_PARITY.md) for the audited differences between the
web application and this terminal client.
