// Bundled-asset emoji renderer: maps a reaction glyph to a local SVG under
// /public/emoji/<codepoint>.svg so every player sees identical artwork
// instead of whatever their OS's font happens to draw. BUNDLED_EMOJI_CODEPOINTS
// is empty today (asset sourcing/licensing is a separate task) — every glyph
// currently falls back to rendering the raw Unicode character, unchanged.
import Image from 'next/image';

export function codepointFor(glyph: string) {
  return Array.from(glyph).map(char => char.codePointAt(0)!.toString(16)).join('-');
}

export const BUNDLED_EMOJI_CODEPOINTS = new Set<string>([]);

export function EmojiGlyph({glyph}: { glyph: string }) {
  const codepoint = codepointFor(glyph);
  return BUNDLED_EMOJI_CODEPOINTS.has(codepoint)
    ? <Image src={`/emoji/${codepoint}.svg`} alt="" aria-hidden="true" width={20} height={20}/>
    : <span aria-hidden="true">{glyph}</span>;
}
