# ui/ — CLAUDE.md

Next.js 16 (App Router) SPA for the poker lobby, tables, and game client. Sandbox play is live;
the wallet-mode switch for real money exists here, but the backend gate (`REAL_MONEY_ENABLED`) is
off by default — do not build UI that assumes real money is on.

## Conventions

- **Reuse shared CTech client libraries:** `@aoctech/auth-client` (OAuth) and
  `@aoctech/ws-client` (WebSocket). Do NOT hand-roll auth or socket clients. (The design
  docs mention `ctech-oauth-client`; the code uses `@aoctech/auth-client` — trust the code.)
- **Static export.** `output: 'export'` in prod (`next.config.ts`); the SPA route manifest is
  published to a CloudFront KeyValueStore by `scripts/publish-routes.sh`. **There is no server
  at runtime** — no API routes, no server actions, no image optimizer. Anything needing a server
  belongs in `api/`.
- **The wire is binary protobuf**, not JSON. Encode/decode through `lib/ws/utils.ts` against
  `lib/api/proto/poker.ts`; regenerate from `../proto/poker.proto` rather than hand-editing.
- **Named constants over literals.** Reuse `lib/api/*`, `lib/utils.ts`, `lib/pokerRules.ts`,
  `lib/tableOutcome.ts` etc. instead of inlining URLs, paths or event strings. The same holds in
  CSS: every colour and radius is a token in `globals.css`'s `:root` (the only literals left there
  are the `[data-table-theme]` blocks, which *define* tokens). A new value means a new named token
  documented in `DESIGN.md`, never an inline hex or px.
- **Never animate a layout property.** `width`, `height`, `padding` and `margin` transitions are
  banned; use `transform`/`opacity` (a progress fill is `scaleX(var(--fill))`, a hover inset is a
  `translateX` on the row's contents). The design hook flags regressions.
- **Landmarks and headings survive every state.** Loading, empty, error and invalid-link branches
  render inside the same `main` with a real `h1` as the success branch — the recovery vocabulary is
  `SystemState` (whole-app) or `RecoveryState` (in-app), not a bare `.form-error` line.
- **Two realtime hooks, no more.** `lib/hooks/useTableRealtime.ts` owns the table surface;
  `lib/hooks/useLobbyRealtime.ts` owns the lobby/user gateway (rooms **and** all social pushes).
  Extend them rather than opening a third socket. `useLobbyRealtime` is mounted once, by
  `lib/providers/RealtimeBridge.tsx` inside `QueryProvider`, which the `(app)` route group
  layout owns — do not mount it from a page again, and never from the `(marketing)` group
  (its `MarketingQueryProvider` deliberately omits it, keep-alive and `NetworkProvider`).
- **A rejected table command is resubmitted, not surfaced.** `act()` retries on `stale_state` via
  `pendingActionRef`; the auxiliary commands (`show_cards`, `request_rabbit_hunt`,
  `request_winner_cards`, `accept`/`decline_winner_cards`, `request_exit`) carry no
  `expected_snapshot_version`, so the server can only answer them with a flat rejection — they go
  through `emitAux`, which keeps the frame in `auxFramesRef` so a resync-class code
  (`RESYNC_ERROR_CODES`) resubmits the **same** frame under the **same** `action_id`. Reusing the id
  is safe because the server rejects these before commit, so no idempotency guard was written.
  Retries are capped at `MAX_ACTION_RETRIES` and back off past the resync scheduled for that same
  id; only an exhausted budget reaches `setLastActionError`. Any new command with an `action_id` the
  server echoes belongs on `emitAux`, not bare `emit`. See
  `docs/plans/2026-08-27-table-load-transaction-conflict.md`.
- **Press-and-hold is one hook.** `lib/hooks/useHoldRepeat.ts` owns the accelerating repeat used by
  the bet stepper's `+`/`−` buttons *and* by the `ArrowLeft`/`ArrowRight` shortcuts, so touch and
  keyboard ramp identically. OS key auto-repeat stays ignored (`isBetAdjustKey` drops
  `event.repeat`); the hook's timers are the cadence. See
  `docs/2026-09-01-bet-hold-repeat.md`.
- **The docked asides are one pattern.** `Chat`, `TableReactions` and `LastWinners` share the
  `column-reverse` toggle/panel stack, `useHoverPanel` (hover open with a close grace period) and
  the `.table-aside-skirt` hit-area class. Extend those three rather than re-deriving hover
  behaviour per aside, and keep the felt's band layout intact (wordmark / dealer call / board +
  street rail; the felt's lower band belongs to the bottom seats' bet chips). See
  `docs/2026-09-01-table-felt-and-aside-polish.md`.
- **Social state is server state.** Every social read is a `['social', …]` query key
  (`SOCIAL_KEYS` in `lib/social.ts`); mutations go through `lib/hooks/useSocialActions.ts`, which
  invalidates that root instead of patching a mirrored relationship locally. Chat/reaction
  suppression for muted or blocked players is applied inside `useTableRealtime` (before state),
  never in a component, and never touches seats, bets or poker actions.
- **State:** the token is a module singleton in `lib/api/client.ts` (not React Context, not
  persisted); server data flows through `QueryProvider` (TanStack Query). No other providers.
- **Animations are CSS** (`src/app/globals.css` keyframes) — no animation library. Keep it that
  way; honor `prefers-reduced-motion`.
- **Type safety:** `zod` for form/API shapes, `react-hook-form` for forms.
- **Quality gate:** `npx vitest run`, `npx tsc --noEmit`, `npx eslint src --max-warnings 0` and
  `npm run build` must all pass with **zero errors and zero warnings**. Coverage thresholds are
  enforced in `vitest.config.ts` (**lines/functions/statements/branches 90**).
- **Every new feature must ship with the tests that cover it** — including the error, empty and
  disabled branches, not just the happy path. Uncovered `??`/optional-field branches are exactly
  where type-shaped bugs survive `tsc`. Never lower a coverage threshold to land a change; write
  the missing test. Conventions and per-area recipes: `docs/testing.md`.

## Security invariants — do not regress

- **Never render or derive hidden information client-side.** The server masks unseen hole cards
  as the literal `"back"` before sending; the client's job is to display what it got, never to
  reconstruct what it didn't.
- **The viewer's own `equity` and `hand_category` are sent unconditionally, gated only in the
  client.** The server can't see the local "peek" toggle (`Seat.tsx`'s `peeked` state — a client-only
  gate on the viewer's own hole cards, distinct from the public `onReveal`), so it always sends the
  viewer's true numbers; `Seat.tsx`'s `showEquity`/`showHandCategory` are what withhold them until
  both cards are peeked (or the hand is complete). Displaying either unconditionally leaks the exact
  hidden state the "all-in/won without peeking" achievements require the player not to have.
- **Fairness verification happens in the browser.** `lib/deckVerify.ts` recomputes hashes with
  WebCrypto. Never replace that with a server-provided boolean — the whole point is that the
  player does not have to trust us.
- **The seed-less partial proof must stay seed-less.** `PartialDeckProof` may only flip positions
  present in `revealed`; "Revelar tudo" must never turn a mucked card face-up. There is a test
  asserting exactly this.

## Layout

**Route groups** (parenthesised folders — no effect on the URL):
`src/app/(marketing)/{page,poker-rules,guide,guide/*}` are the static, indexable,
logged-out-friendly pages; `src/app/(app)/{lobby,people,table,hands,hands/history,hands/replay,leaderboard,achievements,profile,store,share,callback}`
is everything that needs the authenticated/live shell. `(marketing)/layout.tsx`
mounts only `MarketingQueryProvider`; `(app)/layout.tsx` mounts `QueryProvider`
(keep-alive + `NetworkProvider` + `RealtimeBridge`). `robots.ts`, `sitemap.ts`,
the error boundaries, `not-found.tsx` and `unavailable/` stay ungrouped under the
root `layout.tsx`. See `docs/2026-09-02-marketing-app-route-split.md`.

· `src/components/{achievements,hands,lobby,social,table,ui}` (+ root: `TermsGate`, `Notifier`,
`AchievementToast`, `HandRankings`, `SystemState`, `RecoveryState`) · `src/app/{robots,sitemap}.ts`
(crawler surface — see `docs/seo.md`)
· `src/lib/{api,api/proto,auth,hooks,providers,ws}` + domain
modules at `src/lib/*.ts` · `src/dev` (mock runtime, aliased away in prod) · `src/test/setup.ts`.

Profile **editing** is `components/lobby/ProfileMenu.tsx` + `ProfileShowcaseDialog.tsx`, not
`app/profile/` — that route is the public read-only showcase of another player.

## Auth flow

`@aoctech/auth-client` → `lib/auth/oauth.ts` (+ `lib/auth/session.ts` for keep-alive/recovery).
Landing CTAs → `startOAuthFlow`; `/callback` exchanges the code and stores the token; `TermsGate`
gates on `GET /v1.0/players/me` + `poker_terms_accepted`.

`lib/api/client.ts`'s request interceptor also gates: on first load, if there's no token yet, it
awaits `getOrRefreshSession()` once before letting *any* API call through, so page-load requests
never race the silent refresh (previously visible as unauthenticated calls in the network log).
Resolves once per "no token" streak — resets when the token drops back to `null` (logout).

## Not built (do not assume present)

Direct messages · clubs/persistent groups · automatic bans from report volume · any purchase inside
the bust dialog (deliberately removed; the store is its own route) ·
Lobby stake/mode filters · multi-table grid · tournaments · spectator mode · physical chip travel
· avatar images use `PlayerAvatar`, with `initials()` in `lib/utils.ts` as the shared fallback.

## Mandatory Documentation Policy

**Every code change MUST be documented.**

There are NO exceptions.

Any modification affecting behavior, architecture, APIs, integrations, configuration, deployment, security, business rules, or developer workflow MUST include the corresponding documentation update in the same change.

### The in-app guide is part of that policy

`src/app/guide` is user-facing documentation of this product. **Every feature, and every change to
how something looks or behaves for the player, MUST land with the matching guide update in the same
change.** No exceptions — not for "small" tweaks, not for renames, not for a moved control.

That includes: new routes, dialogs, menus and panels; new or renamed controls, labels and states;
changed defaults, shortcuts, limits, prices or timings; new empty/loading/error/disabled states;
anything that changes what the player sees or can do.

- Find the topic page that already owns the surface (`basics`, `table`, `hands`, `achievements`,
  `store`, `profile`, `community`) and extend it; only add a section when none fits.
- If the change alters a screen shown in the guide, re-capture it:
  `npm run dev:mock` then `npm run og:capture -- --guide`. New surfaces get a new entry (and, when
  the state needs a click, a `prepare` action) in `scripts/capture-og-images.mjs`.
- Names in the guide must match the strings shipped in the UI, verbatim.
- Removing a feature means removing its guide copy and its screenshot in the same change.

A change that ships without its guide update is incomplete.

