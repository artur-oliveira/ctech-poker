import {act, render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';

const mocks = vi.hoisted(() => ({
  token: null as string | null,
  username: null as string | null,
  query: vi.fn(),
  acceptMutation: {mutate: vi.fn(), isError: false, isPending: false},
  nameMutation: {mutate: vi.fn(), isError: false, isPending: false},
  setQueryData: vi.fn(),
  subscribe: vi.fn(),
  setAccessToken: vi.fn(),
  setUsername: vi.fn(),
  setPlayerId: vi.fn(),
  refresh: vi.fn(),
  oauth: vi.fn(),
}));

vi.mock('@tanstack/react-query', () => ({
  useQuery: mocks.query,
  useQueryClient: () => ({setQueryData: mocks.setQueryData}),
  useMutation: vi.fn(),
}));
vi.mock('@/lib/api/player', () => ({
  acceptPokerTerms: vi.fn(),
  getMe: vi.fn(),
  updateMe: vi.fn(),
}));
vi.mock('@/lib/auth/oauth', () => ({
  doRefresh: mocks.refresh,
  startOAuthFlow: mocks.oauth,
}));
vi.mock('@/lib/api/client', () => ({
  getAccessToken: () => mocks.token,
  getUsername: () => mocks.username,
  setAccessToken: mocks.setAccessToken,
  setUsername: mocks.setUsername,
  setPlayerId: mocks.setPlayerId,
  subscribeAccessToken: mocks.subscribe,
}));
vi.mock('@/lib/mock', () => ({MOCK_PLAYER_ID: 'mock-player', USE_MOCK: false}));

import {TermsGate} from './TermsGate';
import {useMutation} from '@tanstack/react-query';

function query(overrides = {}) {
  return {data: undefined, isLoading: false, isError: false, refetch: vi.fn(), ...overrides};
}

describe('TermsGate', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.token = null;
    mocks.username = null;
    mocks.refresh.mockResolvedValue(null);
    mocks.subscribe.mockImplementation(() => vi.fn());
    mocks.query.mockReturnValue(query());
    vi.mocked(useMutation)
      .mockReset()
      .mockImplementation(() =>
        (vi.mocked(useMutation).mock.calls.length % 2 ? mocks.acceptMutation : mocks.nameMutation) as never);
  });

  test('refreshes a missing session and keeps the loading gate until boot completes', async () => {
    let finish!: (value: {accessToken: string; username: string} | null) => void;
    mocks.refresh.mockReturnValue(new Promise(resolve => { finish = resolve; }));
    render(<TermsGate><div>conteúdo</div></TermsGate>);
    expect(screen.getByText(/Verificando sua conta/)).toBeInTheDocument();

    await act(async () => finish({accessToken: 'fresh-token', username: 'Ana'}));
    expect(mocks.setAccessToken).toHaveBeenCalledWith('fresh-token');
    expect(mocks.setUsername).toHaveBeenCalledWith('Ana');
    expect(await screen.findByRole('heading', {name: 'Entre para continuar'})).toBeInTheDocument();
  });

  test('starts OAuth with the current path and query when refresh cannot restore a session', async () => {
    window.history.pushState({}, '', '/lobby?invite=abc');
    render(<TermsGate><div>conteúdo</div></TermsGate>);
    await userEvent.click(await screen.findByRole('button', {name: /Entrar com CTech/}));
    expect(mocks.oauth).toHaveBeenCalledWith('/lobby?invite=abc');
  });

  test('shows profile failure and retries the query', async () => {
    mocks.token = 'token';
    const refetch = vi.fn();
    mocks.query.mockReturnValue(query({isError: true, refetch}));
    render(<TermsGate><div>conteúdo</div></TermsGate>);
    await userEvent.click(screen.getByRole('button', {name: 'Tentar novamente'}));
    expect(refetch).toHaveBeenCalledOnce();
  });

  test('requires explicit acceptance and reports pending and failed mutations', async () => {
    mocks.token = 'token';
    mocks.query.mockReturnValue(query({data: {user_id: 'p1', name: 'Ana', poker_terms_accepted: false}}));
    const view = render(<TermsGate><div>conteúdo</div></TermsGate>);
    const accept = screen.getByRole('button', {name: 'Aceitar e continuar'});
    expect(accept).toBeDisabled();
    expect(screen.getByRole('link', {name: 'Termos do CTech Poker'})).toHaveAttribute('target', '_blank');

    await userEvent.click(screen.getByRole('checkbox'));
    await userEvent.click(accept);
    expect(mocks.acceptMutation.mutate).toHaveBeenCalledOnce();
    expect(mocks.setPlayerId).toHaveBeenCalledWith('p1');

    mocks.acceptMutation.isPending = true;
    mocks.acceptMutation.isError = true;
    view.rerender(<TermsGate><div>conteúdo</div></TermsGate>);
    expect(screen.getByRole('button', {name: 'Registrando…'})).toBeDisabled();
    expect(screen.getByText('Não foi possível registrar o aceite.')).toBeInTheDocument();
    mocks.acceptMutation.isPending = false;
    mocks.acceptMutation.isError = false;
  });

  test('syncs an empty profile name once and exposes accepted content', async () => {
    mocks.token = 'token';
    mocks.username = 'Nome OIDC';
    const profile = {user_id: 'p2', name: '', poker_terms_accepted: true};
    mocks.query.mockReturnValue(query({data: profile}));
    render(<TermsGate><div>conteúdo protegido</div></TermsGate>);
    expect(screen.getByText('conteúdo protegido')).toBeInTheDocument();
    await waitFor(() => expect(mocks.nameMutation.mutate).toHaveBeenCalledWith({name: 'Nome OIDC'}));
    expect(mocks.nameMutation.mutate).toHaveBeenCalledOnce();
    expect(mocks.setPlayerId).toHaveBeenCalledWith('p2');
  });
});
