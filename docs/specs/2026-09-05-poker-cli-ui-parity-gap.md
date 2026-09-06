# CTech Poker CLI — UI parity gap

**Date:** 2026-09-05 **Scope:** what the web client (`ui/`) does at the table that the CLI (`cli/`)
does not, plus places where the CLI does something *different* (not just less). Companion to
`docs/specs/2026-09-05-poker-cli.md` (the MVP design) and
`docs/plans/2026-09-05-poker-cli.md`.

The CLI is sandbox-only and GET-scoped by OAuth design, so real-money, buy/rebuy purchases, and anything gated on a
write scope are permanently out of scope and not listed as gaps. Everything below is reachable with the scopes the CLI
already has.

---

## 1. Parity violations (CLI behaves wrong, not just less)

These are bugs against the CLI's *own* spec or against server contracts, not missing features.

### 1.1 Hole cards are always visible — the peek gate is not implemented

**Web:** the viewer's own hole cards render **face-down** during a live hand (`ui/src/components/table/Seat.tsx`). Press
`1` / `2` (or click a card) to toggle-reveal that card. They auto-reveal when the hand completes.

**Also gated behind peeking both cards:** the viewer's equity `~60%` and hand category ("par de nove"). The server sends
`ServerMessage.equity` and the viewer's own `hand_category` **unconditionally**; the web client withholds both until the
player has peeked *both* cards. This is deliberate — the
"all-in without peeking" / "won without peeking" achievements (`api/CLAUDE.md` "Hidden information") require the server
to be unable to tell whether the player looked, so the client must be the thing hiding it.

**CLI today:** `internal/game/view.go` copies `YourHole`, `YourStrength`,
`YourEquity` straight out of the snapshot and `internal/tui/table.go` renders
`Sua mão: 9♣ J♦ · par de nove · ~60%` in the header on every frame, with no peek state. Observed in the wild:

```
Pote: 40 · Mesa: K♠ T♠ 9♠ · Sua mão: 9♣ J♦ · par de nove · ~60%
```

**Impact:** silently defeats the no-peek achievements for every CLI user, and diverges from the spec, which explicitly
says equity is "shown only once the viewer has locally peeked (mirrors the web gate)".

**Fix shape (decided 2026-09-05):** per-hand local peek state — `[2]bool`, reset when `hand_id` changes, both forced
`true` once the hand completes (owner sees their own cards again, same as pre-feature). Hole cards render face-down
(`██ ██`) until peeked; `YourStrength` and `YourEquity` stay hidden in the header until **both** are peeked.

`/peek` command and `k` hotkey — **toggle** semantics:

- `/peek` or `/peek all` or `k` → toggle **both** cards (flip each independently:
  if they differ, this un-syncs further — acceptable; `k` is the "show/hide my hand" key).
- `/peek 1` → toggle card index 0 only.
- `/peek 2` → toggle card index 1 only.

Send the `peek_cards` frame **once per hand**, on the first peek of that hand (fire-and-forget achievement breadcrumb;
`card_index` is just a hint, it fetches nothing). No frame on toggling back to hidden or on subsequent peeks.

### 1.2 `Ctrl+C` at the table kills the process with no guard

**Web:** closing the tab is the only abrupt exit; the in-app "leave" flow is
`request_exit` (see §1.3 of the CLI fix from 2026-09-05).

**CLI today:** `internal/tui/shell.go` `handleKey` intercepts `tea.KeyCtrlC`
first thing, for every state, and returns `tea.Quit`. At the table this:

- does not send `fold` or a sit-out,
- does not `request_exit`,
- does not even `Close()` the socket cleanly (no `leaveTable`),

so the server sees a hard disconnect and the player is left seated until the AFK sweep / `disconnect_sit_out` fires.
`table.go`'s own `KeyCtrlC` handler (line ~242) is dead code — the shell eats the key first.

**Fix shape (user request):** at the table, first `Ctrl+C` → warn ("aperte Ctrl+C de novo para sair — você vai dar
fold / sit-out") and arm a short timer; second `Ctrl+C` within the window → if it's your turn send `fold`, else send
`ready:false` (sit out), then `request_exit` (or force-leave), then quit. Outside the table, keep today's immediate
quit.

### 1.3 `/peek` sends a frame that does nothing useful

Follows from §1.1: `commands.go` `peek()` builds a `peek_cards` frame with
`card_index` and treats it as "reveal my cards". `peek_cards` is an achievements-only breadcrumb; the cards are already
in the snapshot. `/peek`
should flip local reveal state and send the breadcrumb at most once per hand.

---

## 2. Missing table actions (server accepts them, CLI never sends)

The MVP spec (`§8.1`) explicitly deferred these; listing them so the backlog is in one place. All are auxiliary
`emitAux` commands in
`ui/src/lib/hooks/useTableRealtimeSession.ts`.

| Feature                                      | Frame(s)                                           | Web UI                                                          | CLI                                                            |
|----------------------------------------------|----------------------------------------------------|-----------------------------------------------------------------|----------------------------------------------------------------|
| **Show cards at showdown**                   | `show_cards` (`card_index` 0/1/absent)             | `WinnerCards` / seat control — muck or show one/both            | ✅ `/showcards [all\|1\|2]`                                    |
| **Rabbit hunt**                              | `request_rabbit_hunt`, `rabbit_hunt_verify_failed` | `RabbitHunt.tsx` — pay to run out the board after a fold-around | ✅ `/rabbit` (verify-failed refund frame still not sent)       |
| **Request winner's cards (live)**            | `request_winner_cards`                             | `WinnerCards.tsx` — loser pays to see a mucked winning hand     | ✅ `/reqcards`                                                 |
| **Answer a winner-cards request**            | `accept_winner_cards` / `decline_winner_cards`     | winner taps accept/decline                                      | ✅ `/accept` `/decline`                                        |
| **Run It Twice toggle**                      | `set_run_it_twice`                                 | `TablePreferencesDialog`                                        | ✅ `/rit <on\|off>` (RIT board fields still ignored in render) |
| **Preselect action** (check/fold, call-any…) | `preselect_action`                                 | `ActionBar` pre-commit while it's not your turn                 | ✅ `/preselect <mode\|off>` (no fixed-call amount)             |
| **Post big blind out of turn**               | `post_big_blind`                                   | prompt when you owe a BB to re-enter                            | ✅ `/postbb` (no snapshot cue that you owe one)                |
| **Keep seat** (after a would-be removal)     | `keep_seat`                                        | `RebuyDialog` / idle prompt                                     | ✅ `/keep` (no snapshot cue for the removal warning)           |
| **Bot challenge** (Turnstile)                | `bot_challenge`                                    | `BotChallenge.tsx`                                              | by design: surface "continue na web" and disconnect            |

## 3. Missing table information / panels

| Web surface                              | What it shows                                                                             | CLI                                                                                                                                                                                                                                                                                                                                                                                                                        |
|------------------------------------------|-------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `Seat.tsx` per-seat state                | connection dot, sitting-out, dealt-in, time-bank vs normal clock split, chip stack deltas | aligned seat table now shows position/name/stack/bet + folded/out; still no connection dot, dealt-in, or time-bank split                                                                                                                                                                                                                                                                                                   |
| `IdleWarning.tsx`                        | "you're about to be sat out" countdown                                                    | ~~done 2026-09-05~~ `TableModel.idleWarningLine` renders `idle_removal_unix_ms` — "você sai por inatividade em Ns — aja ou /keep" for the viewer, "<nome> sai por inatividade em Ns" for an opponent.                                                                                                                                                                                                                      |
| `PerimeterTimer` / time bank             | visual split between the decision clock and the reserve                                   | ~~done 2026-09-05~~ `TableModel.turnClock` renders the actor countdown for the viewer **and opponents** (`Vez de X · … · 15s (+15s banco)`), switching to "Ns de banco" once `action_base_deadline_unix_ms` passes. Per-seat reserve (`Seat.time_bank_ms`) flattened into `PlayerView` but not yet shown in the seat rows.                                                                                                 |
| `HandOutcome.tsx`                        | structured showdown: board, every revealed hand, side-pot results                         | ~~done 2026-09-05~~ `Narrator.potBreakdownLines` renders each `PotResult` layer — "pote principal / lateral N", "devolvido" for refunds, split-pot amounts per player, "(corrida N)" for run-it-twice. Single uncontested pot still collapses to one line.                                                                                                                                                                 |
| `HandRankingsDialog.tsx`                 | poker hand-ranking reference                                                              | deferred — user judged `/ranks` at the table visual clutter; if added, home-only                                                                                                                                                                                                                                                                                                                                           |
| `SessionRecap.tsx`                       | end-of-session P&L recap on removal                                                       | ~~done 2026-09-05~~ `formatSessionRecap` prints a multi-line recap on exit (tempo na mesa / entrada / retirada / resultado) from the `CurrentSession` fetch the exit handler already does. No hands-played / biggest-pot — needs a hand-history REST client the CLI doesn't have yet.                                                                                                                                      |
| `RealityCheck.tsx`                       | periodic "you've played Nh" responsible-gaming prompt                                     | ~~done 2026-09-05~~ `TableModel.maybeRealityCheck` prints a neutral recap (time at table, hands completed, `/summary` fetch) once per `RealityCheckEvery` boundary (fixed at the web's 60min default — no per-player preference store yet), held rather than dropped while it's the viewer's turn, same gate as the web.                                                                                                   |
| `TodayHighlight.tsx` / `LastWinners.tsx` | table highlights feed / recent big pots                                                   | CLI has `/last-winners` (local, from its own narration) — partial                                                                                                                                                                                                                                                                                                                                                          |
| `EquityTrainerPanel.tsx`                 | sandbox equity-trainer overlay                                                            | none                                                                                                                                                                                                                                                                                                                                                                                                                       |
| Player menu (`renderActionsMenu`)        | profile, add friend, note, mute, block, report                                            | ~~done 2026-09-05 (partial)~~ `/player <jogador>` shows position/stack/note + `/react`/`/note` hints — the CLI's stand-in for the popup. Add friend/mute/block/report have no CLI-reachable endpoint yet (§4 social gap); this only covers what the CLI can already do.                                                                                                                                                    |
| `PlayerNoteDialog.tsx`                   | private per-opponent notes with tags                                                      | ~~done 2026-09-05~~ `rest.Client.Notes`/`SaveNote` + `/note <jogador> [tag <cor>\|clear\|<texto>]` (view/save/clear); notes for seated opponents prefetch once per table (`fetchMissingNotes`); a tagged seat gets a colored `●` in its row. Needs `poker:player-notes:read` added to the CLI OAuth client's scopes (tracked in `cli/CLAUDE.md`) — until then the read 403s silently, same as the web's own `silentError`. |
| `TableReactions.tsx` catalog             | reaction picker with owned/premium reactions                                              | ~~done 2026-09-05~~ `internal/game/reactions.go` ports the catalog; `/react` validates the code + resolves the target by name/position/id and requires one for targeted reactions; Tab autocompletes code then player (`commandMenu.argFn`). Still no owned/premium distinction (server rejects unowned).                                                                                                                  |
| `Chat.tsx`                               | scrollback, unread badge, bubbles                                                         | ~~done 2026-09-05~~ chat is still narrated inline, but also kept in its own `TableModel.chat` history; `/chat` replays the last 20 (timestamped) and clears the unread count; a `💬 N mensagem(ns) nova(s)` badge shows in the header whenever there's unread chat, independent of scroll position. No bubbles (text-only), naturally. |
| `InviteDialog.tsx`                       | share code + invite link + friend invite                                                  | CLI `/share` copies the link — partial (no friend invite)                                                                                                                                                                                                                                                                                                                                                                  |
| `RebuyDialog.tsx` / auto-rebuy           | top-up when stacked low                                                                   | auto-rebuy chosen at join; no mid-session `/rebuy` (needs write scope anyway)                                                                                                                                                                                                                                                                                                                                              |

## 4. Missing non-table surfaces

The CLI home REPL has `/play`, `/enter`, `/profile`, `/achievements`,
`/summary`. The web app also has, all GET-able:

- ~~**Room browser**~~ — **dropped 2026-09-05.** `/play` (join-or-create) plus
  `/enter <id>` covers it; no listing needed.
- **Hand history** — navigable past hands (`docs/plans/...poker-hand-history`). Spec calls this a non-goal; still a gap.
- **Head-to-head stats** / player profiles (`2026-08-21-head-to-head-stats`).
- ~~**Friends / social**~~ — **done 2026-09-05 (read side).** `/friends` (list +
  presence), `/requests [sent]` (incoming/outgoing), `/blocked`, `/recent`,
  `/inbox` — all GET, first page only, via new `rest.Client` methods
  (`internal/rest/social.go`). Needs no extra OAuth scope (see `cli/CLAUDE.md` —
  `/social` is gated by client identity, not `poker:*` scopes), just the same
  pending `poker-cli` registration every write-shaped call already needs.
  Still missing: sending a friend request / accepting / declining / muting /
  blocking (all POST, deliberately deferred — `2026-08-16-social-friends-safety-and-recent`),
  and pagination past the first page.
- **Leaderboards / achievement seasons** browsing.
- **Store / cosmetics / reactions catalog** — view-only would still need the catalog endpoint; purchasing needs a write
  scope (out of scope).
- **Multi-table** — the web grid; spec non-goal.

**Fixed 2026-09-05:** long home-REPL output (`/achievements`, `/help`) was only scrollable via PgUp/PgDn with no hint it
could scroll. Arrow keys now scroll the scrollback when the command menu isn't open, and a "↑↓ rolam" hint shows
whenever the viewport isn't at the bottom (`Shell.layoutHeights` reserves the row).

## 5. Visual / layout (user feedback)

Current header packs pot, board, and hole cards onto one wrapped line:

```
Pote: 40 · Mesa: K♠ T♠ 9♠ · Sua mão: 9♣ J♦ · par de nove · ~60%
Jogadores: [D/SB] VOCÊ 2113 (+20) · ▶ [BB] Artur 9876 1180 (+20)
```

Complaints: cluttered; community cards and hole cards are hard to pick out.

**Direction:** give the board its own line and the hole cards their own line (face-down until peeked per §1.1), e.g.

```
  Board   K♠ T♠ 9♠            Pote 40
  Sua mão ██ ██   (/peek para ver)
  ...
  Sua mão 9♣ J♦ · par de nove · ~60%     ← only after peeking both
```

Also worth checking: the `(+20)` committed-chips suffix and position tags are doing a lot of work in a narrow column; a
two-row seat block or aligned columns would read better. Defer to the `impeccable` skill for the actual redesign.

---

## 6. Priority (decided 2026-09-05)

1. ~~**§1.1 peek gate** + **§1.2 Ctrl+C guard**~~ — **done 2026-09-05.**
   Per-hand `[2]bool` peek state in `TableModel` (reset on `hand_id` change, forced on `stage=="complete"`); `██ ██`
   until peeked, strength/equity gated on both; `/peek [all|1|2]` + `k` are local toggles, one `peek_cards`
   breadcrumb per hand. Ctrl+C at the table: first press warns + arms a 3s window, second folds (if your turn) / sits
   out, `request_exit`, then quits;
   `shell.go` delegates Ctrl+C to the table model while seated.
2. ~~**§5 layout**~~ — **done 2026-09-05.** Board and hole cards each on their own labelled line (`Board  … · Pote …` /
   `Sua mão: …`), all width tiers. Seat list is now an aligned table (`playerRows`): `▶` actor marker, position / name /
   stack in fixed columns, trailing `aposta N` /
   `desistiu` / `fora`; viewer row bold, actor row coloured, folded/out dimmed. Falls back to the old packed two-line
   list when the terminal is under 20 rows tall so the log never vanishes. ~~**§2 `show_cards` / rabbit-hunt /
   winner-cards**, **§2 `keep_seat` /
   `post_big_blind` / `preselect_action`**~~ — **done 2026-09-05.** New fire-and-forget table commands, each an
   `action_id`-carrying frame sent straight through (no per-command pending/lock state — the next snapshot or an `error`
   line is the feedback): `/showcards [all|1|2]`, `/rabbit`,
   `/reqcards`, `/accept`, `/decline`, `/keep`, `/postbb`, `/preselect
   <check_fold|fold|call|call_any|all_in|off>` (echoes
   `expected_snapshot_version|hand_id|stage`; fixed-call amount left to the server). Registered in `tableCommandSpecs`.
3. Everything in §3/§4 — separate, individually-scoped follow-ups, later.
4. ~~Room browser~~ — dropped.

**Note:** `cli/internal/tui/{table,menu,format,shell}.go` were under heavy parallel edit on 2026-09-05 (table.go +500
lines). Re-read the current tree before implementing — `TableExitedMsg` grew fields, exit now correlates on
`action_ack` via `exitActionID`, etc.
