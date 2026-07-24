# ui/ — AGENTS.md (for autonomous agents)

Goal: extend the Next.js poker SPA. **Sandbox mode only; real-money UI is out of scope.**

## Hard rules

1. **Reuse `@aoctech/auth-client` and `@aoctech/ws-client`** — no hand-rolled OAuth/socket code.
2. **Static export only.** No server routes/APIs; everything is client-side talking to
   `/v1.0/*` (dev proxy in `next.config.ts`, or `NEXT_PUBLIC_API_URL` in prod). New SPA
   routes must be exportable (no dynamic server data).
3. **No magic strings.** Use `lib/api/*`, `lib/auth/*`, `lib/table.ts`, `lib/gamification.ts`.
4. **One realtime hook:** extend `lib/hooks/useTableRealtime.ts`; do not add a second socket
   hook. WS URL pattern: `${origin}/v1.0/tables/:id/ws` with `http→ws` swap.
5. **Animations = CSS** (`globals.css` keyframes), no animation library; honor reduced motion.
6. **Token:** module singleton in `lib/api/client.ts`; axios interceptor attaches `Bearer`
   and refreshes on 401. Don't move it to localStorage unless persistence is intended.
7. **Quality gate:** `eslint src --max-warnings 0` + `next build` must be clean (zero
   warnings). No test script exists — lint+build is the gate.

## DESIGNED-ONLY (do NOT implement as if real)

Lobby filters, share-link UI, real-money/wallet, fairness audit surfaces, reactions, hand
history, chat moderation, achievements catalog, roulette wheel visual, chip travel, Geist
font. Confirmed absent in `src/`.

## Tests / verification

No unit tests configured. Verify with `npm run lint` and `npm run build`. `npm run dev:mock`
runs a full in-memory realtime engine (`lib/mock.ts`) for manual testing without the backend.

## Where things live

- Routes: `src/app/*` (App Router). Table room id is a **query param** `?id=`, not a segment.
- Table UI: `src/components/table/*`. Lobby: `src/components/lobby/*`.
- Realtime/auth/data: `src/lib/hooks`, `src/lib/auth`, `src/lib/api`.

## Backend contract

Endpoints/events are documented in `../api/README.md`. Authz note B9 applies server-side;
the UI should keep sending the bearer token on every `/v1.0` call.
