# ui/ — CLAUDE.md

Next.js 16 (App Router) SPA for the poker lobby, tables, and game client. **Sandbox mode
only — real-money UI is DESIGNED-ONLY.**

## Conventions

- **Reuse shared CTech client libraries:** `@aoctech/auth-client` (OAuth) and
  `@aoctech/ws-client` (WebSocket). Do NOT hand-roll auth or socket clients. (The design
  docs mention `ctech-oauth-client`; the code uses `@aoctech/auth-client` — trust the code.)
- **Static export.** `output: 'export'` in prod (`next.config.ts:7`); the SPA route manifest
  is published to a CloudFront KeyValueStore by `scripts/publish-routes.sh`. Do not add
  server-only routes/APIs.
- **Named constants over literals.** Reuse `lib/api/*`, `lib/auth/*`, `lib/table.ts`,
  `lib/gamification.ts` instead of inlining URLs/paths/event strings.
- **One realtime hook.** `lib/hooks/useTableRealtime.ts` owns the entire table surface
  (socket, snapshot, chat, achievements, reconnect). Extend it rather than adding a second
  socket hook.
- **State:** token is a module singleton in `lib/api/client.ts` (not React Context, not
  persisted); table data flows through `QueryProvider` (TanStack Query). No other providers.
- **Animations are CSS** (`src/app/globals.css` keyframes) — no animation library. Keep it
  that way; honor `prefers-reduced-motion`.
- **Type safety:** `zod` for form/API shapes, `react-hook-form` for forms.
- **Quality gate:** `eslint src --max-warnings 0` plus `next build` must pass with **zero
  errors AND zero warnings**. There is no test script — lint + build are the gate.

## DESIGNED-ONLY (do not assume present)

Lobby stake/mode filters · private-room share-link UI · real-money/wallet flow · crypto
fairness/audit surfaces · player reactions · hand history · chat moderation · achievements
catalog screen · roulette wheel visual · physical chip travel · Geist font binding
(`--font-sans`/`--font-mono` not yet wired via `next/font`). All confirmed absent in `src/`.

## Auth flow

`@aoctech/auth-client` → `lib/auth/oauth.ts`. Landing CTAs → `startOAuthFlow`; `/callback`
exchanges code + stores token; `TermsGate` gates on `GET /v1.0/players/me` +
`poker_terms_accepted`.

## Layout

`src/app/{page,lobby,table,leaderboard,roulette,callback}` · `src/components/{lobby,table,ui}`
· `src/lib/{api,auth,hooks,providers}`.
