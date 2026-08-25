import {render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {UnavailableState} from './UnavailableState';

const mocks = vi.hoisted(() => ({check: vi.fn(), replace: vi.fn()}));
vi.mock('@/lib/network/liveness', () => ({
  HTTP_TIMEOUT_MS: 3_000,
  requireApiLiveness: vi.fn().mockResolvedValue(undefined),
  checkApiLiveness: mocks.check,
}));

describe('UnavailableState', () => {
  beforeEach(() => {
    vi.clearAllMocks();
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

  test('stays put when the liveness probe still fails', async () => {
    mocks.check.mockResolvedValue(false);
    render(<UnavailableState/>);
    await userEvent.click(screen.getByRole('button', {name: /Tentar novamente/}));
    expect(mocks.check).toHaveBeenCalledOnce();
    expect(mocks.replace).not.toHaveBeenCalled();
    expect(screen.getByText(/verificar novamente/)).toBeInTheDocument();
  });

  test('returns to the interrupted route only after health recovers', async () => {
    window.sessionStorage.setItem('poker:return-after-outage', '/table?id=room-1');
    mocks.check.mockResolvedValue(true);
    render(<UnavailableState/>);
    await userEvent.click(screen.getByRole('button', {name: /Tentar novamente/}));
    await waitFor(() => expect(mocks.replace).toHaveBeenCalledWith('/table?id=room-1'));
    expect(window.sessionStorage.getItem('poker:return-after-outage')).toBeNull();
  });

  test('prevents duplicate health checks while one is pending', async () => {
    let resolve: (available: boolean) => void = () => undefined;
    mocks.check.mockReturnValue(new Promise<boolean>(done => {
      resolve = done;
    }));
    render(<UnavailableState/>);
    const button = screen.getByRole('button', {name: /Tentar novamente/});
    await userEvent.click(button);
    await userEvent.click(button);
    expect(mocks.check).toHaveBeenCalledOnce();
    expect(screen.getByText('Verificando se o serviço já voltou…')).toBeInTheDocument();
    resolve(false);
  });
});
