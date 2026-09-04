import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import {checkApiLiveness, getApiLivenessSnapshot, resetApiLivenessForTests} from './liveness';

// Separate file from liveness.test.ts on purpose: USE_MOCK is read at module
// scope, so the two modes cannot share one module registry.
vi.mock('@/lib/mockConfig', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/mockConfig')>(),
  USE_MOCK: true,
}));

describe('API liveness in mock mode', () => {
  beforeEach(() => {
    resetApiLivenessForTests();
    vi.stubGlobal('fetch', vi.fn());
    Object.defineProperty(navigator, 'onLine', {configurable: true, value: true});
  });

  afterEach(() => vi.unstubAllGlobals());

  // `npm run dev:mock` has no API behind the dev proxy, so an unguarded probe
  // failed three times and escalated the whole app to /unavailable within
  // seconds — taking `og:capture` and the browser test suite with it.
  test('reports available without ever reaching the network', async () => {
    await expect(checkApiLiveness()).resolves.toBe(true);
    expect(fetch).not.toHaveBeenCalled();
    expect(getApiLivenessSnapshot()).toMatchObject({status: 'available', reason: null});
  });

  test('still reports a device that is offline, so the offline banner survives mock mode', async () => {
    Object.defineProperty(navigator, 'onLine', {configurable: true, value: false});
    await expect(checkApiLiveness()).resolves.toBe(false);
    expect(getApiLivenessSnapshot()).toMatchObject({status: 'unavailable', reason: 'offline'});
  });
});
