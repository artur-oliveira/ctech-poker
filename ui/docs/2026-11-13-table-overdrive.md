# Table overdrive (2026-11-13)

Seats, table dialogs, reaction shortcuts and a few route fixes, in one change.

## What changed

- **Seat.** The 3-dot badge is gone; the player's photo is the button that opens the details popover
  (profile, friendship, note, mute, block, report), on desktop and touch. Hover shows a gold ring, open keeps it,
  keyboard focus uses the focus-ring token. Photos are no longer draggable or saveable (`AvatarImage`).
- **Preferências da mesa.** Two columns on desktop ("Mesa e jogo", "Som, voz e atalhos"), one card
  anatomy for every row, about 610px tall so it fits without scrolling; one column on phones.
- **Atalhos de reação.** Editor in two columns (self | thrown), up to 3 each, each selected reaction shows its key.
  API: `favorite_reactions` now accepts up to 6, at most 3 per kind (legacy lists of up to 3 stay valid).
- **Quick bar.** On desktop (fine pointer, 900px+) the favorites sit in a bar beside the emoji button. Keys 3-5 fire
  self reactions, 6-8 arm a thrown one (then pick the player). Locked premiums never fire from a key.
- **/people** no longer shifts sideways: pages reserve the scrollbar lane (`scrollbar-gutter: stable` when `.app-page`
  exists; the table keeps full width).
- **Loja.** Directory has a full outline with dividers only between cells; a claimable daily reward adds a
  "Pegar recompensa" banner at the top that scrolls and focuses the reward panel; the streak panel is compact on phones.

## Decisions

- Favorites stay one flat list on the server; the kind split is derived from the catalogue (`targeted`), so there is
  no migration and no new field.
- Keys 3-8 were free (1/2 peek, F C P R H A X act, E/T open panels). They are ignored while typing (`isPlainKey`).
- The banner reads the same `['dailyReward','cooldown']` query as the panel: no extra request.

## Not done

- Seat anatomy (name/stack/state on mobile, badge clutter) was reviewed read-only; the findings are a follow-up,
  not part of this change. A pass on a real handset is still required before merge (CLAUDE.md, table geometry).

## Follow-up: seat anatomy (2026-11-13)

- Ring seats (portrait phone) show the player's name again, one 9px line above the stack, in paper with a soft shadow.
  It reverses the earlier "name lives in the avatar initials" call (#9), because a photo avatar has no initials and
  a caption of only "5K" left the table anonymous. Below 620px of viewport height (heads-up at 320x568 pushed the top
  caption 2px past the stage) the name goes back to screen-reader-only.
- Ring role/streak badges move from 8px to the 9px table floor. The avatar button sits under them (z-index 3 vs 4);
  at 5 it hid the SB/D badges that straddle the avatar's corner.
- Desktop: avatar 28px to 36px, name at the compact size in paper above the gold stack.
- The state word stays hidden on touch on purpose: all-in, folded and disconnected already carry non-text cues
  (see the touch block near `(pointer: coarse)` in renderer.css). Not changed.
- Verified with the layout suite in Chromium and Firefox. WebKit does not launch on the dev machine
  (`libWPEWebKit` symbol error), so it is left to CI.
