# Table room background image (2026-09-30)

The room around the table was a flat wine→ink radial gradient. `.game` now paints a real image behind it.

- `public/table/room-landscape.svg` (16:9) and `public/table/room-portrait.svg` (9:16): panelled cardroom, warm pendant glow, bokeh, vignette. Static SVG, ~5 kB each, same-origin (`img-src 'self'`), no runtime cost.
- Tokens in `base.css`: `--table-room-image`, `--table-room-image-portrait`. `renderer.css` `.game` layers the image under a translucent copy of the old gradient (contrast for seat chrome is unchanged) over `--table-room`, the fallback. `@media (orientation: portrait)` swaps the image; `cover` fits every viewport.
- No geometry, layout or animation change; the felt, rail and seats are untouched.
- Guide: `guide/table` "A mesa em cada tela" mentions the room. No screenshot was re-captured (needs `npm run dev:mock`, run by hand).
