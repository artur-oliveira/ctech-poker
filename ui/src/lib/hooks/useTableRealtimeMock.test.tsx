import {act, renderHook, waitFor as rtlWaitFor} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import {useTableRealtime} from './useTableRealtime';

vi.mock('@aoctech/ws-client', () => ({
  useWebSocket: () => ({status: 'disconnected', attempt: 0, send: vi.fn(() => false), reconnect: vi.fn()}),
  MAX_RECONNECT_ATTEMPTS: 8,
}));
vi.mock('@/lib/mockConfig', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/mockConfig')>(), USE_MOCK: true,
}));
vi.mock('@/lib/api/client', () => ({
  getAccessToken: () => 'token',
  subscribeAccessToken: () => () => {},
  setAccessToken: vi.fn(), setUsername: vi.fn(), setPlayerId: vi.fn(),
}));
vi.mock('@/lib/auth/oauth', () => ({doRefresh: vi.fn()}));
vi.mock('@/lib/sound', () => ({playSound: vi.fn()}));

const VIEWER = 'mock_player_ana';

// The in-memory service reaches 'connected' over several scheduled ticks, so a
// loaded machine needs more headroom than waitFor's 1s default.
const waitFor: typeof rtlWaitFor = (callback, options) =>
  rtlWaitFor(callback, {timeout: 5000, ...options});

describe('useTableRealtime against the in-memory mock table', () => {
  beforeEach(() => vi.useRealTimers());
  afterEach(() => vi.useRealTimers());

  test('connects to the requested scenario and publishes its snapshot', async () => {
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER, undefined, {scenario: 'flop', delay: 0}));
    await waitFor(() => expect(result.current.snapshot).not.toBeNull());
    expect(result.current.status).toBe('connected');
    expect(result.current.snapshot?.board).toHaveLength(3);
  });

  test('clamps an out-of-range mock delay instead of trusting the query string', async () => {
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER, undefined,
      {scenario: 'waiting', delay: -5000}));
    await waitFor(() => expect(result.current.snapshot).not.toBeNull());
  });

  test('resets the snapshot and version guard when the scenario changes mid-session', async () => {
    const {result, rerender} = renderHook(
      ({scenario}: {scenario: 'complete' | 'pre_flop'}) =>
        useTableRealtime('table-1', VIEWER, undefined, {scenario, delay: 0}),
      {initialProps: {scenario: 'complete' as 'complete' | 'pre_flop'}},
    );
    await waitFor(() => expect(result.current.snapshot?.stage).toBe('complete'));

    rerender({scenario: 'pre_flop'});
    await waitFor(() => expect(result.current.snapshot?.stage).toBe('pre_flop'));
    expect(result.current.announcement).not.toBe('');
  });

  test('routes player commands into the mock service', async () => {
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER, undefined,
      {scenario: 'pre_flop', delay: 0}));
    await waitFor(() => expect(result.current.snapshot).not.toBeNull());

    act(() => void result.current.sendChat('boa mão'));
    await waitFor(() => expect(result.current.chat.some(item => item.message === 'boa mão')).toBe(true));

    act(() => void result.current.sendReaction('clap'));
    await waitFor(() => expect(result.current.reactions.at(-1)?.reactionId).toBe('clap'));
  });

  test('drops back to a disconnected state and recovers on retry', async () => {
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER, undefined,
      {scenario: 'reconnecting', delay: 0}));
    await waitFor(() => expect(result.current.status).toBe('connected'));

    act(() => result.current.retryNow());
    await waitFor(() => expect(result.current.snapshot).not.toBeNull());
  });

  test('tears the mock service down when the table is left', async () => {
    const {result, unmount} = renderHook(() => useTableRealtime('table-1', VIEWER, undefined,
      {scenario: 'flop', delay: 0}));
    await waitFor(() => expect(result.current.snapshot).not.toBeNull());
    expect(() => unmount()).not.toThrow();
  });
});
