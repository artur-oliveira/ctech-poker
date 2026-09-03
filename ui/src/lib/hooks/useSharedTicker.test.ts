import {afterEach, describe, expect, test, vi} from 'vitest';
import {subscribeTicker, tickerIntervalCount} from './useSharedTicker';

afterEach(() => {
  vi.useRealTimers();
});

describe('useSharedTicker', () => {
  test('runs no interval at all until someone subscribes, and none again once they leave', () => {
    vi.useFakeTimers();
    expect(tickerIntervalCount()).toBe(0);

    const stop = subscribeTicker(1000, () => undefined);
    expect(tickerIntervalCount()).toBe(1);

    stop();
    expect(tickerIntervalCount()).toBe(0);
  });

  test('keeps exactly one interval for every clock a turn puts on the table', () => {
    vi.useFakeTimers();
    // The cadences an active turn actually arms: action bar, two timed seats,
    // the reduced-motion countdown, the idle warning, the reality check.
    const stops = [250, 250, 250, 1000, 1000, 15_000].map(period =>
      subscribeTicker(period, () => undefined));

    expect(tickerIntervalCount()).toBe(1);

    stops.forEach(stop => stop());
    expect(tickerIntervalCount()).toBe(0);
  });

  test('notifies each subscriber on its own period, not on the shared base cadence', () => {
    vi.useFakeTimers();
    const fast = vi.fn();
    const slow = vi.fn();
    const stopFast = subscribeTicker(250, fast);
    const stopSlow = subscribeTicker(1000, slow);

    vi.advanceTimersByTime(1000);
    expect(fast).toHaveBeenCalledTimes(4);
    expect(slow).toHaveBeenCalledTimes(1);

    stopFast();
    stopSlow();
  });

  test('sharpens the base cadence when a faster subscriber joins a slow table', () => {
    vi.useFakeTimers();
    const slow = vi.fn();
    const stopSlow = subscribeTicker(15_000, slow);

    const fast = vi.fn();
    const stopFast = subscribeTicker(250, fast);
    vi.advanceTimersByTime(500);
    expect(fast).toHaveBeenCalledTimes(2);
    expect(tickerIntervalCount()).toBe(1);

    // The slow subscriber's phase survives the base swap: it is still due at
    // its own 15 s mark, not restarted by the faster one joining.
    vi.advanceTimersByTime(14_500);
    expect(slow).toHaveBeenCalledTimes(1);

    stopFast();
    stopSlow();
  });

  test('drops back to the surviving cadence when the fast subscriber leaves', () => {
    vi.useFakeTimers();
    const slow = vi.fn();
    const stopSlow = subscribeTicker(1000, slow);
    const stopFast = subscribeTicker(250, vi.fn());

    stopFast();
    expect(tickerIntervalCount()).toBe(1);
    vi.advanceTimersByTime(2000);
    expect(slow).toHaveBeenCalledTimes(2);

    stopSlow();
  });
});
