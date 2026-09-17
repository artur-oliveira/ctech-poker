# The profile editor is a route, not a popover and a modal

Date: 2026-09-17

## What was wrong

Editing your own profile was split across two cramped surfaces:

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

`app/(app)/player-profile/` is the one place the player edits themselves. The page is a
composition; each section owns its own reads and writes:

| File | Owns |
|---|---|
| `page.tsx` | chrome, the loading branch, and the balances block |
| `IdentitySection.tsx` | display name (`Field` + `Input` + explicit save) and the avatar upload/remove |
| `ShowcaseSection.tsx` | privacy switches, featured achievements, section order, one save, share/preview |
| `TableSection.tsx` | deck picker and, behind `REAL_MONEY_UI_ENABLED`, the wallet-mode switch |

Layout is a settings sheet, not a stack of panels: hairline-separated sections, the heading and its
explanation in a left column, controls in the right, one column below 900px. See DESIGN.md
("Settings sheets").

**One save per resource.** Name, photo, deck and mode write immediately — each is one field and one
request. Privacy + highlights + section order are one resource (`POST /players/me` takes them
together), so they share one "Salvar vitrine" instead of three buttons over the same payload.

**The popover is now only a shortcut.** `ProfileMenu` reads `['player','me']` and nothing else: the
deck catalog read left with the picker. It displays the name, the visibility state and the balances,
and links to `/player-profile`, `/store`, the self-HUD dialog and logout. No field has two editors.

`ProfileShowcaseDialog.tsx` is deleted rather than left as a second entry point.

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

`app/(marketing)/guide/profile` now documents `/player-profile`: the four sections, the per-field
saves, the carousel's pointer and keyboard behaviour, "Ver como visitante" and "Copiar link". The two
stale references outside that page went with it — `guide/achievements` pointed at the deleted
"Vitrine do perfil" dialog, and `guide/store` said an unlocked deck shows up in the profile popover.

Screenshots: `profile-live.webp` is captured from `/player-profile` itself instead of from the
popover opened over the lobby, and `profile-showcase.webp` is a new shot of the picker and the
section order. The whole guide set was re-captured, so no chrome still reads "sandbox".
`guideTopics.test.tsx` pins the route, the shipped button labels and the absence of the old
vocabulary, and asserts every topic page is free of "sandbox" and of em dashes.
