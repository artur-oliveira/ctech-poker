import {act, renderHook} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import {formatDuration, useCountdownMs} from './useCountdown';

describe('formatDuration', () => {
  test('renders mm:ss under an hour', () => {
    expect(formatDuration(65_000)).toBe('1:05');
    expect(formatDuration(0)).toBe('0:00');
  });

  test('renders h:mm:ss at an hour or more', () => {
    expect(formatDuration(3_661_000)).toBe('1:01:01');
  });

  test('never goes negative', () => {
    expect(formatDuration(-5_000)).toBe('0:00');
  });
});

describe('useCountdownMs', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  test('returns 0 when target is null', () => {
    const {result} = renderHook(() => useCountdownMs(null));
    expect(result.current).toBe(0);
  });

  test('counts down toward the target as time passes', () => {
    const target = Date.now() + 5000;
    const {result} = renderHook(() => useCountdownMs(target));
    expect(result.current).toBeGreaterThan(4000);

    act(() => vi.advanceTimersByTime(3000));
    expect(result.current).toBeLessThanOrEqual(2000);
  });

  test('clamps at 0 once the target has passed', () => {
    const target = Date.now() + 1000;
    const {result} = renderHook(() => useCountdownMs(target));

    act(() => vi.advanceTimersByTime(5000));
    expect(result.current).toBe(0);
  });
});
