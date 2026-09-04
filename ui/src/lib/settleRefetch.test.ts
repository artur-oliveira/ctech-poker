import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import {
  invalidateAfterSettle, resetSettleRefetchReads, SETTLE_REFETCH_DELAYS_MS, settleRefetchReads
} from './settleRefetch';

function fakeClient(cached: unknown = undefined) {
  const state = {data: cached};
  return {
    state,
    invalidateQueries: vi.fn(),
    getQueryData: vi.fn(() => state.data),
  } as unknown as import('@tanstack/react-query').QueryClient & {
    state: {data: unknown};
    invalidateQueries: ReturnType<typeof vi.fn>;
    getQueryData: ReturnType<typeof vi.fn>;
  };
}

describe('invalidateAfterSettle', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    resetSettleRefetchReads();
  });
  afterEach(() => vi.useRealTimers());

  test('without a settled predicate it invalidates immediately and once per backoff delay', () => {
    const client = fakeClient();
    invalidateAfterSettle(client, ['hands', 't1']);

    expect(client.invalidateQueries).toHaveBeenCalledTimes(1);
    expect(client.invalidateQueries).toHaveBeenCalledWith({queryKey: ['hands', 't1']});

    vi.advanceTimersByTime(SETTLE_REFETCH_DELAYS_MS.reduce((total, delay) => total + delay, 0));
    expect(client.invalidateQueries).toHaveBeenCalledTimes(1 + SETTLE_REFETCH_DELAYS_MS.length);
  });

  test('spends a single read when the first one lands the projection', () => {
    const client = fakeClient({hand_id: 'other'});
    invalidateAfterSettle<{hand_id: string}>(client, ['highlights', 't1', 'today'], {
      settled: data => data?.hand_id === 'h9',
    });
    expect(client.invalidateQueries).toHaveBeenCalledTimes(1);

    client.state.data = {hand_id: 'h9'};
    vi.advanceTimersByTime(60_000);
    expect(client.invalidateQueries).toHaveBeenCalledTimes(1);
    expect(settleRefetchReads()).toBe(1);
  });

  test('spends no read at all when the projection is already there', () => {
    const client = fakeClient({hand_id: 'h9'});
    invalidateAfterSettle<{hand_id: string}>(client, ['highlights', 't1', 'today'], {
      settled: data => data?.hand_id === 'h9',
    });

    vi.advanceTimersByTime(60_000);
    expect(client.invalidateQueries).not.toHaveBeenCalled();
    expect(settleRefetchReads()).toBe(0);
  });

  test('gives up after the last delay when the projection never lands', () => {
    const client = fakeClient();
    invalidateAfterSettle<{hand_id: string}>(client, ['hands', 't1'], {
      settled: data => data?.hand_id === 'h9',
    });

    vi.advanceTimersByTime(60_000);
    expect(client.invalidateQueries).toHaveBeenCalledTimes(1 + SETTLE_REFETCH_DELAYS_MS.length);
    expect(settleRefetchReads()).toBe(1 + SETTLE_REFETCH_DELAYS_MS.length);
  });

  test('cleanup cancels the pending invalidations', () => {
    const client = fakeClient();
    const cancel = invalidateAfterSettle(client, ['highlights', 't1', 'today']);
    cancel();

    vi.advanceTimersByTime(60_000);
    expect(client.invalidateQueries).toHaveBeenCalledTimes(1);
  });

  test('honours a custom delay list', () => {
    const client = fakeClient();
    invalidateAfterSettle(client, ['hands', 't1'], {delays: [100]});

    vi.advanceTimersByTime(99);
    expect(client.invalidateQueries).toHaveBeenCalledTimes(1);
    vi.advanceTimersByTime(1);
    expect(client.invalidateQueries).toHaveBeenCalledTimes(2);
    vi.advanceTimersByTime(60_000);
    expect(client.invalidateQueries).toHaveBeenCalledTimes(2);
  });
});
