# Frontend module review — Live table experience

> Part of the 2026-09-02 systematic review. Fresh code-anchored pass over `ui/src/app/table/page.tsx`,
> `ui/src/components/table/*`, `ui/src/components/reactions/*`, table-supporting libs. Issues renumbered
> in the parent doc's §8 (F-T1…F-T9 → global numbers).

**Verdict:** genuinely high-quality slice. Timer/reduced-motion discipline (`useReducedMotionCountdown`,
`PerimeterTimer` elapsed-capture, `ChipCountUp`), keyboard coverage, optimistic `pending` feel, and
explicit error/empty/loading branches are mostly at the "impeccable" bar already. The findings below
are the real gaps.

## 1. Per-surface OK-vs-impeccable

### Table shell — `app/table/page.tsx`

| Dimension | Current (file:line) | Impeccable |
|---|---|---|
| Race conditions | `handOutcome` (`page.tsx:565`) set once per resolved hand (`:512`), **never cleared**; `blocked={Boolean(handOutcome)}` (`:809`) and `offerBlocked` (`TableStage.tsx:323`) latch `true` forever after hand 1 → **F-T1** | Outcome-visible state tracks the banner's actual lifecycle, not a monotonic ref |
| Edge states | Session recap on `rt.removed` reads `sessions` (`:272`, `:583`) which may not have resolved; `handledRemovalRef` (`:265`) blocks the corrected re-run → recap shows `buyIn: 0`, `joinedAt: Date.now()` → **F-T2** | Defer recap until `sessions` settled, or recompute when it arrives |
| Cognitive load / re-render | ~30 `useState`, ~15 `useEffect`, 8 queries, 1 Hz `setNow` (parent Issue 39) — plus ~120 lines of pot/kicker/showdown derivation at `:403–524` that belongs in `lib/tableOutcome.ts` → **F-T8** | `useTableOutcome()` hook owns outcome assembly |
| Degradation | Solid: invalid-id, `seatedLoading`, no-snapshot, connection-notice branches all render inside `<main>` with the sr-only `h1` | — |

### `ActionBar.tsx` — verdict: strong

| Dimension | Current | Impeccable |
|---|---|---|
| Latency/optimism | `pending` spinner + `actionLabel` ("Pagando…"), prepared-action suppression (`:440`) | — |
| Keyboard | f/c/p/a/h/r/ArrowKeys, hold-to-accelerate shared with touch via `useHoldRepeat` — excellent | — |
| Voice | `onAct` wired straight through; a misheard "tudo"/"all in" commits `maxRaise` with no confirm (`VoiceActionButton.tsx:69–71`, `voiceActions.ts:16`) → **F-T4** | Irreversible voice actions (all-in, raise) require a second confirm |
| Info hierarchy | "Stack efetivo" = `min(own, max(opponent stacks))` (`page.tsx:124`) — against a field with a short all-in this overstates what's at risk | Effective stack vs. the largest opponent you can actually be all-in against, or omit in multiway |

### `Seat.tsx` — verdict: strong

| Dimension | Current | Impeccable |
|---|---|---|
| A11y | Equity/category peek-gate correct (`:238`, `:244`); `aria-current`, role/streak `aria-label`s present | — |
| Motion | `SeatTurnTimer`/`SeatTimeBank` capture elapsed once at mount (`:68`, `:90`); `useReducedMotionCountdown` fallback | — |
| Screen reader | Per-seat state changes (folded, all-in, disconnected) render only as visual `<small>` text — no per-seat live region | Opt-in per-seat polite announcements for state transitions |

### `TableStage.tsx` / `HandOutcome.tsx` — verdict: strong, one latch bug

| Dimension | Current | Impeccable |
|---|---|---|
| Delight/feedback | `HandOutcomeBanner` hold-open + collapsed badge + next-hand ring is excellent; `ChipCountUp` honors reduced motion (`HandOutcome.tsx:204`) | — |
| Race | `outcomeLayer.dismissed` only true on manual minimize (`:356`); with the stale `outcome` prop, `offerBlocked` (`:323`) keeps `WinnerCards` suppressed the whole 12 s the banner shows → pay-to-see-mucked-winner offer effectively unreachable → **F-T1** | Gate `offerBlocked` on the banner actually being on screen |
| Reactions coupling | `TableReactions.tsx:200` does `document.querySelectorAll('.game-seat[data-player-id]')`; `ReactionEffect` positions from a one-shot `getBoundingClientRect()` (`:117`) — stale on any reflow during the 2 s FX → **F-T6** | Shared seat-rect registry/context; reposition on resize |

### `BuyInPanel.tsx` / `RebuyDialog.tsx` — verdict: strong

| Dimension | Current | Impeccable |
|---|---|---|
| Error states | Table-full, not-found, wallet-not-activated all handled with specific copy (`BuyInPanel.tsx:21–29`, `68–83`) | — |
| Slow connection | `RebuyDialog` `AUTO_REBUY_GRACE_MS = 1500` fixed guess (`:30`); comment acknowledges it should be a real `rebuy_failed` push | Backend emits an explicit auto-rebuy-outcome frame |

### `Chat.tsx` — fine. `BotChallenge.tsx` — **lock-out hole** → **F-T3**

| Dimension | Current | Impeccable |
|---|---|---|
| Degradation | `<Dialog open onOpenChange={() => undefined}>` (`:71`) non-dismissable + focus-trapped; if Turnstile script is blocked or never fires its callback, `status` stays `'loading'` forever, no control — reload button only in the `!siteKey` branch (`:83`), `status === 'error'` is text-only (`:90`) → **F-T3** | Timeout `loading → error` after ~15 s; always render an in-dialog "Recarregar"; lobby exit |

### `RabbitHunt` / `WinnerCards` — fine. `RealityCheck` / `SessionRecap` — depends on backend

| Dimension | Current | Impeccable |
|---|---|---|
| Correctness | "Entrada acumulada" = `openSession.buyin_amount` (`page.tsx:738`); session P/L = `currentStack - buyIn` (`RealityCheck.tsx:69`). If `buyin_amount` is the *initial* buy-in and not cumulative across rebuys, both surfaces overstate winnings → **F-T7** (responsible-gaming) | `buyin_amount` cumulative server-side |
| Motion | `RealityCheck` + page `setNow` + `IdleWarning` + `TimeBankStatus` + `useLiveNow` — 4–6 always-on intervals during a turn → **F-T9** | One shared ticker |

### Sound / dealer voice

| Dimension | Current | Impeccable |
|---|---|---|
| Slow connection | `playSound` = `new Audio(file).play()` per call, no preload (`lib/sound.ts:31`) — the `your_turn` cue can lag its download; first cue after enabling can be swallowed by autoplay policy → **F-T5** | Preload + pool `Audio` on enable; unlock on the enabling gesture |

## 2. New issues (full detail)

### F-T1 · [FRONTEND/table] `handOutcome` is never cleared → achievement toasts and the pay-to-see-winner-cards offer die after hand 1
**High · S · $0.** `setScopedHandOutcome({...})` runs at `page.tsx:512` every resolved hand; nothing ever sets it back to `null` (grep: `:312` declare, `:512` set, `:565` read). So `handOutcome` is non-null forever after hand 1. `AchievementToast blocked={Boolean(handOutcome)}` (`:809`) → toasts queue and overwrite each other (`AchievementToast.tsx:22–28`), never render. `WinnerCards offerBlocked={Boolean(outcome && !outcomeLayer.dismissed)}` (`TableStage.tsx:323`); `outcomeLayer.dismissed` only flips on a manual minimize (`HandOutcome.tsx:356`) → the "Pedir a mão de <vencedor>" offer for a hand won without showdown is suppressed the whole ~12 s window, then the next hand removes it (`WinnerCards.tsx:52`). **Fix:** derive `outcomeVisible` from `HandOutcomeBanner` (it already tracks `shown`/`leaving`) and pass that up instead of `Boolean(handOutcome)`; or clear `scopedHandOutcome` when a new hand deals with no payouts.
Acceptance: after 3+ hands an unlock still toasts · winner-cards button usable without minimizing · `blocked`/`offerBlocked` false whenever no banner (card or badge) is on screen · tests for both.

### F-T2 · [FRONTEND/table] Session recap misreports P/L when `rt.removed` arrives before `sessions` resolves
**Medium · S · $0.** Removal effect (`page.tsx:257–286`) reads `sessions` (a separate query, `:200`, `enabled: valid && seated`) to build the recap. For a "not dealt in, instant leave" right after sitting down, `sessions` may not have settled → `openSessionAtRemoval` undefined → recap built with `buyIn: 0`, `joinedAt: Date.now()` (`:275–277`). `handledRemovalRef.current = rt.removed` is set at `:265` *before* this branch, so the corrected re-run bails at `:258`. **Fix:** gate recap composition on `sessions` having loaded, or store raw `rt.removed` and compute in a derived effect that also depends on `sessions`.

### F-T3 · [FRONTEND/table] BotChallenge can permanently lock a player out when Turnstile fails to load / never calls back
**Medium · S · $0.** `<Dialog open onOpenChange={() => undefined}>` (`BotChallenge.tsx:71`) — non-dismissable, focus-trapped, covers the table. Script blocked (privacy extension, corporate proxy, CF outage) or widget loads but never fires `callback`/`error-callback` → `status` stuck `'loading'`, only "Preparando verificação…", no control. Reload button only in `!siteKey` branch (`:83`); `error` branch text-only (`:90`); no timeout. **Fix:** 15 s timer `loading → error`; always render a real "Recarregar" `<Button>` + "Voltar ao lobby" in every non-ready state; `expired-callback` also surfaces retry.

### F-T4 · [FRONTEND/table] Voice "all in" / "aumentar" commits an irreversible bet with no confirmation
**Medium · S · $0.** `parseVoiceAction` maps a single token (`all in`, `tudo`, `aumentar`, `raise`) → `{action:'raise', allIn:true}`; `onresult` immediately calls `onAct('raise', maxRaise)` (`VoiceActionButton.tsx:69–72`, `voiceActions.ts:16–20`). STT misrecognition is common; this is the only place in the product where an irreversible chip commitment has zero ceremony, and feedback ("Comando enviado.") is sr-only. **Fix:** for `raise`/all-in, show a visible confirm chip with a short timeout / require a spoken "confirmar"; fold/check/call stay immediate.

### F-T5 · [FRONTEND/table] Turn/chip SFX allocated per-play with no preload — `your_turn` cue arrives late
**Low · S · $0.** `playSound` = `new Audio(file).play().catch(()=>{})` per call (`lib/sound.ts:27–33`), no preload, no reuse. First play of each file waits on its fetch; the `your_turn` cue (the audible "you're on the clock" signal) can land a second+ late on a slow link. First cue after enabling can be swallowed by autoplay policy. **Fix:** on `setSoundEffectsEnabled(true)` construct + `.load()` one pooled `Audio` per `SoundName` (`.play().then(pause)` once inside the gesture to unlock); `playSound` rewinds a pooled element.

### F-T6 · [FRONTEND/table] Reaction FX position off a one-shot rect read and an untyped DOM query into `Seat`
**Low · M · $0.** `TableReactions` locates seats with `document.querySelectorAll('.game-seat[data-player-id]')` (`:200`); `ReactionEffect`'s ref captures `getBoundingClientRect()` once (`:117–118`) then sets CSS custom-props for the whole 2 s animation. Orientation flip (`TableStage.tsx:304` vs `:331`), outcome-banner reflow, seat-ring resize → projectile animates toward a stale coordinate. The class/`data-player-id` contract is implicit — a `Seat` refactor silently breaks reactions. **Fix:** publish seat rects through a small context/registry `Seat` writes to; reposition on resize or cancel FX on stage-mode change.

### F-T7 · [BACKEND/sessionlog] Session `buyin_amount` must be cumulative for the reality check and leave recap to be truthful
**Medium · S · $0.** `RealityCheck` labels the figure "Entrada acumulada" and computes result as `currentStack - buyIn` (`RealityCheck.tsx:69`); `SessionRecap` shows net vs the same `buyIn`. Both take `openSession.buyin_amount` verbatim. If the server stores only the *initial* seat buy-in and doesn't add each rebuy / auto-rebuy top-up (`RebuyDialog.tsx:93` re-calls join), both surfaces overstate the player's winnings — the opposite of what a responsible-gaming "neutral summary" is for. The client cannot fix this. **Fix:** the open session row's `buyin_amount` (or a new `total_buyin_amount`) accumulates every buy-in/rebuy/auto-rebuy for that seat occupancy; point the two client surfaces at it.
Acceptance: `GET /sessions` returns a cumulative buy-in · after N rebuys `RealityCheck` "Resultado da sessão" == `currentStack − Σ buy-ins` · `SessionRecap` net matches on leave · backend test seat→bust→rebuy.

### F-T8 · [FRONTEND/table] Outcome-assembly logic (~120 lines) lives in the page, not `tableOutcome.ts`
**Low · M · $0.** `page.tsx:403–524` inlines all showdown reasoning — winner/runner-up/tied-seat hole-card extraction, `bestFiveCardHand` reduction, per-pot `contestedPots` mapping, `resolvedPots` shaping, `couldHaveWon` fold comparison. `lib/tableOutcome.ts` owns the primitives; the composition sits in the god component, untestable without rendering the page — a prime spot for the `??`/optional-field bugs `ui/CLAUDE.md` warns about. Additive to parent Issue 39, not a duplicate. **Fix:** `buildHandOutcome(snapshot, viewer, rememberedStart): HandOutcomeState | null` in `lib/tableOutcome.ts`, unit-tested against fixtures (win/lose/tie/mixed/fold, run-it-twice, side pots).

### F-T9 · [FRONTEND/table] Multiple always-on `setInterval` clocks during a turn
**Low · M · $0.** During an active turn ~4–6 independent timers run: page 1 Hz `setNow`, `useLiveNow` at 250 ms in `ActionBar` and each timed `Seat`, `RealityCheck` 15 s, `IdleWarning` 1 s. Each triggers separate React state updates. Measurable battery/jank on a low-end client. **Fix:** one shared `useSharedTicker(hz)` context the timed components subscribe to (aligns with Issue 39; go further — one ticker for all).

## 3. Verified, no action
Fairness/verify UX; preselection auto-execute (`executedRef` + `!connected || pending` guards); stable seat rule (`stableSeatOccupants`, presentation-only); peek/equity leak gate (`Seat.tsx:238/244` matches the security invariant); `useDealerVoice` (opt-in, off by default); reduced-motion at component level (`ChipCountUp`, `useReducedMotionCountdown`, `PerimeterTimer`).
