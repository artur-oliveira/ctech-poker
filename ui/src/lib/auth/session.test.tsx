import {act, renderHook, waitFor} from '@testing-library/react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {
  endExpiredSession,
  EXPIRED_SESSION_REDIRECT_WATCHDOG_MS,
  getOrRefreshSession,
  recoverSession,
  nextRefreshDelayMs,
  resetExpiredSessionLatchForTests,
  resetSessionRefreshCountForTests,
  sessionRefreshCount,
  TOKEN_REFRESH_INTERVAL_MS,
  TOKEN_REFRESH_MARGIN_MS,
  tokenExpiryMs,
  useOptionalSession,
  useSessionKeepAlive
} from './session';

/** A token whose `exp` is `seconds` from now. Only the payload is real — the
 * client reads `exp` to schedule, never to authorize. */
function tokenExpiringIn(seconds: number) {
  const payload = btoa(JSON.stringify({exp: Math.floor(Date.now() / 1000) + seconds}));
  return `header.${payload}.signature`;
}

function setVisibility(state: 'visible' | 'hidden') {
  Object.defineProperty(document, 'visibilityState', {configurable: true, get: () => state});
}

const mocks = vi.hoisted(() => ({
  token: null as string | null,
  listener: null as null | ((token: string | null) => void),
  unsubscribe: vi.fn(),
  setToken: vi.fn(),
  setUsername: vi.fn(),
  setPlayerId: vi.fn(),
  refresh: vi.fn(),
  endSession: vi.fn(),
  startOAuthFlow: vi.fn(),
  query: vi.fn(),
}));

vi.mock('@tanstack/react-query', () => ({useQuery: mocks.query}));
vi.mock('@/lib/mockConfig', () => ({USE_MOCK: false}));
vi.mock('@/lib/api/player', () => ({getMe: vi.fn()}));
vi.mock('@/lib/auth/oauth', () => ({
  doRefresh: mocks.refresh,
  endSession: mocks.endSession,
  startOAuthFlow: mocks.startOAuthFlow,
}));
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
    resetExpiredSessionLatchForTests();
    resetSessionRefreshCountForTests();
    mocks.token = null;
    setVisibility('visible');
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
    expect(mocks.endSession).toHaveBeenCalledWith('/');
    resetExpiredSessionLatchForTests();
  });

  test('an unauthorized socket does not log out on a transient refresh failure', async () => {
    mocks.refresh.mockRejectedValue(new Error('accounts temporarily unavailable'));
    recoverSession();
    await waitFor(() => expect(mocks.refresh).toHaveBeenCalled());
    expect(mocks.setToken).not.toHaveBeenCalledWith(null);
    expect(mocks.endSession).not.toHaveBeenCalled();
  });

  test('falls back to an interactive sign-in when the logout redirect no-ops', () => {
    vi.useFakeTimers();
    endExpiredSession();
    expect(mocks.setToken).toHaveBeenCalledWith(null);
    expect(mocks.endSession).toHaveBeenCalledWith('/');
    expect(mocks.startOAuthFlow).not.toHaveBeenCalled();

    vi.advanceTimersByTime(EXPIRED_SESSION_REDIRECT_WATCHDOG_MS);
    expect(mocks.startOAuthFlow).toHaveBeenCalledWith('/');

    // Latch was reset, so a later expiry can redirect again.
    endExpiredSession();
    expect(mocks.endSession).toHaveBeenCalledTimes(2);
    vi.useRealTimers();
    resetExpiredSessionLatchForTests();
  });

  test('two rapid endExpiredSession calls trigger one redirect but clear the session each time', () => {
    vi.useFakeTimers();
    endExpiredSession();
    endExpiredSession();
    expect(mocks.endSession).toHaveBeenCalledTimes(1);
    expect(mocks.setToken).toHaveBeenCalledTimes(2);
    expect(mocks.setToken).toHaveBeenNthCalledWith(1, null);
    expect(mocks.setToken).toHaveBeenNthCalledWith(2, null);
    vi.useRealTimers();
    resetExpiredSessionLatchForTests();
  });

  // #231: coming back to the tab is not itself a reason to spend a refresh —
  // only an expiry that has come due is.
  test('renews on return and on reconnect only when the token is due', async () => {
    vi.useFakeTimers();
    mocks.token = tokenExpiringIn(10 * 60);
    const {unmount} = renderHook(() => useSessionKeepAlive());

    await act(async () => {
      document.dispatchEvent(new Event('visibilitychange'));
      window.dispatchEvent(new Event('online'));
      await Promise.resolve();
    });
    expect(mocks.refresh).not.toHaveBeenCalled();

    // Slept past the margin: the first thing back on screen must not be a 401.
    mocks.token = tokenExpiringIn(30);
    await act(async () => {
      document.dispatchEvent(new Event('visibilitychange'));
      await Promise.resolve();
    });
    expect(mocks.refresh).toHaveBeenCalledTimes(1);

    unmount();
    await act(async () => {
      window.dispatchEvent(new Event('online'));
      await Promise.resolve();
    });
    expect(mocks.refresh).toHaveBeenCalledTimes(1);
    vi.useRealTimers();
  });

  test('schedules the renewal from the token expiry, not a fixed cadence', async () => {
    vi.useFakeTimers();
    mocks.token = tokenExpiringIn(15 * 60);
    const {unmount} = renderHook(() => useSessionKeepAlive());

    // The old 4-minute interval would have fired three times by here.
    await act(async () => {
      vi.advanceTimersByTime(15 * 60 * 1000 - TOKEN_REFRESH_MARGIN_MS - 1_000);
      await Promise.resolve();
    });
    expect(mocks.refresh).not.toHaveBeenCalled();

    await act(async () => {
      vi.advanceTimersByTime(2_000);
      await Promise.resolve();
    });
    expect(sessionRefreshCount()).toBe(1);

    unmount();
    vi.useRealTimers();
  });

  test('spends nothing while the tab is hidden', async () => {
    vi.useFakeTimers();
    mocks.token = tokenExpiringIn(15 * 60);
    setVisibility('hidden');
    const {unmount} = renderHook(() => useSessionKeepAlive());

    await act(async () => {
      document.dispatchEvent(new Event('visibilitychange'));
      vi.advanceTimersByTime(60 * 60 * 1000);
      await Promise.resolve();
    });
    // A hidden hour used to cost up to 15 refreshes.
    expect(sessionRefreshCount()).toBe(0);

    unmount();
    vi.useRealTimers();
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
  
  test('reads the expiry it schedules against, and falls back without one', () => {
    const token = tokenExpiringIn(600);
    const near = (value: number | undefined, target: number) =>
      expect(Math.abs((value ?? Number.NaN) - target)).toBeLessThan(2_000);
    near(tokenExpiryMs(token), Date.now() + 600_000);
    near(nextRefreshDelayMs(token), 600_000 - TOKEN_REFRESH_MARGIN_MS);
    // Already past the margin: due now, not negative.
    expect(nextRefreshDelayMs(tokenExpiringIn(10))).toBe(0);

    // The mock token, an opaque token and a malformed payload all fall back to
    // the fixed cadence measured from the last attempt.
    resetSessionRefreshCountForTests();
    for (const opaque of [null, 'opaque-token', 'header.not-base64-json.signature']) {
      expect(tokenExpiryMs(opaque)).toBeUndefined();
      near(nextRefreshDelayMs(opaque), TOKEN_REFRESH_INTERVAL_MS);
    }
  });

  test('shares one refresh promise across concurrent callers', async () => {
    let resolveRefresh: (value: { accessToken: string; username: string }) => void = () => undefined;
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
