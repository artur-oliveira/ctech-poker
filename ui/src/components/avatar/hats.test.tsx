import {describe, expect, test} from 'vitest';
import {AVATAR_ITEMS} from '@/lib/avatarCosmetics';
import {checkArt} from '@/test/avatarArt';
import {HAT_ART} from './hats';

const HAT_IDS = Object.keys(AVATAR_ITEMS.hat);

describe('HAT_ART', () => {
  test('TestEveryHatIdHasArt', () => {
    expect(HAT_IDS).toHaveLength(12);
    expect(Object.keys(HAT_ART).sort()).toEqual([...HAT_IDS].sort());
  });

  test.each(HAT_IDS)('TestHatArtContract %s', (id) => {
    const {band, lo, hi} = checkArt(HAT_ART[id]);
    expect(band).toBe('hat');
    expect(lo).toBeGreaterThanOrEqual(0);
    expect(hi).toBeLessThanOrEqual(26);
  });
});
