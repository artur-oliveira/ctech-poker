# ui/ — CLAUDE.md

Next.js 16 (App Router) SPA for the poker lobby, tables, and game client. Sandbox play is live;
the wallet-mode switch for real money exists here, but the backend gate (`REAL_MONEY_ENABLED`) is
off by default — do not build UI that assumes real money is on.

## Conventions

- **Reuse shared CTech client libraries:** `@aoctech/auth-client` (OAuth) and
  `@aoctech/ws-client` (WebSocket). Do NOT hand-roll auth or socket clients. (The design
  docs mention `ctech-oauth-client`; the code uses `@aoctech/auth-client` — trust the code.)
- **Static export.** `output: 'export'` in prod (`next.config.ts`). `out/` is deployed to a
  Cloudflare Worker's static assets by the shared reusable workflow
  (`artur-oliveira/ctech-cdk/.github/workflows/frontend-cloudflare.yml`, called from
  `.github/workflows/frontend.yml`); `html_handling: auto-trailing-slash` and
  `not_found_handling: 404-page` do what the CloudFront viewer-request Function and its route
  KeyValueStore used to — there is no route manifest to publish any more. **There is no server
  at runtime** — no API routes, no server actions, no image optimizer. Anything needing a server
  belongs in `api/`.
- **The deployed security headers are generated, not committed.** The same reusable workflow
  writes `out/_headers` (CSP, HSTS, `Permissions-Policy`, the `/_next/static/*` immutable rule);
  `frontend.yml` only supplies `extra-connect-src`, `csp-overrides` and `permissions-policy`, and
  `connect-src` is derived from the build env. Anything this repo writes to `out/_headers` is
  overwritten, so a per-build CSP value cannot come from here.
- **The application does not rely on inline scripts.** `npm run build` is `next build && node
  scripts/strip-inline-scripts.mjs`, which rewrites the export's inline
  `self.__next_f.push(…)` flight scripts into `/_next/static/chunks/inline/<sha256>.js` in place
  (bare `<script src>`, so execution order is unchanged) and fails the build if any inline
  `<script>` survives. The deployed `script-src` nevertheless has `'unsafe-inline'` because
  Cloudflare injects a per-request challenge bootstrap after the build; its changing Ray ID and
  timestamp preclude a stable hash, and the static app cannot give it a nonce. Never add an inline
  `<script>` to a layout or page: the postbuild guard must remain effective even though this edge
  compatibility exception exists. See #46/#120.
- **Mock mode runs with no backend at all.** `npm run dev:mock` sets `NEXT_PUBLIC_MOCK_API=true`;
  `probeHealthOnce()` in `lib/network/liveness.ts` short-circuits under `USE_MOCK` so the health
  probe cannot escalate the app to `/unavailable` when nothing is listening on the dev proxy's API
  port. Do not reintroduce a network call on that path — `og:capture` and the browser test suite
  both boot this way.
- **Generated assets are manual, on purpose.** `npm run og:capture` (OG + guide screenshots, needs
  `npm run dev:mock` running) and `npm run cards:variants` (card face SVG variants) are run by
  hand by whoever changes the surface they capture, and their output is committed. No workflow
  regenerates them — a stale screenshot is a review failure, not a CI failure.
- **The wire is binary protobuf**, not JSON. Encode/decode through `lib/ws/utils.ts` against
  `lib/api/proto/poker.ts`; regenerate from `../proto/poker.proto` rather than hand-editing.
- **Named constants over literals.** Reuse `lib/api/*`, `lib/utils.ts`, `lib/pokerRules.ts`,
  `lib/tableOutcome.ts` etc. instead of inlining URLs, paths or event strings. The same holds in
  CSS: every colour and radius is a token in `globals.css`'s `:root` (the only literals left there
  are the `[data-table-theme]` blocks, which *define* tokens). A new value means a new named token
  documented in `DESIGN.md`, never an inline hex or px.
- **Three stylesheets, narrowest wins.** `globals.css` (root layout, every route) → `(app)/app.css`
  (`(app)/layout.tsx`) → `(app)/table/table.css` (`(app)/table/layout.tsx`), plus
  `(app)/table-reactions.css` and the `@import`ed `forms-and-gate.css`. A new rule goes in the
  narrowest sheet that can render it; anything a `(marketing)` page or a root-level boundary can
  match stays in `globals.css`. Seat/board/card/hand-outcome rules stay broad on purpose — `/hands`,
  `/share`, `/profile`, `/lobby` and the landing demo render those components too. The sheets load
  in that order, so moving a rule inward only ever strengthens it: never move one that something in
  an outer sheet is meant to override at equal specificity. Tokens never move. See `DESIGN.md`
  ("Where the CSS lives") and #84.
- **Never animate a layout property.** `width`, `height`, `padding` and `margin` transitions are
  banned; use `transform`/`opacity` (a progress fill is `scaleX(var(--fill))`, a hover inset is a
  `translateX` on the row's contents). The design hook flags regressions.
- **Landmarks and headings survive every state.** Loading, empty, error and invalid-link branches
  render inside the same `main` with a real `h1` as the success branch — the recovery vocabulary is
  `SystemState` (whole-app) or `RecoveryState` (in-app), not a bare `.form-error` line.
- **The table page is a composition, not a component.** `app/(app)/table/page.tsx` wires hooks
  together and renders; it holds no derivation of its own. Server reads live in
  `lib/hooks/useTableSession.ts` (plus `useTableRemoval` for the removed-frame reaction and the
  leave recap), showdown bookkeeping in `lib/hooks/useTableOutcome.ts`, the asides/dialogs/reaction
  cooldown in `lib/hooks/useTableOverlays.ts`, the action-bar derivation in `lib/tableActions.ts`,
  and the whole banner assembly in `buildHandOutcome` (`lib/tableOutcome.ts`). Extend the hook that
  owns the concern; do not grow the page back. See
  `docs/2026-09-02-table-module-decomposition.md`.
- **One lifecycle for every purchase.** Sandbox credits, premium reactions and deck/felt cosmetics
  all track their live status through `lib/hooks/usePurchaseStatus.ts`. The websocket frame is the
  primary confirmation (each key lives under a root `useLobbyRealtime` already invalidates); the
  hook's query is the bounded fallback — it pauses in a hidden tab for free (React Query's
  `refetchIntervalInBackground` is false), backs off 4s → 30s per polls spent, gives up at
  `PURCHASE_POLL_BUDGET`, and seeds `initialData` so opening a dialog costs no read. **Never arm a
  `setInterval` to watch a purchase**; `purchasePollCount()` exists so "at most N reads per pending
  purchase" stays assertable. See `docs/2026-09-04-purchase-status-one-lifecycle.md` and #227.
- **One clock for the whole table.** `lib/hooks/useSharedTicker.ts` runs a single `setInterval` at
  the shortest cadence anyone asked for and notifies each subscriber on its own period. Every
  countdown goes through `useLiveNow` (which subscribes to it) — the action bar, each timed seat,
  the reduced-motion countdown, `RealityCheck`, `IdleWarning`. Never arm a bare `setInterval` for a
  countdown; `tickerIntervalCount()` exists so "at most one interval during a turn" stays
  assertable.
- **A ring that captures its elapsed offset once, at mount, must never be handed a value computed
  in a `useEffect`.** `HandOutcome.tsx`'s `HandOutcomeRing` deliberately captures `elapsedMs` via a
  lazy `useState` initializer so a dismiss/reopen toggle continues the same countdown instead of
  restarting it — but that means whatever `durationMs` it is first mounted with is what its CSS
  animation is stuck with. `useTableOutcome.ts` used to learn a freshly-armed `next_hand_unix_ms`
  inside a `useEffect`, which only runs *after* commit — so the very render that first mounts the
  ring (the same commit the deadline arrives in) still saw the *previous* hand's armed deadline,
  computed `nextHandDurationMs` as `0`, and permanently froze the ring's `animation-duration` at
  `0ms` (2026-09-04, `docs/specs/2026-09-04-cross-instance-stale-turn-timer.md`). Fixed by deriving
  the armed deadline during render instead — React's supported "adjust state when a prop changes"
  pattern — so the ring's one and only real mount already gets the correct duration. Any other
  countdown-ring consumer built on this "capture once at mount" shape must derive its inputs the
  same way, not through an effect.
- **Seats publish their own position.** `lib/seatRects.ts` is where `Seat` registers its element
  and where the reaction layer reads seat centres from. Do not locate a seat with a DOM query, and
  re-measure on `resize`/`orientationchange` rather than caching a rect for the length of an
  animation — the stage swaps between oval and portrait-ring layouts mid-flight.
- **Two public realtime hooks, no more.** `lib/hooks/useTableRealtime.ts` is the thin, stable
  composition boundary for the table surface; its private state-machine implementation lives in
  `lib/hooks/useTableRealtimeSession.ts`. Transport/auth/liveness/mock lifecycle is isolated in
  `useTableSocket`; auxiliary frame correlation and retry timers are isolated in
  `useTableActionQueue`; pure version reconciliation and social suppression live in
  `lib/tableSnapshotReducer.ts`. Retry vocabulary and snapshot-transition narration live in
  `lib/tableResilience.ts` and `lib/tableNarration.ts`. These are private slices of the table hook,
  not new public realtime hooks or socket endpoints. Table code imports only the public hook.
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
  server echoes belongs on `emitAux`, not bare `emit`. The vocabulary itself —
  `RESYNC_ERROR_CODES`, `TERMINAL_ERROR_CODES`, `MAX_ACTION_RETRIES`, the timeouts, the
  `auxRetryDelayMs` backoff and the player-facing copy — lives in `lib/tableResilience.ts` and is
  unit-tested there; `useTableActionQueue` owns the retry registry and timer lifecycle, with its
  retry decision independently covered as a pure function.
  Snapshot-transition narration (`describeSnapshot`, `playSoundForTransition`) is
  `lib/tableNarration.ts`. See `docs/plans/2026-08-27-table-load-transaction-conflict.md`.
- **Press-and-hold is one hook.** `lib/hooks/useHoldRepeat.ts` owns the accelerating repeat used by
  the bet stepper's `+`/`−` buttons *and* by the `ArrowLeft`/`ArrowRight` shortcuts, so touch and
  keyboard ramp identically. OS key auto-repeat stays ignored (`isBetAdjustKey` drops
  `event.repeat`); the hook's timers are the cadence. See
  `docs/2026-09-01-bet-hold-repeat.md`.
- **The docked asides are one pattern.** `Chat`, `TableReactions` and `LastWinners` share the
  `column-reverse` toggle/panel stack, `useHoverPanel` (hover open with a close grace period) and
  the `.table-aside-skirt` hit-area class. The toggle's click belongs to that hook too
  (`toggleFromClick`, destructured out of the spread so it never reaches the DOM node): a bare
  `onClick={() => onOpenChange(!open)}` races the hover the same pointer movement just performed,
  and which one wins depends on the engine — on Firefox the first click on the reactions toggle
  closed the panel hover had just opened. See
  `docs/2026-09-03-aside-toggle-firefox-and-mock-liveness.md`.
  Extend those three rather than re-deriving hover
  behaviour per aside, and keep the felt's band layout intact (wordmark / dealer call / board +
  street rail; the felt's lower band belongs to the bottom seats' bet chips). See
  `docs/2026-09-01-table-felt-and-aside-polish.md`.
- **A shared hand has two records, on purpose.** `GET /players/me/hand-shares` (via
  `listMyHandShares`, key `HAND_SHARES_QUERY_KEY`) is the authority on which links are live and
  powers `MyHandSharesPanel`'s Revogar; `lib/handShareStorage.ts` is a per-browser map of
  hand → token, and the only thing that can answer "I already shared *this* hand" in
  `ShareHandDialog`, because the list endpoint does not carry the source hand. Revoking must clear
  both. See `docs/2026-09-02-hand-shares-history-filters-achievement-recency.md`.
- **Social state is server state.** Every social read is a `['social', …]` query key
  (`SOCIAL_KEYS` in `lib/social.ts`); mutations go through `lib/hooks/useSocialActions.ts`, which
  invalidates that root instead of patching a mirrored relationship locally. Chat/reaction
  suppression for muted or blocked players is applied inside `useTableRealtime` (before state),
  never in a component, and never touches seats, bets or poker actions.
- **State:** the token is a module singleton in `lib/api/client.ts` (not React Context, not
  persisted); server data flows through `QueryProvider` (TanStack Query). No other providers.
- **Animations are CSS** (keyframes in `globals.css` / `app.css` / `table.css`) — no animation library. Keep it that
  way; honor `prefers-reduced-motion`.
- **Type safety:** `zod` for form/API shapes, `react-hook-form` for forms.
- **Quality gate:** `npx vitest run`, `npx tsc --noEmit`, `npx eslint src --max-warnings 0` and
  `npm run build` must all pass with **zero errors and zero warnings**. Coverage thresholds are
  enforced in `vitest.config.ts` (**lines/functions/statements/branches 90**).
- **The table's geometry has one source.** `--table-rail-inset-top/-inline/-bottom` place
  `.game-rail`; `.game-felt` is that inset **plus `--table-rail-band`**; `--table-orbit-*` is the
  band's centreline and is where seats go. `balancedSeatPosition` (`TableStage.tsx`) returns
  normalised `{s, t}` in `[0, 1]` on that orbit — not percentages of the stage — and `Seat.tsx`
  publishes them as `--seat-s`/`--seat-t`, which the CSS resolves against the same tokens. Change
  the band and rail, felt and every seat move together. Do not reintroduce a per-side felt inset,
  and note that `--table-orbit-*` is declared on `.game-table`, never on `:root`: a custom property
  containing `var()` resolves where it is *declared*, so a `:root` copy would freeze the desktop
  numbers and ignore the `.stage-v`/`.stage-h` overrides. Occupancy narrows the ring
  (`.stage-v[data-capacity='2'] .stage-v-ring`), never the felt. See
  `docs/2026-09-03-table-geometry-one-band.md` and DESIGN.md's One Band Rule.
- **Layout is gated in real engines, not in jsdom.** `npm run e2e` (Playwright,
  `e2e/tableLayout.spec.ts`) runs the table across six viewports and four scenarios in Chromium,
  Firefox and WebKit and asserts that nothing a player needs is clipped by an ancestor's
  `overflow`, that the page never scrolls sideways, and that the aside toggles behave the same in
  every engine. Playwright starts `dev:mock` itself. Coverage percentage cannot see any of this —
  jsdom has no layout engine. **WebKit here is not an iPhone**: it misses mobile Safari's dynamic
  viewport, rasterisation scale and flex intrinsic sizing, and this repo has already shipped a
  `min-height` regression that looked fine in DevTools' device toolbar and squeezed the real table
  on iOS. Any table-geometry change needs a pass on a real handset on top of a green run. Prefer
  length tokens over percentage insets (which resolve against a different axis per side) and over
  anything that has to travel through a flex cross size. See
  `docs/2026-09-03-cross-browser-layout-suite.md`.
- **Automated a11y and size gates.** `src/test/axe.ts`'s `expectNoAxeViolations(container)` runs axe-core in jsdom
  and fails on a new `serious`/`critical` violation; it is asserted in the six main route tests and
  `recoveryA11y.test.tsx`. `npm run bundle:check` compares per-route first-load JS in `out/` against
  `bundle-budget.json` (re-pin with `npm run bundle:pin`, in the same commit as the change that moved it) and proves
  the dev mock runtime is off every route's critical path. `lighthouserc.json` audits the static export. All three run
  in the `quality` job of `.github/workflows/frontend.yml`.
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
