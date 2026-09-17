import {describe, expect, test} from 'vitest';
import {
  achievementDescription, achievementExample, achievementLabel, achievementValueFormat, achievementWalletMode
} from './achievements';
import {ACHIEVEMENT_LABELS} from './utils';

const KEYS = Object.keys(ACHIEVEMENT_LABELS);

describe('achievement presentation metadata', () => {
  // The card art IS the achievement's icon. `sandbox_chips_earned` shipped
  // without one and rendered as a nameless blank space at the top of its card;
  // this is the guard that stops the next key from doing the same.
  test.each(KEYS)('%s has illustrative cards', key => {
    expect(achievementExample(key).length).toBeGreaterThan(0);
  });

  test.each(KEYS)('%s has a label and a description', key => {
    expect(achievementLabel(key)).not.toBe(key);
    expect(achievementDescription(key).length).toBeGreaterThan(0);
  });

  test('an unknown key degrades to readable text instead of throwing', () => {
    expect(achievementLabel('from_a_newer_server')).toBe('from a newer server');
    expect(achievementDescription('from_a_newer_server')).toBe('');
    expect(achievementExample('from_a_newer_server')).toEqual([]);
    expect(achievementExample('win_category_from_a_newer_server')).toEqual([]);
  });

  test('only the two chip totals are wallet-scoped', () => {
    expect(KEYS.filter(key => achievementWalletMode(key))).toEqual(['sandbox_chips_earned', 'real_money_earned']);
    expect(achievementWalletMode('sandbox_chips_earned')).toBe('sandbox');
    expect(achievementWalletMode('real_money_earned')).toBe('real');
  });

  test('no_rush counts milliseconds, everything else counts events', () => {
    expect(achievementValueFormat('no_rush')(2_592_000_000)).toBe('1 mês');
    expect(achievementValueFormat('no_rush')(45_000)).toBe('45 segundos');
    expect(achievementValueFormat('wins')(1234)).toBe('1.234');
  });
});
