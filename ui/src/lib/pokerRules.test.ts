import assert from 'node:assert/strict';
import {describe, expect, test} from 'vitest';
import {
  bestFiveCardHand,
  bestHandCategory,
  BUY_IN_MAX_BB,
  BUY_IN_MIN_BB,
  buyInRange,
  compareHands,
  wasDecidedByKicker
} from './pokerRules.ts';

describe('shared buy-in window', () => {
  test('one floor and ceiling in big blinds, min below max', () => {
    expect(BUY_IN_MIN_BB).toBe(40);
    expect(BUY_IN_MAX_BB).toBe(100);
    expect(BUY_IN_MIN_BB).toBeLessThan(BUY_IN_MAX_BB);
  });

  test('buyInRange scales both bounds by the big blind', () => {
    expect(buyInRange(20)).toEqual({min: 800, max: 2000});
    expect(buyInRange(200)).toEqual({min: 8000, max: 20000});
  });
});

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

// #296: short-deck (6+ hold'em) ranks flush above full house — the inverse
// of standard — and recognizes A-6-7-8-9 as its own low straight. Every
// function defaults to 'standard' so these must never change the assertions
// above; the short-deck behaviour only appears when the variant is passed.
describe('short-deck (#296) variant scoring', () => {
  const flush = ['6C', '8C', 'TC', 'QC', 'AC'];
  const fullHouse = ['KC', 'KD', 'KH', 'QC', 'QD'];

  test('flush beats full house under short_deck', () => {
    expect(bestHandCategory(flush, 'short_deck')).toBe('flush');
    expect(bestHandCategory(fullHouse, 'short_deck')).toBe('full_house');
    expect(compareHands(flush, fullHouse, 'short_deck')).toBeGreaterThan(0);
  });

  test('full house beats flush under standard (sanity check, same cards)', () => {
    expect(compareHands(flush, fullHouse)).toBeLessThan(0);
  });

  test('A-6-7-8-9 is a straight under short_deck, and 6-7-8-9-10 outranks it', () => {
    const lowStraight = ['AC', '6D', '7H', '8S', '9C'];
    const sixToTen = ['6C', '7D', '8H', '9S', 'TC'];
    expect(bestHandCategory(lowStraight, 'short_deck')).toBe('straight');
    expect(compareHands(sixToTen, lowStraight, 'short_deck')).toBeGreaterThan(0);
  });

  test('A-6-7-8-9 is NOT a straight under standard (Two-Five ranks matter there)', () => {
    expect(bestHandCategory(['AC', '6D', '7H', '8S', '9C'])).toBe('high_card');
  });

  test('bestFiveCardHand orders the short-deck low straight ace-low, like the wheel', () => {
    expect(bestFiveCardHand(['AC', '6D', '7H', '8S', '9C'], 'short_deck'))
      .toEqual(['9C', '8S', '7H', '6D', 'AC']);
  });
});
