# Reactions use bundled Noto emoji

OS emoji fonts drew the 30 reactions differently per device. `EmojiGlyph` now renders bundled Google Noto art
(`public/emoji`, listed in the generated `components/ui/emojiManifest.ts`).

- **Two weights.** Default is the original static Noto (flat SVG, a few KB; not a frame of the animation, which is
  often a meaningless mid-motion pose) for the picker, store, history and dialogs. The one exception is the
  cucumber: the flat Noto 🥒 is the sliced one, so its still is frame 0 of the animation (whole). `animated` swaps in the 160px animated WebP and is passed only by the thrown/emote
  projectile in `TableReactions`. Under `prefers-reduced-motion` the still is always used.
- **Five glyphs have no animated Noto** (🧲 🦆 🗡️ 🔦 🩹): one flat Noto SVG serves both weights. `chip` keeps its
  own CSS chip stack.
- **A glyph missing from the manifest falls back to the raw character**; `EmojiGlyph.test.tsx` fails if a
  reaction is added without art.
- **License:** animated Noto is CC BY 4.0. Credit is in both footers and `LICENSES/NOTO.md`.
- **Regenerate:** `node scripts/fetch-noto-emoji.mjs` by hand; output is committed (like `og:capture`).
