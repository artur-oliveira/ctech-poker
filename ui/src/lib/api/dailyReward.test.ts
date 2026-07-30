import {describe, expect, test, vi} from 'vitest';

const get = vi.fn();
const post = vi.fn();
vi.mock('./client', () => ({apiClient: {get: (...a: unknown[]) => get(...a), post: (...a: unknown[]) => post(...a)}}));

import {getCooldown, spin} from './dailyReward';

describe('dailyReward api', () => {
  test('getCooldown GETs remaining time', async () => {
    get.mockResolvedValueOnce({data: {remaining_time_seconds: 3600}});
    const res = await getCooldown();
    expect(get).toHaveBeenCalledWith('/v1.0/sandbox-credits/');
    expect(res.remaining_time_seconds).toBe(3600);
  });

  test('spin POSTs and returns amount + remaining time', async () => {
    post.mockResolvedValueOnce({data: {amount: 10000, remaining_time_seconds: 86400}});
    const res = await spin();
    expect(post).toHaveBeenCalledWith('/v1.0/sandbox-credits/', undefined, {silentError: true});
    expect(res.amount).toBe(10000);
  });
});
