# Table room background image (2026-10-13)

The room around the table was a flat wine→ink radial gradient. `.game` now paints a picture behind it.

- `public/table/room-landscape.webp` (1672×940) and `public/table/room-portrait.webp` (941×1672): a card room with a bar, a leather armchair and other tables out of focus. The centre is dark and empty so the game table can sit on it. Same-origin (`img-src 'self'`), WebP like the rest of `public/`.
- Tokens in `base.css`: `--table-room-image`, `--table-room-image-portrait`. `renderer.css` `.game` layers the image under a light wine→ink wash over `--table-room` (the fallback). `@media (orientation: portrait)` swaps the image and `cover` fits every viewport.
- The source images are 1672 px on the long side, so a 4K screen upscales them. They are dark and soft, so it is hardly visible; replace the two files with larger exports under the same names if better ones exist.
- No geometry, layout or animation change; the felt, rail and seats are untouched.
- Guide: `guide/table` "A mesa em cada tela" describes the room. The guide screenshots were not re-captured (needs `npm run dev:mock`, run by hand).
