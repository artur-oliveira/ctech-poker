# Whole-App UI Critique — 2026-08-24

> Scope: every `src/app/**/page.tsx` route and its rendered component tree.
> Live evidence: `localhost:3003` in mock mode, desktop and mobile.
> Design health: **34/40**.

Method: dual-agent (A: /root/assessment_a · B: /root/assessment_b)

## Design Health Score

| #         | Heuristic                       |     Score | Key issue                                                                                                                                                         |
|-----------|---------------------------------|----------:|-------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| 1         | Visibility of System Status     |         4 | Connection, turn, pending action, purchase, reward, loading, and replay states are unusually thorough.                                                            |
| 2         | Match System / Real World       |         3 | The poker model is excellent, but `Fold`, `Check`, `Stakes`, `Run it twice`, `Provably Fair`, `VPIP`, `PFR`, and `3-bet` still require translation for newcomers. |
| 3         | User Control and Freedom        |         3 | Back, cancel, close, and blocked-leave explanations are strong; some secondary mutations have no immediate reversal path.                                         |
| 4         | Consistency and Standards       |         3 | Shared chrome and controls are cohesive, but invalid hand history, public artifacts, and heading hierarchies diverge.                                             |
| 5         | Error Prevention                |         4 | Mid-hand exit prevention, disabled real-money mode, constrained buy-ins, purchase confirmation, and selection limits are excellent.                               |
| 6         | Recognition Rather Than Recall  |         3 | Labels and page identity are strong; four mobile destinations disappear under generic “Mais,” while the table is visually icon-heavy.                             |
| 7         | Flexibility and Efficiency      |         4 | Keyboard actions, pre-actions, raise presets, voice, replay speed, deep links, and preferences support expert play.                                               |
| 8         | Aesthetic and Minimalist Design |         3 | The main decision stays clear, but reactions, private-room stakes, store departments, and guide directories create choice walls.                                  |
| 9         | Error Recovery                  |         3 | Most errors preserve context and offer retry; missing hand history is visibly unfinished and hydration errors weaken reliability.                                 |
| 10        | Help and Documentation          |         4 | Seven task-specific guides, rules, inline terms, and fairness explanations are outstanding.                                                                       |
| **Total** |                                 | **34/40** | **Good — strong foundation, focused release issues remain.**                                                                                                      |

## Anti-Patterns Verdict

### LLM assessment

This does **not** read as indiscriminate AI slop. The physical rail/felt/card system, genuinely different desktop and
portrait tables, inspectable fairness proof, public replayer, responsible-money copy, and real screenshots form a
recognizable CTech Poker identity. The authenticated product register is disciplined and generally trustworthy.

The marketing layer carries mild AI-era reflexes: a sparkle hero kicker, six repeated icon/heading/body features, four
near-identical achievement cards, a dark-maroon glow composition, repeated `rise` entrances, and compact uppercase
metadata. These are diluted by specific poker artifacts and asymmetrical screenshot stories, but the landing page is
less distinctive than the table.

One contradiction is more serious than aesthetic sameness: a product positioned as friendly and trustworthy sells or
exposes “Jogar cocô,” “Rir da cara,” “Chamar de lento,” “Jogar faca,” and “Jogar tomate.” That is not harmless visual
flavor; it turns hostility into a first-party affordance.

### Deterministic scan

The CLI detector returned **68 findings: 5 warnings and 63 advisories**.

- 2 real layout-property animations: guide-directory `padding` and achievement progress `width`.
- 50 off-ramp font sizes. Many are legitimate responsive/readout sizes, but the 7–9px phone-seat labels agree with the
  browser’s nine-seat legibility evidence.
- 6 undocumented categorical note-tag colors. Intentional, but still design-system/documentation drift.
- 7 undocumented radii. The 2px card thumbnails and 18px reaction target are defensible; four 10px values are minor
  drift.
- 3 “broken image” warnings are false positives: `next/image` sources rendered correctly, and browser checks found zero
  missing `alt` images.

The injected live detector reported 24 anti-patterns on landing, 11 on Guide, 7 on Lobby, 7 on Store, and 2 on Table.
Labels included dark/glowing palette, hairline-plus-shadow, tiny text, long lines, and hero eyebrow. Several are
contextual false positives—dialog elevation and the table’s dark material system are intentional—but tiny table text and
the hero kicker are corroborated.

### Visual overlays

Mutable injection succeeded on landing, guide, lobby, store, and table. Because the available browser was headless, the
overlays existed only in temporary screenshots and console logs; **there is no user-visible `[Human]` overlay**. The
temporary detector server and browser were stopped and cleaned up.

## Overall Impression

The app already feels like a real poker product rather than a themed CRUD shell. Its biggest opportunity is not a
redesign; it is protecting that strong trust system from three kinds of erosion: hostile social affordances, mobile
density/occlusion, and inconsistent archive/error semantics. Fix those and the product moves from “good” toward
release-grade excellent.

## What’s Working

1. **The living table is real architecture.** At 320, 360, 375, 390, and 430px, heads-up, 6-max, and 9-max remain within
   a blocking `100dvh` composition with no document-level horizontal overflow. Desktop and portrait are structurally
   different rather than scaled copies.
2. **Trust and responsible-money semantics are unusually clear.** Sandbox labels, disabled real-money controls, explicit
   Pix language, buy-in bounds, mid-hand leave protection, local deck verification, anonymized share links, and neutral
   reality checks support the positioning.
3. **Accessibility and state coverage are deeply integrated.** Across 56 fresh browser sessions there were zero
   approximately unnamed controls and zero images missing alt text. Focus, reduced motion, aria-live table feedback,
   skeletons, retry paths, and dialog containment are substantive strengths.

## Cognitive Load

Checklist result: **3 failures of 8 — moderate cognitive load**.

- Pass: single focus, grouping, hierarchy, one decision at a time, and working-memory support.
- Fail: chunking, minimal choices, and progressive disclosure.

Decision points over four visible choices include desktop landing navigation (8), authenticated navigation (8), guide
hub (8), guide topic strip (7), rules TOC (6), People filters (5), Store directory (5), private-room stakes (11 plus
seats and run-it-twice), profile showcase achievements (9 while selecting 3), and reactions (22 plus favorite
duplicates).

## Emotional Journey

The beginning is warm and credible: “Chame seus amigos” is supported by a believable table rather than casino hype.
Lobby onboarding lowers anxiety, and the table creates a strong peak through clear turn ownership, cards, pot, result,
hand category, and payout. Fairness history converts excitement into trust.

The valley is social: the reaction vocabulary can turn a celebratory or competitive moment into ridicule. High-stakes
reassurance elsewhere is excellent—blocked leave explains why, buy-in promises no debit before confirmation, the store
repeats sandbox restrictions, and reality checks wait until the player is not acting. The weakest ending is
missing-parameter `/hands/history`, which looks like unfinished content rather than intentional recovery.

## Priority Issues

### [P1] Hostile reactions violate the product’s social and safety promise

**Why it matters:** The first-party reaction vocabulary productizes ridicule and aggression, contradicts “friendly
without becoming childish,” and undermines the otherwise strong community/reporting system.

**Fix:** Remove or replace hostile labels/animations with sportsmanlike equivalents such as “Boa mão,” “Respeito,” “Que
virada,” “Boa leitura,” and “Café.” If teasing remains, make it opt-in per table and run every reaction through the same
moderation standard as chat.

**Suggested command:** `$impeccable quieter`

### [P1] The nine-seat phone table is structurally stable but locally illegible

**Why it matters:** The browser found no page overflow, yet the rendered 390px nine-seat scene has captions and stacks
colliding with the rail and one another; “Desconecta…” truncates, chip totals clip, and several visible labels are only
7–9px. Player identity and stack are core gameplay information.

**Fix:** Define a nine-seat micro-layout separately from 6-max: reserve caption lanes, collapse nonessential state into
icons plus accessible text, enforce a readable minimum type size, abbreviate only standardized states, and validate at
320×568 through 430×932 with long localized names and five-digit stacks. Keep the documented invisible 44px seat-menu
hit zones.

**Suggested command:** `$impeccable adapt`

### [P1] Public profile/share hydration mismatches undermine reliability

**Why it matters:** Valid and missing profile states and missing share states reproducibly emitted React hydration
mismatches at desktop and mobile; the mobile Next dev error badge was visible. The diff involved `RouteAnnouncer`
-related `tabindex=-1` mutations on `<main>`/`h1` before hydration. Public artifacts are trust surfaces.

**Fix:** Prevent pre-hydration DOM mutation. Move focus only after hydration and route transitions, use a stable
ref/explicit focus target, and ensure server/client markup agrees. Add hydration-focused browser tests for valid/missing
profile and share states.

**Suggested command:** `$impeccable harden`

### [P2] Fixed mobile navigation obscures interactive content on five routes

**Why it matters:** At the initial mobile viewport, the fixed tab bar intersects filters or actionable rows on
Achievements, Hands, Lobby, People, and Store. Content remains scrollable, so this is not blocking, but a user can see
or tap only part of an interactive target.

**Fix:** Give every `.has-tab-bar` scroll surface measured bottom padding equal to tab height plus safe area; add
`scroll-padding-bottom`; ensure virtualized lists include the same trailing inset; retest filtered two-row controls and
purchase/room cards.

**Suggested command:** `$impeccable adapt`

### [P2] Archive/help surfaces need an accessibility and immediacy pass

**Why it matters:** Active filters measure 4.25:1 instead of 4.5:1; Achievements jumps H1→H3; valid Replay has H2 but no
H1; invalid Hand History has no heading; its recovery surface is visually unfinished. Static Hands/History/Share content
also replays live card-deal choreography for up to ~2.06 seconds.

**Fix:** Darken the active filter token; repair heading trees; reuse the polished replay/SystemState recovery vocabulary
for invalid hand history; add a context prop to `PlayingCard` so static archives render immediately or crossfade
together while live table/replayer retains sequential dealing.

**Suggested commands:** `$impeccable audit`, then `$impeccable animate`

## Persona Red Flags

### Jordan — first-time player

- Must scan 11 stakes before receiving a recommendation.
- Encounters `Fold`, `Check`, `Stakes`, `Run it twice`, `VPIP`, `PFR`, `3-bet`, and `Provably Fair` before or outside
  their best explanations.
- Four mobile destinations sit under generic “Mais,” weakening the map to Guide and Hands.
- Positive counterweight: onboarding, buy-in copy, rules, and guide content are excellent once found.

### Sam — accessibility-dependent player

- Active filters fail AA at 4.25:1.
- Achievements, Replay, and invalid Hand History have heading defects.
- Nine-seat mobile labels fall to 7–9px; visible table header controls are icon-only and 40px wide, although accessible
  names are present.
- Strong counterweight: zero unnamed controls or missing alt text, visible focus, route announcements, aria-live
  feedback, reduced motion, and semantic dialogs.

### Casey — distracted mobile player

- Bottom navigation and action bar are well placed, but the tab bar covers parts of interactive content on five routes.
- A 22-option reaction sheet competes with a timed poker decision.
- The private-room modal exposes 11 stakes at once.
- Achievements (~8,619px), Community guide (~7,695px), Table guide (~8,161px), and Store (~5,926px) make return
  orientation difficult after interruption.

## Page-by-Page Critique — all 23 `page.tsx` files

| Page                                                    | Specific critique                                                                                                                                                                                                                                                                                       |
|---------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `src/app/page.tsx` — `/`                                | Strong, distinctive hero and credible real-product screenshot stories. Mobile CTA and table preview are clear. The sparkle kicker, six uniform feature items, repeated `rise` sequence, and four achievement cards are the closest template tells; at ~7,203px mobile, one repeated section could go.   |
| `src/app/callback/page.tsx` — `/callback`               | “Autenticando seu lugar…” fits the voice; expired-code copy is specific and offers retry/back. No-param redirect is correct. Progress/error were source-inspected because a valid external OAuth exchange was intentionally not fabricated.                                                             |
| `src/app/unavailable/page.tsx` — `/unavailable`         | Excellent 503 composition: artifact, code, reassurance about chips/history, retry, home escape, and active checking state make recovery calm and credible.                                                                                                                                              |
| `src/app/lobby/page.tsx` — `/lobby`                     | Clear hierarchy and strong onboarding. Stake→table-size sequencing is sensible, but 11 blind choices still overload setup. Skeletons resemble final content; private-room validation is strong but option-dense. Mobile tab bar obscures part of a room action at the initial viewport.                 |
| `src/app/achievements/page.tsx` — `/achievements`       | Gold/value semantics, mastery, next-star cue, and tier ladders work. Mobile becomes an ~8,619px repeated-card catalog; active filters fail AA, H1 jumps to H3, and the second filter row can sit behind the tab bar.                                                                                    |
| `src/app/leaderboard/page.tsx` — `/leaderboard`         | Viewer position before podium is excellent prioritization; rank and stats scan immediately. Active Sandbox control shares the contrast defect. Small datasets leave harmless desktop emptiness.                                                                                                         |
| `src/app/people/page.tsx` — `/people`                   | Mutual friendship/privacy copy is unusually clear, and every tab has useful actions and empty/error states. Five top-level filters plus request direction push scanning; the mobile tab bar intersects an underlying action. Hostile reactions contradict this surface’s safety posture.                |
| `src/app/profile/page.tsx` — `/profile?id=`             | Missing profile is calm and actionable; valid profile is concise and avoids vanity-dashboard clutter. The valid mobile profile fits in ~863px. Both valid and missing states emit hydration mismatches tied to pre-hydration focus mutation.                                                            |
| `src/app/guide/page.tsx` — `/guide`                     | Three-step quick start is the right lede. Seven topic choices plus rules form a choice wall, though “Tudo sobre a mesa” and real screenshots create hierarchy and credibility.                                                                                                                          |
| `src/app/guide/achievements/page.tsx`                   | Clear mastery terminology and useful mobile disclosures. Screenshot grounds the explanation; five sections are long but scannable.                                                                                                                                                                      |
| `src/app/guide/basics/page.tsx`                         | Best first-timer content: account, lobby, public/private room, buy-in, and first hand are sequenced naturally. Length is justified by the task.                                                                                                                                                         |
| `src/app/guide/community/page.tsx`                      | Thorough and responsible, but ~7,695px on mobile. Its safety guidance is directly contradicted by the hostile first-party reaction vocabulary.                                                                                                                                                          |
| `src/app/guide/hands/page.tsx`                          | Clear path from finding a hand through replay and proof. “Provably Fair” remains jargon in the title, but the description promptly translates it.                                                                                                                                                       |
| `src/app/guide/profile/page.tsx`                        | Strong separation between public badges and private detailed stats. VPIP/PFR/3-bet are defined locally; the live profile-menu screenshot helps recognition.                                                                                                                                             |
| `src/app/guide/store/page.tsx`                          | Excellent sandbox/Pix/status/refund guidance. The index could more forcefully separate free daily reward from paid packages.                                                                                                                                                                            |
| `src/app/guide/table/page.tsx`                          | Comprehensive and appropriately rich, but ~8,161px mobile. Returning users need the “Nesta página” control to remain easy to reopen or become sticky.                                                                                                                                                   |
| `src/app/poker-rules/page.tsx` — `/poker-rules`         | Six-section TOC, real card art, ordered hierarchy, and paired English/Portuguese action names work as a quick reference. One inline guide link is only 24px high under a strict 44px target policy.                                                                                                     |
| `src/app/hands/page.tsx` — `/hands`                     | Stats/result hierarchy and virtualized rows are strong; local seed status adds trust. Static card dealing delays readability and initially shows edge-on slivers. Mobile card faces clip ~7px, and the tab bar covers part of lower rows.                                                               |
| `src/app/hands/history/page.tsx` — `/hands/history?...` | Valid state is an excellent forensic story: result, seats, board, timeline, replay, export/share, and local proof. Missing params produce an unfinished top-left line with no heading. Copy-ID is 28×28; a mobile card back extends ~30px within clipped content.                                       |
| `src/app/hands/replay/page.tsx` — `/hands/replay?...`   | Valid replay is a credible full-table artifact with familiar transport controls; portrait keeps hierarchy. It has H2 but no H1, and some status/speed controls miss the project’s 44px target. Missing-address recovery is polished and should model Hand History.                                      |
| `src/app/share/page.tsx` — `/share?...`                 | Public summary is appropriately anonymized, shows expiry, and embeds a useful replay. Mobile board scrolling needs an edge fade or “deslize” cue. Missing state emitted a hydration mismatch; use `token=` consistently in docs/tests because the implementation reads `token`, not `id`.               |
| `src/app/store/page.tsx` — `/store`                     | The five-item directory is necessary on a ~5,926px mobile page and purchase copy is exceptionally explicit. Departments are controlled but still long; mobile tab bar overlaps purchase content.                                                                                                        |
| `src/app/table/page.tsx` — `/table?id=`                 | Strongest surface: physical without casino clutter, honest reconnect, excellent action bar, leave protection, reality check, result, and responsive dual layout. Structural overflow is clean, but nine-seat phone labels/stacks become locally illegible. Reactions are overloaded and tonally unsafe. |

## Component-by-Component Critique

### Shared chrome, state, and primitives

| Component                   | Specific critique                                                                                                                        |
|-----------------------------|------------------------------------------------------------------------------------------------------------------------------------------|
| `AppPageChrome` / `AppPage` | Cohesive shared shell; `.has-tab-bar` needs measured bottom clearance on every scroll surface.                                           |
| `AppPageBody`               | Consistent content width and rhythm; solid.                                                                                              |
| `AppPageNav`                | Labeled desktop routes and badges are clear; eight visible choices exceed the minimal-choice guideline.                                  |
| `AppTabBar`                 | Good thumb-zone compromise and labels; generic “Mais” weakens location recognition, and fixed placement obscures content on five routes. |
| `AppPageHeader`             | Compact icon/title/description hierarchy works; ensure page-specific heading levels remain valid in child content.                       |
| `AppPageFooter`             | Quiet, relevant exit/help links; solid.                                                                                                  |
| `PokerLogo`                 | Distinctive, compact, and consistently applied; detector broken-image warning was false.                                                 |
| `CurrencyModeTabs`          | Unavailable real-money copy is honest; active bright-red contrast is 4.25:1 and must be corrected.                                       |
| `FilterGroup`               | Pressed-state semantics and wrapping are good; same contrast failure, and five-option use pushes cognitive load.                         |
| `HandRankings`              | Compact visual reference with category-specific value; solid.                                                                            |
| `PlaystyleBadges`           | Progressive detail without exposing private raw metrics; solid.                                                                          |
| `TermsGate`                 | Explicit unprechecked consent, retry, and honest mode language; solid.                                                                   |
| `SystemState`               | Strong branded recovery grammar; reuse it for invalid hand history.                                                                      |
| `UnavailableState`          | Retry preserves safe destination and provides honest checking copy; solid.                                                               |
| `Notifier`                  | Correct info/error live roles and dismiss path; solid.                                                                                   |
| `RouteAnnouncer`            | Valuable SPA focus/announcement behavior, but DOM mutation must not occur before hydration.                                              |
| `AchievementToast`          | Correctly queues behind hand outcome; excellent transient-layer ownership.                                                               |
| `Button`                    | Consistent 44/48px sizing, focus, variants, and pending compatibility; solid.                                                            |
| `Input`                     | Clear focus/error vocabulary and readable placeholder token; solid.                                                                      |
| `Label`                     | Standard explicit association; solid.                                                                                                    |
| `Select`                    | Portal avoids clipping and items meet target height; manually verify focus ring after hydration fixes.                                   |
| `Checkbox`                  | Strong checked/focus semantics; 20px visual is acceptable inside a full label hit area.                                                  |
| `Switch`                    | Familiar track/thumb model with expanded hit wrapper; solid.                                                                             |
| `Dialog`                    | Focus trap, Escape, close label, viewport cap, scroll containment, and mobile action stacking are strong.                                |
| `Popover`                   | Correct portal/positioning choice for nav and action menus; solid.                                                                       |
| `Avatar`                    | Stable primitive; solid.                                                                                                                 |
| `PlayerAvatar`              | Shared initials/image fallback prevents cross-page identity drift; solid.                                                                |
| `Progress`                  | Semantic progress role and restrained treatment; animate transform/scale rather than `width` where possible.                             |
| `Skeleton`                  | Labeled and layout-matched; solid.                                                                                                       |
| `EmojiGlyph`                | Normalizes emoji rendering, but inherits the hostile reaction taxonomy.                                                                  |

### Landing and route-local helpers

| Component/helper     | Specific critique                                                                                              |
|----------------------|----------------------------------------------------------------------------------------------------------------|
| `HeroTable`          | Memorable and category-specific; strongest landing artifact.                                                   |
| `RealScreen`         | Real product screenshots materially improve trust; solid.                                                      |
| `Callback`           | Specific progress/failure voice and clear escape paths; solid.                                                 |
| `HandRow`            | Excellent compact hierarchy after settling; static deal animation and mobile clipping hurt first-read clarity. |
| `VirtualHandsList`   | Efficient long-list behavior; must include fixed-tab-bar trailing inset.                                       |
| `HandHistoryContent` | Excellent valid state, weak/no-heading invalid state.                                                          |
| `ReplayContent`      | Strong valid and invalid branches; add a real H1 in valid state.                                               |
| `ProfileContent`     | Privacy-aware and concise; hydration mismatch is the critical defect.                                          |
| `SharedHandContent`  | Anonymization and expiry are clear; add horizontal-scroll affordance and eliminate hydration mismatch.         |
| `IdleWarning`        | Timely, nonjudgmental, and actionable; solid.                                                                  |
| `TableContent`       | Strong state orchestration and one-panel ownership; preserve while adapting nine-seat micro-layout.            |
| `usePurchaseHistory` | Nonvisual helper gives consistent pagination/loading behavior; solid.                                          |
| `useCountdown`       | Clear time semantics across Pix/reward flows; solid.                                                           |

### Guide system

| Component        | Specific critique                                                                                                           |
|------------------|-----------------------------------------------------------------------------------------------------------------------------|
| `GuidePage`      | Responsive native disclosures and real screenshots are strong; desktop seven-topic strip exceeds the four-choice guideline. |
| `GuideSteps`     | Correct ordered structure for real sequences; solid.                                                                        |
| `GuideBullets`   | Readable grouping without fake card scaffolding; solid.                                                                     |
| `GuideCallout`   | Icon, title, copy, and semantic kind avoid color-only meaning; solid.                                                       |
| `GuideLink`      | Clear contextual continuation; one rendered instance is only 24px high.                                                     |
| `GuideTerm`      | Excellent inline jargon translation.                                                                                        |
| `GuideTerms`     | Good chunking of definitions; solid.                                                                                        |
| `GuideEmptyIcon` | Unused on inspected routes; no user-visible issue.                                                                          |

### Achievements, lobby, and profile

| Component               | Specific critique                                                                                                                |
|-------------------------|----------------------------------------------------------------------------------------------------------------------------------|
| `AchievementCard`       | Good tier ladder and requirement disclosure; use H2-compatible grouping to repair the page heading tree.                         |
| `ActiveTableBanner`     | Excellent continuity cue and direct recovery; solid.                                                                             |
| `CreateRoomDialog`      | Clear stakes, seats, run-it-twice, validation, and radio behavior; 11 stakes need progressive disclosure.                        |
| `OnboardingIntro`       | Specific reassurance without a forced tutorial; strong.                                                                          |
| `StakesGrid`            | Excellent constraints and buy-in explanation; the 11-item rail needs recommended/common choices first.                           |
| `ProfileMenu`           | Useful identity/settings hub; density approaches a settings page inside a popover, so preserve sectioning and avoid adding more. |
| `ProfileShowcaseDialog` | Strong privacy and public-link rules; solid.                                                                                     |
| `ShowcaseEditor`        | Three-item constraint prevents over-selection and communicates visibility; solid.                                                |
| `SelfHudDialog`         | Detailed yet private; jargon is locally explained.                                                                               |
| `HudContent`            | Good separation of summary from detail; solid.                                                                                   |
| `PokerStyle`            | Radar is supplemental rather than sole encoding and is labeled; solid.                                                           |

### Hands and fairness

| Component            | Specific critique                                                                                            |
|----------------------|--------------------------------------------------------------------------------------------------------------|
| `ActionTimeline`     | Street grouping and secondary chat/reaction disclosure are clear; solid.                                     |
| `BoardSlots`         | Preserves undealt versus hidden states; mobile clipping should be visually signaled or removed.              |
| `OutcomeBadge`       | Text/icon/shape supplement color; solid.                                                                     |
| `DeckReveal`         | Excellent plain-language verification with optional technical detail.                                        |
| `PartialDeckProof`   | Correctly limits proof to revealed positions; solid.                                                         |
| `HandExportButton`   | Appropriate low-priority utility placement; solid.                                                           |
| `ShareHandDialog`    | Explicit public-data warning and expiry control; solid.                                                      |
| `HandReplayer`       | Strong media metaphor and step context; valid page needs H1, controls should meet the project’s 44px target. |
| `RevealWinnerButton` | Consentful reveal and already-revealed handling; solid.                                                      |

### Social and reactions

| Component                 | Specific critique                                                                                       |
|---------------------------|---------------------------------------------------------------------------------------------------------|
| `FriendCodeLookup`        | Exact-code explanation prevents privacy mistakes; copy/search feedback are strong.                      |
| `PeopleDrawer`            | Useful quick access; the full People page remains the clearer primary destination.                      |
| `PeopleList`              | Good empty hints, stale mode, retry, loading-more, and action grouping; ensure last row clears tab bar. |
| `PeopleNavBadge`          | Accessible alternative and consistent desktop/mobile placement; solid.                                  |
| `PlayerActionsMenu`       | Centralizes relationship, mute, block, report, and note behavior; strong consistency.                   |
| `ReportPlayerDialog`      | Evidence-aware reporting and explicit scope; solid.                                                     |
| `SocialInbox`             | Actionable invitations and useful chronology; ensure fixed-nav clearance.                               |
| `TableReactions`          | Principal component failure: 22 choices, duplicated favorites, timed context, and hostile labels.       |
| `ReactionFavoritesDialog` | Good scanning-reduction idea; favorites should become the default picker surface.                       |
| `ReactionStoreSection`    | Ownership/status model is clear, but selling hostile reactions compounds the tone problem.              |
| `ReactionPurchaseDialog`  | Excellent permanent-use, dual-payment, and price-source copy.                                           |
| `ReactionRefundDialog`    | Clear consequence and confirmation; solid.                                                              |

### Store

| Component                  | Specific critique                                                                       |
|----------------------------|-----------------------------------------------------------------------------------------|
| `DailyRewardPanel`         | Responsible, non-urgent reward framing; solid.                                          |
| `SkuGrid`                  | Clear total/base/bonus comparison and pending protection; solid.                        |
| `PurchaseModal`            | Good Pix lifecycle and expiration semantics.                                            |
| `PixPaymentView`           | QR, copy, countdown, and status model are clear; solid.                                 |
| `PurchaseHistoryList`      | Good resume/refund/status affordances; preserve bottom clearance.                       |
| `RefundConfirmationDialog` | Projected balance and consequence are unusually clear; solid.                           |
| `CosmeticStoreSection`     | Preview/ownership distinction works; catalog length contributes to the long page.       |
| `DeckStoreSection`         | Four-card previews are meaningful and compact; solid.                                   |
| `FeltStoreSection`         | Swatches map directly to a table material; solid.                                       |
| `CosmeticPurchaseDialog`   | Consistent payment vocabulary and clear entitlement; solid.                             |
| `CosmeticRefundDialog`     | Consistent consequence/reversal language; solid.                                        |
| `PurchaseActivityList`     | Good cross-department receipt grouping; ensure lower actions are not hidden by tab bar. |

### Table

| Component                | Specific critique                                                                                                                            |
|--------------------------|----------------------------------------------------------------------------------------------------------------------------------------------|
| `TableStage`             | Excellent dual-layout architecture and no document overflow; nine-seat caption lanes need a dedicated micro-layout.                          |
| `Seat`                   | Rich viewer/winner/fold/disconnect states and documented 44px pseudo-hit areas; visible labels fall too small/collide at 9-max phone widths. |
| `PlayingCard`            | Strong hidden-information semantics and variants; add static/live motion context so archives do not deal on mount.                           |
| `Board`                  | Clear pot, runout, and street composition; solid.                                                                                            |
| `ChipStack`              | Material/value cue is meaningful, not decoration; solid.                                                                                     |
| `PerimeterTimer`         | Integrated time state plus reduced-motion numeric fallback; strong.                                                                          |
| `ActionBar`              | Excellent hierarchy, shortcuts, pre-actions, disabled reasons, and mobile raise sheet.                                                       |
| `VoiceActionButton`      | Useful accelerator with explicit recognition semantics; solid.                                                                               |
| `BuyInPanel`             | Clear bounds, balance, no-debit promise, and auto-rebuy explanation.                                                                         |
| `RebuyDialog`            | Clear zero-stack recovery and opt-in behavior; solid.                                                                                        |
| `HandOutcome`            | Strong emotional peak with result, category, payout, and dismiss hierarchy.                                                                  |
| `WinnerCards`            | Correctly defers until the outcome is minimized; solid.                                                                                      |
| `RabbitHunt`             | Clearly post-game and non-outcome-changing; solid.                                                                                           |
| `Chat`                   | Good empty, disconnected, character-limit, and send states.                                                                                  |
| `TableUtilityMenu`       | Mobile utilities are appropriately constrained; solid.                                                                                       |
| `HandRankingsDialog`     | Useful in-context reference; solid.                                                                                                          |
| `InviteDialog`           | Clear link and share semantics; solid.                                                                                                       |
| `LeaveDialog`            | Excellent destructive-action prevention and reasoned disabled confirmation.                                                                  |
| `LastWinners`            | Useful secondary context and correctly behind More on mobile.                                                                                |
| `EquityTrainerPanel`     | Correct sandbox-only disclosure and disabled during the player’s decision.                                                                   |
| `TablePreferencesDialog` | Dense but justified grouping; do not add more without subnavigation.                                                                         |
| `RealityCheck`           | Neutral, deferrable, and never interrupts the player’s turn; excellent.                                                                      |
| `SessionRecap`           | Strong closure after leaving; solid.                                                                                                         |
| `BotChallenge`           | Necessary protection with explicit state; source-inspected branches are clear.                                                               |
| `PlayerNoteDialog`       | Private-note semantics and tag model are clear; document the six tag colors in the design system.                                            |
| `TodayHighlight`         | Locally evaluated category and readable player name make the highlight trustworthy.                                                          |
| `MockControls`           | Clearly development-only and visually separated; not a production concern.                                                                   |

## Minor Observations

- Mobile share/history boards need an edge fade or “deslize para ver” cue when locally scrollable.
- The compact mobile brand link is 38×44; a wider invisible hit zone would meet the documented 44×44 policy.
- Table header controls are 40×44: above WCAG’s 24px minimum but below the project target.
- Guide closed-disclosure collision reports were false positives; clipped descendants retained geometry but were not
  painted/clickable.
- Bottom hero-card overlaps are intentional physical composition, not accidental control collisions.
- Two layout animations should move to transform-based treatments: guide hover padding and achievement progress width.
- Six player-note tag colors and several 10px radii should be documented/tokenized; 2px playing-card radii are
  intentional.
- Next Image emitted LCP dev warnings for above-the-fold card backs/queen art on callback, replay, and table; review
  `priority`/preload only where the asset is genuinely LCP.
- Public profile, shared hand, replay, and hand detail use different shells. A small common public-artifact header
  vocabulary would improve family resemblance.

## Questions to Consider

- If the brand promises “friendly without becoming childish,” why can it sell throwing poop or a knife at another
  player?
- What are the three or four stakes most people actually use, and why must every creator scan eleven?
- Should a completed hand history behave like an archive and become readable immediately, or replay a live deal on
  mount?
- Is “Mais” the right active-location label for Guide, Ranking, Achievements, and Hands?
- What would invalid hand history look like if treated as a trust moment rather than a missing-parameter exception?
- Can the reaction surface open to three favorites, with the full catalog deliberately one level deeper?
