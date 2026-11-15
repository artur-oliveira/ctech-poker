import {describe, expect, test} from 'vitest';
import {AVATAR_ITEMS} from '@/lib/avatarCosmetics';
import {render} from '@testing-library/react';
import {checkArt, elementBox} from '@/test/avatarArt';
import {ACCESSORY_ART} from './accessories';

const ACCESSORY_IDS = Object.keys(AVATAR_ITEMS.accessory);

// Eye line by default; the mustaches and the cigar sit on the mouth line; the headset's band
// arcs over the head (hat zone) down to an ear cup on the eye line.
const BANDS: Record<string, [number, number]> = {mustache: [40, 48], handlebar: [40, 48], cigar: [40, 48], headset: [0, 40]};

describe('ACCESSORY_ART', () => {
  test('TestEveryAccessoryIdHasArt', () => {
    expect(ACCESSORY_IDS).toHaveLength(10);
    expect(Object.keys(ACCESSORY_ART).sort()).toEqual([...ACCESSORY_IDS].sort());
  });

  test.each(ACCESSORY_IDS)('TestAccessoryArtContract %s', (id) => {
    const {band, lo, hi} = checkArt(ACCESSORY_ART[id]);
    const [min, max] = BANDS[id] ?? [24, 40];
    expect(band).toBe('accessory');
    expect(lo).toBeGreaterThanOrEqual(min);
    expect(hi).toBeLessThanOrEqual(max);
  });

  test('TestHeadsetPiecesStayInTheirZones', () => {
    // Every shape sits wholly in the hat zone (band, y 0-26) or on the ear line (cup and mic, y 24-40).
    const Headset = ACCESSORY_ART.headset;
    const {container} = render(<Headset/>);
    for (const el of container.querySelectorAll('svg *')) {
      const {y0, y1} = elementBox(el);
      expect(y1 <= 26 || y0 >= 24, `${el.outerHTML} spans y ${y0}-${y1}`).toBe(true);
    }
  });
});
