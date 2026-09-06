import {describe, expect, test} from 'vitest';
import {DEFAULT_SHOWCASE_ORDER, normalizeShowcaseLayout, type ShowcaseLayout} from './player';

describe('normalizeShowcaseLayout', () => {
  test('returns the default layout when the field is missing', () => {
    expect(normalizeShowcaseLayout()).toEqual({order: DEFAULT_SHOWCASE_ORDER, hidden: []});
  });

  test('fills in missing sections and drops unknown ones, preserving order', () => {
    const layout = {order: ['matchup', 'nonsense', 'achievements'], hidden: ['best_hand']} as unknown as ShowcaseLayout;
    expect(normalizeShowcaseLayout(layout)).toEqual({
      order: ['matchup', 'achievements', 'best_hand'],
      hidden: ['best_hand']
    });
  });

  test('never hides achievements', () => {
    const layout = {order: DEFAULT_SHOWCASE_ORDER, hidden: ['achievements', 'matchup']} as unknown as ShowcaseLayout;
    expect(normalizeShowcaseLayout(layout).hidden).toEqual(['matchup']);
  });

  test('tolerates null/absent order and hidden from an older profile row', () => {
    expect(normalizeShowcaseLayout({order: null, hidden: null} as unknown as ShowcaseLayout))
      .toEqual({order: DEFAULT_SHOWCASE_ORDER, hidden: []});
    expect(normalizeShowcaseLayout({} as unknown as ShowcaseLayout))
      .toEqual({order: DEFAULT_SHOWCASE_ORDER, hidden: []});
  });
});
