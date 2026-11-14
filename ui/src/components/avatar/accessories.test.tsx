import {describe, expect, test} from 'vitest';
import {AVATAR_ITEMS} from '@/lib/avatarCosmetics';
import {checkArt} from '@/test/avatarArt';
import {ACCESSORY_ART} from './accessories';

const ACCESSORY_IDS = Object.keys(AVATAR_ITEMS.accessory);

// Eye line by default; the mustache sits on the mouth line; the headset's band
// arcs over the head (hat zone) down to an ear cup on the eye line.
const BANDS: Record<string, [number, number]> = {mustache: [40, 48], headset: [0, 40]};

describe('ACCESSORY_ART', () => {
  test('TestEveryAccessoryIdHasArt', () => {
    expect(ACCESSORY_IDS).toHaveLength(5);
    expect(Object.keys(ACCESSORY_ART).sort()).toEqual([...ACCESSORY_IDS].sort());
  });

  test.each(ACCESSORY_IDS)('TestAccessoryArtContract %s', (id) => {
    const {band, lo, hi} = checkArt(ACCESSORY_ART[id]);
    const [min, max] = BANDS[id] ?? [24, 40];
    expect(band).toBe('accessory');
    expect(lo).toBeGreaterThanOrEqual(min);
    expect(hi).toBeLessThanOrEqual(max);
  });
});
