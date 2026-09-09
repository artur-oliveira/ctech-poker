import {describe, expect, test} from 'vitest';
import {chipsExact, chipsShort, chipTier} from './chips';

describe('chipsExact', () => {
  test('groups pt-BR, always in full', () => {
    expect(chipsExact(0)).toBe('0');
    expect(chipsExact(1250)).toBe('1.250');
    expect(chipsExact(1_250_000)).toBe('1.250.000');
    expect(chipsExact(-400)).toBe('-400');
  });
});

describe('chipsShort', () => {
  test.each([
    [0, '0'],
    [1, '1'],
    [999, '999'],
    [1000, '1K'],
    [1049, '1K'],
    [1050, '1K'],
    [1100, '1,1K'],
    [1250, '1,2K'],
    // Truncated, never rounded up into the next unit.
    [1299, '1,2K'],
    [9999, '9,9K'],
    [10_000, '10K'],
    [600_000, '600K'],
    [999_999, '999K'],
    [1e6, '1M'],
    [1_250_000, '1,2M'],
    [1e9, '1B'],
    [1.5e9, '1,5B'],
    [1e12, '1T'],
    [1e15, '1.000T'],
  ])('%s renders as %s', (amount, expected) => {
    expect(chipsShort(amount)).toBe(expected);
  });

  test('never spends more than four characters below a quadrillion', () => {
    for (const amount of [999, 1000, 1250, 99_999, 999_999, 1e6, 9.9e11]) {
      expect(chipsShort(amount).length).toBeLessThanOrEqual(4);
    }
  });

  test('a negative amount keeps its sign (a refund correction, never a stack)', () => {
    expect(chipsShort(-2500)).toBe('-2,5K');
    expect(chipsShort(-999)).toBe('-999');
  });

  test('a non-finite amount falls back to the exact formatter rather than inventing a unit', () => {
    expect(chipsShort(Number.NaN)).toBe(chipsExact(Number.NaN));
  });
});

describe('chipTier', () => {
  test('scales against the big blind and saturates', () => {
    expect(chipTier(0, 25)).toBe(0);
    expect(chipTier(25, 25)).toBe(2);
    expect(chipTier(10, 25)).toBe(1);
    expect(chipTier(10_000, 25)).toBe(5);
  });
});
