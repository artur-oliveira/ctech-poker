# Cosmetic catalogs load on open, not on mount (#232)

`ProfileMenu` is mounted on every authenticated page and `TablePreferencesDialog` on every table,
yet the only thing either of them needs the wallet's cosmetic catalog for is the picker *inside*
the popover/dialog. Both queries ran on mount, so every visit paid a deck-catalog read (lobby,
people, hands, store, profile, …) and every table paid the felt one, for a control most players
never open.

## What changed

- `ProfileMenu`: the deck catalog query is `enabled` only after the popover has been opened once.
  The popover's existing `onOpenChange` handler sets that latch.
- `TablePreferencesDialog`: same for the felt catalog. The dialog can be opened either by its own
  trigger or by a parent that controls `open` (the table page does), so the latch is set from
  `onOpenChange` **and** during render when a controlled `open` arrives.

The latch is one-way on purpose: it turns the query on at the first open and leaves it on, so
React Query keeps the entry cached and re-opening the control is instant, with no second request
while the data is fresh.

No prefetch on hover/focus was added — the catalogs are small and, after the first open, every
later open renders from cache.

## The beat before the catalog lands

Deferring the fetch moves a (short) unknown-ownership window inside the open control, so both
pickers now say so honestly instead of guessing. While the catalog is still in flight a premium
felt/deck is neither labelled `Premium bloqueado` nor turned into a store link — it renders in the
Select's existing disabled state, so a player who owns the item never sees a padlock on it and is
never routed to the store for something they already bought. A *failed* request is not "still
loading": it falls back to the locked treatment, which at least offers the store.

## Guarantees under test

- `ProfileMenu.test.tsx`: the deck catalog key is not read before the menu opens, and is read after.
  The suite's `useQuery` mock now honours `enabled: false` so the assertion measures a real request
  budget instead of a hook call.
- `TablePreferencesDialog.test.tsx`: `listCosmeticCatalog` is never called while the dialog is
  closed. The other cases render with `open`, since the suite's `Dialog` mock always renders its
  content.
