import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import {
  installGlobalErrorReporter,
  reportBoundaryError,
  reportClientError,
  resetClientErrorBudgetForTests,
} from './telemetry';

function bodyOf(call = 0) {
  const [, init] = vi.mocked(fetch).mock.calls[call];
  return JSON.parse(init?.body as string);
}

describe('reportClientError', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ok: true}));
    resetClientErrorBudgetForTests();
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

  test('carries the route and release, and never the query string', () => {
    reportClientError('boom');
    expect(bodyOf()).toMatchObject({route: '/', release: 'dev'});
    expect(bodyOf().route).not.toContain('?');
  });

  test('trims a runaway stack instead of shipping a megabyte', () => {
    reportClientError('boom', {stack: 'x'.repeat(10_000)});
    expect(bodyOf().stack).toHaveLength(2_000);
  });

  test('drops a repeat of the same message and caps the page load at ten', () => {
    reportClientError('boom');
    reportClientError('boom');
    expect(fetch).toHaveBeenCalledOnce();
    for (let i = 0; i < 20; i += 1) reportClientError(`distinct ${i}`);
    expect(fetch).toHaveBeenCalledTimes(10);
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

describe('installGlobalErrorReporter', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ok: true}));
    resetClientErrorBudgetForTests();
    installGlobalErrorReporter();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  test('reports an uncaught error', () => {
    window.dispatchEvent(new ErrorEvent('error', {
      message: 'Uncaught boom', error: new Error('Uncaught boom'),
    }));
    expect(bodyOf()).toMatchObject({message: 'Uncaught boom', context: {kind: 'uncaught'}});
    expect(bodyOf().stack).toContain('Error: Uncaught boom');
  });

  test('reports an unhandled rejection, Error or not', () => {
    const rejection = new Event('unhandledrejection') as Event & {reason?: unknown};
    rejection.reason = new Error('promise boom');
    window.dispatchEvent(rejection);
    expect(bodyOf()).toMatchObject({message: 'promise boom', context: {kind: 'unhandledrejection'}});

    const thrownString = new Event('unhandledrejection') as Event & {reason?: unknown};
    thrownString.reason = 'plain string';
    window.dispatchEvent(thrownString);
    expect(bodyOf(1)).toMatchObject({message: 'plain string'});
  });

  test('ignores a failed asset load, which is not a crash', () => {
    window.dispatchEvent(new ErrorEvent('error', {message: ''}));
    expect(fetch).not.toHaveBeenCalled();
  });

  test('installs its listeners once, however often it is called', () => {
    installGlobalErrorReporter();
    installGlobalErrorReporter();
    window.dispatchEvent(new ErrorEvent('error', {message: 'once', error: new Error('once')}));
    expect(fetch).toHaveBeenCalledOnce();
  });
});

describe('reportBoundaryError', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ok: true}));
    resetClientErrorBudgetForTests();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  test("names the boundary and keeps Next's digest for grouping", () => {
    const error = Object.assign(new Error('render blew up'), {digest: 'abc123'});
    reportBoundaryError(error, 'route');
    expect(bodyOf()).toMatchObject({
      message: 'render blew up',
      context: {kind: 'boundary', boundary: 'route', digest: 'abc123'},
    });
  });

  test('falls back to a label when the error carries no message and no digest', () => {
    reportBoundaryError(new Error(''), 'global');
    expect(bodyOf()).toMatchObject({message: 'Boundary error', context: {kind: 'boundary', boundary: 'global'}});
    expect(bodyOf().context.digest).toBeUndefined();
  });
});
