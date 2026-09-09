import {act, renderHook} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import {useReducedMotionCountdown} from './useReducedMotionCountdown';

function mockReducedMotion(matches: boolean) {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches, media: query, onchange: null,
    addListener: vi.fn(), removeListener: vi.fn(),
    addEventListener: vi.fn(), removeEventListener: vi.fn(), dispatchEvent: vi.fn()
  })) as unknown as typeof window.matchMedia;
}

describe('useReducedMotionCountdown', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => {
    vi.useRealTimers();
    mockReducedMotion(false);
  });

  test('returns null when motion is not reduced, leaving the ring as the only signal', () => {
    mockReducedMotion(false);
    const {result} = renderHook(() => useReducedMotionCountdown(Date.now() + 10_000));
    expect(result.current).toBeNull();
  });

  test('ticks whole seconds remaining once per second under reduced motion', () => {
    mockReducedMotion(true);
    const deadline = Date.now() + 3_000;
    const {result} = renderHook(() => useReducedMotionCountdown(deadline));
    expect(result.current).toBe(3);
    act(() => vi.advanceTimersByTime(1000));
    expect(result.current).toBe(2);
    act(() => vi.advanceTimersByTime(2000));
    expect(result.current).toBe(0);
  });
});
