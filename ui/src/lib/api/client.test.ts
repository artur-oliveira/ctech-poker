import {beforeEach, describe, expect, test, vi} from 'vitest';
import {
  getAccessToken,
  getPlayerId,
  getUsername,
  isNotFound,
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
vi.mock('@/lib/auth/oauth', () => ({doRefresh: mocks.refresh}));
vi.mock('@/lib/mockConfig', () => ({USE_MOCK: false}));
vi.mock('@/lib/notify', () => ({notifyApiError: mocks.notify}));

describe('API client session and interceptors', () => {
  beforeEach(() => {
    vi.clearAllMocks();
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
  
  test('adds the current bearer token only when one exists', () => {
    const applyRequest = mocks.requestHandlers[0];
    const anonymous = {headers: {}};
    expect(applyRequest(anonymous)).toBe(anonymous);
    expect(anonymous.headers).toEqual({});
    
    setAccessToken('access');
    const authenticated = {headers: {} as Record<string, string>};
    expect(applyRequest(authenticated)).toBe(authenticated);
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
    expect(mocks.notify).toHaveBeenCalledWith(expect.objectContaining({name: 'ApiError', original: error}));
    
    const alreadyRetried = {response: {status: 401}, config: {_retried: true}};
    await expect(mocks.errorHandlers[0](alreadyRetried)).rejects.toMatchObject({
      name: 'ApiError',
      original: alreadyRetried
    });
    expect(mocks.refresh).toHaveBeenCalledOnce();
  });
  
  test('honors silentError even for errors without a response', async () => {
    const silent = {config: {silentError: true}};
    await expect(mocks.errorHandlers[0](silent)).rejects.toMatchObject({name: 'ApiError', original: silent});
    expect(mocks.notify).not.toHaveBeenCalled();
  });
});
