import {render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';

const mocks = vi.hoisted(() => ({
  code: 'code-1',
  state: 'state-1',
  replace: vi.fn(),
  exchangeCode: vi.fn(),
  startOAuthFlow: vi.fn(),
  setAccessToken: vi.fn(),
  setUsername: vi.fn(),
}));

vi.mock('next/navigation', () => ({
  useRouter: () => ({replace: mocks.replace}),
  useSearchParams: () => ({
    get: (key: string) => key === 'code' ? mocks.code : key === 'state' ? mocks.state : null,
  }),
}));
vi.mock('@/lib/auth/oauth', () => ({
  exchangeCode: mocks.exchangeCode,
  startOAuthFlow: mocks.startOAuthFlow,
}));
vi.mock('@/lib/api/client', () => ({
  setAccessToken: mocks.setAccessToken,
  setUsername: mocks.setUsername,
}));

import CallbackPage from './page';

describe('OAuth callback page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.code = 'code-1';
    mocks.state = 'state-1';
    mocks.exchangeCode.mockResolvedValue({accessToken: 'token', username: 'Ana', returnTo: '/table?id=1'});
  });

  test('exchanges credentials, persists the session and restores the destination', async () => {
    render(<CallbackPage/>);
    expect(screen.getByText(/Autenticando sua cadeira/)).toBeInTheDocument();
    await waitFor(() => expect(mocks.replace).toHaveBeenCalledWith('/table?id=1'));
    expect(mocks.exchangeCode).toHaveBeenCalledWith('code-1', 'state-1');
    expect(mocks.setAccessToken).toHaveBeenCalledWith('token');
    expect(mocks.setUsername).toHaveBeenCalledWith('Ana');
  });

  test('uses lobby as the safe default destination', async () => {
    mocks.exchangeCode.mockResolvedValue({accessToken: 'token', username: 'Ana'});
    render(<CallbackPage/>);
    await waitFor(() => expect(mocks.replace).toHaveBeenCalledWith('/lobby'));
  });

  test('returns malformed callbacks to the landing page', async () => {
    mocks.code = '';
    render(<CallbackPage/>);
    await waitFor(() => expect(mocks.replace).toHaveBeenCalledWith('/'));
    expect(mocks.exchangeCode).not.toHaveBeenCalled();
  });

  test('offers a new OAuth flow after an expired code', async () => {
    mocks.exchangeCode.mockRejectedValue(new Error('expired'));
    render(<CallbackPage/>);
    expect(await screen.findByRole('heading', {name: 'Não foi possível autenticar'})).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Tentar novamente'}));
    expect(mocks.startOAuthFlow).toHaveBeenCalledOnce();
    expect(screen.getByRole('button', {name: 'Voltar ao início'})).toHaveAttribute('href', '/');
  });
});
