import {act, renderHook} from '@testing-library/react';
import {afterEach, describe, expect, test, vi} from 'vitest';
import {useEnteredKeys} from './useEnteredKeys';

afterEach(() => vi.useRealTimers());

describe('useEnteredKeys', () => {
  test('keys present at first render are never "entered"', () => {
    const {result} = renderHook(({keys}) => useEnteredKeys(keys, 200), {
      initialProps: {keys: ['a', 'b', 'c']},
    });
    expect([...result.current]).toEqual([]);
  });

  test('marks only the keys added since the previous render, then drops them', () => {
    vi.useFakeTimers();
    const {result, rerender} = renderHook(({keys}) => useEnteredKeys(keys, 200), {
      initialProps: {keys: ['a', 'b'] as string[]},
    });
    rerender({keys: ['a', 'b', 'c']});
    expect([...result.current]).toEqual(['c']);

    act(() => void vi.advanceTimersByTime(200));
    expect([...result.current]).toEqual([]);

    // A re-render that changes nothing must not re-arm.
    rerender({keys: ['a', 'b', 'c']});
    expect([...result.current]).toEqual([]);
  });

  test('ignores falsy entries and survives a full reset', () => {
    const {result, rerender} = renderHook(({keys}) => useEnteredKeys(keys, 200), {
      initialProps: {keys: ['a', ''] as string[]},
    });
    rerender({keys: [] as string[]});
    expect([...result.current]).toEqual([]);
    rerender({keys: ['a', 'b']});
    expect([...result.current].sort()).toEqual(['a', 'b']);
  });
});
