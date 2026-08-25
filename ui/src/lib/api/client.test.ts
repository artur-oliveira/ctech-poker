import {beforeEach, describe, expect, test, vi} from 'vitest';
import {
  getAccessToken,
  getPlayerId,
  getUsername,
  ApiError,
  httpRetryDelay,
  isNotFound,
  normalizeApiError,
  redirectOnServiceUnavailable,
  setAccessToken,
  setPlayerId,
  setUsername,
  subscribeAccessToken,
  subscribePlayerId,
  subscribeUsername,
} from './client';

const mocks = vi.hoisted(() => {
  const requestHandlers: Array<(config: Record<string, unknown>) => unknown> = [];
  const responseHandlers: Array<(value: unknown) => unknown> = [];
  const errorHandlers: Array<(error: Record<string, unknown>) => unknown> = [];
  const instance = {
    interceptors: {
      request: {use: vi.fn((handler) => requestHandlers.push(handler))},
      response: {
        use: vi.fn((ok, fail) => {
          responseHandlers.push(ok);
          errorHandlers.push(fail);
        })
      },
    },
    request: vi.fn(),
  };
  return {
    instance,
    requestHandlers,
    responseHandlers,
    errorHandlers,
    refresh: vi.fn(),
    endSession: vi.fn(),
    healthCheck: vi.fn(),
    requireLiveness: vi.fn(),
    notify: vi.fn(),
    isAxiosError: vi.fn(),
  };
});

vi.mock('axios', () => ({
  default: {
    create: vi.fn(() => mocks.instance),
    isAxiosError: mocks.isAxiosError,
  },
}));
vi.mock('@/lib/auth/oauth', () => ({doRefresh: mocks.refresh, endSession: mocks.endSession}));
vi.mock('@/lib/mockConfig', () => ({USE_MOCK: false}));
vi.mock('@/lib/notify', () => ({notifyApiError: mocks.notify}));
vi.mock('@/lib/network/liveness', () => ({
  HTTP_TIMEOUT_MS: 3_000,
  requireApiLiveness: mocks.requireLiveness,
  checkApiLiveness: mocks.healthCheck,
}));

describe('API client session and interceptors', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.instance.request.mockReset();
    mocks.requireLiveness.mockResolvedValue(undefined);
    mocks.healthCheck.mockResolvedValue(true);
    setAccessToken(null);
    setUsername(null);
    setPlayerId(null);
  });
  
  test('stores identity fields, notifies subscribers, and unsubscribes cleanly', () => {
    const tokenListener = vi.fn();
    const usernameListener = vi.fn();
    const playerListener = vi.fn();
    const unsubscribeToken = subscribeAccessToken(tokenListener);
    const unsubscribeUsername = subscribeUsername(usernameListener);
    const unsubscribePlayer = subscribePlayerId(playerListener);
    
    setAccessToken('token-1');
    setUsername('Ana');
    setPlayerId('player-1');
    expect([getAccessToken(), getUsername(), getPlayerId()]).toEqual(['token-1', 'Ana', 'player-1']);
    expect(tokenListener).toHaveBeenCalledWith('token-1');
    expect(usernameListener).toHaveBeenCalledWith('Ana');
    expect(playerListener).toHaveBeenCalledWith('player-1');
    
    unsubscribeToken();
    unsubscribeUsername();
    unsubscribePlayer();
    setAccessToken('token-2');
    setUsername('Bia');
    setPlayerId('player-2');
    expect(tokenListener).toHaveBeenCalledOnce();
    expect(usernameListener).toHaveBeenCalledOnce();
    expect(playerListener).toHaveBeenCalledOnce();
  });
  
  test('gates an anonymous request behind one session check, then adds the bearer token once resolved', async () => {
    const applyRequest = mocks.requestHandlers[0];
    mocks.refresh.mockResolvedValueOnce(null);
    const anonymous = {headers: {}};
    await expect(applyRequest(anonymous)).resolves.toBe(anonymous);
    expect(anonymous.headers).toEqual({});
    expect(mocks.refresh).toHaveBeenCalledOnce();

    // Confirmed guest: a second anonymous request must not re-check.
    const stillAnonymous = {headers: {}};
    await expect(applyRequest(stillAnonymous)).resolves.toBe(stillAnonymous);
    expect(mocks.refresh).toHaveBeenCalledOnce();

    setAccessToken('access');
    const authenticated = {headers: {} as Record<string, string>};
    await expect(applyRequest(authenticated)).resolves.toBe(authenticated);
    expect(authenticated.headers.Authorization).toBe('Bearer access');
  });
  
  test('passes successful responses through and recognizes only Axios 404 errors', () => {
    const response = {data: {ok: true}};
    expect(mocks.responseHandlers[0](response)).toBe(response);
    mocks.isAxiosError.mockReturnValueOnce(true).mockReturnValueOnce(true).mockReturnValue(false);
    expect(isNotFound({response: {status: 404}})).toBe(true);
    expect(isNotFound({response: {status: 500}})).toBe(false);
    expect(isNotFound(new Error('404'))).toBe(false);
  });
  

  test('normalizes an already-typed error, a non-Axios failure and an API problem document', () => {
    const typed = new ApiError('boom', 400);
    expect(normalizeApiError(typed)).toBe(typed);

    mocks.isAxiosError.mockReturnValue(false);
    const raw = new Error('socket hang up');
    const wrapped = normalizeApiError(raw);
    expect(wrapped).toBeInstanceOf(ApiError);
    expect(wrapped.message).toBe('Unexpected client error');
    expect(wrapped.original).toBe(raw);
  });

  test('keeps the problem detail, status and a usable Retry-After', () => {
    mocks.isAxiosError.mockReturnValue(true);
    const error = normalizeApiError({
      response: {status: 429, data: {detail: 'Muitas tentativas', title: 'Rate limited'}, headers: {'retry-after': '2'}},
      message: 'Request failed',
    });
    expect(error).toMatchObject({message: 'Muitas tentativas', status: 429, retryAfterMs: 2000});
  });

  test('uses full jitter for transient retries and adds small jitter after Retry-After', () => {
    expect(httpRetryDelay(1, undefined, () => 0)).toBe(0);
    expect(httpRetryDelay(2, undefined, () => 1)).toBe(500);
    expect(httpRetryDelay(1, '2', () => 0.5)).toBe(2125);
  });

  test('retries a safe transient request with the same config before surfacing it', async () => {
    vi.spyOn(Math, 'random').mockReturnValue(0);
    const config = {method: 'get', headers: {}, url: '/rooms'};
    const error = {response: {status: 503, headers: {}}, config};
    mocks.instance.request.mockResolvedValue({data: 'recovered'});
    await expect(mocks.errorHandlers[0](error)).resolves.toEqual({data: 'recovered'});
    expect(config).toMatchObject({_networkRetryCount: 1});
    expect(mocks.instance.request).toHaveBeenCalledWith(config);
  });

  test('does not automatically repeat a mutation without an idempotency key', async () => {
    mocks.isAxiosError.mockReturnValue(true);
    const error = {response: {status: 500, data: {}, headers: {}}, config: {method: 'post', headers: {}}};
    await expect(mocks.errorHandlers[0](error)).rejects.toMatchObject({status: 500});
    expect(mocks.instance.request).not.toHaveBeenCalled();
  });

  test('does not retry the failed endpoint when the health probe says the server is down', async () => {
    mocks.healthCheck.mockResolvedValue(false);
    mocks.isAxiosError.mockReturnValue(true);
    const error = {message: 'Network Error', request: {}, config: {method: 'get', headers: {}}};
    await expect(mocks.errorHandlers[0](error)).rejects.toMatchObject({message: 'Network Error'});
    expect(mocks.healthCheck).toHaveBeenCalledOnce();
    expect(mocks.instance.request).not.toHaveBeenCalled();
  });

  test('retries an idempotent mutation but stops at the shared cap', async () => {
    vi.spyOn(Math, 'random').mockReturnValue(0);
    const config = {method: 'post', headers: {'Idempotency-Key': 'key-1'}};
    mocks.instance.request.mockResolvedValue({data: 'ok'});
    await expect(mocks.errorHandlers[0]({response: {status: 500, headers: {}}, config})).resolves.toEqual({data: 'ok'});
    expect(config).toMatchObject({_networkRetryCount: 1});

    mocks.isAxiosError.mockReturnValue(true);
    const exhausted = {response: {status: 500, data: {}, headers: {}}, config: {...config, _networkRetryCount: 2}};
    await expect(mocks.errorHandlers[0](exhausted)).rejects.toMatchObject({status: 500});
  });

  test('falls back through title, axios message and a generic label, ignoring a bad Retry-After', () => {
    mocks.isAxiosError.mockReturnValue(true);
    expect(normalizeApiError({response: {status: 500, data: {title: 'Server error'}, headers: {}}}))
      .toMatchObject({message: 'Server error', retryAfterMs: undefined});
    expect(normalizeApiError({message: 'Network Error'})).toMatchObject({message: 'Network Error'});
    expect(normalizeApiError({response: {status: 503, headers: {'retry-after': 'later'}}}))
      .toMatchObject({message: 'API request failed', retryAfterMs: undefined});
  });

  test('sends the player to the maintenance page exactly once on a 503', () => {
    const replace = vi.fn();
    Object.defineProperty(window, 'location', {
      configurable: true, value: {pathname: '/lobby', replace},
    });
    expect(redirectOnServiceUnavailable(503)).toBe(true);
    expect(replace).toHaveBeenCalledWith('/unavailable');

    expect(redirectOnServiceUnavailable(500)).toBe(false);
    Object.defineProperty(window, 'location', {
      configurable: true, value: {pathname: '/unavailable', replace},
    });
    expect(redirectOnServiceUnavailable(503)).toBe(false);
    expect(replace).toHaveBeenCalledOnce();
  });

  test('refreshes once after 401 and retries with the new identity', async () => {
    const config = {headers: {}, url: '/protected'};
    const error = {response: {status: 401}, config};
    mocks.refresh.mockResolvedValue({accessToken: 'fresh', username: 'Carla'});
    mocks.instance.request.mockResolvedValue({data: 'retried'});
    
    await expect(mocks.errorHandlers[0](error)).resolves.toEqual({data: 'retried'});
    expect(config).toMatchObject({_retried: true, headers: {Authorization: 'Bearer fresh'}});
    expect(getAccessToken()).toBe('fresh');
    expect(getUsername()).toBe('Carla');
    expect(mocks.instance.request).toHaveBeenCalledWith(config);
    expect(mocks.notify).not.toHaveBeenCalled();
  });
  
  test('rejects and reports failures when refresh is unavailable, without recursive retry', async () => {
    const error = {response: {status: 401}, config: {headers: {}}};
    mocks.refresh.mockResolvedValue(null);
    await expect(mocks.errorHandlers[0](error)).rejects.toMatchObject({name: 'ApiError', original: error});
    expect(mocks.endSession).toHaveBeenCalledWith('/');
    expect(mocks.notify).toHaveBeenCalledWith(expect.objectContaining({name: 'ApiError', original: error}));
    
    const alreadyRetried = {response: {status: 401}, config: {_retried: true}};
    await expect(mocks.errorHandlers[0](alreadyRetried)).rejects.toMatchObject({
      name: 'ApiError',
      original: alreadyRetried
    });
    expect(mocks.refresh).toHaveBeenCalledOnce();
  });
  
  test('honors silentError even for errors without a response', async () => {
    const silent = {config: {method: 'post', silentError: true}};
    await expect(mocks.errorHandlers[0](silent)).rejects.toMatchObject({name: 'ApiError', original: silent});
    expect(mocks.notify).not.toHaveBeenCalled();
  });
});
