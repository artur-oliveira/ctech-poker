import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';

// The probe is a module singleton, so each test loads a fresh copy.
async function load() {
  vi.resetModules();
  return import('./liveness');
}

describe('API liveness', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
    Object.defineProperty(navigator, 'onLine', {configurable: true, value: true});
  });

  afterEach(() => vi.unstubAllGlobals());

  test('starts as checking, marks an OK health response available and shares concurrent probes', async () => {
    const {checkApiLiveness, getApiLivenessSnapshot, subscribeApiLiveness} = await load();
    expect(getApiLivenessSnapshot().status).toBe('checking');
    let resolveFetch: (value: Response) => void = () => undefined;
    vi.mocked(fetch).mockReturnValue(new Promise(resolve => {
      resolveFetch = resolve;
    }));
    const listener = vi.fn();
    const unsubscribe = subscribeApiLiveness(listener);
    const first = checkApiLiveness();
    const second = checkApiLiveness();
    expect(fetch).toHaveBeenCalledOnce();
    resolveFetch({ok: true} as Response);
    await expect(Promise.all([first, second])).resolves.toEqual([true, true]);
    expect(getApiLivenessSnapshot()).toMatchObject({status: 'available', reason: null});
    expect(getApiLivenessSnapshot()).toBe(getApiLivenessSnapshot());
    expect(listener).toHaveBeenCalledOnce();
    unsubscribe();
  });

  test.each([
    ['a non-OK response', () => vi.mocked(fetch).mockResolvedValue({ok: false} as Response)],
    ['a CORS-shaped rejection', () => vi.mocked(fetch).mockRejectedValue(new TypeError('Failed to fetch'))],
  ])('treats %s as server unavailability only after three consecutive failures', async (_label, arrange) => {
    const {checkApiLiveness, getApiLivenessSnapshot, requireApiLiveness} = await load();
    arrange();
    await expect(checkApiLiveness()).resolves.toBe(false);
    await expect(checkApiLiveness()).resolves.toBe(false);
    // Two failures are a blip, not an outage: calls are still let through.
    expect(getApiLivenessSnapshot().status).toBe('available');
    await expect(checkApiLiveness()).resolves.toBe(false);
    expect(fetch).toHaveBeenCalledTimes(3);
    expect(getApiLivenessSnapshot()).toMatchObject({status: 'unavailable', reason: 'server'});
    await expect(requireApiLiveness()).rejects.toThrow('Poker API is unavailable');
  });

  test('recovers without an outage when a later probe succeeds', async () => {
    const {checkApiLiveness, getApiLivenessSnapshot} = await load();
    vi.mocked(fetch)
      .mockRejectedValueOnce(new TypeError('Failed to fetch'))
      .mockResolvedValueOnce({ok: true} as Response);
    await checkApiLiveness();
    await expect(checkApiLiveness()).resolves.toBe(true);
    expect(getApiLivenessSnapshot()).toMatchObject({status: 'available', reason: null});
  });

  test('aborts a health request after three seconds', async () => {
    vi.useFakeTimers();
    try {
      const {checkApiLiveness, HTTP_TIMEOUT_MS} = await load();
      vi.mocked(fetch).mockImplementation((_url, init) => new Promise((_resolve, reject) => {
        init?.signal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')));
      }));
      const probe = checkApiLiveness();
      await vi.advanceTimersByTimeAsync(HTTP_TIMEOUT_MS);
      await expect(probe).resolves.toBe(false);
      expect(HTTP_TIMEOUT_MS).toBe(3_000);
    } finally {
      vi.useRealTimers();
    }
  });

  test('does not call the server while the browser reports offline, and never escalates it', async () => {
    const {checkApiLiveness, getApiLivenessSnapshot, requireApiLiveness} = await load();
    Object.defineProperty(navigator, 'onLine', {configurable: true, value: false});
    for (let i = 0; i < 5; i++) await expect(checkApiLiveness()).resolves.toBe(false);
    expect(fetch).not.toHaveBeenCalled();
    expect(getApiLivenessSnapshot()).toMatchObject({status: 'unavailable', reason: 'offline'});
    await expect(requireApiLiveness()).rejects.toThrow('Device is offline');
  });

  test('requires the initial probe once, then passes while available', async () => {
    const {requireApiLiveness} = await load();
    vi.mocked(fetch).mockResolvedValue({ok: true} as Response);
    await expect(requireApiLiveness()).resolves.toBeUndefined();
    await expect(requireApiLiveness()).resolves.toBeUndefined();
    expect(fetch).toHaveBeenCalledOnce();
  });

  describe('navigateToUnavailable', () => {
    const replace = vi.fn();

    beforeEach(() => {
      replace.mockClear();
      window.sessionStorage.clear();
      Object.defineProperty(window, 'location', {
        configurable: true, value: {pathname: '/table', search: '?id=room-1', replace},
      });
    });

    test('saves the interrupted route and navigates to the maintenance page', async () => {
      const {navigateToUnavailable} = await load();
      expect(navigateToUnavailable()).toBe(true);
      expect(replace).toHaveBeenCalledWith('/unavailable');
      expect(window.sessionStorage.getItem('poker:return-after-outage')).toBe('/table?id=room-1');
    });

    test('is a no-op when already on the maintenance page', async () => {
      const {navigateToUnavailable} = await load();
      Object.defineProperty(window, 'location', {
        configurable: true, value: {pathname: '/unavailable', search: '', replace},
      });
      expect(navigateToUnavailable()).toBe(false);
      expect(replace).not.toHaveBeenCalled();
    });

    test('still navigates when sessionStorage throws', async () => {
      const {navigateToUnavailable} = await load();
      const setItem = vi.spyOn(window.sessionStorage.__proto__, 'setItem').mockImplementation(() => {
        throw new Error('storage disabled');
      });
      expect(navigateToUnavailable()).toBe(true);
      expect(replace).toHaveBeenCalledWith('/unavailable');
      setItem.mockRestore();
    });
  });
});
