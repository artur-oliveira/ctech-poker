import {describe, expect, test} from 'vitest';
import {AVATAR_ITEMS} from '@/lib/avatarCosmetics';
import {BADGE_ART, BADGE_TONE} from '@/components/avatar/badges';

const ids = Object.keys(AVATAR_ITEMS.badge).sort();
const TOKENS = ['gold', 'gold-pale', 'oxblood', 'felt', 'walnut', 'paper', 'ink', 'wine'];

describe('badges', () => {
  test('TestEveryBadgeIdHasGlyph', () => {
    expect(Object.keys(BADGE_ART).sort()).toEqual(ids);
  });
  test('every badge has a tone made of valid tokens', () => {
    expect(Object.keys(BADGE_TONE).sort()).toEqual(ids);
    for (const {bg, fg} of Object.values(BADGE_TONE)) {
      expect(TOKENS).toContain(bg);
      expect(TOKENS).toContain(fg);
    }
  });
});
