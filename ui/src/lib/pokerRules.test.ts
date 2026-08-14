import assert from 'node:assert/strict';
import {describe, expect, test} from 'vitest';
import {bestFiveCardHand, bestHandCategory, compareHands, wasDecidedByKicker} from './pokerRules.ts';

test('different two-pair values are not mislabeled as a kicker decision', () => {
  assert.equal(wasDecidedByKicker(
    ['Th', 'Ts', '2d', '2s', 'Qh'],
    ['7h', '7s', '2d', '2s', 'Qh']
  ), false);
});

test('the fifth card can decide otherwise identical two-pair hands', () => {
  assert.equal(wasDecidedByKicker(
    ['Th', 'Ts', '2d', '2s', 'Ah'],
    ['Tc', 'Td', '2d', '2s', 'Qh']
  ), true);
});

describe('five-card evaluation edge cases', () => {
  test('names every made category from an exact five-card hand', () => {
    expect(bestHandCategory(['AH', 'KH', 'QH', 'JH', 'TH'])).toBe('royal_flush');
    expect(bestHandCategory(['9H', 'KH', 'QH', 'JH', 'TH'])).toBe('straight_flush');
    expect(bestHandCategory(['9H', '9D', '9C', '9S', 'TH'])).toBe('four_of_a_kind');
    expect(bestHandCategory(['9H', '9D', '9C', 'TS', 'TH'])).toBe('full_house');
    expect(bestHandCategory(['2H', '5H', '9H', 'JH', 'KH'])).toBe('flush');
    expect(bestHandCategory(['9H', 'KD', 'QC', 'JS', 'TH'])).toBe('straight');
    expect(bestHandCategory(['9H', '9D', '9C', '2S', 'TH'])).toBe('three_of_a_kind');
    expect(bestHandCategory(['9H', '9D', 'TC', 'TS', '2H'])).toBe('two_pair');
    expect(bestHandCategory(['9H', '9D', 'TC', '5S', '2H'])).toBe('pair');
    expect(bestHandCategory(['9H', '7D', 'TC', '5S', '2H'])).toBe('high_card');
  });

  test('leaves a hand of five or fewer cards in canonical order', () => {
    expect(bestFiveCardHand(['2H', 'AH'])).toEqual(['AH', '2H']);
  });

  test('ranks by category first, then by every tiebreak position', () => {
    expect(compareHands(['AH', 'KH', 'QH', 'JH', 'TH'], ['9H', '9D', '9C', '9S', 'TH'])).toBeGreaterThan(0);
    expect(compareHands(['9H', '9D', '2C', '5S', '7H'], ['TH', 'TD', '2C', '5S', '7D'])).toBeLessThan(0);
    expect(compareHands(['9H', '9D', '2C', '5S', '7H'], ['9C', '9S', '2D', '5H', '7C'])).toBe(0);
  });

  test('only calls a decision a kicker when the made combination is identical', () => {
    expect(wasDecidedByKicker(['AH', 'AD', 'KC', 'QS', '2D'], ['AS', 'AC', 'KH', 'QD', '3D'])).toBe(true);
    expect(wasDecidedByKicker(['AH', 'AD', 'KC', 'QS', '2D'], ['KS', 'KC', 'AH', 'QD', '3D'])).toBe(false);
    expect(wasDecidedByKicker(['AH', 'AD', 'KC', 'QS', '2D'], ['AS', 'AC', 'KH', 'QD', '2C'])).toBe(false);
    expect(wasDecidedByKicker(['AH', 'AD'], ['AS', 'AC', 'KH', 'QD', '2C'])).toBe(false);
  });
});
