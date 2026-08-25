import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import {
  checkApiLiveness,
  getApiLivenessSnapshot,
  livenessPollDelay,
  markApiOffline,
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
  ])('treats %s as server unavailability', async (_label, arrange) => {
    arrange();
    await expect(checkApiLiveness()).resolves.toBe(false);
    expect(getApiLivenessSnapshot()).toMatchObject({status: 'unavailable', reason: 'server'});
    await expect(requireApiLiveness()).rejects.toThrow('Poker API is unavailable');
  });

  test('does not call the server while the browser reports offline', async () => {
    Object.defineProperty(navigator, 'onLine', {configurable: true, value: false});
    await expect(checkApiLiveness()).resolves.toBe(false);
    expect(fetch).not.toHaveBeenCalled();
    expect(getApiLivenessSnapshot()).toMatchObject({status: 'unavailable', reason: 'offline'});
  });

  test('aborts a health request after three seconds', async () => {
    vi.useFakeTimers();
    vi.mocked(fetch).mockImplementation((_url, init) => new Promise((_resolve, reject) => {
      init?.signal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')));
    }));
    const probe = checkApiLiveness();
    await vi.advanceTimersByTimeAsync(3_000);
    await expect(probe).resolves.toBe(false);
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
});
