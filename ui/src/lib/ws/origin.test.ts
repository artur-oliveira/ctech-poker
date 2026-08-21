import {describe, expect, it, vi} from 'vitest';

describe('wsOrigin', () => {
  const load = async (env: Record<string, string | undefined>) => {
    vi.resetModules();
    for (const [key, value] of Object.entries(env)) {
      if (value === undefined) delete process.env[key];
      else process.env[key] = value;
    }
    return (await import('./origin')).wsOrigin();
  };

  it('prefers the explicit wss:// origin', async () => {
    expect(await load({
      NEXT_PUBLIC_WS_URL: 'wss://poker-api.aoctech.app',
      NEXT_PUBLIC_API_URL: 'https://poker-api.aoctech.app',
    })).toBe('wss://poker-api.aoctech.app');
  });

  // The fallback exists for local work only. It is what the deployed build must
  // never rely on: a derived origin appears nowhere in the build environment,
  // so the generated connect-src would not list the wss:// scheme and the
  // socket would be blocked.
  it('falls back to the API origin with the scheme swapped', async () => {
    expect(await load({
      NEXT_PUBLIC_WS_URL: undefined,
      NEXT_PUBLIC_API_URL: 'http://localhost:8003',
    })).toBe('ws://localhost:8003');
  });
});
