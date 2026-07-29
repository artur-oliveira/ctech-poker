import {act, renderHook, waitFor} from '@testing-library/react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {
  getOrRefreshSession,
  recoverSession,
  TOKEN_REFRESH_INTERVAL_MS,
  useOptionalSession,
  useSessionKeepAlive
} from './session';

const mocks = vi.hoisted(() => ({
  token: null as string | null,
  listener: null as null | ((token: string | null) => void),
  unsubscribe: vi.fn(),
  setToken: vi.fn(),
  setUsername: vi.fn(),
  setPlayerId: vi.fn(),
  refresh: vi.fn(),
  query: vi.fn(),
}));

vi.mock('@tanstack/react-query', () => ({useQuery: mocks.query}));
vi.mock('@/lib/mockConfig', () => ({USE_MOCK: false}));
vi.mock('@/lib/api/player', () => ({getMe: vi.fn()}));
vi.mock('@/lib/auth/oauth', () => ({doRefresh: mocks.refresh}));
vi.mock('@/lib/api/client', () => ({
  getAccessToken: () => mocks.token,
  setAccessToken: mocks.setToken,
  setUsername: mocks.setUsername,
  setPlayerId: mocks.setPlayerId,
  subscribeAccessToken: (listener: (token: string | null) => void) => {
    mocks.listener = listener;
    return mocks.unsubscribe;
  },
}));

describe('session keep-alive', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.refresh.mockResolvedValue({accessToken: 'fresh', username: 'Ana'});
  });
  
  test('renews the token on a timer for as long as the app is mounted', async () => {
    vi.useFakeTimers();
    const {unmount} = renderHook(() => useSessionKeepAlive());
    expect(mocks.refresh).not.toHaveBeenCalled();
    
    await act(async () => {
      vi.advanceTimersByTime(TOKEN_REFRESH_INTERVAL_MS);
      await Promise.resolve();
    });
    expect(mocks.setToken).toHaveBeenCalledWith('fresh');
    
    unmount();
    await act(async () => {
      vi.advanceTimersByTime(TOKEN_REFRESH_INTERVAL_MS);
      await Promise.resolve();
    });
    expect(mocks.refresh).toHaveBeenCalledTimes(1);
    vi.useRealTimers();
  });
  
  test('an unauthorized socket clears the session when the refresh is refused', async () => {
    mocks.refresh.mockResolvedValue(null);
    recoverSession();
    await waitFor(() => expect(mocks.setToken).toHaveBeenCalledWith(null));
    expect(mocks.setPlayerId).toHaveBeenCalledWith(null);
  });
  
  test('keeps the session when a refresh fails on a network error', async () => {
    mocks.refresh.mockRejectedValue(new Error('offline'));
    vi.useFakeTimers();
    const {unmount} = renderHook(() => useSessionKeepAlive());
    await act(async () => {
      vi.advanceTimersByTime(TOKEN_REFRESH_INTERVAL_MS);
      await Promise.resolve();
    });
    expect(mocks.setToken).not.toHaveBeenCalled();
    unmount();
    vi.useRealTimers();
  });

  test('shares one refresh promise across concurrent callers', async () => {
    let resolveRefresh: (value: {accessToken: string; username: string}) => void = () => undefined;
    mocks.refresh.mockReturnValue(new Promise(resolve => {
      resolveRefresh = resolve;
    }));
    const first = getOrRefreshSession();
    const second = getOrRefreshSession();
    expect(mocks.refresh).toHaveBeenCalledOnce();
    resolveRefresh({accessToken: 'one-token', username: 'Ana'});
    await expect(Promise.all([first, second])).resolves.toEqual([
      {accessToken: 'one-token', username: 'Ana'},
      {accessToken: 'one-token', username: 'Ana'},
    ]);
    expect(mocks.setToken).toHaveBeenCalledOnce();
  });
});

describe('useOptionalSession', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.token = null;
    mocks.listener = null;
    mocks.refresh.mockResolvedValue(null);
    mocks.query.mockReturnValue({data: undefined});
  });
  
  test('checks a missing session and finishes unauthenticated when refresh fails', async () => {
    const {result, unmount} = renderHook(() => useOptionalSession());
    expect(result.current).toEqual({authed: false, checking: true});
    expect(mocks.query).toHaveBeenCalledWith(expect.objectContaining({enabled: false}));
    await waitFor(() => expect(result.current.checking).toBe(false));
    unmount();
    expect(mocks.unsubscribe).toHaveBeenCalledOnce();
  });
  
  test('restores identity and reacts to future access-token events', async () => {
    mocks.refresh.mockResolvedValue({accessToken: 'fresh', username: 'Ana'});
    const {result} = renderHook(() => useOptionalSession());
    await waitFor(() => expect(mocks.setToken).toHaveBeenCalledWith('fresh'));
    expect(mocks.setUsername).toHaveBeenCalledWith('Ana');
    expect(result.current.checking).toBe(false);
    
    act(() => mocks.listener?.('fresh'));
    expect(result.current.authed).toBe(true);
    act(() => mocks.listener?.(null));
    expect(result.current.authed).toBe(false);
  });
  
  test('uses an existing session immediately and syncs the backend player id', () => {
    mocks.token = 'existing';
    mocks.query.mockReturnValue({data: {user_id: 'player-7'}});
    const {result, rerender} = renderHook(() => useOptionalSession());
    expect(result.current).toEqual({authed: true, checking: false});
    expect(mocks.refresh).not.toHaveBeenCalled();
    expect(mocks.query).toHaveBeenCalledWith(expect.objectContaining({enabled: true}));
    expect(mocks.setPlayerId).toHaveBeenCalledWith('player-7');
    
    mocks.query.mockReturnValue({data: undefined});
    rerender();
    expect(mocks.setPlayerId).toHaveBeenLastCalledWith(null);
  });
});
