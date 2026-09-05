import {render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import CallbackPage from './page';
import {OAuthExchangeError} from '@/lib/auth/oauth';

const mocks = vi.hoisted(() => ({
  code: 'code-1',
  state: 'state-1',
  replace: vi.fn(),
  exchangeCode: vi.fn(),
  startOAuthFlow: vi.fn(),
  lastReturnTo: vi.fn(),
  setAccessToken: vi.fn(),
  setUsername: vi.fn(),
  navigateToUnavailable: vi.fn(),
  reportClientError: vi.fn(),
}));

vi.mock('next/navigation', () => ({
  useRouter: () => ({replace: mocks.replace}),
  useSearchParams: () => ({
    get: (key: string) => key === 'code' ? mocks.code : key === 'state' ? mocks.state : null,
  }),
}));
vi.mock('@/lib/auth/oauth', async () => {
  const actual = await vi.importActual<typeof import('@/lib/auth/oauth')>('@/lib/auth/oauth');
  return {
    OAuthExchangeError: actual.OAuthExchangeError,
    exchangeCode: mocks.exchangeCode,
    startOAuthFlow: mocks.startOAuthFlow,
    lastReturnTo: mocks.lastReturnTo,
  };
});
vi.mock('@/lib/api/client', () => ({
  setAccessToken: mocks.setAccessToken,
  setUsername: mocks.setUsername,
}));
vi.mock('@/lib/network/liveness', () => ({
  navigateToUnavailable: mocks.navigateToUnavailable,
}));
vi.mock('@/lib/telemetry', () => ({
  reportClientError: mocks.reportClientError,
}));

describe('OAuth callback page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.code = 'code-1';
    mocks.state = 'state-1';
    mocks.reportClientError.mockReturnValue('corr-1');
    mocks.exchangeCode.mockResolvedValue({accessToken: 'token', username: 'Ana', returnTo: '/table?id=1'});
  });

  test('exchanges credentials, persists the session and restores the destination', async () => {
    render(<CallbackPage/>);
    // A route that can sit on screen for seconds has to be a landmark with a
    // heading, and has to announce the wait rather than leave silence.
    expect(screen.getByRole('main')).toBeInTheDocument();
    expect(screen.getByRole('heading', {level: 1})).toHaveTextContent('Autenticando');
    expect(screen.getByRole('status')).toHaveTextContent('Autenticando seu lugar…');
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

  test('a transient failure (network blip / deadline) offers to retry the exchange, not a full re-auth', async () => {
    mocks.exchangeCode.mockRejectedValue(new OAuthExchangeError('transient', 'Token request timed out after 3000ms'));
    render(<CallbackPage/>);
    expect(await screen.findByRole('heading', {level: 1, name: 'Não foi possível confirmar seu login'})).toBeInTheDocument();
    expect(screen.getByRole('main')).toContainElement(screen.getByRole('heading', {level: 1}));
    expect(screen.getByRole('alert')).toHaveTextContent(/instabilidade passageira/);
    expect(screen.getByText('Código de referência: corr-1')).toBeInTheDocument();
    expect(mocks.reportClientError).toHaveBeenCalledWith('OAuth callback exchange failed', expect.objectContaining({
      context: {kind: 'transient'},
    }));

    mocks.exchangeCode.mockResolvedValueOnce({accessToken: 'retried', username: 'Ana', returnTo: '/lobby'});
    await userEvent.click(screen.getByRole('button', {name: 'Tentar novamente'}));
    expect(mocks.startOAuthFlow).not.toHaveBeenCalled();
    expect(mocks.exchangeCode).toHaveBeenCalledTimes(2);
    expect(mocks.exchangeCode).toHaveBeenLastCalledWith('code-1', 'state-1');
    await waitFor(() => expect(mocks.replace).toHaveBeenCalledWith('/lobby'));
  });

  test('an unrecognized rejection (e.g. a raw network error) is treated as transient, not a hard failure', async () => {
    mocks.exchangeCode.mockRejectedValue(new TypeError('Failed to fetch'));
    render(<CallbackPage/>);
    expect(await screen.findByRole('heading', {level: 1, name: 'Não foi possível confirmar seu login'})).toBeInTheDocument();
  });

  test('an invalid code or state mismatch shows a distinct restart-sign-in message', async () => {
    mocks.exchangeCode.mockRejectedValue(new OAuthExchangeError('invalid', 'OAuth state mismatch'));
    render(<CallbackPage/>);
    expect(await screen.findByRole('heading', {level: 1, name: 'Não foi possível autenticar'})).toBeInTheDocument();
    expect(screen.getByRole('main')).toContainElement(screen.getByRole('heading', {level: 1}));
    expect(screen.getByRole('alert')).toHaveTextContent(/código de acesso expirou/);
    expect(screen.getByText('Código de referência: corr-1')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Tentar novamente'}));
    expect(mocks.startOAuthFlow).toHaveBeenCalledOnce();
    expect(screen.getByRole('button', {name: 'Voltar ao início'})).toHaveAttribute('href', '/');
  });

  test('#342: retrying after a failure resends the originally captured intent, not the /lobby default', async () => {
    mocks.lastReturnTo.mockReturnValue('/table/t1');
    mocks.exchangeCode.mockRejectedValue(new OAuthExchangeError('invalid', 'OAuth state mismatch'));
    render(<CallbackPage/>);
    await screen.findByRole('heading', {level: 1, name: 'Não foi possível autenticar'});
    await userEvent.click(screen.getByRole('button', {name: 'Tentar novamente'}));
    expect(mocks.startOAuthFlow).toHaveBeenCalledWith('/table/t1');
  });

  test('an IdP outage (5xx) navigates straight to the maintenance page instead of rendering a dead sign-in', async () => {
    mocks.exchangeCode.mockRejectedValue(new OAuthExchangeError('unavailable', 'Token exchange failed (503): bad gateway'));
    render(<CallbackPage/>);
    await waitFor(() => expect(mocks.navigateToUnavailable).toHaveBeenCalledOnce());
    expect(mocks.reportClientError).not.toHaveBeenCalled();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
});
