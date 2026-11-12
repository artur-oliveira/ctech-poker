import {render, screen} from '@testing-library/react';
import {describe, expect, test} from 'vitest';
import {TABLE_REACTIONS} from '@/lib/reactions';
import {codepointFor, emojiSrc, EmojiGlyph} from './EmojiGlyph';
import {NOTO_EMOJI} from './emojiManifest';

describe('codepointFor', () => {
  test('hex-encodes a single-codepoint glyph', () => {
    expect(codepointFor('👏')).toBe('1f44f');
  });

  test('joins multi-codepoint glyphs (variation selectors) with a dash, Twemoji-style', () => {
    expect(codepointFor('🗡️')).toBe('1f5e1-fe0f');
  });
});

describe('manifest', () => {
  test('covers every reaction glyph except the chip, which draws its own stack', () => {
    const missing = Object.entries(TABLE_REACTIONS)
      .filter(([id]) => id !== 'chip')
      .filter(([, def]) => !NOTO_EMOJI[codepointFor(def.glyph)]);
    expect(missing).toEqual([]);
  });
});

describe('EmojiGlyph', () => {
  test('falls back to the raw character for a glyph with no bundled art', () => {
    render(<EmojiGlyph glyph="🟠"/>);
    const span = screen.getByText('🟠');
    expect(span.tagName).toBe('SPAN');
    expect(span).toHaveAttribute('aria-hidden', 'true');
  });

  test('renders the original static Noto by default and the animation only when asked', () => {
    expect(emojiSrc('👏')).toBe('/emoji/1f44f.svg');
    expect(emojiSrc('👏', true)).toBe('/emoji/1f44f.webp');
    render(<EmojiGlyph glyph="👏"/>);
    const img = document.querySelector('img')!;
    expect(img).toHaveAttribute('src', '/emoji/1f44f.svg');
    expect(img).toHaveAttribute('aria-hidden', 'true');
    expect(screen.queryByText('👏')).not.toBeInTheDocument();
  });

  test('the cucumber still is a frame of the animation: the flat Noto one is sliced', () => {
    expect(emojiSrc('🥒')).toBe('/emoji/1f952-still.webp');
    expect(emojiSrc('🥒', true)).toBe('/emoji/1f952.webp');
  });

  test('a glyph Google never animated uses the flat SVG for both weights', () => {
    expect(emojiSrc('🗡️')).toBe('/emoji/1f5e1-fe0f.svg');
    expect(emojiSrc('🗡️', true)).toBe('/emoji/1f5e1-fe0f.svg');
  });
});
