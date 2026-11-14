import {render} from '@testing-library/react';
import {describe, expect, test} from 'vitest';
import {AVATAR_ITEMS} from '@/lib/avatarCosmetics';
import {FRAME_ART} from './frames';

const FRAME_IDS = Object.keys(AVATAR_ITEMS.frame);

describe('FRAME_ART', () => {
  test('TestEveryFrameIdHasArt', () => {
    expect(FRAME_IDS).toHaveLength(6);
    expect(Object.keys(FRAME_ART).sort()).toEqual([...FRAME_IDS].sort());
  });

  test.each(FRAME_IDS)('TestFrameSvgContract %s', (id) => {
    const Art = FRAME_ART[id];
    const {container} = render(<Art/>);
    const svg = container.querySelector('svg');
    expect(svg).not.toBeNull();
    expect(svg!.getAttribute('viewBox')).toBe('0 0 64 64');
    expect(svg!.getAttribute('aria-hidden')).toBe('true');
    expect(svg!.getAttribute('focusable')).toBe('false');
    const markup = container.innerHTML;
    expect(markup).not.toMatch(/#[0-9a-f]{3,8}\b/i);
    expect(markup).not.toMatch(/outline/i);
    // Every paint comes from a cosmetic token.
    for (const el of svg!.querySelectorAll('[stroke],[fill]')) {
      for (const attr of ['stroke', 'fill']) {
        const v = el.getAttribute(attr);
        if (v && v !== 'none') expect(v).toMatch(/^var\(--cosmetic-[a-z-]+\)$/);
      }
    }
  });
});
