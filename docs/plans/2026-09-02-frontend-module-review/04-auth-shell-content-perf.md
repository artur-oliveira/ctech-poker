# Frontend module review — Auth, app shell, cross-cutting, content/SEO, build & perf

> Part of the 2026-09-02 systematic review. Fresh code-anchored pass. Issues renumbered in §8
> (F-A1…F-A12). Static export to Cloudflare confirmed (`next.config.ts:19`), no runtime server.
> **Token model confirmed correct:** access token / username / player id are all in-memory module
> singletons (`lib/api/client.ts:48/77/100`), nothing writes any to `localStorage`/`sessionStorage`.
> The refresh token itself is owned by `@aoctech/auth-client` (out of this repo — see F-A11).

## 1. OK-vs-impeccable

### (a) Auth & session lifecycle

| Dimension | Current (file:line) | Impeccable |
|---|---|---|
| silent refresh single-flight | `lib/auth/session.ts:52–72` — one shared `refreshPromise`, cleared in `finally`. Solid | keep |
| keep-alive cadence | `session.ts:16` refresh every 4 min for a 15-min token; interval only (`:99–105`), no refresh on tab-focus/visibility or wake-from-sleep → **F-A(keep-alive)** | also refresh on `visibilitychange`→visible and on `online` |
| callback error taxonomy | `app/callback/page.tsx:25` — every `exchangeCode` rejection (network, 3 s deadline, PKCE `state` mismatch, IdP 5xx) → the same "O código de acesso expirou ou já foi usado" + one "Tentar novamente" → **F-A4** | distinguish transient / terminal / IdP-down; report a digest |
| token request abort | `oauth.ts:14–19` — `withTokenDeadline` rejects at 3 s but never aborts the underlying fetch → **F-A4** | wire an `AbortController` through |
| expired-session latch | `session.ts:21,35–37` — `endingExpiredSession` set `true` permanently; if `endSession()` redirect no-ops, the app is wedged (token cleared, latch stuck, no recovery path) → **F-A5** | watchdog: reset the latch / fall back to `startOAuthFlow()` if navigation hasn't happened in N ms |
| 401 recovery | `client.ts:199–214` single retry then `endExpiredSession()`; WS `unauthorized` → `recoverSession()` (`session.ts:85–91`). Good, well-commented (325-reconnect incident) | keep |
| logout UX | `ProfileMenu.tsx:241` — `onClick={() => logout()}`, no pending/disabled state, no error surface; `logout()` waits up to 3 s on `revoke()` → **F-A6** | disable + spinner on click; if `revoke` fails, still `endSession` but tell the user |
| TermsGate gating | booting/login/profile-error/terms branches all render; `nameSync` one-shot guard via ref. Reasonable | the `me.isError` branch (`:75`) offers only "Tentar novamente" — a 401 here should hard-restart auth |
| session fixation | new token overwrites singletons directly; nothing persisted to fixate. Fine | — |

### (b) App shell & navigation

| Dimension | Current (file:line) | Impeccable |
|---|---|---|
| route announcing | `RouteAnnouncer.tsx:32` announces `document.title` in a `usePathname` effect — no guarantee Next committed the new `<title>` yet on client nav → a stale title can be announced → **F-A3** | announce the focused heading's `textContent` instead |
| focus management | `RouteAnnouncer.tsx:29–30` sets a permanent `tabindex="-1"` on the `<h1>` and `.focus()`; focusing a heading **and** an `aria-live` region with the same text = double announcement → **F-A3** | focus a wrapper/`main`, or use only the live region; remove the tabindex on blur |
| error boundary fidelity | `app/error.tsx` / `global-error.tsx` — every thrown error → generic "500". A thrown `ApiError` with `status 503` should go to `/unavailable`; `404` should read as not-found → **F-A7** | branch on `error instanceof ApiError` and route 503/404 to their dedicated states |
| deep links / hard refresh | `output: 'export'` + Cloudflare Pages resolves `/table`, `/guide/basics` and serves `404.html`. Works | — |
| outage handling | `client.ts:139–148` `redirectOnServiceUnavailable` → full reload to `/unavailable`, saves return path. OK for a rare event | — |
| chrome on error routes | `SystemState` renders its own `<main>` with only "início"/"lobby" buttons — an authed user hitting a bad `/table?id=` loses the nav/tab bar | render `SystemState` inside the app shell for authed users |

### (c) Design-system primitives

| Dimension | Current (file:line) | Impeccable |
|---|---|---|
| Button | `components/ui/button.tsx` — CVA, 5 variants, `focus-visible` ring, no layout props animated. Good. **No `loading`/`pending` variant** — every async button re-implements its own disabled+label swap → **F-A9** | add a `loading` prop (spinner, `disabled` + `aria-busy`, stable width) |
| Input | `components/ui/input.tsx` — `aria-invalid` styling, but no built-in label/description/error wiring → **F-A9** | a `Field` wrapper pairing `<label for>` + `aria-describedby` + error id |
| Skeleton | `aria-hidden` bars + one `LoadingRegion` that announces. Clean | keep |
| Toaster | `Notifier.tsx` + `lib/notify.ts` — `role="alert"` for errors, dedupe 600 ms, `MAX_VISIBLE=3`. But `AUTO_DISMISS_MS=6000` applies even to error toasts carrying a retry action (`notify.ts:39`) → the actionable error vanishes mid-read → **F-A8** | errors, and any toast with `actions`, persist until dismissed |

### (d) Content / SEO / OG

| Dimension | Current (file:line) | Impeccable |
|---|---|---|
| metadata coverage | `app/layout.tsx:13–71` root + `lib/routeMetadata.ts` per-route; `robots.ts`/`sitemap.ts` `force-static`, cross-checked by `crawlerSurface.test.ts`. Strong | keep |
| guide OG images | all 7 chapters pass `image: 'guide'` → identical `/og/guide.webp`; `routeMetadata` also always appends `/og-image.webp` as a second image | per-chapter OG (or at least basics/table/hands distinct); drop the redundant second image |
| structured data | no JSON-LD anywhere — `/poker-rules` and `/guide` are textbook `Article`/`HowTo` candidates | add `HowTo` to `/guide/basics`, `Article` to `/poker-rules` — pure static, zero runtime cost |
| content-page weight | `/poker-rules` ships ~21 JS chunks, near-identical to `/lobby` → **F-A1** | marketing surface < 100 KB JS |

### (e) Performance & bundle

| Dimension | Current | Impeccable |
|---|---|---|
| marketing/app code split | **None.** `/poker-rules` pulls the protobuf+WS chunk (`3g-_7j6a-9oaz.js`, 77.8 KB, contains `protobufPackage`, `TableSnapshot`, ws-client) + react-query + session refresh + `ProfileMenu`+social. ~1 MB uncompressed JS on a text page. Root cause: `RealtimeBridge`/`useSessionKeepAlive`/`NetworkProvider` mount in the root `QueryProvider` (`lib/providers/QueryProvider.tsx:10,29–31`); content pages consume `AppPageChrome` → **F-A1** | route-group split `(marketing)` vs `(app)` |
| `proto/poker.ts` (4771 L) | bundled into a shared chunk via `lib/ws/utils.ts` → `useLobbyRealtime` → root provider ⇒ **every route** | lazy-import the WS layer |
| `@bufbuild/protobuf` | `proto/poker.ts:8` imports it at runtime; it is **not in `package.json`** — resolves only transitively via `ts-proto` (devDep, `package-lock.json:638`). A `ts-proto` major bump breaks the prod bundle → **F-A11** | add `@bufbuild/protobuf` to `dependencies` |
| `mockRuntime.ts` (2383 L) | **Correctly aliased out** in prod: `next.config.ts:9–15` swaps `@/dev/mockRuntime` for a 640-byte fail-closed stub; `USE_MOCK` is `false` at build; mock adapter is a dynamic `import()` behind `USE_MOCK`. Verified: not in the prod graph | looks fine |
| `globals.css` | **One 18 058-line stylesheet** (`app/globals.css`) `@import`ed in root layout → compiled to a single ~308 KB chunk served on **every** route (OAuth callback, `/poker-rules`, 404). Contains the full felt/seat/card system + **86 `@keyframes`** → **F-A2** | split the table/game CSS into a stylesheet imported only by `app/table/` |
| fonts | `layout.tsx:10–11` — IBM Plex Sans ×4 weights + IBM Plex Mono ×4 weights, both global, 8 woff2 preloaded on every route → **F-A10** | drop mono to 400/600; sans to weights actually used |
| `optimizePackageImports` | `next.config.ts:17` covers `lucide-react` only; `@base-ui/react`, `@tanstack/*` not listed | add the barrel-file ones |
| `playSound` | `lib/sound.ts:31` — `new Audio(file)` per call, no preload/pool (also F-T5) | preload the 6 clips into an element pool on table mount |
| CSP `unsafe-inline` (parent Issue 18 — depth) | `script-src 'unsafe-inline'` is **load-bearing**: the static export emits inline `__next_f.push([…])` hydration `<script>` blocks with no nonce (no server). Dropping it breaks hydration | the reusable workflow must extract per-build script **hashes** into `script-src` — not a config toggle. Note this in Issue 18 |
| dead route-publish tooling (parent Issue 19 — depth) | `scripts/publish-routes.sh` is AWS `cloudfront-keyvaluestore` code, referenced **only** by the dead `frontend-cloudfront.yml:148`. `ui/CLAUDE.md` still documents it as *the* route-manifest mechanism → **F-A12** | delete both with the dead workflow; fix `ui/CLAUDE.md` |

## 2. New issues (summaries; full detail was produced by the review agent)

- **F-A1 · [FRONTEND/build] Marketing/SEO pages ship the full authenticated-app JS bundle** — **High · M · $0.** `/poker-rules` and `/guide/*` download ~1 MB uncompressed JS (protobuf+ws chunk 77.8 KB, react-query, `ProfileMenu`+social) on a text page, hurting LCP/TBT/Lighthouse on the exact surface that exists for SEO. Root cause: root `QueryProvider` unconditionally mounts `useSessionKeepAlive` + `RealtimeBridge` + `NetworkProvider`; content pages consume `AppPageChrome`→`ProfileMenu`. Fix: `(marketing)` route group with a lightweight provider (no keep-alive, no realtime); `(app)` group wraps only authed routes; lazy-`import()` `lib/ws/*` so protobuf never lands in the shared chunk. Acceptance: `out/poker-rules.html` references no chunk containing `protobufPackage`; total JS for `/poker-rules` < 120 KB gzip; Lighthouse Performance ≥ 95 mobile.
- **F-A2 · [FRONTEND/build] Single 18k-line global stylesheet loads the full table renderer CSS on every route** — Medium · M · $0. Extract `.game-table`/`.seat-*`/card-flip/board keyframes/reconnect UI into `app/table/table.css`, imported from the table subtree. Keep tokens/reset/chrome/forms/content in `globals.css`. Acceptance: CSS served on `/poker-rules` < 60 KB uncompressed; no `@keyframes board-deal` / `.seat-` in the global sheet.
- **F-A3 · [FRONTEND/a11y] RouteAnnouncer announces a possibly-stale title and double-announces** — Medium · S · $0. Announce the focused heading's `textContent` (guaranteed in sync); drop either the focus move or the live region (keep focus, live only when no heading found); remove the injected `tabindex` on blur.
- **F-A4 · [FRONTEND/auth] OAuth callback collapses every failure into "code expired", no abort, no telemetry** — Medium · M · $0. Type the failure (`transient` / `invalid` / `unavailable`); retry the exchange for transient, restart flow for invalid, `/unavailable` for IdP-down; thread an `AbortController` through `withTokenDeadline`; emit a digest via the Issue-25 sink.
- **F-A5 · [FRONTEND/auth] `endExpiredSession` latches permanently and wedges the app if the IdP redirect no-ops** — Medium · S · $0. Watchdog ~1.5 s after `endSession`: if still here, reset the latch and `startOAuthFlow('/')`. Or drop the latch and dedupe on a timestamp.
- **F-A6 · [FRONTEND/auth] Logout button has no pending state or failure feedback** — Low · S · $0. `isPending` → disable + spinner + "Saindo…"; a second click can't fire a second `revoke`; a stalled logout surfaces a manual "Sair agora".
- **F-A7 · [FRONTEND/shell] Error boundary treats every thrown error as a generic 500** — Medium · S · $0. In `error.tsx`, `ApiError(503)` → `/unavailable`-style nav; `404` → `SystemState code="404"`; else current 500. `global-error.tsx` stays generic.
- **F-A8 · [FRONTEND/ux] Actionable error toasts auto-dismiss after 6 seconds** — Medium · S · $0. Persist `variant === 'error'` toasts and any toast with `actions` until dismissed (20 s ceiling). Keep the 6 s timer for plain `info`. `MAX_VISIBLE` eviction still applies.
- **F-A9 · [FRONTEND/design-system] No pending/loading affordance in Button; no Field wrapper for Input** — Low · M · $0. Add `loading?: boolean` to `Button` (spinner, `disabled`, `aria-busy`, stable width); add a `Field` primitive (label + description + error region with generated ids). Convert TermsGate accept, logout, callback retry.
- **F-A10 · [FRONTEND/perf] Eight Google font files loaded globally; sound clips fetched on first play** — Low · S · $0. Trim mono to 400/600, sans to referenced weights (audit `font-weight` usages); preload the 6 SFX into an `HTMLAudioElement` pool on table mount.
- **F-A11 · [FRONTEND/build] `@bufbuild/protobuf` is a runtime dependency but only present transitively** — Low · S · $0. Add `"@bufbuild/protobuf": "^2.13.0"` to `dependencies`; sanity-check the build with `ts-proto` removed from the tree.
- **F-A12 · [INFRA/.github] Dead route-publish script + stale `ui/CLAUDE.md` after the Cloudflare cutover** (depth on Issue 19) — Low · S · $0. Delete `frontend-cloudfront.yml` + `scripts/publish-routes.sh` together; rewrite the "Static export" section of `ui/CLAUDE.md` to describe the Cloudflare Pages deploy (no KVS, `404.html` fallback); document how/when `og:capture` and `cards:variants` are run.

## 3. Verified, no action
Refresh single-flight (`session.ts:52–72`), WS `unauthorized` recovery, the `hasCheckedSession` gate that prevents every first-load query from eating a 401 (`client.ts:184–198`); `USE_MOCK` production stripping (alias swap + fail-closed stub + `NODE_ENV` guard + dynamic-import adapter — `mockRuntime.ts` is **not** in the prod graph); `robots.ts`/`sitemap.ts` (`force-static`, private routes disallowed, cross-checked); retry policy (safe/idempotent reads get one bounded jittered retry; TanStack retry disabled to avoid 3×3 amplification); `Notifier` roles + dedupe + cap (only the auto-dismiss timing is wrong, F-A8); deep-link/hard-refresh via Cloudflare Pages `*.html` + `404.html`; no banned layout-property transitions in `button.tsx`/`input.tsx`; 44px touch floor consistent; callback `started` ref guards StrictMode double-invoke.
