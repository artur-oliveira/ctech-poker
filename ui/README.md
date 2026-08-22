# ctech-poker — UI (Next.js SPA)

Next.js 16 SPA (App Router) for the poker lobby, tables, and game client. Everything below is
anchored to `ui/src` as of **2026-07-28**, not to `DESIGN.md`/`PRODUCT.md` (which are design specs).
Sandbox play is live end to end; the wallet-mode switch for real money exists in the UI but the
backend gate (`REAL_MONEY_ENABLED`) is off by default.

## Stack

- Next.js **16.3.1** (App Router, `src/app/`), React **19.2.8**, TypeScript 6 (`package.json`).
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
- Fonts: IBM Plex Sans / IBM Plex Mono via `next/font/google`, bound to `--font-sans` / `--font-mono` in
  `src/app/layout.tsx:8-9`. (Older docs claim this is unwired — it is wired.)
- Dev proxy: `next.config.ts` rewrites `/v1.0/*` → `DEV_API_ORIGIN` (default
  `http://localhost:8003`). Mock mode: `NEXT_PUBLIC_MOCK_API=true` (`dev:mock`) runs a full
  in-memory realtime engine (`src/dev/mockRuntime.ts`, aliased to no-op stubs in prod builds).

## Configuration (environment variables)

| Key | Where read | Purpose |
|---|---|---|
| `NEXT_PUBLIC_API_URL` | `src/lib/api/client.ts` | poker API base URL (HTTP); dev default `http://localhost:8003` via proxy |
| `NEXT_PUBLIC_WS_URL` | `src/lib/ws/origin.ts` | WebSocket origin for both gateways. Set explicitly in deployed builds and **must** be: the deploy workflow builds the CSP's `connect-src` from the origin literals in the build environment and `connect-src` is scheme-exact, so a `wss://` host derived at runtime never appears in the policy and every socket is blocked. Falls back to `NEXT_PUBLIC_API_URL` with the scheme swapped, which is the local-dev path |
| `NEXT_PUBLIC_APP_URL` | `src/app/layout.tsx` | `metadataBase` for OG/meta tags |
| `NEXT_PUBLIC_MOCK_API` | `src/lib/mockConfig.ts` | `true` runs the in-memory mock realtime engine instead of a live API |
| `NEXT_PUBLIC_REAL_MONEY_ENABLED` | `src/lib/capabilities.ts` | `true` exposes real-money statistics/history; absent or any other value keeps the UI sandbox-only and coerces real-money hand URLs to sandbox |
| `NEXT_PUBLIC_CTECH_URL` | `src/lib/auth/oauth.ts` | ctech-account base URL for `OAuthClient` |
| `NEXT_PUBLIC_CTECH_CLIENT_ID` | `src/lib/auth/oauth.ts` | poker's OAuth client id |
| `DEV_API_ORIGIN` | `next.config.ts` | dev-only rewrite target for `/v1.0/*` |

No `.env.example` exists in `ui/`. `NEXT_PUBLIC_*` values are injected at build time by
`frontend.yml` per environment — static export bakes them in, there is no runtime env lookup.
`frontend.yml` is a thin caller of `ctech-cdk/.github/workflows/frontend-cloudflare.yml`, which deploys
the export to a Cloudflare Worker and derives `connect-src` from those same values; it also carries an
`AVATAR_UPLOAD_ORIGIN` that no component reads, purely so the presigned S3 upload host reaches the policy.

Avatar URLs are absolute and come from the API (`AVATAR_BASE_URL` → `https://poker-api[-env].aoctech.app/v1.0/avatars`),
which is why `img-src` names the API host. `images: {unoptimized: true}`, so no `remotePatterns` entry is involved.

## Routes (App Router, `src/app/`)

Every page is `'use client'`; only `layout.tsx` and `share/layout.tsx` are server components.

| Route | File | Purpose |
|---|---|---|
| `/` | `page.tsx` | Landing: hero demo table, features, achievement teaser, OAuth CTAs |
| `/lobby` | `lobby/page.tsx` | Stakes grid with explicit join/create states and sandbox buy-in ranges, compact active-session resume strip, create-room dialog, daily spin, onboarding |
| `/table?id=<id>` | `table/page.tsx` | The live table (room id is a **query param**, not a segment) |
| `/hands` | `hands/page.tsx` | Immediate infinite-scroll hand history in the API's paginated order; no technical-ID or client-only filter surface |
| `/hands/history?table_id=&hand_id=` | `hands/history/page.tsx` | One hand: seats, board, street-grouped actions, progressive fairness proof, resilient export/share, and action retry |
| `/hands/replay?table_id=&hand_id=` | `hands/replay/page.tsx` | Frame-by-frame `HandReplayer` |
| `/leaderboard` | `leaderboard/page.tsx` | Podium + ranking list, highlights the viewer's row |
| `/achievements` | `achievements/page.tsx` | Catalog + own progress, all/unlocked/in-progress/completed tabs |
| `/store` | `store/page.tsx` | Durable Fichas hub: sandbox balance, daily reward, Pix chip packages, and expandable recent purchase activity |
| `/people` | `people/page.tsx` | Friends, requests (in/out), recent players, blocked list and activity feed, plus the friend-code header |
| `/profile?id=<playerId>` | `profile/page.tsx` | **Public read-only showcase** of another player, with the shared player menu |
| `/share?id=<token>` | `share/page.tsx` | Public anonymized shared hand (`robots: noindex`) |
| `/guide` | `guide/page.tsx` | Illustrated how-to-play |
| `/poker-rules` | `poker-rules/page.tsx` | Rules + hand rankings reference |
| `/callback` | `callback/page.tsx` | OAuth code→token exchange, then `returnTo` or `/lobby` |

**Profile editing does not live at `/profile`.** That route is the public showcase; the editor is
the `ProfileMenu` popover in the lobby header (display name, wallet mode, deck variant), plus
`ProfileShowcaseDialog` (showcase visibility, ≤3 featured achievements). All of them call
`updateMe` from `src/lib/api/player.ts`.

## Lobby

- `StakesGrid.tsx` keeps the lobby compact as inventory grows: a visible-focus, keyboard-accessible blind-level radio rail with
  dark, token-aligned horizontal overflow treatment controls three format choices (heads-up, 6-max, full-ring), preferring a stake with an
  open public table by default. It intentionally keeps every stake directly discoverable rather than hiding them
  behind carousel navigation.
  The page frames the decision as blinds plus table size and explains once that matchmaking joins a compatible
  public table or creates one when no seat is available. Each size states the resulting action and sandbox buy-in;
  `ActiveTableBanner.tsx` surfaces an open seat.
- `CreateRoomDialog.tsx` (react-hook-form + zod) sets visibility, stake and seat count.
- `InviteDialog.tsx` (at the table) copies the private-room share URL.
- `ProfileMenu.tsx` is the account popover: avatar upload/removal with initials fallback, name edit, wallet-mode switch,
  deck-variant picker, balances, showcase dialog, self-HUD, logout.
- `OnboardingIntro.tsx` is a compact, dismissible first-visit note below the lobby header. It keeps table entry
  immediately available and points newcomers to the inline explanations for blinds, capacity, and buy-in.

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
- `BuyInPanel` has an "Auto rebuy" toggle (sandbox rooms only), sent as `auto_rebuy` on
  `joinRoom`. When a busted seat has it on, `RebuyDialog` waits a short grace window (server auto-
  rebuy runs async right after the bust snapshot) before falling back to the manual slider, or —
  if the player's sandbox balance is exactly zero — an embedded PIX top-up (`SkuGrid` +
  `PixPaymentView`, extracted from the store's `PurchaseModal`) instead of a dead-end error.
- **Animations are pure CSS** (`src/app/globals.css` keyframes) — no animation library. Deal,
  flip, street reveals, wager-in, pot count, turn signal, winner, reconnect progress, and
  `prefers-reduced-motion` are all handled there.
- Chat messages are capped at `CHAT_MESSAGE_MAX_LENGTH` (`lib/chat.ts`, 50 chars, mirrored
  server-side). A message actually delivered live (never chat history hydrated from a snapshot on
  connect/reconnect) pops a speech bubble on the sender's `Seat` — `useTableRealtime`'s
  `chatBubbles` map, keyed by player id, drives it.

## Real-time hooks & providers

- **`useTableRealtime`** (`src/lib/hooks/useTableRealtime.ts`, ~724 lines) owns the whole table
  surface: socket lifecycle, snapshot reconcile, chat, achievement unlocks, pending-action
  watchdog, reconnect/backoff, `ConnectionStatus`, `ActionError`, and the action senders.
  Extend it rather than opening a second table socket.
- **`useLobbyRealtime`** subscribes to the separate lobby/user gateway (`GET /v1.0/ws`) for live
  room-list updates and user-scoped events, including the social pushes (`social_event`,
  `social_presence_changed`, `social_inbox_count`). It is mounted **exactly once**, by
  `RealtimeBridge` inside `QueryProvider` — never again from a page or a component, or the same
  account opens two sockets. Social frames are invalidation-only: they refresh the `['social', …]`
  query keys (`SOCIAL_KEYS` in `src/lib/social.ts`) and the unread badge; the durable state always
  comes back over HTTP.
- **Table-side suppression** is a `useTableRealtime` argument, not a filter in a component: the
  page loads the muted/blocked ids for the seated players (`GET /social/relationships?player_ids=`)
  and passes the set in, so suppressed chat and reactions never reach React state — no bubble, no
  animation, no live-region announcement. Seats, stacks, bets and every poker action stay visible.
- Other hooks: `useCountUp` (animated stack deltas), `useDeckVariant`, `useDealerVoice`.
- **The only React provider is `QueryProvider`** (TanStack Query). The access token is a
  module-level singleton in `src/lib/api/client.ts` (set/get/subscribe), **not persisted** —
  lost on full reload, recovered via `doRefresh()`. An axios interceptor attaches `Bearer` and
  auto-refreshes on 401.

## Auth flow

`src/lib/auth/oauth.ts` builds `OAuthClient` from `NEXT_PUBLIC_CTECH_URL` /
`NEXT_PUBLIC_CTECH_CLIENT_ID` and exposes `startOAuthFlow`, `exchangeCode`, `doRefresh`,
`logout`, `decodeIdToken`. Its scope request is centralized in
`src/lib/auth/scopes.ts`: `openid profile` plus all 11 public active
`poker:*:read` scopes from the API manifest. A contract test rejects drift or
an accidental `internal:*` browser grant. Creating/joining tables, gameplay,
WebSockets, and purchases have no browser-confidential scope; the API binds
those interactive operations to user tokens issued to the first-party
`poker` client through `azp`. `src/lib/auth/session.ts` adds `refreshSession`, `recoverSession`,
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
- Both proof views lead with the browser-calculated verification result. Hashes and the 52-position
  deck sit inside a native disclosure so technical controls are traversed only when requested.

Hands recorded before the partial proof shipped have no stored proof and still render as
unverifiable — there is no backfill, because the seed is not retained anywhere.

## Hand-history semantics and recovery

- `/hands` aggregates only the records currently loaded, or the record returned by direct hand-ID
  lookup. Labels explicitly say **carregadas**; ties are separate from wins and never inflate the
  victory rate.
- The list intentionally exposes no ID, date, outcome, or sort controls. IDs are implementation
  details for most players, and the API has no full-history date/outcome/sort parameters; the UI
  therefore opens directly on paginated history and never applies global-looking operations to a
  partial client page.
- The action timeline groups poker actions by street and collapses system/social events. If action
  history fails, the summary, fairness proof, sharing, and a summary-only text export remain usable;
  the timeline provides an independent retry.

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
  the dialog restores focus to the package that started the flow. The modal is opened
  programmatically (no `DialogTrigger`), so that restore is base-ui's `finalFocus` on
  `PurchaseModal`'s `DialogContent`, fed the trigger ref the store keeps — not a `requestAnimationFrame`
  callback, which raced base-ui's own close teardown and intermittently left focus on `<body>`.
- **Equity** — `Seat` renders `seat.equity` from the server snapshot. There is no client-side
  equity calculator; the client cannot be given the remaining-deck composition. The readout is
  additionally gated on the viewer having peeked **both** of their own hole cards (see below): the
  server sends equity as soon as it has it, and showing it earlier would hand the player the exact
  knowledge the "all-in/won without peeking" achievements require them not to have — with one card
  peeked, equity narrows the other down too.
- **Private card peek** — mid-hand, the viewer's own hole cards stay face-down behind a
  click-to-flip gate (`PlayingCard`'s `peekable`/`peeked`, state owned per hand by `Seat`), and the
  first flip reports one `peek_cards` breadcrumb over the socket for the "without peeking"
  achievements. Click only, never hover: hover reveal made the following click a no-op and left no
  way to hide a card while the pointer rested on it. Both faces stay mounted in both states so the
  flip is a CSS transition off `.is-peeked` and animates in *both* directions; face-down cards
  carry a bounded `peek-nudge` wobble so the affordance is discoverable without hover. The gate
  lifts once the hand completes (or when `onPeekCards` is absent, e.g. hand-history replay).
- **Turn clock vs. time bank** — the seat ring and the `ActionBar` reserve readout both switch at
  `action_base_deadline_unix_ms`, an instant no broadcast coincides with, so both drive that gate
  off `useLiveNow` rather than the frozen `snapshotAt`. Countdown *geometry* still comes from
  snapshot props (a running CSS animation must not be rewritten by unrelated snapshots); only the
  phase decision is live.
- **Sandbox refund safety** — confirmed sandbox purchases expose a **Solicitar estorno** review, not an unconditional reversal. The dialog shows the exact Pix amount, package credits, projected sandbox balance, and the server-enforced eligibility rule: any sandbox-wallet debit after the purchase credit makes it ineligible. The server remains authoritative; the dialog owns verification, pending, success, and recoverable error states. This flow only reverses unused sandbox purchases and remains architecturally separate from gated real-money deposits.
- **Self-HUD** (`SelfHudDialog`) — own VPIP/PFR/3-bet from `GET /v1.0/players/me/poker-stats`.
- **Hand export** (`src/lib/handExport.ts`) and **hand sharing** (`ShareHandDialog`).

## Tests & CI

The local quality gate includes the production supply-chain audit:

```bash
npm ci
npm test
npm run lint
npm run build
npm audit --omit=dev
```

The production dependency audit must remain at zero known vulnerabilities.

- **`vitest` + `@testing-library/react`**, jsdom, config in `vitest.config.ts` (`@`→`src` alias,
  v8 coverage with thresholds **90 lines / 90 statements / 90 functions / 90 branches**). Setup
  in `src/test/setup.ts` mocks `matchMedia`, `ResizeObserver` and `scrollTo`, and clears storage
  between tests. **81 test files.** Mock snapshots come from `src/dev/mockRuntime.ts`
  (`snapshotForScenario`, `MOCK_PLAYER_ID`) so tests exercise realistic table state.
- **Every new feature ships with the tests that cover it** — error, empty and disabled paths
  included. The 90% thresholds may be raised, never lowered to land a change. Rules and
  per-area recipes: [`docs/testing.md`](docs/testing.md).
- `frontend.yml`: `npm ci` → `eslint src --max-warnings 0` → `npm run build` (static export) →
  S3 sync + route-manifest publish + CloudFront invalidation.
- Quality bar: lint, typecheck, tests and build must all pass with **zero errors and zero
  warnings**.
- ESLint is pinned to the 9.x line because Next 16.3's bundled `eslint-plugin-import`,
  `eslint-plugin-jsx-a11y`, and `eslint-plugin-react` declare peer support through ESLint 9.
  Keep ESLint on that line until all of those plugin peer ranges support a newer major.
- ⚠️ The deploy step is `aws s3 sync out/ s3://$S3_BUCKET/ --delete` — anything else stored in
  that bucket under a synced prefix is deleted on every frontend deploy. Relevant if
  user-uploaded assets are ever put there.

## CSS

Two files: `src/app/globals.css` (~7.7k lines, imports Tailwind 4 and `forms-and-gate.css`),
imported once in `layout.tsx`. No CSS modules. Design tokens are CSS vars on `:root`; class names
are hand-written and semantic (`game-seat`, `seat-info`, `deck-reveal`, `app-page`, `shell`).
Tailwind utilities are used sparingly, mostly inside `src/components/ui/*`.

## People, safety and the bust dialog

- **`/people` and the lobby drawer share their components.** `PeopleList`, `PlayerActionsMenu`,
  `SocialInbox` and `FriendCodeLookup` (`src/components/social/`) are used by both, plus by the
  table seat menu, the public profile and the invite dialog, so the safety actions cannot drift
  apart between surfaces. `useSocialActions` runs every mutation and reports failures as one
  toast with curated pt-BR copy (`socialErrorMessage`); `useSocialList` owns cursor pagination.
- **Discovery is exact-code only** (`PKR-XXXX-XXXX-XXXX`). There is no name search, and presence
  is only ever a status — never the table, room code, blinds or balance of an `in_table` friend.
- **Blocking is not matchmaking.** A blocked player can still be dealt into the same public table;
  only chat, reactions and social interaction are suppressed.
- **The bust dialog contains no purchase.** `RebuyDialog` compares the available balance against
  `buy_in_min` (not "is it zero"), and offers a rebuy, the free daily reward, or a way back to the
  lobby. No SKU grid, QR code or store CTA lives in that flow; the store stays a separate route.

## Not built

Lobby stake/mode **filters** · multi-table grid · tournaments · spectator mode · physical chip
  travel between positions.

## Cross-links

- Backend these screens call: [`../api/README.md`](../api/README.md)
- Infrastructure that serves this: [`../cdk/README.md`](../cdk/README.md)
- Docs index and feature status: [`../docs/README.md`](../docs/README.md)
