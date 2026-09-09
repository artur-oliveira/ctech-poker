import {act, renderHook} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import {TURN_COUNTDOWN_SECONDS, useTurnCountdown} from './useTurnCountdown';

function mockReducedMotion(matches: boolean) {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches, media: query, onchange: null,
    addListener: vi.fn(), removeListener: vi.fn(),
    addEventListener: vi.fn(), removeEventListener: vi.fn(), dispatchEvent: vi.fn()
  })) as unknown as typeof window.matchMedia;
}

describe('useTurnCountdown', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => {
    vi.useRealTimers();
    mockReducedMotion(false);
  });

  test('stays silent while the ring alone still answers the question', () => {
    mockReducedMotion(false);
    const {result} = renderHook(() => useTurnCountdown(Date.now() + (TURN_COUNTDOWN_SECONDS + 5) * 1000));
    expect(result.current).toBeNull();
  });

  test('appears for the last seconds and ticks down once per second', () => {
    mockReducedMotion(false);
    const deadline = Date.now() + (TURN_COUNTDOWN_SECONDS + 2) * 1000;
    const {result} = renderHook(() => useTurnCountdown(deadline));
    expect(result.current).toBeNull();
    act(() => vi.advanceTimersByTime(2000));
    expect(result.current).toBe(TURN_COUNTDOWN_SECONDS);
    act(() => vi.advanceTimersByTime(1000));
    expect(result.current).toBe(TURN_COUNTDOWN_SECONDS - 1);
    // Never negative once the deadline is behind us.
    act(() => vi.advanceTimersByTime(30_000));
    expect(result.current).toBe(0);
  });

  test('counts the whole turn under reduced motion, where the ring is frozen', () => {
    mockReducedMotion(true);
    const deadline = Date.now() + (TURN_COUNTDOWN_SECONDS + 20) * 1000;
    const {result} = renderHook(() => useTurnCountdown(deadline));
    expect(result.current).toBe(TURN_COUNTDOWN_SECONDS + 20);
    act(() => vi.advanceTimersByTime(1000));
    expect(result.current).toBe(TURN_COUNTDOWN_SECONDS + 19);
  });
});
