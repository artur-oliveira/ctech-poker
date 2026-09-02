import {act, render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {NetworkProvider} from './NetworkProvider';

const mocks = vi.hoisted(() => {
  const refetchQueries = vi.fn();
  return {
    snapshot: {status: 'unavailable' as 'checking' | 'available' | 'unavailable', reason: 'server' as 'server' | 'offline' | null, checkedAt: 1},
    listeners: new Set<() => void>(),
    check: vi.fn(),
    offline: vi.fn(),
    navigateToUnavailable: vi.fn(),
    refetchQueries,
    queryClient: {refetchQueries},
  };
});

vi.mock('@tanstack/react-query', () => ({useQueryClient: () => mocks.queryClient}));

vi.mock('./liveness', () => ({
  HTTP_TIMEOUT_MS: 3_000,
  SERVER_OUTAGE_ESCALATION_THRESHOLD: 2,
  requireApiLiveness: vi.fn().mockResolvedValue(undefined),
  checkApiLiveness: mocks.check,
  getApiLivenessSnapshot: () => mocks.snapshot,
  getServerApiLivenessSnapshot: () => ({status: 'checking', reason: null, checkedAt: null}),
  livenessPollDelay: () => 30_000,
  markApiOffline: mocks.offline,
  navigateToUnavailable: mocks.navigateToUnavailable,
  subscribeApiLiveness: (listener: () => void) => {
    mocks.listeners.add(listener);
    return () => mocks.listeners.delete(listener);
  },
}));

describe('NetworkProvider', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.snapshot = {status: 'unavailable', reason: 'server', checkedAt: 1};
    mocks.check.mockResolvedValue(false);
  });

  test('keeps the app mounted and offers an immediate server check', async () => {
    render(<NetworkProvider><main>Cached table</main></NetworkProvider>);
    expect(screen.getByText('Cached table')).toBeInTheDocument();
    expect(screen.getByRole('status')).toHaveTextContent('Servidor temporariamente indisponível');
    await userEvent.click(screen.getByRole('button', {name: 'Verificar agora'}));
    expect(mocks.check).toHaveBeenCalledTimes(2);
  });

  // The strip reserves its own height instead of floating over the page, so
  // navigation and the table's Lobby/Sair controls stay reachable exactly when
  // recovery matters. The height itself is CSS (--api-bar-h); this flag is what
  // switches it on.
  test('marks the document while the strip is up and clears it on recovery', () => {
    const {rerender, unmount} = render(<NetworkProvider><span>content</span></NetworkProvider>);
    expect(document.documentElement.dataset.apiOffline).toBe('true');

    mocks.snapshot = {status: 'available', reason: null, checkedAt: 2};
    act(() => mocks.listeners.forEach(listener => listener()));
    rerender(<NetworkProvider><span>content</span></NetworkProvider>);
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    expect(document.documentElement.dataset.apiOffline).toBeUndefined();
    unmount();
  });

  test('never reserves the strip on the dedicated unavailable route', () => {
    window.history.replaceState({}, '', '/unavailable');
    render(<NetworkProvider><span>content</span></NetworkProvider>);
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    expect(document.documentElement.dataset.apiOffline).toBeUndefined();
    window.history.replaceState({}, '', '/');
  });

  test('distinguishes a device outage and responds to browser recovery events', () => {
    mocks.snapshot = {status: 'unavailable', reason: 'offline', checkedAt: 1};
    render(<NetworkProvider><span>content</span></NetworkProvider>);
    expect(screen.getByRole('status')).toHaveTextContent('Você está sem internet');
    act(() => window.dispatchEvent(new Event('offline')));
    expect(mocks.offline).toHaveBeenCalledOnce();
    act(() => window.dispatchEvent(new Event('online')));
    expect(mocks.check).toHaveBeenCalledTimes(2);
  });

  test('refetches active screen data once the health probe recovers', async () => {
    mocks.check
      .mockResolvedValueOnce(false)
      .mockImplementationOnce(async () => {
        mocks.snapshot = {status: 'available', reason: null, checkedAt: 2};
        mocks.listeners.forEach(listener => listener());
        return true;
    });
    render(<NetworkProvider><span>content</span></NetworkProvider>);
    await waitFor(() => expect(mocks.check).toHaveBeenCalledOnce());
    await userEvent.click(screen.getByRole('button', {name: 'Verificar agora'}));
    await waitFor(() => expect(mocks.refetchQueries).toHaveBeenCalledWith({type: 'active'}));
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  // A dead HAProxy is more likely to surface as a rejected fetch (reason
  // 'server' from repeated failed health probes) than a clean HTTP 503, and
  // that shape must reach the same prominent outage UI, not linger forever as
  // the thin strip.
  test('escalates a persistent server-shaped outage to the full outage screen', async () => {
    vi.useFakeTimers();
    try {
      render(<NetworkProvider><span>content</span></NetworkProvider>);
      await vi.advanceTimersByTimeAsync(0);
      expect(mocks.check).toHaveBeenCalledTimes(1);
      expect(mocks.navigateToUnavailable).not.toHaveBeenCalled();
      await vi.advanceTimersByTimeAsync(30_000);
      expect(mocks.check).toHaveBeenCalledTimes(2);
      expect(mocks.navigateToUnavailable).toHaveBeenCalledOnce();
    } finally {
      vi.useRealTimers();
    }
  });

  test('never escalates a device-offline outage, no matter how long it persists', async () => {
    mocks.snapshot = {status: 'unavailable', reason: 'offline', checkedAt: 1};
    vi.useFakeTimers();
    try {
      render(<NetworkProvider><span>content</span></NetworkProvider>);
      await vi.advanceTimersByTimeAsync(0);
      await vi.advanceTimersByTimeAsync(30_000);
      await vi.advanceTimersByTimeAsync(30_000);
      expect(mocks.navigateToUnavailable).not.toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });

  test('resets the escalation count once a poll cycle recovers', async () => {
    mocks.check
      .mockResolvedValueOnce(false) // 1st consecutive server failure
      .mockImplementationOnce(async () => { // recovers before the threshold
        mocks.snapshot = {status: 'available', reason: null, checkedAt: 2};
        return true;
      })
      .mockImplementationOnce(async () => { // drops again — should be back at 1, not 2
        mocks.snapshot = {status: 'unavailable', reason: 'server', checkedAt: 3};
        return false;
      });
    vi.useFakeTimers();
    try {
      render(<NetworkProvider><span>content</span></NetworkProvider>);
      await vi.advanceTimersByTimeAsync(0);
      await vi.advanceTimersByTimeAsync(30_000);
      await vi.advanceTimersByTimeAsync(30_000);
      expect(mocks.check).toHaveBeenCalledTimes(3);
      expect(mocks.navigateToUnavailable).not.toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });
});
