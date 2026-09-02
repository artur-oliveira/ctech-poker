import type {ReactNode} from 'react';
import {renderHook, waitFor} from '@testing-library/react';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import type {AchievementsSummary} from '@/lib/api/achievements';
import {useAchievementsSummary} from './useAchievementsSummary';

const {getMyAchievementsSummary} = vi.hoisted(() => ({getMyAchievementsSummary: vi.fn()}));
vi.mock('@/lib/api/achievements', () => ({getMyAchievementsSummary}));

const summary: AchievementsSummary = {
  mode: 'sandbox',
  totals: {revealed: 2, unlocked: 1, completed: 0, stars: 1, max_stars: 10},
  achievements: [
    {key: 'wins', metric: 'hand_won', tiers: [{stars: 1, threshold: 1}], progress: 3, stars: 1, unlocked: true, completed: false, next_target: null, max_target: 1},
    {key: 'bluff', metric: 'bluff', tiers: [{stars: 1, threshold: 1}], progress: 0, stars: 0, unlocked: false, completed: false, next_target: 1, max_target: 1},
  ],
};

let client: QueryClient;

function wrapper({children}: {children: ReactNode}) {
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe('useAchievementsSummary', () => {
  beforeEach(() => {
    getMyAchievementsSummary.mockReset().mockResolvedValue(summary);
    client = new QueryClient({defaultOptions: {queries: {retry: false}}});
  });

  test('fetches the full-state summary once for the given mode', async () => {
    const {result} = renderHook(() => useAchievementsSummary('real', true), {wrapper});
    await waitFor(() => expect(result.current.data).toEqual(summary));
    expect(getMyAchievementsSummary).toHaveBeenCalledTimes(1);
    expect(getMyAchievementsSummary).toHaveBeenCalledWith('real');
  });

  test('does not fetch while disabled', () => {
    renderHook(() => useAchievementsSummary('sandbox', false), {wrapper});
    expect(getMyAchievementsSummary).not.toHaveBeenCalled();
  });
});
