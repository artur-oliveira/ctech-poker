import assert from 'node:assert/strict';
import {act, renderHook} from '@testing-library/react';
import {afterEach, test, vi} from 'vitest';
import {
  advancePendingAction, planAuxiliaryRetry, resyncDelayMs, shouldRetryPendingAction,
  useTableActionQueue, type PendingTableAction
} from './useTableActionQueue.ts';
import {AUX_RETRY_BASE_MS, MAX_ACTION_RETRIES} from '@/lib/tableResilience';

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

test('pure retry planning classifies errors, preserves the frame and caps the budget', () => {
  const frame = {type: 'show_cards', action_id: 'a1'};
  assert.equal(planAuxiliaryRetry(undefined, 'stale_state', 0), null);
  assert.equal(planAuxiliaryRetry({frame, retries: 0}, 'forbidden', 0), null);
  assert.deepEqual(planAuxiliaryRetry({frame, retries: 0}, 'stale_state', 0), {
    frame, retries: 1, delayMs: AUX_RETRY_BASE_MS
  });
  assert.equal(planAuxiliaryRetry({frame, retries: MAX_ACTION_RETRIES}, 'stale_state', 0), null);
});

test('primary action retry planning is pure, correlated and bounded', () => {
  const pending: PendingTableAction = {
    id: 'a1', action: 'call', amount: 20, snapshotVersion: 4, handId: 'h1',
    retries: 0, awaitingRetry: true
  };
  assert.equal(shouldRetryPendingAction(pending, 'stale_state', 'a1'), true);
  assert.equal(shouldRetryPendingAction(pending, 'stale_state', 'other'), false);
  assert.equal(shouldRetryPendingAction({...pending, retries: MAX_ACTION_RETRIES}, 'stale_state', 'a1'), false);
  assert.deepEqual(advancePendingAction(pending, 5, 'h2'), {
    ...pending, snapshotVersion: 5, handId: 'h2', retries: 1, awaitingRetry: false
  });
  assert.equal(pending.retries, 0, 'the transition must not mutate the queued action');
  assert.equal(resyncDelayMs('stale_state', 0, 0), 50);
  assert.equal(resyncDelayMs('rate_limited', 0, 0), 800);
});

test('the queue resends the same frame after backoff and cancels dropped work', () => {
  vi.useFakeTimers();
  vi.spyOn(Math, 'random').mockReturnValue(0);
  const send = vi.fn(() => true);
  const failed = vi.fn();
  const {result} = renderHook(() => useTableActionQueue(send, failed));
  const frame = {type: 'request_exit', action_id: 'a1'};
  act(() => {
    result.current.track('a1', frame);
    assert.equal(result.current.retry('a1', 'stale_state'), true);
    vi.advanceTimersByTime(AUX_RETRY_BASE_MS - 1);
  });
  assert.equal(send.mock.calls.length, 0);
  act(() => vi.advanceTimersByTime(1));
  assert.deepEqual(send.mock.calls, [[frame]]);

  act(() => {
    assert.equal(result.current.retry('a1', 'stale_state'), true);
    result.current.drop('a1');
    vi.runAllTimers();
  });
  assert.equal(send.mock.calls.length, 1);
  assert.equal(result.current.retry('a1', 'stale_state'), false);
  assert.equal(failed.mock.calls.length, 0);
});

test('a failed retry delivery is returned to the session boundary', () => {
  vi.useFakeTimers();
  vi.spyOn(Math, 'random').mockReturnValue(0);
  const send = vi.fn(() => false);
  const failed = vi.fn();
  const {result, unmount} = renderHook(() => useTableActionQueue(send, failed));
  act(() => {
    result.current.track('a1', {type: 'show_cards'});
    result.current.retry('a1', 'invalid_action');
    vi.runAllTimers();
  });
  assert.deepEqual(failed.mock.calls, [['a1']]);
  act(() => {
    result.current.track('a2', {type: 'request_exit'});
    result.current.retry('a2', 'unavailable');
  });
  unmount();
  vi.runAllTimers();
  assert.equal(send.mock.calls.length, 1);
});
