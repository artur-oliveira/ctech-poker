import {beforeEach, describe, expect, test, vi} from 'vitest';
import {decodeIdToken, doRefresh, exchangeCode, lastReturnTo, logout, OAuthExchangeError, startOAuthFlow} from './oauth';
import {OAUTH_SCOPE} from './scopes';

const mocks = vi.hoisted(() => ({
  start: vi.fn(),
  exchange: vi.fn(),
  refresh: vi.fn(),
  revoke: vi.fn(),
  endSession: vi.fn(),
  decode: vi.fn(),
}));

// The client is constructed while this module's import is evaluated, before
// any test runs. vitest 5 clears spies before each test (`clearMocks`
// defaults to true), so a spy would have no record of that call by the time
// the first test asserts on it — hence a plain array instead of vi.fn().
const constructorCalls = vi.hoisted(() => [] as unknown[]);

vi.mock('@aoctech/auth-client', () => ({
  OAuthClient: class {
    startOAuthFlow = mocks.start;
    exchangeCode = mocks.exchange;
    refresh = mocks.refresh;
    revoke = mocks.revoke;
    endSessionRedirect = mocks.endSession;
    
    constructor(options: unknown) {
      constructorCalls.push(options);
    }
  },
  decodeIdToken: mocks.decode,
}));

describe('OAuth integration', () => {
  beforeEach(() => {
    mocks.start.mockClear();
    mocks.exchange.mockClear();
    mocks.refresh.mockClear();
    mocks.revoke.mockClear();
    mocks.endSession.mockClear();
    mocks.decode.mockClear();
  });
  
  test('configures the browser callback and delegates flow startup', async () => {
    expect(constructorCalls[0]).toEqual(expect.objectContaining({
      redirectUri: `${window.location.origin}/callback`,
      scope: OAUTH_SCOPE,
    }));
    mocks.start.mockResolvedValue(undefined);
    await startOAuthFlow('/table/t1');
    expect(mocks.start).toHaveBeenCalledWith('/table/t1');
  });
  
  // #342: @aoctech/auth-client clears its own `returnTo` copy as soon as
  // exchangeCode reads it, even on a non-state-mismatch failure — so a
  // manual retry from the callback's failure screens needs its own copy to
  // resend the same intent instead of falling back to /lobby.
  test('#342: remembers the last startOAuthFlow destination for a retry to resend', async () => {
    mocks.start.mockResolvedValue(undefined);
    expect(lastReturnTo()).toBeNull();
    await startOAuthFlow('/table/t1');
    expect(lastReturnTo()).toBe('/table/t1');
    await startOAuthFlow('/store');
    expect(lastReturnTo()).toBe('/store');
  });

  test('exchanges a callback and derives the username from the ID token', async () => {
    mocks.exchange.mockResolvedValue({
      accessToken: 'access',
      idToken: 'id-token',
      returnTo: '/lobby',
    });
    mocks.decode.mockReturnValue({username: 'Ana'});
    await expect(exchangeCode('code', 'state')).resolves.toEqual({
      accessToken: 'access',
      username: 'Ana',
      returnTo: '/lobby',
    });
    expect(decodeIdToken).toBe(mocks.decode);
  });
  
  test('handles refreshes with missing tokens, claims, or sessions', async () => {
    mocks.refresh.mockResolvedValueOnce({accessToken: 'fresh', idToken: null});
    await expect(doRefresh()).resolves.toEqual({accessToken: 'fresh', username: null});
    expect(mocks.decode).not.toHaveBeenCalled();
    
    mocks.refresh.mockResolvedValueOnce({accessToken: 'fresh-2', idToken: 'id'});
    mocks.decode.mockReturnValue(undefined);
    await expect(doRefresh()).resolves.toEqual({accessToken: 'fresh-2', username: null});
    
    mocks.refresh.mockResolvedValueOnce(null);
    await expect(doRefresh()).resolves.toBeNull();
  });

  test('bounds a token issue attempt to three seconds', async () => {
    vi.useFakeTimers();
    try {
      mocks.refresh.mockReturnValue(new Promise(() => undefined));
      const refresh = doRefresh();
      const rejection = expect(refresh).rejects.toThrow('Token request timed out after 3000ms');
      await vi.advanceTimersByTimeAsync(3_000);
      await rejection;
    } finally {
      vi.useRealTimers();
    }
  });
  
  test('bounds a callback exchange to three seconds and classifies it transient', async () => {
    vi.useFakeTimers();
    try {
      mocks.exchange.mockReturnValue(new Promise(() => undefined));
      const attempt = exchangeCode('code', 'state');
      const assertion = expect(attempt).rejects.toMatchObject({
        name: 'OAuthExchangeError', kind: 'transient', message: 'Token request timed out after 3000ms',
      });
      await vi.advanceTimersByTimeAsync(3_000);
      await assertion;
    } finally {
      vi.useRealTimers();
    }
  });

  test('ignores a late exchange response once the deadline has already rejected', async () => {
    vi.useFakeTimers();
    try {
      let resolveExchange: (value: {accessToken: string; idToken: null; returnTo: string}) => void = () => undefined;
      mocks.exchange.mockReturnValue(new Promise(resolve => {
        resolveExchange = resolve;
      }));
      const attempt = exchangeCode('code', 'state');
      const assertion = expect(attempt).rejects.toMatchObject({kind: 'transient'});
      await vi.advanceTimersByTimeAsync(3_000);
      await assertion;
      // The underlying request finally settles after we already gave up on
      // it — this must not resolve a second, already-rejected promise.
      resolveExchange({accessToken: 'late', idToken: null, returnTo: '/lobby'});
      await vi.advanceTimersByTimeAsync(0);
    } finally {
      vi.useRealTimers();
    }
  });

  test.each([
    ['a PKCE state mismatch', new Error('OAuth state mismatch'), 'invalid'],
    ['an expired/used code (4xx)', new Error('Token exchange failed (400): invalid_grant'), 'invalid'],
    ['an IdP outage (5xx)', new Error('Token exchange failed (503): bad gateway'), 'unavailable'],
    ['a rejected fetch with no status', new TypeError('Failed to fetch'), 'transient'],
    ['a non-Error rejection', 'boom', 'transient'],
  ] as const)('classifies %s as %s', async (_label, error, kind) => {
    mocks.exchange.mockRejectedValue(error);
    const failure = await exchangeCode('code', 'state').catch(e => e);
    expect(failure).toBeInstanceOf(OAuthExchangeError);
    expect(failure.kind).toBe(kind);
  });

  test('ignores a late exchange rejection once the deadline has already rejected', async () => {
    vi.useFakeTimers();
    try {
      let rejectExchange: (error: unknown) => void = () => undefined;
      mocks.exchange.mockReturnValue(new Promise((_resolve, reject) => {
        rejectExchange = reject;
      }));
      const attempt = exchangeCode('code', 'state');
      const assertion = expect(attempt).rejects.toMatchObject({kind: 'transient'});
      await vi.advanceTimersByTimeAsync(3_000);
      await assertion;
      // The underlying request finally rejects after we already gave up on
      // it — this must not reject an already-settled promise a second time.
      rejectExchange(new Error('too late'));
      await vi.advanceTimersByTimeAsync(0);
    } finally {
      vi.useRealTimers();
    }
  });

  test('revokes before redirecting logout and supports its safe default', async () => {
    const order: string[] = [];
    mocks.revoke.mockImplementation(async () => {
      order.push('revoke');
    });
    mocks.endSession.mockImplementation(() => {
      order.push('redirect');
    });
    await logout();
    expect(order).toEqual(['revoke', 'redirect']);
    expect(mocks.endSession).toHaveBeenCalledWith('/');
  });
});
