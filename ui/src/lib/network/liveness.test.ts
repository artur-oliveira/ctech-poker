import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import {
  checkApiLiveness,
  getApiLivenessSnapshot,
  livenessPollDelay,
  markApiOffline,
  navigateToUnavailable,
  requireApiLiveness,
  resetApiLivenessForTests,
  subscribeApiLiveness,
} from './liveness';

describe('API liveness', () => {
  beforeEach(() => {
    resetApiLivenessForTests();
    vi.stubGlobal('fetch', vi.fn());
    Object.defineProperty(navigator, 'onLine', {configurable: true, value: true});
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  test('marks an OK health response available and shares concurrent probes', async () => {
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
    expect(listener).toHaveBeenCalledOnce();
    unsubscribe();
  });

  test.each([
    ['a non-OK response', () => vi.mocked(fetch).mockResolvedValue({ok: false} as Response)],
    ['a CORS-shaped rejection', () => vi.mocked(fetch).mockRejectedValue(new TypeError('Failed to fetch'))],
  ])('treats %s as server unavailability only after exhausting retries', async (_label, arrange) => {
    vi.useFakeTimers();
    arrange();
    const probe = checkApiLiveness();
    await vi.advanceTimersByTimeAsync(2_000);
    await expect(probe).resolves.toBe(false);
    // One initial attempt plus two retries before the outage is published.
    expect(fetch).toHaveBeenCalledTimes(3);
    expect(getApiLivenessSnapshot()).toMatchObject({status: 'unavailable', reason: 'server'});
    await expect(requireApiLiveness()).rejects.toThrow('Poker API is unavailable');
  });

  test('does not publish an outage while retries are still pending', async () => {
    vi.useFakeTimers();
    vi.mocked(fetch).mockResolvedValue({ok: false} as Response);
    const probe = checkApiLiveness();
    await vi.advanceTimersByTimeAsync(0); // first attempt resolved, backoff armed
    expect(getApiLivenessSnapshot().status).toBe('checking');
    await vi.advanceTimersByTimeAsync(2_000);
    await probe;
    expect(getApiLivenessSnapshot().status).toBe('unavailable');
  });

  test('recovers without an outage when a later retry succeeds', async () => {
    vi.useFakeTimers();
    vi.mocked(fetch)
      .mockRejectedValueOnce(new TypeError('Failed to fetch'))
      .mockResolvedValueOnce({ok: true} as Response);
    const probe = checkApiLiveness();
    await vi.advanceTimersByTimeAsync(2_000);
    await expect(probe).resolves.toBe(true);
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(getApiLivenessSnapshot()).toMatchObject({status: 'available', reason: null});
  });

  test('stops retrying and reports offline if the browser drops mid-retry', async () => {
    vi.useFakeTimers();
    vi.mocked(fetch).mockResolvedValue({ok: false} as Response);
    const probe = checkApiLiveness();
    await vi.advanceTimersByTimeAsync(0);
    Object.defineProperty(navigator, 'onLine', {configurable: true, value: false});
    await vi.advanceTimersByTimeAsync(2_000);
    await expect(probe).resolves.toBe(false);
    expect(fetch).toHaveBeenCalledOnce();
    expect(getApiLivenessSnapshot()).toMatchObject({status: 'unavailable', reason: 'offline'});
  });

  test('does not call the server while the browser reports offline', async () => {
    Object.defineProperty(navigator, 'onLine', {configurable: true, value: false});
    await expect(checkApiLiveness()).resolves.toBe(false);
    expect(fetch).not.toHaveBeenCalled();
    expect(getApiLivenessSnapshot()).toMatchObject({status: 'unavailable', reason: 'offline'});
  });

  test('aborts each health request after three seconds and gives up after every retry', async () => {
    vi.useFakeTimers();
    vi.mocked(fetch).mockImplementation((_url, init) => new Promise((_resolve, reject) => {
      init?.signal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')));
    }));
    const probe = checkApiLiveness();
    await vi.advanceTimersByTimeAsync(3 * 3_000 + 2 * 1_000);
    await expect(probe).resolves.toBe(false);
    expect(fetch).toHaveBeenCalledTimes(3);
    expect(getApiLivenessSnapshot().status).toBe('unavailable');
  });

  test('requires the initial probe, passes when available, and publishes explicit offline state', async () => {
    vi.mocked(fetch).mockResolvedValue({ok: true} as Response);
    await expect(requireApiLiveness()).resolves.toBeUndefined();
    await expect(requireApiLiveness()).resolves.toBeUndefined();
    expect(fetch).toHaveBeenCalledOnce();
    markApiOffline();
    await expect(requireApiLiveness()).rejects.toThrow('Device is offline');
  });

  test('uses bounded equal jitter for outage polling', () => {
    expect(livenessPollDelay(0, () => 0)).toBe(30_000);
    expect(livenessPollDelay(1, () => 0)).toBe(500);
    expect(livenessPollDelay(1, () => 1)).toBe(1_000);
    expect(livenessPollDelay(20, () => 1)).toBe(30_000);
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

    test('saves the interrupted route and navigates to the maintenance page', () => {
      expect(navigateToUnavailable()).toBe(true);
      expect(replace).toHaveBeenCalledWith('/unavailable');
      expect(window.sessionStorage.getItem('poker:return-after-outage')).toBe('/table?id=room-1');
    });

    test('is a no-op when already on the maintenance page', () => {
      Object.defineProperty(window, 'location', {
        configurable: true, value: {pathname: '/unavailable', search: '', replace},
      });
      expect(navigateToUnavailable()).toBe(false);
      expect(replace).not.toHaveBeenCalled();
    });

    test('still navigates when sessionStorage throws', () => {
      const setItem = vi.spyOn(window.sessionStorage.__proto__, 'setItem').mockImplementation(() => {
        throw new Error('storage disabled');
      });
      expect(navigateToUnavailable()).toBe(true);
      expect(replace).toHaveBeenCalledWith('/unavailable');
      setItem.mockRestore();
    });
  });
});
