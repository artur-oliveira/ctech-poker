# The aside toggle did nothing on Firefox; `dev:mock` needed a backend

**Date:** 2026-09-03
**Scope:** `src/lib/hooks/useHoverPanel.ts`,
`src/components/table/{TableReactions,Chat,LastWinners}.tsx`,
`src/lib/network/liveness.ts`, `src/app/(marketing)/guide/table/page.tsx`

Two defects found while reproducing a player report ("não consigo ver a
animação de emojis no Firefox"). Both are cross-browser or cross-environment
problems that unit tests in jsdom could not have caught, which is why they
survived a 90%-covered suite.

## 1. The docked asides never opened on the first click in Firefox

`Chat`, `TableReactions` and `LastWinners` share one pattern: the `<aside>`
carries `useHoverPanel`'s `onMouseEnter`/`onMouseLeave`, and the toggle button
inside it carried its own `onClick={() => onOpenChangeAction(!open)}`.

A pointer moving onto the toggle fires **both**: `mouseenter` opens the aside,
then `click` toggles it. Which way the click goes depends on whether the engine
committed the mouseenter render before dispatching the click:

| | click 1 | click 2 | click 3 |
|---|---|---|---|
| Chromium (before) | open | open | open |
| **Firefox (before)** | **closed** | open | closed |
| Firefox (after) | open | closed | open |

Measured with Playwright at 1440×900 against `npm run dev:mock`, asserting the
`open` class on `.table-reactions`. On Firefox the first click on the reactions
toggle opened and immediately re-closed the panel, so a player who clicks
rather than hovers never reached the reaction catalog at all — reported, from
the other side of the glass, as "the emojis don't work on Firefox".

The fix belongs in the shared hook, not in the three call sites:

- `useHoverPanel(onOpenChangeAction, enabled, isOpen)` takes the current open
  state. `onMouseEnter` timestamps `hoverOpenedAt` **only when the aside was
  closed** — a pointer re-entering an already-open aside (its panel's own
  layout shift moves the box under a stationary cursor) must not count, or the
  toggle would stop closing anything.
- The hook now owns the toggle's click as `toggleFromClick(open)`. Within
  `HOVER_PANEL_CLICK_PIN_MS` (300 ms) of a hover-open it *pins* the panel open
  instead of toggling; past that window it toggles as before, so the button's
  own "Fechar reações" label stays honest.
- Call sites destructure `const {toggleFromClick, ...hover} = useHoverPanel(…)`.
  `toggleFromClick` must never reach the DOM node through the `{...hover}`
  spread.

300 ms is the "moved onto the button and pressed it" gesture window: short
enough that a deliberate later click still closes the panel, long enough that
no engine's event ordering can fall outside it.

The in-app guide (`guide/table`, "Mesa, HUD e controles") now states the
hover-opens / click-keeps-open / click-again-closes behaviour, which was
previously undocumented in either direction.

## 2. `npm run dev:mock` escalated to `/unavailable` without a backend

`checkApiLiveness()` probes `GET /v1.0/health`, which the dev server proxies to
the real API's port. With nothing listening, three failed attempts published a
`reason: 'server'` outage, and two such cycles tripped
`SERVER_OUTAGE_ESCALATION_THRESHOLD` — the whole app navigated to
`/unavailable` a few seconds after boot.

That made mock mode, which exists precisely so the UI can run with no backend,
depend on one. It also silently broke `npm run og:capture` (the guide and OG
screenshots are captured against `dev:mock`) and would have blocked the
browser-level regression suite.

`probeHealthOnce()` now short-circuits to `true` under `USE_MOCK`. The guard
sits at the single network call, so every caller — `NetworkProvider`,
`UnavailableState`, `client.ts`'s retry gate and `useTableRealtime` — is
covered by one line. `USE_MOCK` is itself `NODE_ENV !== 'production'`-gated in
`lib/mockConfig.ts`, so no production build can reach it.

A device that is genuinely offline still reports `reason: 'offline'` in mock
mode: that check runs before the probe, and is asserted in
`src/lib/network/livenessMock.test.ts` (a separate file from `liveness.test.ts`
because `USE_MOCK` is read at module scope).

## Why the existing gates missed both

Neither defect is expressible in jsdom. The first is an event-ordering
difference between two real engines; the second only appears when the app boots
against a port with nothing behind it. Coverage percentage is orthogonal to
both — this is the case for the browser-level visual/behaviour suite tracked
separately.
