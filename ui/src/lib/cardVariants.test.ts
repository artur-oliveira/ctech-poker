import {describe, expect, test} from 'vitest';
import {DECK_VARIANTS, DEFAULT_DECK_VARIANT, type DeckVariantId, PREMIUM_DECK_IDS} from './cardVariants';

describe('cardVariants', () => {
  test('drops the removed dark variant', () => {
    expect(Object.keys(DECK_VARIANTS)).not.toContain('dark');
  });

  test('adds the new golden, pink and alt decks with real color entries', () => {
    for (const id of ['golden', 'pink', 'alt'] as DeckVariantId[]) {
      expect(DECK_VARIANTS[id]).toBeDefined();
      expect(DECK_VARIANTS[id].colors.spade).toMatch(/^#[0-9A-Fa-f]{6}$/);
      expect(DECK_VARIANTS[id].colors.heart).toMatch(/^#[0-9A-Fa-f]{6}$/);
      expect(DECK_VARIANTS[id].colors.diamond).toMatch(/^#[0-9A-Fa-f]{6}$/);
      expect(DECK_VARIANTS[id].colors.club).toMatch(/^#[0-9A-Fa-f]{6}$/);
    }
  });

  test('classifies casino, bicycle, vintage and the three new decks as premium', () => {
    expect(PREMIUM_DECK_IDS).toEqual(new Set(['casino', 'bicycle', 'vintage', 'golden', 'pink', 'alt']));
  });

  test('keeps accessibility/free decks out of the premium set', () => {
    for (const id of ['four-color', 'two-color', 'colorblind', 'high-constrast'] as DeckVariantId[]) {
      expect(PREMIUM_DECK_IDS.has(id)).toBe(false);
    }
  });

  test('four-color stays the default deck', () => {
    expect(DEFAULT_DECK_VARIANT).toBe('four-color');
  });
});
