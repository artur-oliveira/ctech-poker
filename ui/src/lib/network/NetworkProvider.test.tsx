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
    refetchQueries,
    queryClient: {refetchQueries},
  };
});

vi.mock('@tanstack/react-query', () => ({useQueryClient: () => mocks.queryClient}));

vi.mock('./liveness', () => ({
  HTTP_TIMEOUT_MS: 3_000,
  requireApiLiveness: vi.fn().mockResolvedValue(undefined),
  checkApiLiveness: mocks.check,
  getApiLivenessSnapshot: () => mocks.snapshot,
  getServerApiLivenessSnapshot: () => ({status: 'checking', reason: null, checkedAt: null}),
  livenessPollDelay: () => 30_000,
  markApiOffline: mocks.offline,
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
});
