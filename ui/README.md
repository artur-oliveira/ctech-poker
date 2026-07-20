# ctech-poker — UI (Next.js SPA)

Next.js 16 SPA (App Router) for the poker lobby, tables, and game client. **Sandbox mode is
implemented end-to-end.** Real-money mode, cryptographic fairness surfaces, hand history,
and several gamification visuals are **DESIGNED-ONLY** (see below). Everything below is
anchored to `ui/src`, not to `DESIGN.md`/`PRODUCT.md` (which are design specs).

> Source of truth is the code. The design system in `DESIGN.md` is accurate for colors/
> tokens but flags its own gap: `--font-sans` / `--font-mono` are not bound (no `next/font`),
> so Geist is not actually applied yet.

## Stack

- Next.js **16.2.10** (App Router, `src/app/`), React **19.2.7**, TypeScript 5 (`package.json`).
- **Static export** in prod (`output: 'export'`, `next.config.ts:7`); served from S3 +
  CloudFront. SPA route manifest published to a CloudFront **KeyValueStore**
  (`scripts/publish-routes.sh`).
- Real-time: **`@aoctech/ws-client`** (`package.json:9`) — `useWebSocket`, not hand-rolled.
- Auth: **`@aoctech/auth-client`** (`package.json:8`). Note: the design docs mention
  `ctech-oauth-client`; the implementation uses `@aoctech/auth-client` instead.
- Data: `@tanstack/react-query`; forms: `react-hook-form` + `zod`; UI: `base-ui`/`shadcn`
  + Tailwind 4; icons: `lucide-react`.
- Dev proxy: `next.config.ts:9-15` rewrites `/v1.0/*` → `DEV_API_ORIGIN`
  (default `http://localhost:8003`). Mock mode: `NEXT_PUBLIC_MOCK_API=true`
  (`dev:mock` script) runs a full in-memory realtime engine (`lib/mock.ts`).

## Routes (App Router, `src/app/`)

| Route | File | Purpose | Status |
|---|---|---|---|
| `/` | `src/app/page.tsx:25` | Landing/marketing; OAuth CTAs | IMPLEMENTED |
| `/lobby` | `src/app/lobby/page.tsx:6` | Room list + create; wrapped in `TermsGate` | IMPLEMENTED |
| `/table?id=<id>` | `src/app/table/page.tsx:54` | Table/game client (room id is a **query param**, not a segment) | IMPLEMENTED |
| `/leaderboard` | `src/app/leaderboard/page.tsx:7` | Ranking readout | IMPLEMENTED |
| `/roulette` | `src/app/roulette/page.tsx:9` | Daily sandbox-chip spin | PARTIAL (no wheel visual) |
| `/callback` | `src/app/callback/page.tsx:7` | OAuth code→token exchange + store | IMPLEMENTED |
| (root layout) | `src/app/layout.tsx:55` | `QueryProvider` + `Notifier` | IMPLEMENTED |

## Lobby — IMPLEMENTED (with DESIGNED-ONLY gaps)

- `RoomList.tsx` polls `listRooms()` every 5s (`useQuery`, `:10`) and renders room cards
  (stakes, max seats, status dot); click → `/table?id=<room_id>` (`:25`).
- `CreateRoomDialog.tsx` (react-hook-form + zod) selects visibility (public/private),
  stake index (`listStakes()`), and seat count 2–9; calls `createRoom()` then navigates.
- **DESIGNED-ONLY:** lobby **filters by stakes/mode** are not implemented (`RoomList` maps
  every room with no filter controls). A "private" visibility option exists in the form, but
  there is **no share-link UI and no private-room join flow** in code (the WS/HTTP gates
  exist server-side; the client never surfaces the code).

## Table / game client — IMPLEMENTED

- `table/page.tsx:54` (`TableContent`) composes the header, `Board`, seat grid (`.map` over
  `snapshot.seats` → `Seat`), `ActionBar`, `Chat`, `AchievementToast`, and (in mock mode)
  `MockControls`.
- Components (`src/components/table/`): `Board.tsx` (pot/rake/community cards),
  `Seat.tsx` (stack, contributed-bet pill, state labels, equity %, winner pill),
  `ActionBar.tsx` (Fold/Check/Call + Raise slider), `PlayingCard.tsx` (SVG by rank/suit),
  `Chat.tsx` (500-char max), `AchievementToast.tsx`.
- **Supported actions** (`lib/mock.ts`/`lib/table.ts` `PokerAction`): `fold | check | call | raise`.
- **Animations are pure CSS** (`src/app/globals.css` keyframes) — no animation library.
  Deal/flip/flop/turn/river, wager-in, pot-count, turn-signal, winner, reconnect progress,
  and `prefers-reduced-motion` are all present. **Chips do not physically travel between
  positions** — only bet/pot pills animate (DESIGNED-ONLY in the "living table" spec).

## Real-time hook & providers

- **`useTableRealtime(id, viewerId?, mockOptions?)`** (`src/lib/hooks/useTableRealtime.ts:67`)
  is the **only custom hook** and owns the whole table surface: WS connection, snapshot,
  chat, achievement unlock, pending-action/timeout (8s watchdog), reconnect/backoff, and the
  `act/ready/sendChat` API.
- **WS URL**: `${origin}/v1.0/tables/${id}/ws` where `origin` swaps `http→ws`
  (`useTableRealtime.ts:110-111`). Client sends `ping`/`ready`/`act`/`chat`
  (`:194,:211,:212`); on open it calls `getRoom(id)` + `joinRoom(id, amount)` to auto-seat
  (`:151-169`) — the lobby never calls join itself.
- **No React Context providers for auth/socket/table.** The only provider is
  `QueryProvider` (`src/lib/providers/QueryProvider.tsx:5`). The access token is a
  module-level singleton in `src/lib/api/client.ts:12` (set/get/subscribe); it is **not
  persisted** (lost on full reload). Axios interceptor attaches `Bearer` and auto-refreshes
  on 401 (`client.ts:31-51`).

## Auth flow — IMPLEMENTED (via `@aoctech/auth-client`)

- `src/lib/auth/oauth.ts` builds `OAuthClient` from `NEXT_PUBLIC_CTECH_URL` /
  `NEXT_PUBLIC_CTECH_CLIENT_ID` and exposes `startOAuthFlow`, `exchangeCode`, `doRefresh`,
  `decodeIdToken`.
- Landing CTAs call `startOAuthFlow('/lobby')`; the provider redirects to `/callback`, which
  exchanges the code, stores the token, and routes to `returnTo||'/lobby'`.
- `TermsGate.tsx:15` boots with `doRefresh()` if no token, fetches `GET /v1.0/players/me`,
  and gates the UI until `poker_terms_accepted`, calling `POST /players/me/terms/accept`.
- **DESIGNED-ONLY:** any real-money / wallet-linkage UI. All room labels hardcode "SANDBOX"
  (`RoomList.tsx:28`).

## Gamification UI

- **Leaderboard — IMPLEMENTED** (`leaderboard/page.tsx` → `gamification.ts`), shows
  hands-played/won and win-rate. (Note the server-side B31 ranking bug affects
  `achievement_points`.)
- **Achievements toast — IMPLEMENTED (display only):** fires from the server
  `achievement_unlocked` message. **No achievements catalog/list screen** exists.
- **Roulette — PARTIAL:** spin API + result text are implemented (`roulette/page.tsx:9`,
  `POST /v1.0/roulette/spin`); the **wheel visual/spin animation is DESIGNED-ONLY** — the
  page is a button + "+N fichas" text.
- **Hand-equity display — IMPLEMENTED as a pass-through:** `Seat.tsx` shows `seat.equity`
  when present; the value comes from the server snapshot (`lib/table.ts`). There is **no
  client-side equity calculator**. The "fairness inspectable" copy (`page.tsx:23`) is text
  only — no audit surface (see API B32).

## CI / build

- `frontend.yml`: `npm ci` → `eslint src --max-warnings 0` (zero-warning gate, no test
  script) → `npm run build` (static export) → sync to S3 + publish route manifest +
  CloudFront invalidation. Env-specific `NEXT_PUBLIC_*` (api URL, ctech URL, client id) are
  injected by the workflow.
- Quality bar: lint + build must pass with **zero errors AND zero warnings** (root
  `CLAUDE.md` convention).

## DESIGNED-ONLY summary (vs DESIGN/PRODUCT docs)

Lobby stake/mode filters · private-room share-link UI · real-money/wallet flow ·
cryptographic deck/audit surfaces · player reactions · **hand history** · chat moderation /
toxicity filter · achievements catalog screen · roulette **wheel** visual · physical chip
travel · Geist font binding. None of these exist in `ui/src` today.

## Cross-links

- Backend these screens call: [`../api/README.md`](../api/README.md)
- Infrastructure that serves this: [`../cdk/README.md`](../cdk/README.md)
