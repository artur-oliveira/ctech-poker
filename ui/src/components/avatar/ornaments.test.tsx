import {describe, expect, test} from 'vitest';
import {AVATAR_ITEMS} from '@/lib/avatarCosmetics';
import {checkArt, closestToCentre} from '@/test/avatarArt';
import {HORNS_VIEWBOX, ORNAMENT_ART, ORNAMENTS, WINGS_VIEWBOX} from './ornaments';

const IDS = Object.keys(AVATAR_ITEMS.accessory);
const band = (b: string) => IDS.filter(id => ORNAMENTS[id].band === b);
const within = (box: { x0: number; x1: number; y0: number; y1: number }, [vx, vy, vw, vh]: readonly number[]) => {
  expect(box.x0).toBeGreaterThanOrEqual(vx);
  expect(box.x1).toBeLessThanOrEqual(vx + vw);
  expect(box.y0).toBeGreaterThanOrEqual(vy);
  expect(box.y1).toBeLessThanOrEqual(vy + vh);
};

describe('ORNAMENTS', () => {
  test('TestEveryOrnamentIdHasArt', () => {
    expect(IDS).toHaveLength(7);
    expect(Object.keys(ORNAMENTS).sort()).toEqual([...IDS].sort());
    expect(Object.keys(ORNAMENT_ART).sort()).toEqual([...IDS].sort());
    expect(band('wings')).toEqual(['angel-wings', 'bat-wings', 'gold-wings', 'dragon-wings']);
    expect(band('horns')).toEqual(['devil-horns', 'ram-horns', 'bull-horns']);
  });

  test.each(band('wings'))('TestWingsSpreadBehindTheAvatar %s', (id) => {
    const {svg, box} = checkArt(ORNAMENT_ART[id]);
    expect(svg.getAttribute('data-band')).toBe('wings');
    expect(svg.getAttribute('viewBox')).toBe(WINGS_VIEWBOX.join(' '));
    within(box, WINGS_VIEWBOX);
    // Both sides spread well past the avatar box, and the root hides behind the photo.
    expect(box.x0).toBeLessThan(-12);
    expect(box.x1).toBeGreaterThan(76);
    expect(box.x0 + box.x1).toBeCloseTo(64, 0);
  });

  test.each(band('horns'))('TestHornsSitOnTopOfTheAvatar %s', (id) => {
    const {svg, shapes, box} = checkArt(ORNAMENT_ART[id]);
    expect(svg.getAttribute('data-band')).toBe('horns');
    expect(svg.getAttribute('viewBox')).toBe(HORNS_VIEWBOX.join(' '));
    within(box, HORNS_VIEWBOX);
    expect(box.y0).toBeLessThan(-6);
    // Rooted on the ring (r<=32), never over the photo (r>=28.5).
    expect(Math.min(...shapes.map(closestToCentre))).toBeLessThanOrEqual(32);
    for (const el of shapes) expect(closestToCentre(el), el.outerHTML.slice(0, 80)).toBeGreaterThanOrEqual(28.5);
  });
});
