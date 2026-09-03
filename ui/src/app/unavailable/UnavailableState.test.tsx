import {render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {UnavailableState} from './UnavailableState';

const mocks = vi.hoisted(() => ({check: vi.fn(), replace: vi.fn(), delay: vi.fn()}));
vi.mock('@/lib/network/liveness', () => ({
  HTTP_TIMEOUT_MS: 3_000,
  requireApiLiveness: vi.fn().mockResolvedValue(undefined),
  checkApiLiveness: mocks.check,
  livenessPollDelay: mocks.delay,
}));

describe('UnavailableState', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Long enough that the automatic retry never fires inside a test.
    mocks.delay.mockReturnValue(60_000);
    window.sessionStorage.clear();
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: {
        origin: 'http://localhost:3000',
        href: 'http://localhost:3000/unavailable',
        pathname: '/unavailable',
        search: '',
        replace: mocks.replace,
      },
    });
  });

  // Issue #105: a failed check used to leave the screen exactly as it was.
  test('reports that a check ran and failed, with a timestamp and the next-retry countdown', async () => {
    mocks.check.mockResolvedValue(false);
    render(<UnavailableState/>);

    expect(screen.getByText('Verificando se o serviço já voltou…')).toBeInTheDocument();
    const detail = await screen.findByText(/Ainda fora do ar/);
    expect(detail).toHaveTextContent(/última verificação às \d{2}:\d{2}:\d{2}/);
    expect(detail).toHaveTextContent('nova tentativa automática em 60s');
    expect(detail).toHaveAttribute('role', 'status');
    expect(mocks.replace).not.toHaveBeenCalled();
  });

  test('states the failure again after a manual retry', async () => {
    mocks.check.mockResolvedValue(false);
    render(<UnavailableState/>);
    await screen.findByText(/Ainda fora do ar/);

    await userEvent.click(screen.getByRole('button', {name: /Tentar novamente/}));
    await waitFor(() => expect(mocks.check).toHaveBeenCalledTimes(2));
    expect(await screen.findByText(/Ainda fora do ar/)).toBeInTheDocument();
  });

  test('returns to the interrupted route on background recovery, with no tap', async () => {
    window.sessionStorage.setItem('poker:return-after-outage', '/table?id=room-1');
    mocks.check.mockResolvedValue(true);
    render(<UnavailableState/>);

    await waitFor(() => expect(mocks.replace).toHaveBeenCalledWith('/table?id=room-1'));
    expect(window.sessionStorage.getItem('poker:return-after-outage')).toBeNull();
  });

  test('falls back to the lobby when no route was saved', async () => {
    mocks.check.mockResolvedValue(true);
    render(<UnavailableState/>);
    await waitFor(() => expect(mocks.replace).toHaveBeenCalledWith('/lobby'));
  });

  test('prevents duplicate health checks while one is pending', async () => {
    let resolve: (available: boolean) => void = () => undefined;
    mocks.check.mockReturnValue(new Promise<boolean>(done => {
      resolve = done;
    }));
    render(<UnavailableState/>);

    const button = screen.getByRole('button', {name: /Verificando/});
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute('aria-busy', 'true');
    await userEvent.click(button);
    expect(mocks.check).toHaveBeenCalledOnce();
    resolve(false);
    await screen.findByText(/Ainda fora do ar/);
  });
});
