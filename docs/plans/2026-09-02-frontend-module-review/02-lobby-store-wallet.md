# Frontend module review — Lobby, room create/join, store, wallet, outage

> Part of the 2026-09-02 systematic review. Fresh code-anchored pass. Issues renumbered in the parent
> doc §8 (F-L1…F-L6, F-W1…F-W2, plus backend F-B* / frontend F-S*).

**Verdict:** the sandbox PIX purchase flow and the liveness/outage architecture are close to impeccable.
The gaps are: cosmetic PIX purchases have no webhook confirmation at all (backend), the lobby's
join-vs-create decision only sees the first page of rooms, and the ungated real-money wallet switch.

## 1. Per-screen OK-vs-impeccable

### Lobby list & stakes grid (`app/lobby/page.tsx`, `components/lobby/StakesGrid.tsx`, `lib/api/rooms.ts`)

| Dimension | Current (file:line) | Impeccable |
|---|---|---|
| Real-time freshness | `useQuery(['rooms'])` `staleTime: 30s`; live `room_created`/`room_updated` deltas patched (`useLobbyRealtime.ts:55–78`); full invalidate on socket (re)open (`:158`). Solid while the socket is up; between load and first frame the grid can be 30 s stale | `refetchOnReconnect` + a ~20 s `refetchInterval` floor; "atualizado há Xs" affordance |
| List completeness | `listRooms()` fetches **only the first page** (`rooms.ts:36–38`, `cursor` never followed); `joinOrCreate` decides join-vs-create from that page (`StakesGrid.tsx:41–43`) → an open room on page 2 is invisible → **creates a duplicate room** → **F-L1** | Server-side "open rooms per bucket" aggregate (server already writes `seats_taken` through) |
| Currency filter | `listRooms()` sends no `currency_mode`; works only because real money is gated off | pass `currency_mode: 'sandbox'` explicitly |
| Room-full / race | `openRoom` from possibly-30 s-stale cache (`StakesGrid.tsx:41`); on hit `router.push('/table?id=…')` immediately, nothing in the lobby reports a lost seat → surfaces later as a buy-in error → **F-L2** | `POST /rooms/join-or-create` — server returns the room it actually seated you into |
| Filter/sort UX | none — radio for blinds + 3 fixed seat cards; `ui/CLAUDE.md` lists lobby filters as *not built*. Fine for a playtest | "só mesas com vaga" toggle, sort by fill, remember last bucket |
| Empty/loading/error | Skeleton, `role="alert"` error with retry, "nenhum stake" branch, per-card `failedKey` inline alert — good | distinguish "stakes catalog down" vs "room list down" |
| Buy-in floor consistency | quick-join `bigBlind*20` (`StakesGrid.tsx:119`) vs `CreateRoomDialog` `bigBlind*40` (`:84`) — two floors, both client-authored → **F-L4** | one named constant; server clamps for public rooms |
| Keyboard / SR | stake radios are real `<input type="radio">` (good); seat-size options are `<Button>` wrapping `<h3>` + spans (`:121–147`) — heading-in-button, invalid, pollutes the SR outline; `disabled` on one join disables all three → **F-L5** | `radiogroup` of `role="radio"`, plain-text labels, per-card busy state |
| Copy (pt-BR) | landing typos on the pre-auth page: "novamnete" (`page.tsx:53`), "compartilhe seus amigos" (`:178`), "a vontade" → "à vontade" (`:48`, `:127`) | fix the four typos |
| `ActiveTableBanner` | `sessions.find(s => s.ended_at === 0)` (`:11`), no realtime refresh — `['sessions','me']` never invalidated by `useLobbyRealtime`; "Retomar mesa" lingers ≤30 s after cashing out elsewhere → **F-L2** family | invalidate `['sessions','me']` on the lobby socket's leave-class frames |

### Create private room (`components/lobby/CreateRoomDialog.tsx`)

| Dimension | Current (file:line) | Impeccable |
|---|---|---|
| Real-money exposure | currency radiogroup only renders when `realStakes.length > 0` (`:106`), which only fetches when `me.wallet_mode === 'real'` (`:58–61`). Clean with the flag off | gate the whole `real` branch behind `REAL_MONEY_UI_ENABLED` |
| Error states | `form.setError('root', …)` → `role="alert"` (`:94`, `:181`). But `submit` `await`s `invalidateQueries(['rooms'])` **before** `router.push` (`:87–92`); a transient reject shows "Não foi possível criar a mesa" **even though the room was created** (`:93`) → **F-L3** | push first (you have the id), invalidate after / fire-and-forget |
| Keyboard / SR | full APG radiogroup, roving tabindex, arrow keys (`:42–49`, `:114–127`) — genuinely good; model for the rest of the lobby | — |
| Copy | responsible-gaming real-money line is a `.form-error` (red) used as an info notice (`:129–131`) | neutral note, not `.form-error` |

### Store — chip packs / PIX (`app/store/page.tsx`, `PurchaseModal.tsx`, `PixPaymentView.tsx`, `useCountdown.ts`)

| Dimension | Current (file:line) | Impeccable |
|---|---|---|
| pending → confirmed → credited | 5 s poll of `getPurchase` while `pending` (`PurchaseModal.tsx:46–67`) **and** `useLobbyRealtime` `sandbox_purchase_update` → invalidate wallet + `['player','me']` + toast (`:79–91`); webhook is the real trigger (`walletwebhook.go:64–79`); success panel with `total_credits`. **Well built.** | add a transient "pagamento recebido, creditando…" state (backend `code:'processing'`) — today it jumps |
| on failure | `{refunded,expired,failed}` → `role="alert"` + ws toast — covered | "falhou" gives no next step — offer "gerar novo Pix" for `failed` too, not only `expired` |
| on expiry | `recoverableExpired` → "Código Pix expirado" + "Gerar novo Pix" via `regeneratePurchase(sku)` (`:34–35`, `131–139`). Good | button disabled if `!purchase.sku` (`:135`) — a resumed history purchase may lack `sku` → stuck. Ensure `sku` always present |
| countdown | `useCountdownMs` = `setInterval(1000)`, not second-aligned, no immediate tick (`useCountdown.ts:9–16`); background-tab throttling makes it jump on return | compute from `Date.now()` per render + visibilitychange resync; recheck `expired` on focus |
| slow connection | `getPurchase` poll failure → `pollFailed` `role="alert"` + manual "Verificar pagamento" — nicely handled | — |
| QR type detection | `qr_code_base64?.startsWith('PHN2Zy')` sniffs SVG-vs-PNG from base64 magic bytes (`PixPaymentView.tsx:31`) — fragile | backend sends `qr_mime` |
| keyboard / SR | `Dialog` focus-trap + `finalFocus` via `purchaseTriggerRef`; copy button `aria-live` + `copyFailed` fallback — careful | — |
| copy | "As fichas são apenas do modo sandbox e não têm valor em dinheiro" on a real-BRL QR — exactly right | — |

### Store — reactions & cosmetics (`ReactionPurchaseDialog.tsx`, `CosmeticPurchaseDialog.tsx`)

| Dimension | Current (file:line) | Impeccable |
|---|---|---|
| Reaction PIX confirm | polls `getReactionPurchase` every 4 s **and** `useLobbyRealtime` `reaction_purchase_update` (`walletwebhook.go:45–58`) — covered even when the dialog is closed | — |
| **Cosmetic (deck/felt) PIX confirm** | **No realtime path at all.** `RegisterWalletWebhook` built with only `sandboxSvc` + `reactionSvc` (`walletwebhook.go:27`); a cosmetic webhook falls through to `sandboxSvc.ConfirmFromWebhook` → `changed:false` → no broadcast. `cosmeticpurchase.Service.ConfirmFromWebhook` exists (`service.go:279`) but is never wired to the route. `useLobbyRealtime` has no `cosmetic_purchase_update` branch. The dialog polls 4 s **only while open** and its own copy says "Você pode fechar esta janela" (`CosmeticPurchaseDialog.tsx:140`). Close it during a PIX wait → deck/felt **paid but not granted and not announced** until the user manually reopens that purchase → **F-B1** | wire the cosmetic service into the webhook; emit `cosmetic_purchase_update`; add the client branch |
| Insufficient balance (fichas) | `insufficientFichas` disables the fichas button, but `sandboxBalance` is a threaded prop — if stale, button enabled, server 400s → generic "Não foi possível usar este meio de pagamento" (`CosmeticPurchaseDialog.tsx:32–35`), never "saldo insuficiente" → **F-L6** family | map the specific 400 code to "Saldo sandbox insuficiente — complete com Pix ou resgate a recompensa diária" |
| ownership counters | "N de M liberadas" from a single catalog response's `owned` flag — the prior numerator-outruns-denominator bug is fixed | — |

### Balance display consistency

| Dimension | Current (file:line) | Impeccable |
|---|---|---|
| source of truth | sandbox balance from `['player','me'].sandbox_balance` in both the pill (`ProfileMenu.tsx:94`) and the store (`store/page.tsx:191`) — consistent | — |
| **dead query key** | `['wallet','balance']` invalidated in **three** places (`PurchaseModal.tsx:40`, `DailyRewardPanel.tsx:42`, `useLobbyRealtime.ts:160`) but **no `useQuery` reads it**. Balance freshness actually rides on the co-located `['player','me']` invalidate. A future balance widget wired to `['wallet','balance']` would appear to work in those 3 paths and go stale everywhere else → **F-S1** | delete the 3 dead invalidations + comment the canonical key, OR make balance a real query and invalidate that everywhere |
| real-money balance while gated | `ProfileMenu` **always** renders "Dinheiro real R$ 0,00" (`:222`); `DESIGN.md`: the UI must never imply real money is enabled by default → **F-L3 (wallet switch)** | hide the row + switch unless `REAL_MONEY_UI_ENABLED` |

### Currency-mode switching

| Dimension | Current (file:line) | Impeccable |
|---|---|---|
| gating consistency | `CurrencyModeTabs` correctly disables + relabels the real option when `!REAL_MONEY_UI_ENABLED` (`:15–18`). **But `ProfileMenu`'s wallet-mode `Switch` is not gated at all** (`:171–173`), fires `save.mutate({wallet_mode:'real'})` unconditionally, and `save` has **no `onError`** for it → generic toast only → **F-L3** | gate the row behind the same flag; add `onError` |
| pointless control | `SelfHudDialog` renders `<CurrencyModeTabs>` unconditionally (`:135`) — a segmented control with one permanently-disabled option, on every open | render only when `REAL_MONEY_UI_ENABLED` |

### Outage handling & `/unavailable`

| Dimension | Current (file:line) | Impeccable |
|---|---|---|
| detection | dependency-free `/v1.0/health` probe, 3 attempts × 600 ms (`liveness.ts:6–8`, `96–111`); single snapshot via `useSyncExternalStore`; request interceptor gates every call on `requireApiLiveness()`. **Genuinely well-architected** | — |
| **two divergent outage UIs** | explicit HTTP 503 → full-page `/unavailable` (`client.ts:139–147`); a hard-down that surfaces as a fetch `TypeError` (the *expected* dead-HAProxy shape, `liveness.ts:64–69`) → `redirectOnServiceUnavailable(undefined)` returns false → user gets only the thin `NetworkProvider` strip. The more common shape gets the quieter treatment → **F-W1** | one severity model — escalate `reason:'server'` to the full screen after N failed probes |
| recovery | "Verificar novamente" → `checkApiLiveness`, restores `poker:return-after-outage` route on success; `NetworkProvider` auto-polls with jittered backoff + `refetchQueries({type:'active'})`. Good | — |
| **retry gives no feedback on failure** | `UnavailableState.retry`: still-unavailable → `setChecking(false); return` — nothing else (`:14–17`); `detail` reverts to the static line → looks like the button doesn't work → **F-W2** | "Ainda fora do ar — última verificação HH:MM · nova tentativa automática em Xs" from `livenessPollDelay`; subscribe to the snapshot for auto-recovery |
| slow-but-alive | 3 s timeout + 2 retries on safe methods | slightly longer timeout for `POST …/join` / purchase creates (they carry `Idempotency-Key`, retry is safe) |

## 2. New issues (full detail in the parent doc §8; summaries here)

- **F-B1 · [BACKEND/cosmeticpurchase] Cosmetic PIX purchases have no webhook confirmation or realtime push** — **High · S–M · $0.** `walletwebhook.go:27` only knows sandbox + reactions; cosmetic ids fall through to sandbox → no `ConfirmFromWebhook`, no broadcast; `useLobbyRealtime` has no `cosmetic_purchase_update` branch; the dialog polls only while open and invites closing it. Paid-but-ungranted, silent. Fix: pass the cosmetic service into `RegisterWalletWebhook`; try `cosmeticSvc.ConfirmFromWebhook` before the sandbox fallthrough; broadcast `cosmetic_purchase_update`; add the client branch mirroring reactions. Acceptance: closed dialog + confirmed cosmetic PIX → toast + catalog shows owned within one socket round-trip; refunded/expired/failed also broadcast; webhook test with a cosmetic id.
- **F-S1 · [FRONTEND/store] Dead `['wallet','balance']` query key invalidated in 3 places, read nowhere** — Low · S · $0. Delete the dead invalidations + document `['player','me']` as canonical, or introduce a real balance query and route every balance-moving path through it.
- **F-L1 · [FRONTEND/lobby] "join vs create" uses only the first page of `/rooms`** — Medium · M · $0. → duplicate rooms once >1 page; also no `currency_mode`. Fix: lobby-scoped aggregate (open counts + one joinable id per bucket, per currency), or walk the cursor.
- **F-L2 · [FRONTEND/lobby] Room-full seat race invisible in the lobby** — Medium · M · $0. `joinOrCreate` navigates on a stale-cache selection; failure only surfaces on the table page. Fix: `POST /rooms/join-or-create` server-resolved intent; interim bounce-back + auto-retry.
- **F-L3 · [FRONTEND/lobby] Ungated real-money wallet-mode `Switch` in ProfileMenu, no error handler** — Medium · S · $0. Gate the "Modo de jogo" row + the "Dinheiro real" balance behind `REAL_MONEY_UI_ENABLED`; add `onError` that reverts + explains.
- **F-W1 · [FRONTEND] Network-error total outage never escalates past the thin strip** — Medium · S–M · $0. After N consecutive `reason:'server'` probe failures, navigate to `/unavailable` (writing the return path) or render a full-bleed blocking state; keep the strip for `offline`.
- **F-W2 · [FRONTEND] `/unavailable` "Verificar novamente" gives no feedback when still down** — Low · S · $0. Show last-failed timestamp + auto-retry countdown; subscribe to the liveness snapshot for hands-free recovery.
- **F-L4 · [FRONTEND/lobby] Buy-in min/max is client-authored and inconsistent (20 vs 40 BB)** — Low · S · $0. `BUY_IN_MIN_BB`/`BUY_IN_MAX_BB` constants; server clamps public rooms.
- **F-L5 · [FRONTEND/lobby] Seat-size options are buttons wrapping headings; whole grid disables on any one join** — Low · S · $0. `radiogroup` of `role="radio"`, plain-text labels, per-card busy state.
- **F-L6 · [FRONTEND] Duplicate/divergent daily-reward + leaderboard API wrappers** — Low · S · $0. `lib/api/gamification.ts` `spin`/`remainingTime` (no trailing slash) vs `lib/api/dailyReward.ts` (trailing slash, the one actually used); `gamification.ts` also re-declares `leaderboard()`. Consolidate.

## 3. Verified, no action
Sandbox PIX flow (dual poll+socket, all states, regenerate-on-expiry, `pollFailed` recovery — near impeccable); reaction PIX flow; the liveness architecture; store-dialog focus management (`purchaseTriggerRef` + `finalFocus`); ownership counters; `CreateRoomDialog` radiogroup keyboard impl; PIX disclosure copy; `mockConfig.ts` / `cardVariants.ts` / `tablePreferences.ts`.
