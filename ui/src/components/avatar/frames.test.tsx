import {render} from '@testing-library/react';
import {describe, expect, test} from 'vitest';
import {AVATAR_ITEMS} from '@/lib/avatarCosmetics';
import {FRAME_ART} from './frames';

const FRAME_IDS = Object.keys(AVATAR_ITEMS.frame);
const TOKEN = /^var\(--cosmetic-[a-z-]+\)$/;

describe('FRAME_ART', () => {
  test('TestEveryFrameIdHasArt', () => {
    expect(FRAME_IDS).toHaveLength(14);
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
    // Every paint is a cosmetic token, or a gradient in this same svg whose stops are tokens.
    const ids = new Set([...svg!.querySelectorAll('[id]')].map(el => el.id));
    for (const el of svg!.querySelectorAll('[stroke],[fill]')) {
      for (const attr of ['stroke', 'fill']) {
        const v = el.getAttribute(attr);
        if (!v || v === 'none') continue;
        const ref = v.match(/^url\(#(.+)\)$/);
        if (ref) expect(ids.has(ref[1])).toBe(true);
        else expect(v).toMatch(TOKEN);
      }
    }
    for (const stop of svg!.querySelectorAll('stop')) expect(stop.getAttribute('stop-color')).toMatch(TOKEN);
  });

  test('TestFrameGradientIdsAreUniquePerInstance', () => {
    // The same frame renders on many seats at once; a repeated id breaks url(#id) once one copy is hidden.
    const {container} = render(<>{FRAME_IDS.flatMap(id => {
      const Art = FRAME_ART[id];
      return [<Art key={`${id}-a`}/>, <Art key={`${id}-b`}/>];
    })}</>);
    const ids = [...container.querySelectorAll('[id]')].map(el => el.id);
    expect(ids.length).toBeGreaterThan(0);
    expect(new Set(ids).size).toBe(ids.length);
  });
});
