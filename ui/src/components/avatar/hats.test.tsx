import {describe, expect, test} from 'vitest';
import {AVATAR_ITEMS} from '@/lib/avatarCosmetics';
import {checkArt, closestToCentre} from '@/test/avatarArt';
import {HAT_ART, HAT_VIEWBOX} from './hats';

const HAT_IDS = Object.keys(AVATAR_ITEMS.hat);
const [VX, VY, VW, VH] = HAT_VIEWBOX;

describe('HAT_ART', () => {
  test('TestEveryHatIdHasArt', () => {
    expect(HAT_IDS).toHaveLength(12);
    expect(Object.keys(HAT_ART).sort()).toEqual([...HAT_IDS].sort());
  });

  test('TestHatViewBoxRisesAboveTheAvatar', () => {
    // Same 64 units wide as the avatar box; at most 0.35 of it above the box.
    expect([VX, VW]).toEqual([0, 64]);
    expect(-VY).toBeLessThanOrEqual(0.35 * 64);
    expect(VY + VH).toBeGreaterThan(0);
  });

  test.each(HAT_IDS)('TestHatSitsOnTopOfTheAvatar %s', (id) => {
    const {svg, shapes, box} = checkArt(HAT_ART[id]);
    expect(svg.getAttribute('data-band')).toBe('hat');
    expect(svg.getAttribute('viewBox')).toBe(HAT_VIEWBOX.join(' '));
    // Nothing leaves the hat's viewBox.
    expect(box.x0).toBeGreaterThanOrEqual(VX);
    expect(box.x1).toBeLessThanOrEqual(VX + VW);
    expect(box.y0).toBeGreaterThanOrEqual(VY);
    expect(box.y1).toBeLessThanOrEqual(VY + VH);
    // The hat rises above the box and rests on the circle (reaches the ring, r<=32)...
    expect(box.y0).toBeLessThan(-6);
    expect(Math.min(...shapes.map(closestToCentre))).toBeLessThanOrEqual(32);
    // ...but never paints over the photo: everything stays outside r=28.5.
    for (const el of shapes) expect(closestToCentre(el), el.outerHTML.slice(0, 80)).toBeGreaterThanOrEqual(28.5);
  });
});
