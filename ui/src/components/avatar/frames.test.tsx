import {render} from '@testing-library/react';
import {describe, expect, test} from 'vitest';
import {AVATAR_ITEMS} from '@/lib/avatarCosmetics';
import {FRAME_ART} from './frames';

const FRAME_IDS = Object.keys(AVATAR_ITEMS.frame);
const TOKEN = /^var\(--cosmetic-[a-z-]+\)$/;
const ANIMATED = ['inferno', 'arcane-aura', 'storm-ring', 'gold-shimmer'];

describe('FRAME_ART', () => {
  test('TestEveryFrameIdHasArt', () => {
    expect(FRAME_IDS).toHaveLength(22);
    expect(Object.keys(FRAME_ART).sort()).toEqual([...FRAME_IDS].sort());
  });

  test.each(FRAME_IDS)('TestFrameSvgContract %s', (id) => {
    const Art = FRAME_ART[id];
    const {container} = render(<Art/>);
    // A static frame is one svg; an animated one adds whole-svg Fx layers on the same box.
    const svgs = [...container.children];
    expect(svgs.length).toBeGreaterThan(0);
    for (const svg of svgs) {
      expect(svg.tagName).toBe('svg');
      expect(svg.getAttribute('viewBox')).toBe('0 0 64 64');
      expect(svg.getAttribute('aria-hidden')).toBe('true');
      expect(svg.getAttribute('focusable')).toBe('false');
    }
    const markup = container.innerHTML;
    expect(markup).not.toMatch(/#[0-9a-f]{3,8}\b/i);
    expect(markup).not.toMatch(/outline|filter|<animate/i);
    // Every paint is a cosmetic token, or a gradient in this frame whose stops are tokens.
    const ids = new Set([...container.querySelectorAll('[id]')].map(el => el.id));
    for (const el of container.querySelectorAll('[stroke],[fill]')) {
      for (const attr of ['stroke', 'fill']) {
        const v = el.getAttribute(attr);
        if (!v || v === 'none') continue;
        const ref = v.match(/^url\(#(.+)\)$/);
        if (ref) expect(ids.has(ref[1])).toBe(true);
        else expect(v).toMatch(TOKEN);
      }
    }
    for (const stop of container.querySelectorAll('stop')) expect(stop.getAttribute('stop-color')).toMatch(TOKEN);
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

  test('TestOnlyTheAnimatedFramesHaveFxLayers', () => {
    const fx = (id: string) => {
      const Art = FRAME_ART[id];
      const {container, unmount} = render(<Art/>);
      const layers = [...container.querySelectorAll('svg[data-fx]')].map(el => el.getAttribute('data-fx'));
      unmount();
      return layers;
    };
    expect(Object.fromEntries(ANIMATED.map(id => [id, fx(id)]))).toEqual({
      'inferno': ['flame', 'flame-core'],
      'arcane-aura': ['breathe', 'orbit', 'orbit-back'],
      'storm-ring': ['arc', 'arc-back'],
      'gold-shimmer': ['sweep', 'twinkle', 'twinkle-late'],
    });
    for (const id of FRAME_IDS.filter(id => !ANIMATED.includes(id))) expect(fx(id), id).toEqual([]);
  });
});
