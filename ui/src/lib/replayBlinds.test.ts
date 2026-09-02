import {describe, expect, test} from 'vitest';
import type {HandHistoryAction} from '@/lib/api/table';
import {deriveBigBlind, FALLBACK_BIG_BLIND} from './replayBlinds';

const row = (action: HandHistoryAction['action'], amount: number): Pick<HandHistoryAction, 'action' | 'amount'> =>
  ({action, amount});

describe('deriveBigBlind', () => {
  test('prefers the stored blind level when it is present and positive', () => {
    expect(deriveBigBlind([row('post_big_blind', 25)], 100)).toBe(100);
  });

  test('ignores a missing or non-positive stored level', () => {
    expect(deriveBigBlind([row('post_big_blind', 50)], 0)).toBe(50);
    expect(deriveBigBlind([row('post_big_blind', 50)], undefined)).toBe(50);
  });

  test('derives from the largest post_big_blind amount in the timeline', () => {
    expect(deriveBigBlind([
      row('post_big_blind', 50),
      row('post_big_blind', 100),
      row('raise', 400),
    ])).toBe(100);
  });

  test('falls back for legacy hands with no post_big_blind action', () => {
    expect(deriveBigBlind([row('call', 25), row('raise', 75)])).toBe(FALLBACK_BIG_BLIND);
    expect(deriveBigBlind([])).toBe(FALLBACK_BIG_BLIND);
  });
});
