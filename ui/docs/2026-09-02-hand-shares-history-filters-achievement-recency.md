# Shared-hand links, history filters and achievement recency

2026-09-02 · closes #96, #115, #117, #118, #119, #144

Six frontend follow-ups from the 2026-09-02 systematic review, all consuming endpoints and
payload fields added by the backend branches `feat/api-lobby-handshare-issues` and
`fix/player-achievements-matchup-issues`. **This branch must merge after those two.**

## #96 — Listing and revoking your shared-hand links

`revokeHandShare` existed and was reachable only from `ShareHandDialog` reopened on the same hand
in the same browser, so a regretted link circulated for its whole 1–30 day TTL.

- `listMyHandShares()` (`lib/api/handShares.ts`) wraps `GET /v1.0/players/me/hand-shares` under the
  `HAND_SHARES_QUERY_KEY` react-query key. The response row is `HandShareSummary` — token, kind,
  outcome, net change, created/expires — matching `handShareSummary` in
  `api/internal/api/v1/handshares.go`.
- `components/hands/MyHandSharesPanel.tsx` renders "Meus links compartilhados" at the foot of
  `/hands`: one row per live share with **Copiar link** and **Revogar**. Revoking mutates, then
  invalidates the same key, so the row disappears because the server says so.
- `clearPersistedHandShareByToken` (`lib/handShareStorage.ts`) drops the per-hand local memory of a
  revoked token. The local map stays: the list endpoint does not say which hand a share came from,
  so it cannot answer "I already shared *this* hand".

## #115 — History filters, day grouping, lifetime totals

`lib/handsHistory.ts` holds the pure shaping — `filterHands`, `handTables`, `loadedTotals`,
`groupHandsByDay`/`dayLabel` — deliberately over the pages already in the query cache, so a filter
never costs a refetch. Tests: `lib/handsHistory.test.ts`.

- Outcome chips (Todas / Só vitórias / Só derrotas / Só empates) plus a table row of chips when more
  than one table is loaded, both through the shared `FilterGroup`.
- `groupHandsByDay` flattens to `[day header, …hands]` because the virtualizer measures one row at a
  time. Header keys carry the row index: the list is server-ordered, so the same day can reappear
  and two `day-<date>` keys would collide.
- The day bar above the list is the sticky one. `position: sticky` cannot work on the rows
  themselves — each is placed with a `translateY` — so one bar outside the transformed rows names
  whichever day is at the top of the viewport.
- The stat bar is relabelled "nesta lista" (it follows the filter) and a `Desde o início` strip
  beside it reads the real lifetime numbers from `GET /v1.0/leaderboard/me` (`myRank`), which is the
  only aggregate that describes every hand ever played. `ranked: false` renders nothing.
- The virtualized list lost its `role="list"`: with real `h3` day headings interleaved, a list whose
  children are half headings is a worse tree than links under headings. A named `section` carries
  the count the old `aria-label` announced.

## #117 — Legacy hands are not a fairness failure

`hands/history` used `.deck-reveal-status.mismatch` — the same red `DeckReveal` uses for "hash não
confere" — for a hand with no proof at all. It now uses a neutral `.is-unavailable` panel and says
the hand was recorded before the cryptographic proof existed. A genuine mismatch keeps the red.

## #118 — Rich preview for shared-hand links

`/share` already had full `routeMetadata` (OG title, description, `og/shared-hand.webp`) and
`robots: {index: false}`. What broke the preview was `robots.ts`: `/share` sat in `PRIVATE_ROUTES`,
and every unfurler (WhatsApp, Slack, Discord, X) honours robots.txt, so the bot never fetched the
page. `/share` is now crawlable-but-not-indexable — out of the sitemap, `noindex` retained in the
route meta. A per-hand card would need an image function, which a static export has no server for.

## #119 — "Recém-desbloqueadas" rail and arrival celebration

- `AchievementSummaryEntry.unlocked_at` (backend #72, `omitempty`) drives `lib/achievementRecency.ts`:
  `recentUnlocks` (newest first, undated legacy rows skipped rather than dated to the epoch) and
  `byRecencyFirst` for the "Mais recentes" sort.
- `components/achievements/RecentUnlocksRail.tsx` shows the last five with stars and a relative date.
- The table's `AchievementToast` parks the key it just showed in `sessionStorage`;
  `takeAchievementUnlock()` reads **and clears** it on mount, which is what makes the celebration
  fire once. `sessionStorage`, so it dies with the tab and cannot re-fire days later.
- The celebration is a gold ring (`--gold-33`, new token) plus one `role="status"` line. The ring is
  animation-only, so `prefers-reduced-motion` keeps the gold border and drops the pulse.

## #144 — `cosmetic_purchase_update`

`CosmeticPurchaseDialog` kept its live status in local state behind a hand-rolled 4s interval, which
a websocket frame had no way to reach. The status now lives in the query cache under
`cosmeticPurchaseKey(kind, purchaseId)`, with `refetchInterval` as the same 4s fallback;
`useLobbyRealtime` invalidates `COSMETIC_PURCHASE_QUERY_ROOT` on the frame, so the dialog resolves
on the next tick. The poll is unchanged as a fallback.
