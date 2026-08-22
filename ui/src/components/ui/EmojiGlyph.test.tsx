import {render, screen} from '@testing-library/react';
import {afterEach, describe, expect, test} from 'vitest';
import {BUNDLED_EMOJI_CODEPOINTS, codepointFor, EmojiGlyph} from './EmojiGlyph';

describe('codepointFor', () => {
  test('hex-encodes a single-codepoint glyph', () => {
    expect(codepointFor('👏')).toBe('1f44f');
  });

  test('joins multi-codepoint glyphs (variation selectors) with a dash, Twemoji-style', () => {
    expect(codepointFor('🗡️')).toBe('1f5e1-fe0f');
  });
});

describe('EmojiGlyph', () => {
  afterEach(() => BUNDLED_EMOJI_CODEPOINTS.clear());

  test('renders the raw character in a fallback span when no bundled asset exists', () => {
    render(<EmojiGlyph glyph="👏"/>);
    const span = screen.getByText('👏');
    expect(span.tagName).toBe('SPAN');
    expect(span).toHaveAttribute('aria-hidden', 'true');
  });

  test('renders the bundled SVG asset for a glyph whose codepoint is in the manifest', () => {
    BUNDLED_EMOJI_CODEPOINTS.add(codepointFor('👏'));
    render(<EmojiGlyph glyph="👏"/>);
    const img = document.querySelector('img')!;
    expect(img).toHaveAttribute('src', '/emoji/1f44f.svg');
    expect(img).toHaveAttribute('aria-hidden', 'true');
    expect(screen.queryByText('👏')).not.toBeInTheDocument();
  });
});
