# Noto Emoji (bundled reaction art)

`ui/public/emoji/` holds artwork from Google's Noto Emoji, so reactions look identical on every device.

| Files | Source | License |
|---|---|---|
| `*.webp` (animated), `1f952-still.webp` | [Noto Emoji Animation](https://googlefonts.github.io/noto-emoji-animation/), re-encoded to 160px; the still is frame 0 of the cucumber only | CC BY 4.0, https://creativecommons.org/licenses/by/4.0/ |
| `*.svg` (the stills of every animated glyph, plus 🧲 🦆 🗡️ 🔦 🩹 which Google never animated) | [googlefonts/noto-emoji](https://github.com/googlefonts/noto-emoji) | Apache 2.0 |

Attribution shown to players: the marketing footer and the app footer (`NOTO_CREDIT` in `ui/src/lib/reactions.ts`).
Changes made: resized and re-encoded; stills are a single frame. Regenerate with `node ui/scripts/fetch-noto-emoji.mjs`.
