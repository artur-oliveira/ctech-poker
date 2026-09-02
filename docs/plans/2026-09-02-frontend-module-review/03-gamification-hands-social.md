# Frontend module review — Gamification, hand history, profile, social

> Part of the 2026-09-02 systematic review. Extends the leaderboard OK-vs-impeccable analysis (§7 of
> the parent doc, Issues 34–36/40) to every other screen in this slice. Issues renumbered in §8
> (F-G1…F-G15). The public-share anonymization path (`api/internal/api/v1/handshares.go`) was audited
> and is **solid**.

## 1. Per-screen OK-vs-impeccable

### Achievements (`app/achievements/page.tsx`, `components/achievements/*`, `lib/achievements.ts`)

| Dimension | Current (file:line) | Impeccable |
|---|---|---|
| Sees own progress | `getMyAchievements(mode)` fetched **first page only** (`lib/api/achievements.ts:30–34` returns `.data.data`, cursor never followed). `progressMap` (`page.tsx:29`) → `stats`, `completionRate`, `nextMilestone`, secret-unlock gate (`:34–35`) all computed over a truncated counter set → **F-G1** | loop the cursor, or a `/achievements/me/summary` returning every touched key |
| "You just unlocked X" moment | none — `AchievementToast` is table-only, gone in 4.2 s; API returns `{key,count}` only, no `unlocked_at` → **F-G2** | "Recém-desbloqueadas" rail; a celebration on arriving from a table unlock deep-link |
| Empty state (new player) | authed + zero → all-zeros overview + full ladder; not-authed → catalog + sign-in nudge (`:97–99`). Reasonable | "Jogue sua primeira mão para acender a primeira estrela" + lobby CTA |
| Keyboard / SR | cards are `h3`, sr-only `h2`, progressbars have aria. But tier **stars are `<button>`s that do nothing on click/Enter/Space** — hover/focus-only tier preview (`AchievementCard.tsx:40–47`) → **F-G13** | non-interactive `<span aria-label>` ladder, or give activation a real function |
| Wallet mode | always defaults `'sandbox'` (`page.tsx:20`), never seeds from `me.wallet_mode` | default to the player's active wallet |
| Stale data | none — keys/counts only. Copy: excellent, idiomatic pt-BR | — |

### Leaderboard (`app/leaderboard/page.tsx`) — beyond §7

| Dimension | Current (file:line) | Impeccable |
|---|---|---|
| Your position | §7 / Issue 34 (not re-filed). Also `data.find` + `data.findIndex` for the same viewer — two scans (`:24`, `:42`) | per Issue 34; compute the index once |
| list size / virtualization | `data.slice(3)` renders **every** remaining row with a per-row `--delay` (`:85–97`); `leaderboard(mode)` reads first page only (`lib/api/gamification.ts:13–15`). Hand history is virtualized; this isn't → **F-G7** | window-virtualize; cap the intro stagger to the first ~10 |
| mode tabs | sandbox/real only; `Entry` carries `hands_played`/`hands_won`/`win_rate` but there's **no metric switch** and no season toggle (§7.3) | metric SegmentedControl + season toggle |
| copy bug | `de {data.length} jogadore{data.length === 1 ? 'r' : 's'}` (`:42`) → renders **"1 jogadorer"** → **F-G7** | `1 jogador` / `N jogadores` |
| error state | `.lobby-empty` + bare `.link-retry` button (`:54–56`) — CLAUDE.md wants `RecoveryState`/`SystemState` → **F-G7** | `RecoveryState nested` |

### Hand history list (`app/hands/page.tsx`)

| Dimension | Current (file:line) | Impeccable |
|---|---|---|
| pagination | hybrid `IntersectionObserver` sentinel **and** a real "Carregar mais" button (`:168–247`) — keyboard covered; window-virtualized. Good | — |
| own progress | `stats` over **loaded** hands, honestly labelled "Mãos carregadas / Saldo carregado" (`:151–207`) → **F-G12** | add lifetime totals from a stats endpoint alongside the "carregadas" subset |
| filtering / grouping | none — no outcome/table/date filter, no date headers → **F-G12** | filter chips + day/session grouping |
| empty state | first-hand card, two face-down cards, "Sua primeira mão começa no lobby" + CTA — **impeccable already** | — |
| `ended_at` units | `formatDate(hand.ended_at / 1000)` then `*1000` inside (`:27–31`, `:80`) — treats `ended_at` as ms → **F-G8** | consistent units |

### Hand history detail (`app/hands/history/page.tsx`)

| Dimension | Current (file:line) | Impeccable |
|---|---|---|
| stale opponent name/avatar | `o.name`, `o.avatar_url` denormalized (`:180–181`) — Issue 36 / 40, not re-filed | read-time resolution |
| opponent actionable? | **No.** opponent `<article>` is inert text + avatar — can't open profile, add friend, mute, report, note → **F-G4** | reuse `PlayerActionsMenu` on each opponent row |
| provably-fair UX | `DeckReveal` / `PartialDeckProof` chosen correctly (`:213–221`), WebCrypto recompute, 52-slot grid — impeccable | — |
| fairness-unavailable | falls to `<p className="deck-reveal-status mismatch">` (`:220–221`) — **error/red styling for a neutral "not available"** → **F-G14** | neutral `.deck-reveal-status` |
| landmarks | `h1` + `h2` per section survive loading/error via `RecoveryState nested` | — |
| clipboard failure | `copyTableId` catch → `setTableCopied(false)` silently (`:103–110`) | toast / inline "selecione e copie" |

### Hand replayer (`components/hands/HandReplayer.tsx`)

| Dimension | Current (file:line) | Impeccable |
|---|---|---|
| blind context | `<TableStage ... bigBlind={25}>` **hardcoded** (`:182`) → wrong pot/blind rendering for any table not at BB 25 → **F-G3** | pass the hand's real blind level |
| keyboard transport | buttons + range focusable, but no Space=play/pause, no ←/→ step (`:200–243`) — the live table has arrow-key control, this doesn't → **F-G11** | key handlers on the replayer region |
| reduced motion | autoplay auto-advances (`:92–104`) regardless of `prefers-reduced-motion` → **F-G11** | pause-by-default / step-only under reduced motion |
| speed | 1×/2× only | add 0.5× |
| stale names | history path uses `HandItem.opponents` (real); share path fully aliased server-side (verified) | read-time resolve for the history path (Issue 36) |

### Public shared hand (`app/share/page.tsx`, `handshares.go`)

| Dimension | Current (file:line) | Impeccable |
|---|---|---|
| consent / anonymization | **Solid.** every `player_id` → `hero`/`player_N` incl. unknown ids (`handshares.go:125–135`); every seat name hardcoded `"Jogador"`/`"Você"` (`:172–181`); `Action` has **no chat/reaction text field** (`handshare/store.go:54–62`); opponent hole cards only if genuinely shown | add a test asserting no raw `player_id` survives a share payload |
| revocation | `revokeHandShare` exists (`lib/api/handShares.ts:35–37`) but **no UI lists or revokes your shares** → **F-G5** | "Meus links compartilhados" panel (list + revoke) |
| link preview | `/share` is `noindex`, static export can't SSR per-token OG → links render generic → **F-G15** | one generic static share OG card |
| landmarks | `<main>` + nav + `<h1>` = chip amount — h1 is a number, not a title | `h1` "Mão compartilhada" with the amount as subhead |

### Profile showcase — public view (`app/profile/page.tsx`)

| Dimension | Current (file:line) | Impeccable |
|---|---|---|
| private / own / logged-out | `showcase.isError || !showcase.data` → one generic "Vitrine indisponível / Este perfil não existe" (`:53–57`) — can't tell private / your-own / not-found apart → **F-G9** | distinct copy per case; if it's the viewer's own id, link to the editor |
| loading state h1 | loading branch (`:46–52`) + Suspense fallback (`:112–119`) render **no `<h1>`** — CLAUDE.md: a real `h1` survives every state → **F-G9** | `<h1>` skeleton in both |
| featured achievements | shows raw `count` "registradas" (`:80`), no stars/tier | show tier/stars earned |
| `ended_at` units | `best_hand.ended_at < 1e12 ? *1000 : ended_at` (`:93`) — **a live symptom of the units inconsistency** → **F-G8** | fix the serializer, delete the heuristic |
| playstyle privacy | `playstyle_public` default off, 200-hand floor, per-badge disclosure of the reason (`ProfileShowcaseDialog.tsx:60–63`) — leak is consensual and explained | — |

### People / friends (`app/people/page.tsx`, `components/social/*`)

| Dimension | Current (file:line) | Impeccable |
|---|---|---|
| pagination | "Carregar mais" per list — consistent, keyboard-fine | — |
| activity feed names | `nameResolver(friends.items, requests.items)` (`people/page.tsx:98`) — inbox events carry **only `actor_id`** (`api/social.ts:28–37`); an actor not in your friends/current-direction-requests → "Visitante" → **F-G6** | inbox events carry `actor_name` + `avatar_url`, or a batch resolve |
| tab action counts | tab labels static ("Solicitações"), no "(3)" — you can't see which tab needs attention | pending counts on tab labels |
| discovery / enumeration | exact friend-code only, explicit "nome de exibição não é único" (`FriendCodeLookup.tsx:15–16`) — good | — |
| privacy copy | header + blocked-tab hint explain presence-only-for-friends, table-only-if-opted-in, block≠table — excellent | — |
| stale friend name/avatar | `player.name`/`avatar_url` on every row — denormalized; a renamed friend shows stale until the row is rewritten | read-time resolution (Issue 36 family) |

### Matchup view (inside `app/profile/page.tsx:97–104`)

| Dimension | Current (file:line) | Impeccable |
|---|---|---|
| data shown | `hands_together`, `viewer_wins`, `opponent_wins` only. `MatchupStats` **also** has `ties`, `heads_up_hands_together`, `net_change_viewer` (`api/player.ts:71–78`) — all fetched, none shown → **F-G10** | show ties + a net-chips line; "venceu 3, ele 4" of 10 leaves 3 unexplained |
| self guard | query enabled for your own id too; relies on `hands_together > 0` to hide (API 400s for self) | disable when `id === me` |

## 2. New issues (summaries; full acceptance criteria in the parent §8)

- **F-G1 · [FRONTEND/achievements] Progress map is first-page-only → wrong stars, completion %, next-star, secret unlocks** — **High · S · $0.** `getMyAchievements` returns one page, cursor never followed; `page.tsx:29` + `ProfileShowcaseDialog.tsx:29` derive everything from the partial map. Fix: follow the cursor, or `GET /players/me/achievements/summary` (Valkey hash). Acceptance: completion % matches summing every tier; a secret achievement past its first tier appears even on page 2; showcase editor lists every `count>0` key.
- **F-G2 · [BACKEND/achievements] No unlock timestamp → no "recently unlocked" moment** — Medium · M · $0. Record `last_tier_at`/`unlocked_at` when `RecordHand` crosses a threshold; expose on `/players/me/achievements`; frontend "Recém-desbloqueadas" rail. Idempotent with Issue 38's guard.
- **F-G3 · [FRONTEND/hand-replayer] Hardcoded `bigBlind={25}` misrepresents pot/blinds** — Medium · S · $0. Add `big_blind` (+ level) to `HandItem` and the share `Share` struct; thread into `HandReplayer`; legacy hands derive from `post_big_blind` amount.
- **F-G4 · [FRONTEND/hands] Hand-history opponents are inert** — Medium · M · $0. Wrap each opponent identity in a profile link + `PlayerActionsMenu` (add-friend / mute / block / report / note; report carries `hand_id`). Hidden for own seat / logged out.
- **F-G5 · [FRONTEND/share] No way to list or revoke your shared-hand links** — Medium · M · $0. `revokeHandShare` is implemented and called nowhere. `GET /players/me/hand-shares` + a "Meus links compartilhados" panel with Revogar; show the existing link when the dialog is reopened for the same hand.
- **F-G6 · [BACKEND/social] Inbox events carry only `actor_id`; feed can't name most actors** — Medium · S · $0. `nameResolver` only knows friends + current-direction requests; a friend request from a stranger renders "Visitante". Denormalize `actor_name`+`actor_avatar_url` at write time, or a batch `GET /social/players?ids=`.
- **F-G7 · [FRONTEND/leaderboard] "1 jogadorer" plural bug + no virtualization + non-standard error state** — Low · S · $0. Fix plural; virtualize once >~50 rows; error → `RecoveryState nested`; compute the viewer index once.
- **F-G8 · [BACKEND/player] `ended_at` unit inconsistency across hand endpoints** — Low · S · $0. `/hands` + `/hand/:id` return ms; `ProfileShowcase.best_hand.ended_at` needs a `< 1e12 ? *1000` runtime heuristic (`profile/page.tsx:93`) — a dev already hit seconds. Pick epoch ms everywhere; delete the heuristic; add a shared `handEndedAtMs()`.
- **F-G9 · [FRONTEND/profile] Private / own / not-found showcase collapse to one wrong message; loading has no `h1`** — Low · S · $0. Distinguish by status/problem type; add an `<h1>` skeleton to both loading branches.
- **F-G10 · [FRONTEND/profile] Matchup drops ties and net result** — Low · S · $0. Add ties to the sentence + a net-chips line (`net_change_viewer`); disable the query for your own id.
- **F-G11 · [FRONTEND/hand-replayer] No keyboard transport; autoplay ignores reduced-motion** — Low · S · $0. Space/←/→/Home on the section (respecting `event.repeat`); no auto-start under `prefers-reduced-motion`.
- **F-G12 · [FRONTEND/hands] History has no filters/grouping and no lifetime totals** — Low · M · $0. Outcome + table filter chips; day/session group headers; a lifetime-totals strip from a cheap aggregate (Valkey counters already exist for the leaderboard).
- **F-G13 · [FRONTEND/achievements] Tier stars are `<button>`s that do nothing on activation** — Low · S · $0. Make the ladder one non-interactive labelled element; drive the tooltip from `:focus-within`. Tab stops per card 5 → ≤1.
- **F-G14 · [FRONTEND/hand-history] "Fairness proof unavailable" styled as an error** — Low · XS · $0. Drop the `mismatch` class for legacy-hand-no-proof; calmer copy. Genuine hash mismatch keeps the failure style.
- **F-G15 · [FRONTEND/share] Shared links have no rich preview** — Low · M · $0. One generic static OG card for `/share` via the existing `og:capture` pipeline; keep the page `noindex`; self-hosted assets.

## 3. Verified, no action
Public-share anonymization (server-side, thorough — aliases incl. unknown ids, no chat text in the payload, opponent cards only if genuinely shown); provably-fair verify UX on the history detail (WebCrypto, correct full-seed vs partial-proof selection); friend-code discovery (exact match only, non-enumerable); playstyle privacy (opt-in, 200-hand floor, explained); the people/friends privacy model and copy; hand-history list empty state and hybrid pagination.
