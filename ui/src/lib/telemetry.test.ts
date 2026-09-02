import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import {reportClientError} from './telemetry';

describe('reportClientError', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ok: true}));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  test('beacons a compact payload with a correlation id', () => {
    const id = reportClientError('boom', {stack: 'at x', context: {kind: 'transient'}});
    expect(id).toEqual(expect.any(String));
    expect(fetch).toHaveBeenCalledWith(expect.stringContaining('/v1.0/client-errors'), expect.objectContaining({method: 'POST'}));
    const [, init] = vi.mocked(fetch).mock.calls[0];
    const body = JSON.parse(init?.body as string);
    expect(body).toMatchObject({message: 'boom', stack: 'at x', correlationId: id, context: {kind: 'transient'}});
  });

  test('never throws when the beacon rejects', () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('network down')));
    expect(() => reportClientError('boom')).not.toThrow();
  });

  test('still returns a correlation id when fetch is unavailable', () => {
    vi.stubGlobal('fetch', undefined);
    const id = reportClientError('boom');
    expect(id).toEqual(expect.any(String));
  });
});
