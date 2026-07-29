import {beforeEach, describe, expect, test, vi} from 'vitest';
import {decodeIdToken, doRefresh, exchangeCode, logout, startOAuthFlow} from './oauth';

const mocks = vi.hoisted(() => ({
  start: vi.fn(),
  exchange: vi.fn(),
  refresh: vi.fn(),
  revoke: vi.fn(),
  endSession: vi.fn(),
  decode: vi.fn(),
  constructor: vi.fn(),
}));

vi.mock('@aoctech/auth-client', () => ({
  OAuthClient: class {
    constructor(options: unknown) {
      mocks.constructor(options);
    }
    
    startOAuthFlow = mocks.start;
    exchangeCode = mocks.exchange;
    refresh = mocks.refresh;
    revoke = mocks.revoke;
    endSessionRedirect = mocks.endSession;
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
    expect(mocks.constructor).toHaveBeenCalledWith(expect.objectContaining({
      redirectUri: `${window.location.origin}/callback`,
      scope: 'openid profile',
    }));
    mocks.start.mockResolvedValue(undefined);
    await startOAuthFlow('/table/t1');
    expect(mocks.start).toHaveBeenCalledWith('/table/t1');
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
