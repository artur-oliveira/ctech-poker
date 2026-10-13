# Table room background image (2026-10-13)

The room around the table was a flat wine→ink radial gradient. `.game` now paints a picture behind it.

- `public/table/room-landscape.webp` (1672×940) and `public/table/room-portrait.webp` (941×1672): a card room with a bar, a leather armchair and other tables out of focus. The centre is dark and empty so the game table can sit on it. Same-origin (`img-src 'self'`), WebP like the rest of `public/`.
- One token in `base.css`, `--table-room-picture`, a complete `background` layer swapped on `:root` by `@media (orientation: portrait)`; `cover` fits every viewport.
- Surfaces: `.game` (`renderer.css`) puts it over the original wine→ink gradient, and `.replay-live-table` (`app.css`) over `--table-room`. The replay mounts the same `TableStage` but not the `.game` shell, which is why it had no picture at first; any new table surface must use the token too.
- **A failed fetch is not an error.** The picture is the top layer; when it does not load that layer paints nothing and the one beneath is the previous background, unchanged. There is no script, no `onerror` and no state. This is a CSS behaviour, so jsdom cannot test it; it was checked in Chromium with a 404 URL.
- `.game-chrome > header` gets a scrim (a gradient from `--table-room`) because the picture has lit shelves and lamps exactly where the controls are.
- The source images are 1672 px on the long side, so a 4K screen upscales them. They are dark and soft, so it is hardly visible; replace the two files with larger exports under the same names if better ones exist.
- No geometry, layout or animation change; the felt, rail and seats are untouched.
- Guide: `guide/table` "A mesa em cada tela" describes the room. The guide screenshots were not re-captured (needs `npm run dev:mock`, run by hand).
