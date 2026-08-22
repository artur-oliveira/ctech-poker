import {describe, expect, test} from 'vitest';
import {back, cardLabel, cardPath} from './cards';
import type {DeckVariantId} from './cardVariants';

describe('cardPath', () => {
  test('builds the variant path for a valid card', () => {
    expect(cardPath('As', 'four-color')).toBe('/svgs/variants/four-color/spade-ace.svg');
  });

  test('falls back to the default deck variant for an unknown/removed id instead of a 404 path', () => {
    expect(cardPath('As', 'dark' as DeckVariantId)).toBe('/svgs/variants/four-color/spade-ace.svg');
    expect(cardPath('As', 'not-a-deck' as DeckVariantId)).toBe('/svgs/variants/four-color/spade-ace.svg');
  });

  test('resolves the new premium deck variants', () => {
    expect(cardPath('Th', 'golden')).toBe('/svgs/variants/golden/heart-10.svg');
    expect(cardPath('Th', 'pink')).toBe('/svgs/variants/pink/heart-10.svg');
    expect(cardPath('Th', 'alt')).toBe('/svgs/variants/alt/heart-10.svg');
  });

  test('returns the card back for an unparseable card', () => {
    expect(cardPath('')).toBe(back);
    expect(cardPath('zz')).toBe(back);
  });
});

describe('cardLabel', () => {
  test('labels a known card in Portuguese', () => {
    expect(cardLabel('As')).toBe('ás de espadas');
  });

  test('falls back for an unknown card', () => {
    expect(cardLabel('zz')).toBe('carta desconhecida');
  });
});
