import {describe, expect, test} from 'vitest';
import {boardHasFlushDraw, boardHasStraightDraw, explainEquity, matchingCards} from './equityTrainer';

describe('boardHasFlushDraw', () => {
  test('true with three board cards sharing a suit', () => {
    expect(boardHasFlushDraw(['AH', 'KH', '2H', '9C', '4D'])).toBe(true);
  });

  test('false with no suit reaching three', () => {
    expect(boardHasFlushDraw(['AH', 'KH', '2C', '9C', '4D'])).toBe(false);
  });
});

describe('boardHasStraightDraw', () => {
  test('true when board ranks span 4 or less', () => {
    expect(boardHasStraightDraw(['9C', '8D', '5H'])).toBe(true);
  });

  test('false when board ranks are too spread out', () => {
    expect(boardHasStraightDraw(['AC', '9D', '2H'])).toBe(false);
  });
});

describe('matchingCards', () => {
  test('trims to the cards that make up the category, dropping kickers', () => {
    const cards = matchingCards('pair', ['AH', 'AD'], ['KC', '5H', '2D']);
    expect(cards).toHaveLength(2);
    expect(cards.every(card => card.startsWith('A'))).toBe(true);
  });

  test('drops the masked back placeholder instead of treating it as a real card', () => {
    expect(matchingCards('pair', ['AH', 'back'], ['AD', '2C', '3D'])).toEqual(['AH', 'AD']);
  });

  test('returns nothing without a known category', () => {
    expect(matchingCards(undefined, ['AH', 'AD'], [])).toEqual([]);
  });
});

describe('explainEquity', () => {
  test('flags a made pair on a three-flush board', () => {
    const text = explainEquity('pair', ['AH', 'KH', '2H', '9C', '4D']);
    expect(text).toContain('par');
    expect(text).toContain('flush');
  });

  test('flags a two-pair on a connected straight-y board', () => {
    const text = explainEquity('two_pair', ['9C', '8D', '5H']);
    expect(text).toContain('dois pares');
    expect(text).toContain('sequência');
  });

  test('made flush needs no flush warning even on a three-flush board', () => {
    const text = explainEquity('flush', ['AH', 'KH', '2H', '9C', '4D']);
    expect(text).not.toContain('um flush');
  });

  test('no category yet reads as a waiting message, not a false read', () => {
    expect(explainEquity(undefined, [])).toMatch(/aguardando/i);
  });
});
