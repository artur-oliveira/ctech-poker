# The profile editor gets a route; the popover keeps its quick edits

Date: 2026-09-17 (revised the same day, see "Course correction" at the end)

## What was wrong

Editing your own profile lived entirely in two cramped surfaces:

- `components/lobby/ProfileMenu.tsx`, a 360px popover that edited the display name inline, uploaded
  and removed the avatar, switched wallet mode, and carried the whole deck picker (plus the
  `['wallet','cosmetic-catalog','deck']` read that fed it, latched on first open because it is
  mounted on every authenticated page).
- `components/lobby/ProfileShowcaseDialog.tsx`, a 448px modal holding three privacy switches, a
  checkbox list of every scored achievement, a collapsed `<details>` with the section-order editor,
  the share row, and the save button.

Neither had room for what it held, and the showcase's featured achievements were a bare checkbox
list of names: no card art, no value, no stars — nothing of the vocabulary the achievements page
had already taught the player.

Worse, `/profile?id=<your own id>` refused to render. The owner got a dead-end panel ("Esta é a sua
vitrine") whose only action re-opened the same modal, so nobody could actually see what they were
publishing.

## What replaced it

### `/player-profile`

`app/(app)/player-profile/` is where the player edits everything about themselves. The page is a
composition; each section owns its own reads and writes:

| File | Owns |
|---|---|
| `page.tsx` | chrome, the loading branch, and the balances block |
| `IdentitySection.tsx` | display name (`Field` + `Input` + explicit save) and the avatar upload/remove |
| `ShowcaseSection.tsx` | privacy switches, featured achievements, section order, one save, share/preview |
| `TableSection.tsx` | deck picker and, behind `REAL_MONEY_UI_ENABLED`, the wallet-mode switch |

Everything a section *writes* comes from `lib/hooks/useProfileEdits.ts`, and the controls the
popover also renders come from `components/profile/` — see "Course correction".

Layout is a settings sheet, not a stack of panels: hairline-separated sections, the heading and its
explanation in a left column, controls in the right, one column below 900px. See DESIGN.md
("Settings sheets").

**One save per resource.** Name, photo, deck and mode write immediately — each is one field and one
request. Privacy + highlights + section order are one resource (`POST /players/me` takes them
together), so they share one "Salvar vitrine" instead of three buttons over the same payload.

**The popover keeps the quick edits.** `ProfileMenu` still edits the display name inline, uploads
and removes the photo, and picks the deck, next to the identity readout, the balances and the links
to `/player-profile`, `/store`, the self-HUD dialog and logout. The route is a superset of it, not a
replacement (see "Course correction").

`ProfileShowcaseDialog.tsx` is deleted rather than left as a second entry point: the showcase is
exactly the part that needed a page, and nothing about it fits a 360px panel.

### The featured-achievement carousel

`components/achievements/FeaturedAchievementCarousel.tsx` plus
`components/achievements/AchievementCompactCard.tsx`.

`achievementSummaryView(achievement, count)` is the single derivation behind the compact card *and*
its accessible name, and it goes through the same `achievementProgress`, `achievementValueFormat`,
`achievementLabel` and `achievementExample` that `AchievementCard` uses — so the compact vocabulary
(cards, name, current value, X/Y stars) cannot drift from the catalogue's.

- The rail is a multi-select `listbox` with a roving tabindex (`orderedForCarousel` decides the
  order; the roving key is derived during render, never in an effect). Sixty-odd achievements as
  sixty-odd tab stops was the failure mode `AchievementCard` already fixed once (#116).
- Highlighting prepends the key, so the card jumps to the front of the rail — the same order the
  public showcase reads.
- The picker's data source is `useAchievementsSummary('sandbox', true)` alone. The old dialog also
  fetched `['achievements','catalog']` to get names and tiers; the summary already carries both, so
  the screen costs one read instead of two.

### Previewing your own showcase

`app/(app)/profile/page.tsx` now enables the showcase query for the viewer's own id. The endpoint
(`GET /players/:id/showcase`) has no viewer check: it returns the same payload a visitor gets, or
404s for exactly one reason — the showcase is not public.

- Own profile, no `preview`: an owner strip above the showcase with "Editar perfil" and "Ver como
  visitante". Everything below is already the visitor's view.
- `?preview=1`: the strip goes dashed gold and says so. It keeps "Sair da pré-visualização" and
  "Editar perfil" on purpose: the strip is the only thing on screen a visitor would not see, and
  without it leaving the preview would mean hand-editing the URL. Everything *below* the strip is
  the visitor's view, and that is what preview is for.
- Own profile, showcase private: "Vitrine privada" with the reason and a link to publish it, in
  both modes. The visitor's "este perfil não existe" copy would be a lie for the owner.

The matchup and relationship queries stay disabled for the viewer's own id (both 400 there), so
"Cara a Cara" simply does not render in preview.

## Missing achievement art

`sandbox_chips_earned` ("Montanha de Fichas") had no entry in `EXAMPLES`, so its card rendered with
a blank art slot — the illustrative playing cards *are* an achievement's icon. `real_money_earned`
had the same gap. Both now carry four-of-a-kind art (a stack, matching a counter whose metric is
chips rather than events), and `lib/achievements.test.ts` asserts that **every** key in
`ACHIEVEMENT_LABELS` has cards, a label and a description, so the next key cannot ship without them.

## Routing and crawl surface

`/player-profile` gets a `layout.tsx` with `routeMetadata({index: false})`, reusing the `profile` OG
capture, and is listed in `robots.ts`'s `PRIVATE_ROUTES`. It is session-gated, so there is nothing
there for a crawler.

## Copy

Player-facing copy on these screens no longer says "sandbox": the chip wallet is "Fichas", its mode
notification is "Modo fichas selecionado." The API value, the query keys and `WalletMode` are
unchanged — only the words on screen.

## The guide chapter

`app/(marketing)/guide/profile` documents both surfaces: the menu's three quick edits, the route's
four sections, the per-field saves, the carousel's pointer and keyboard behaviour, "Ver como
visitante" and "Copiar link". The stale reference in `guide/achievements` to the deleted
"Vitrine do perfil" dialog went with it.

Screenshots: `profile-live.webp` is captured from `/player-profile` itself instead of from the
popover opened over the lobby, and `profile-showcase.webp` is a new shot of the picker and the
section order. The whole guide set was re-captured, so no chrome still reads "sandbox".
`guideTopics.test.tsx` pins the route, the shipped button labels and the absence of the old
vocabulary, and asserts every topic page is free of "sandbox" and of em dashes.

## Course correction

The first cut of this change moved name, photo and deck *out* of the popover, leaving it a
read-only shortcut, and justified that as "one source of truth". That was the wrong trade: the ask
was a dedicated screen for the showcase, which the popover never served well, not the removal of
three edits the popover served fine. Opening a route to rename yourself is worse than what it
replaced, and the menu is one tap away from every authenticated page.

So the route is a **superset**. Both surfaces offer name, photo and deck; only the route offers the
showcase, the wallet mode and room to explain a field.

**One implementation, two surfaces.** Two copies of the same mutation is how this repo got thirteen
private BRL formatters, so nothing is duplicated:

| Module | Shared by both surfaces |
|---|---|
| `lib/hooks/useProfileEdits.ts` | `PLAYER_ME_KEY`, `useProfileNameSave` (trim + empty guard included), `useAvatarUpload`, `useAvatarRemove`, `useTablePreferenceSave` |
| `components/profile/ProfilePhoto.tsx` | `ProfilePhotoEditor` (avatar + camera + file input), `ProfilePhotoRemoveButton` |
| `components/profile/DeckPicker.tsx` | the whole deck setting: label, four-ace preview, variant list, premium locks, the write |

What is *not* shared is layout, and deliberately: the popover edits the name inline behind a pencil
(Enter saves, Escape cancels and is intercepted so it does not also close the menu), the route uses
a labelled `Field` with an explicit "Salvar nome". Different shapes, same write.

Coherence is structural rather than manual. Every mutation replaces `['player','me']` with the
server's response and neither surface keeps a mirrored profile — the only local state is the draft
being typed — so a save in the menu repaints the route's field and the other way round, with no
invalidation to keep in sync. `DeckPicker` generates its label id with `useId()`, because the menu
can be open on top of `/player-profile` and two pickers must not share one id.

**The bundle.** Restoring the picker naively puts `Select`, the variant catalogue and the cosmetic
catalog client into the chunk `AppPageChrome` pulls, which is every authenticated route. Measured
against `98f39ed`, a static import cost `/lobby` +57.4 kB, `/store` +55.1 kB, `/achievements`
+60.1 kB, `/profile` +59.6 kB, `/people` +57.4 kB and `/hands` +56.0 kB of first-load JS. With
`next/dynamic` those become +15.3, +19.2, +21.0, +21.3, +19.2 and +19.8 kB, and the deck catalogue
and `Select` are provably absent from `/lobby`'s first-load assets.

The residual is not the picker. Introducing one more async chunk group re-partitions Turbopack's
shared chunks app-wide: `/404` and `/unavailable`, which contain none of this code and were
byte-identical under the static import, also move by +4.0 and +7.1 kB. `ssr: false` is not the
cause (a build without it is byte-identical), and the config exposes no chunking knob. The budget
in `bundle-budget.json` is re-pinned in the same commit, per this repo's rule.
