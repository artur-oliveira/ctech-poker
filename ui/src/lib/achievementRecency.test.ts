import {afterEach, describe, expect, test, vi} from 'vitest';
import {
  byRecencyFirst, recentUnlocks, rememberAchievementUnlock, takeAchievementUnlock
} from './achievementRecency';
import type {AchievementSummaryEntry} from '@/lib/api/achievements';

function entry(overrides: Partial<AchievementSummaryEntry> = {}): AchievementSummaryEntry {
  return {
    key: 'wins', tiers: [], progress: 10, stars: 2, unlocked: true, completed: false,
    next_target: 20, max_target: 100, ...overrides
  } as AchievementSummaryEntry;
}

afterEach(() => {
  vi.restoreAllMocks();
  window.sessionStorage.clear();
});

describe('recentUnlocks (#119)', () => {
  test('is empty without data at all', () => {
    expect(recentUnlocks(undefined)).toEqual([]);
    expect(recentUnlocks([])).toEqual([]);
  });

  test('orders unlocks newest first and caps the rail', () => {
    const rows = ['2026-09-01T10:00:00Z', '2026-08-01T10:00:00Z', '2026-09-02T10:00:00Z']
      .map((at, i) => entry({key: `k${i}`, unlocked_at: at}));
    expect(recentUnlocks(rows).map(u => u.key)).toEqual(['k2', 'k0', 'k1']);
    expect(recentUnlocks(rows, 2).map(u => u.key)).toEqual(['k2', 'k0']);
    expect(recentUnlocks(rows)[0].unlockedAtMs).toBe(Date.parse('2026-09-02T10:00:00Z'));
  });

  test('skips locked rows, undated legacy unlocks and unparseable stamps', () => {
    expect(recentUnlocks([
      entry({key: 'locked', unlocked: false, unlocked_at: '2026-09-02T10:00:00Z'}),
      entry({key: 'legacy'}),
      entry({key: 'garbage', unlocked_at: 'not a date'}),
      entry({key: 'good', unlocked_at: '2026-09-02T10:00:00Z'})
    ]).map(u => u.key)).toEqual(['good']);
  });
});

describe('byRecencyFirst (#119)', () => {
  test('sorts newest first and parks undated or unparseable stamps last', () => {
    expect(['2026-08-01T00:00:00Z', 'nope', '2026-09-01T00:00:00Z'].sort(byRecencyFirst))
      .toEqual(['2026-09-01T00:00:00Z', '2026-08-01T00:00:00Z', 'nope']);
    expect(byRecencyFirst('2026-09-01T00:00:00Z', undefined)).toBeLessThan(0);
    expect(byRecencyFirst(undefined, '2026-09-01T00:00:00Z')).toBeGreaterThan(0);
    expect(byRecencyFirst(undefined, undefined)).toBe(0);
  });
});

describe('the table-to-page celebration handoff (#119)', () => {
  test('is read exactly once, so a remount cannot replay it', () => {
    rememberAchievementUnlock('all_in_wins');
    expect(takeAchievementUnlock()).toBe('all_in_wins');
    expect(takeAchievementUnlock()).toBeNull();
  });

  test('survives storage the browser refuses, on both sides', () => {
    const deny = () => {
      throw new Error('denied');
    };
    vi.spyOn(window, 'sessionStorage', 'get')
      .mockReturnValue({setItem: deny, getItem: deny, removeItem: deny} as unknown as Storage);
    expect(() => rememberAchievementUnlock('wins')).not.toThrow();
    expect(takeAchievementUnlock()).toBeNull();
  });
});
