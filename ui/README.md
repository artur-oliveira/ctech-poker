# ctech-poker — UI (Next.js SPA)

Next.js 16 SPA (App Router) for the poker lobby, tables, and game client. Everything below is
anchored to `ui/src` as of **2026-07-28**, not to `DESIGN.md`/`PRODUCT.md` (which are design specs).
Sandbox play is live end to end; the wallet-mode switch for real money exists in the UI but the
backend gate (`REAL_MONEY_ENABLED`) is off by default.

## Stack

- Next.js **16.2.10** (App Router, `src/app/`), React **19.2.7**, TypeScript 5 (`package.json`).
- **Static export** in prod (`output: 'export'`, `next.config.ts:18`); served from S3 +
  CloudFront. SPA route manifest published to a CloudFront **KeyValueStore**
  (`scripts/publish-routes.sh`). `images: {unoptimized: true}` — there is no Next image
  optimizer and no server route at runtime.
- Real-time: **`@aoctech/ws-client`** with **binary protobuf** frames — `src/lib/ws/utils.ts`
  encodes/decodes against `src/lib/api/proto/poker.ts` (ts-proto, generated from
  `../proto/poker.proto`). Not JSON.
- Auth: **`@aoctech/auth-client`** (the design docs say `ctech-oauth-client`; trust the code).
- Data: `@tanstack/react-query`; forms: `react-hook-form` + `zod`; UI: `@base-ui/react`/shadcn
  + Tailwind 4; icons: `lucide-react`.
- Fonts: Geist / Geist Mono via `next/font/google`, bound to `--font-sans` / `--font-mono` in
  `src/app/layout.tsx:8-9`. (Older docs claim this is unwired — it is wired.)
- Dev proxy: `next.config.ts` rewrites `/v1.0/*` → `DEV_API_ORIGIN` (default
  `http://localhost:8003`). Mock mode: `NEXT_PUBLIC_MOCK_API=true` (`dev:mock`) runs a full
  in-memory realtime engine (`src/dev/mockRuntime.ts`, aliased to no-op stubs in prod builds).

## Configuration (environment variables)

| Key | Where read | Purpose |
|---|---|---|
| `NEXT_PUBLIC_API_URL` | `src/lib/api/client.ts` | poker API base URL (HTTP + derives WS origin); dev default `http://localhost:8003` via proxy |
| `NEXT_PUBLIC_APP_URL` | `src/app/layout.tsx` | `metadataBase` for OG/meta tags |
| `NEXT_PUBLIC_MOCK_API` | `src/lib/mockConfig.ts` | `true` runs the in-memory mock realtime engine instead of a live API |
| `NEXT_PUBLIC_CTECH_URL` | `src/lib/auth/oauth.ts` | ctech-account base URL for `OAuthClient` |
| `NEXT_PUBLIC_CTECH_CLIENT_ID` | `src/lib/auth/oauth.ts` | poker's OAuth client id |
| `DEV_API_ORIGIN` | `next.config.ts` | dev-only rewrite target for `/v1.0/*` |

No `.env.example` exists in `ui/`. `NEXT_PUBLIC_*` values are injected at build time by
`frontend.yml` per environment — static export bakes them in, there is no runtime env lookup.

## Routes (App Router, `src/app/`)

Every page is `'use client'`; only `layout.tsx` and `share/layout.tsx` are server components.

| Route | File | Purpose |
|---|---|---|
| `/` | `page.tsx` | Landing: hero demo table, features, achievement teaser, OAuth CTAs |
| `/lobby` | `lobby/page.tsx` | Stakes grid with explicit join/create states and sandbox buy-in ranges, active-table banner, create-room dialog, daily spin, onboarding |
| `/table?id=<id>` | `table/page.tsx` | The live table (room id is a **query param**, not a segment) |
| `/hands` | `hands/page.tsx` | Infinite-scroll hand history with won/lost filter |
| `/hands/history?table_id=&hand_id=` | `hands/history/page.tsx` | One hand: seats, board, action timeline, fairness proof, export, share |
| `/hands/replay?table_id=&hand_id=` | `hands/replay/page.tsx` | Frame-by-frame `HandReplayer` |
| `/leaderboard` | `leaderboard/page.tsx` | Podium + ranking list, highlights the viewer's row |
| `/achievements` | `achievements/page.tsx` | Catalog + own progress, all/unlocked/in-progress/completed tabs |
| `/store` | `store/page.tsx` | Durable Fichas hub: sandbox balance, daily reward, Pix chip packages, and expandable recent purchase activity |
| `/profile?id=<playerId>` | `profile/page.tsx` | **Public read-only showcase** of another player |
| `/share?id=<token>` | `share/page.tsx` | Public anonymized shared hand (`robots: noindex`) |
| `/guide` | `guide/page.tsx` | Illustrated how-to-play |
| `/poker-rules` | `poker-rules/page.tsx` | Rules + hand rankings reference |
| `/callback` | `callback/page.tsx` | OAuth code→token exchange, then `returnTo` or `/lobby` |

**Profile editing does not live at `/profile`.** That route is the public showcase; the editor is
the `ProfileMenu` popover in the lobby header (display name, wallet mode, deck variant), plus
`ProfileShowcaseDialog` (showcase visibility, ≤3 featured achievements). All of them call
`updateMe` from `src/lib/api/player.ts`.

## Lobby

- `StakesGrid.tsx` drives joining by stake tier; `ActiveTableBanner.tsx` surfaces an open seat.
- `CreateRoomDialog.tsx` (react-hook-form + zod) sets visibility, stake and seat count.
- `InviteDialog.tsx` (at the table) copies the private-room share URL.
- `ProfileMenu.tsx` is the account popover: avatar upload/removal with initials fallback, name edit, wallet-mode switch,
  deck-variant picker, balances, showcase dialog, self-HUD, logout.
- `OnboardingIntro.tsx` first-visit intro, gated by `src/lib/onboarding.ts`.

## Table / game client

`table/page.tsx` composes header, `Board`, `TableStage` (which lays out `Seat`s), `ActionBar`,
`Chat`, reactions and the dialogs. `TableStage` has **two layouts**: an oval for desktop and a
vertical `stage-v` ring for portrait handhelds.

- Actions: `fold | check | call | raise`, plus `post_big_blind`, `show_cards`, `keep_seat` and
  `preselect_action` as separate socket messages.
- Notable components: `PerimeterTimer` (SVG countdown ring), `RabbitHunt` (post-fold runout),
  `RealityCheck` (session-length nudge), `BotChallenge` (Turnstile), `PlayerNoteDialog` (private
  opponent notes + colour tag), `LastWinners`, `ChipStack`, `TablePreferencesDialog` (theme,
  sound, voice), `VoiceActionButton` (speech recognition), `HandOutcome` (win banner + confetti).
- **Animations are pure CSS** (`src/app/globals.css` keyframes) — no animation library. Deal,
  flip, street reveals, wager-in, pot count, turn signal, winner, reconnect progress, and
  `prefers-reduced-motion` are all handled there.

## Real-time hooks & providers

- **`useTableRealtime`** (`src/lib/hooks/useTableRealtime.ts`, ~724 lines) owns the whole table
  surface: socket lifecycle, snapshot reconcile, chat, achievement unlocks, pending-action
  watchdog, reconnect/backoff, `ConnectionStatus`, `ActionError`, and the action senders.
  Extend it rather than opening a second table socket.
- **`useLobbyRealtime`** subscribes to the separate lobby/user gateway (`GET /v1.0/ws`) for live
  room-list updates and user-scoped events.
- Other hooks: `useCountUp` (animated stack deltas), `useDeckVariant`, `useDealerVoice`.
- **The only React provider is `QueryProvider`** (TanStack Query). The access token is a
  module-level singleton in `src/lib/api/client.ts` (set/get/subscribe), **not persisted** —
  lost on full reload, recovered via `doRefresh()`. An axios interceptor attaches `Bearer` and
  auto-refreshes on 401.

## Auth flow

`src/lib/auth/oauth.ts` builds `OAuthClient` from `NEXT_PUBLIC_CTECH_URL` /
`NEXT_PUBLIC_CTECH_CLIENT_ID` and exposes `startOAuthFlow`, `exchangeCode`, `doRefresh`,
`logout`, `decodeIdToken`. `src/lib/auth/session.ts` adds `refreshSession`, `recoverSession`,
`useSessionKeepAlive`, `useOptionalSession`. `TermsGate` boots with `doRefresh()` when there is
no token, fetches `GET /v1.0/players/me`, and blocks the UI until `poker_terms_accepted`.

## Provably-fair surface

`src/lib/deckVerify.ts` does all verification **in the browser with WebCrypto** — it never
trusts a server-side "verified" flag:

- `verifyDeck(seed, commitHash)` — full reveal, for hands that ended in a real showdown
  (`components/hands/DeckReveal.tsx`).
- `verifyWirePartialDeck(rootCommitHash, revealed, unrevealed)` — the seed-less proof for hands
  that ended without one (`components/hands/PartialDeckProof.tsx`): revealed positions carry
  card + salt, the rest only their committed hash, and together they recompute the root commit.
  "Revelar tudo" deliberately never flips a position the viewer was not entitled to see.

Hands recorded before the partial proof shipped have no stored proof and still render as
unverifiable — there is no backfill, because the seed is not retained anywhere.

## Gamification & player tooling

- **Leaderboard** (`leaderboard/page.tsx`) — hands played/won and win rate. `achievement_points`
  is rejected server-side (no GSI), so it is not offered as a metric.
- **Achievements** — full catalog screen with progress tabs, plus the `AchievementToast` fired
  by the socket's `achievement_unlocked`.
- **Fichas hub** — `/store` keeps its exportable route for compatibility, but the durable user-facing destination is **Fichas**, broad enough to organize sandbox balance, rewards, packages, and activity without implying that gated real-money deposits are enabled.
- **Daily reward** — the spin is available in Fichas (`POST /v1.0/sandbox-credits/`), with
  cooldown from the matching GET. Its ready state uses one prominent heading, availability line,
  and claim action; after a claim, it recedes to a compact status row with the next available time.
  The store also loads Pix packages and shows the three most recent purchases before progressively
  disclosing the full purchase list.
- **Package comparison** — the store initially keeps the Pix catalog to four choices. Every option
  exposes total credits, base credits, the exact bonus-credit contribution (and percentage), and
  price without inferred urgency or “best value” claims; larger catalogs are disclosed on request.
- **Expired Pix recovery** — an expired sandbox charge can create a fresh purchase for its original
  SKU directly from the payment dialog. The action has pending and retryable error states; closing
  the dialog restores focus to the package that started the flow.
- **Equity** — `Seat` renders `seat.equity` from the server snapshot. There is no client-side
  equity calculator; the client cannot be given the remaining-deck composition.
- **Sandbox refund safety** — confirmed sandbox purchases expose a **Solicitar estorno** review, not an unconditional reversal. The dialog shows the exact Pix amount, package credits, projected sandbox balance, and the server-enforced eligibility rule: any sandbox-wallet debit after the purchase credit makes it ineligible. The server remains authoritative; the dialog owns verification, pending, success, and recoverable error states. This flow only reverses unused sandbox purchases and remains architecturally separate from gated real-money deposits.
- **Self-HUD** (`SelfHudDialog`) — own VPIP/PFR/3-bet from `GET /v1.0/players/me/poker-stats`.
- **Hand export** (`src/lib/handExport.ts`) and **hand sharing** (`ShareHandDialog`).

## Tests & CI

- **`vitest` + `@testing-library/react`**, jsdom, config in `vitest.config.ts` (`@`→`src` alias,
  v8 coverage with thresholds **lines/functions/statements 80, branches 70**). Setup in
  `src/test/setup.ts` mocks `matchMedia`, `ResizeObserver` and `scrollTo`, and clears storage
  between tests. **59 test files.** Mock snapshots come from `src/dev/mockRuntime.ts`
  (`snapshotForScenario`, `MOCK_PLAYER_ID`) so tests exercise realistic table state.
- `frontend.yml`: `npm ci` → `eslint src --max-warnings 0` → `npm run build` (static export) →
  S3 sync + route-manifest publish + CloudFront invalidation.
- Quality bar: lint, typecheck, tests and build must all pass with **zero errors and zero
  warnings**.
- ⚠️ The deploy step is `aws s3 sync out/ s3://$S3_BUCKET/ --delete` — anything else stored in
  that bucket under a synced prefix is deleted on every frontend deploy. Relevant if
  user-uploaded assets are ever put there.

## CSS

Two files: `src/app/globals.css` (~7.7k lines, imports Tailwind 4 and `forms-and-gate.css`),
imported once in `layout.tsx`. No CSS modules. Design tokens are CSS vars on `:root`; class names
are hand-written and semantic (`game-seat`, `seat-info`, `deck-reveal`, `app-page`, `shell`).
Tailwind utilities are used sparingly, mostly inside `src/components/ui/*`.

## Not built

Lobby stake/mode **filters** · multi-table grid · tournaments · spectator mode · physical chip
  travel between positions.

## Cross-links

- Backend these screens call: [`../api/README.md`](../api/README.md)
- Infrastructure that serves this: [`../cdk/README.md`](../cdk/README.md)
- Docs index and feature status: [`../docs/README.md`](../docs/README.md)
