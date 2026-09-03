import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import {invalidateAfterSettle, SETTLE_REFETCH_DELAYS_MS} from './settleRefetch';

function fakeClient() {
  return {invalidateQueries: vi.fn()} as unknown as import('@tanstack/react-query').QueryClient & {
    invalidateQueries: ReturnType<typeof vi.fn>;
  };
}

describe('invalidateAfterSettle', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  test('invalidates immediately and once per backoff delay', () => {
    const client = fakeClient();
    invalidateAfterSettle(client, ['hands', 't1']);

    expect(client.invalidateQueries).toHaveBeenCalledTimes(1);
    expect(client.invalidateQueries).toHaveBeenCalledWith({queryKey: ['hands', 't1']});

    vi.advanceTimersByTime(Math.max(...SETTLE_REFETCH_DELAYS_MS));
    expect(client.invalidateQueries).toHaveBeenCalledTimes(1 + SETTLE_REFETCH_DELAYS_MS.length);
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
    invalidateAfterSettle(client, ['hands', 't1'], [100]);

    vi.advanceTimersByTime(99);
    expect(client.invalidateQueries).toHaveBeenCalledTimes(1);
    vi.advanceTimersByTime(1);
    expect(client.invalidateQueries).toHaveBeenCalledTimes(2);
  });
});
