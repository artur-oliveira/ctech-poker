# 13× CPU Performance Pass — 2026-08-29

## Scope

The mock runtime was started with `npm run dev:mock` and the primary application
routes were inspected with Chrome DevTools CPU throttling set to 13×. The pass
covers the home page, lobby, store, guide, achievements, hand history,
leaderboard, rules, and a live mock table (`/table?id=01ARZ3NDEKTSV4RRFFQ69G5FAV&scenario=flop`).

The deep follow-up repeated the audit at **20× CPU throttling** with fresh
targets for all 25 exported application routes, including every guide topic,
hand history/replay, profile/share, unavailable state, and waiting/flop/
showdown table scenes. The development-server trace has a 1.08 s median LCP,
0 median CLS (0.0288 worst), and 2.68 s worst long task. Those long tasks
include on-demand dev compilation, so they are used to find work rather than
as production Core Web Vitals claims.

## Finding and remediation

`table-reactions.css` contains the full reaction-theatre animation system. It
was imported by the root layout, making every route download, parse, and retain
styles that are only reachable from the live table. This increases main-thread
style work during cold navigation, which is especially visible under CPU
throttling and competes with LCP.

The stylesheet is now imported by `TableReactions` instead. Next App Router
loads global styles from the component's route boundary, so non-table routes do
not request or parse the reaction theatre. Table functionality, reduced-motion
rules, and reaction rendering remain unchanged.

The production export confirms the split: the shared routes reference the
313.5 KB global CSS chunk and 13.4 KB route-base chunk; `/table` is the only
route that additionally references the 41.6 KB reaction-theatre CSS chunk.

The 20× CPU route matrix identified the private-room form as a lobby-only
startup cost. It imports React Hook Form, its Zod resolver, and Zod although
the lobby normally needs none of them to find or join a public table. The
lightweight trigger now requests that form only after the player chooses
`Mesa privada`; the request opens the dialog on completion and keeps an
announced disabled control in place while it loads.

A production webpack export confirms that `/lobby` no longer references the
private-room form chunk and successfully prerenders all 27 static entries.
Turbopack's equivalent verification was attempted after the trace but hit an
environment-level worker-port error while parsing global CSS; this does not
reproduce with webpack or the TypeScript, lint, and test checks.

The production export check also identified `/people` as unable to prerender:
its URL-driven tab selection called `useSearchParams()` outside a Suspense
boundary. The page now keeps its stable app shell and a fixed-height skeleton
inside the fallback. This restores static export while preventing the fallback
from shifting the heading and list region when the client tab resolves.

## Guardrails

- The table still loads the stylesheet before `TableReactions` renders; no
  reaction UI is deferred behind a network round trip after the table is usable.
- The root stylesheet retains only truly application-wide styles.
- Further performance work should use a production build and a fresh Chrome
  profile for comparable Core Web Vitals numbers; Next development mode adds
  instrumentation and compilation work that should not be treated as field
  telemetry.
