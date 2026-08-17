import {act, renderHook} from '@testing-library/react';
import {afterEach, describe, expect, test, vi} from 'vitest';
import {useLiveNow} from './useLiveNow';

afterEach(() => {
  vi.useRealTimers();
});

describe('useLiveNow', () => {
  test('advances on its own interval while active', () => {
    vi.useFakeTimers();
    const {result} = renderHook(() => useLiveNow(true, 250));
    const first = result.current;

    act(() => void vi.advanceTimersByTime(500));
    expect(result.current).toBe(first + 500);
  });

  test('stands still while inactive', () => {
    vi.useFakeTimers();
    const {result} = renderHook(() => useLiveNow(false, 250));
    const first = result.current;

    act(() => void vi.advanceTimersByTime(5000));
    expect(result.current).toBe(first);
  });

  test('catches up immediately on re-activation instead of reporting the stale idle timestamp', () => {
    vi.useFakeTimers();
    const {result, rerender} = renderHook(({active}) => useLiveNow(active, 250), {
      initialProps: {active: false},
    });
    const idle = result.current;

    act(() => void vi.advanceTimersByTime(30_000));
    rerender({active: true});
    expect(result.current).toBe(idle + 30_000);
  });

  test('stops ticking once it goes inactive again', () => {
    vi.useFakeTimers();
    const {result, rerender} = renderHook(({active}) => useLiveNow(active, 250), {
      initialProps: {active: true},
    });

    act(() => void vi.advanceTimersByTime(250));
    rerender({active: false});
    const stopped = result.current;
    act(() => void vi.advanceTimersByTime(5000));
    expect(result.current).toBe(stopped);
  });
});
