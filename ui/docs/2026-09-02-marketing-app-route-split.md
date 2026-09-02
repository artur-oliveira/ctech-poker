# Marketing vs app route-group split (#80)

## Why

`/`, `/poker-rules` and `/guide/*` are static, indexable, logged-out-friendly
pages. They were served under the same root layout as the table, so every one of
them downloaded and executed the full authenticated-app client tree:

- root `app/layout.tsx` → `QueryProvider` unconditionally called
  `useSessionKeepAlive()` and mounted `<NetworkProvider><RealtimeBridge/></…>`;
- `RealtimeBridge` → `useLobbyRealtime` → `lib/ws/utils.ts` → the
  protobuf-generated `lib/api/proto/poker.ts`.

Net effect: a text page opened a lobby WebSocket, ran the token keep-alive timer,
started the health poller, and shipped the ~76 KB protobuf chunk
(`3g-_7j6a-9oaz.js`).

## What changed

App Router **route groups** (folder names in parentheses — no effect on the URL):

```
src/app/
  layout.tsx            root: <html>/<body>, fonts, Notifier, RouteAnnouncer — no providers
  (marketing)/
    layout.tsx          MarketingQueryProvider only
    page.tsx            /
    poker-rules/…       /poker-rules
    guide/…             /guide, /guide/*
  (app)/
    layout.tsx          QueryProvider (keep-alive + NetworkProvider + RealtimeBridge)
    lobby/… table/… hands/… leaderboard/… achievements/… profile/…
    people/… store/… share/… callback/…
  robots.ts sitemap.ts error.tsx global-error.tsx not-found.tsx unavailable/
                        stay ungrouped under the root layout
```

- `lib/providers/createQueryClient.ts` — shared `QueryClient` factory (`createQueryClient`)
  plus `getQueryClient()`, which memoises **one** client per browser session. Both group
  providers use `getQueryClient()`, so an authenticated player crossing the
  `(app)` ↔ `(marketing)` boundary keeps the warm `['player','me']` / wallet /
  social-unread cache instead of flashing a `?` avatar and a zeroed balance pill
  while `getMe` re-resolves (very visible on 3G). On the server every request
  still gets a fresh client.
- `lib/providers/MarketingQueryProvider.tsx` — plain `QueryClientProvider` for the
  `(marketing)` group. No keep-alive, no `NetworkProvider`, no `RealtimeBridge`.
- `lib/providers/QueryProvider.tsx` — unchanged behaviour, now mounted only by
  `(app)/layout.tsx`.

URLs, metadata, `INDEXABLE_ROUTES`/`PRIVATE_ROUTES` and every `Link` target are
untouched — route groups are a source-tree concern only.

## Bundle impact

Measured as the sum of every `/_next/static/*.js` chunk referenced by the
prerendered HTML, `main` vs this branch (`next build`, Next 16.3.1):

| Page            | before (raw / gzip) | after (raw / gzip) | protobuf/ws chunk |
|-----------------|---------------------|--------------------|-------------------|
| `/`             | 886.5 / 276.1 KB    | 828.8 / 267.1 KB   | dropped           |
| `/poker-rules`  | 1119.3 / 353.6 KB   | 1067.0 / 346.4 KB  | dropped           |
| `/guide`        | 1117.6 / 353.1 KB   | 1065.3 / 345.9 KB  | dropped           |
| `/guide/basics` | 1122.3 / 354.5 KB   | 1070.0 / 347.5 KB  | dropped           |
| `/lobby`        | 1147.8 / 362.2 KB   | 1148.8 / 363.6 KB  | kept (correct)    |
| `/table`        | 1257.5 / 396.1 KB   | 1258.5 / 397.5 KB  | kept (correct)    |

Plus the runtime win that does not show up in a byte count: on `/`, `/poker-rules`
and `/guide/*` the lobby socket, the token keep-alive interval and the health
poller no longer start.

## Still open (follow-up, not in this change)

Issue #80's stretch targets (`/poker-rules` under 120 KB gzip, Lighthouse ≥ 95)
are **not** reached by the split alone. The residual ~346 KB gzip on the marketing
pages is React + `react-dom`, TanStack Query and the shared `lib/api/*` client
tree, still pulled in because `AppPageChrome` statically imports `ProfileMenu`
(and `useSocialUnread`) for the logged-in variant of the guide/rules nav. Getting
under the target needs a lighter marketing nav (or a `next/dynamic` `ProfileMenu`)
and dropping `react-query`/the axios client from the `(marketing)` group. This
change is the structural precondition for that work.
