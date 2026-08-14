# ui/ — AGENTS.md (for autonomous agents)

Goal: extend the Next.js poker SPA. Sandbox play is live; the real-money wallet-mode switch
exists, but the backend gate is off by default — never ship UI that assumes real money is on.

## Hard rules

1. **Reuse `@aoctech/auth-client` and `@aoctech/ws-client`** — no hand-rolled OAuth/socket code.
2. **Static export only.** No server routes, server actions, or image optimization; everything is
   client-side talking to `/v1.0/*` (dev proxy in `next.config.ts`, `NEXT_PUBLIC_API_URL` in
   prod). New SPA routes must be exportable. Anything that needs a server belongs in `api/`.
3. **The wire is binary protobuf.** Go through `lib/ws/utils.ts` +`lib/api/proto/poker.ts`;
   regenerate from `../proto/poker.proto`, never hand-edit the generated file.
4. **No magic strings.** Use `lib/api/*`, `lib/auth/*`, `lib/utils.ts`, `lib/pokerRules.ts`.
5. **Two realtime hooks only:** extend `lib/hooks/useTableRealtime.ts` (table, WS
   `${origin}/v1.0/tables/:id/ws`) or `lib/hooks/useLobbyRealtime.ts` (lobby/user, `/v1.0/ws`).
   Do not add a third.
6. **Animations = CSS** (`globals.css` keyframes), no animation library; honor reduced motion.
7. **Token:** module singleton in `lib/api/client.ts`; axios interceptor attaches `Bearer` and
   refreshes on 401. Don't move it to localStorage unless persistence is intended.
8. **Never reconstruct hidden information.** Unseen hole cards arrive as the literal `"back"`;
   render what you got. Fairness checks run in the browser (`lib/deckVerify.ts`) — never accept a
   server-provided "verified" boolean, and never let `PartialDeckProof` flip a position that is
   not in `revealed`.
9. **Quality gate:** `npx vitest run`, `npx tsc --noEmit`, `npx eslint src --max-warnings 0` and
   `npm run build` must all be clean. Coverage thresholds (**90% lines/statements/functions/
   branches**) are enforced in `vitest.config.ts`.
10. **Every new feature ships with its tests**, covering the error, empty and disabled paths, not
    only the happy one. Never lower a coverage threshold to land a change — write the test. See
    `docs/testing.md`.

## Tests / verification

`vitest` + `@testing-library/react`, jsdom; 81 test files; setup at `src/test/setup.ts`. Mock
snapshots come from `src/dev/mockRuntime.ts` (`snapshotForScenario`, `MOCK_PLAYER_ID`) — prefer
those over hand-built fixtures. `npm run dev:mock` runs a full in-memory realtime engine for
manual testing without the backend. Coverage is gated at 90% on all four metrics; conventions
for realtime hooks, page tests, fake timers and the shared `matchMedia` mock are in
`docs/testing.md`.

## Where things live

- Routes: `src/app/*` (App Router). Table room id is a **query param** `?id=`, not a segment.
- Table UI: `src/components/table/*` (`TableStage` has separate oval and portrait-ring layouts).
  Lobby: `src/components/lobby/*`. Hand history/fairness: `src/components/hands/*`.
- Realtime/auth/data: `src/lib/hooks`, `src/lib/auth`, `src/lib/api`, `src/lib/ws`.
- Profile **editing** is `components/lobby/ProfileMenu.tsx` + `ProfileShowcaseDialog.tsx`.
  `app/profile/` is the public read-only showcase of another player.
- `initials()` in `lib/utils.ts` is the one shared initials helper — two ad-hoc duplicates exist
  in `app/profile/page.tsx` and `app/page.tsx`; prefer the helper.

## Not built (do NOT implement as if real)

Lobby stake/mode filters · multi-table grid · tournaments · spectator mode · physical chip travel
· player avatar images are implemented with shared initials fallback.

## Backend contract

Endpoints and socket messages are documented in `../api/README.md`. Every `/v1.0` call carries the
bearer token; the API requires both `sub` and `sid` claims and rejects M2M tokens.

## Mandatory Documentation Policy

**Every code change MUST be documented.**

There are NO exceptions.

Any modification affecting behavior, architecture, APIs, integrations, configuration, deployment, security, business rules, or developer workflow MUST include the corresponding documentation update in the same change.

<!-- BEGIN:nextjs-agent-rules -->

# This is NOT the Next.js you know

This version has breaking changes — APIs, conventions, and file structure may all differ from your training data. Read the relevant guide in `node_modules/next/dist/docs/` (resolved from this file's directory; in monorepos the `next` package may not be visible from the repo root) before writing any code. Heed deprecation notices.

This block is written and re-added by `next dev` — verify at `node_modules/next/dist/server/lib/generate-agent-files.js`. Removing it from a diff only re-creates the uncommitted change; committing it with your work keeps the tree clean.

<!-- END:nextjs-agent-rules -->
