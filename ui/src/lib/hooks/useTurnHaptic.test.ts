import {renderHook} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import {TURN_VIBRATION_MS, useTurnHaptic} from './useTurnHaptic';

const vibrate = vi.fn();

function setVibrate(supported: boolean) {
  Object.defineProperty(navigator, 'vibrate', {
    configurable: true, writable: true, value: supported ? vibrate : undefined
  });
}

function setHidden(hidden: boolean) {
  Object.defineProperty(document, 'visibilityState', {
    configurable: true, get: () => hidden ? 'hidden' : 'visible'
  });
}

describe('useTurnHaptic', () => {
  beforeEach(() => {
    vibrate.mockClear();
    setVibrate(true);
    setHidden(false);
  });
  afterEach(() => setVibrate(false));

  test('buzzes once when the turn opens', () => {
    const {rerender} = renderHook(({turn}) => useTurnHaptic(turn), {initialProps: {turn: false}});
    expect(vibrate).not.toHaveBeenCalled();
    rerender({turn: true});
    expect(vibrate).toHaveBeenCalledExactlyOnceWith(TURN_VIBRATION_MS);
  });

  test('does not re-fire on re-renders inside the same turn', () => {
    const {rerender} = renderHook(({turn}) => useTurnHaptic(turn), {initialProps: {turn: true}});
    rerender({turn: true});
    rerender({turn: true});
    expect(vibrate).toHaveBeenCalledOnce();
  });

  test('fires again on the next turn, once the previous one closed', () => {
    const {rerender} = renderHook(({turn}) => useTurnHaptic(turn), {initialProps: {turn: true}});
    rerender({turn: false});
    rerender({turn: true});
    expect(vibrate).toHaveBeenCalledTimes(2);
  });

  test('stays silent in a hidden tab, and does not buzz late when the tab comes back', () => {
    setHidden(true);
    const {rerender} = renderHook(({turn}) => useTurnHaptic(turn), {initialProps: {turn: false}});
    rerender({turn: true});
    expect(vibrate).not.toHaveBeenCalled();
    setHidden(false);
    rerender({turn: true});
    expect(vibrate).not.toHaveBeenCalled();
  });

  test('does nothing where the platform has no Vibration API', () => {
    setVibrate(false);
    const {rerender} = renderHook(({turn}) => useTurnHaptic(turn), {initialProps: {turn: false}});
    expect(() => rerender({turn: true})).not.toThrow();
    expect(vibrate).not.toHaveBeenCalled();
  });
});
