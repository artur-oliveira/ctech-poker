import {fireEvent, render, screen, within} from '@testing-library/react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import type {Entry} from '@/lib/api/gamification';
import Ranking from './page';

const mocks = vi.hoisted(() => ({
  session: {authed: true, checking: false},
  viewer: 'viewer-id',
  query: {} as Record<string, unknown>,
  refetch: vi.fn(),
}));

vi.mock('@/lib/auth/session', () => ({useOptionalSession: () => mocks.session}));
vi.mock('@/lib/utils', async (importOriginal) => ({
  ...await importOriginal<typeof import('@/lib/utils')>(),
  getViewerId: () => mocks.viewer,
  playerName: (id: string, viewer: string, name?: string) => name || (id === viewer ? 'Você' : id),
}));
vi.mock('@tanstack/react-query', () => ({useQuery: () => mocks.query}));
vi.mock('@/components/lobby/ProfileMenu', () => ({ProfileMenu: () => <div>profile-menu</div>}));

const entries: Entry[] = [
  {player_id: 'first', player_name: 'Ana', hands_played: 100, hands_won: 40, win_rate: .4},
  {player_id: 'viewer-id', player_name: 'Beto', hands_played: 80, hands_won: 24, win_rate: .3},
  {player_id: 'third', player_name: 'Caio', hands_played: 70, hands_won: 14, win_rate: .2},
  {player_id: 'fourth', player_name: 'Dani', hands_played: 50, hands_won: 5, win_rate: .1},
];

function queryState(data: Entry[] = [], overrides: Record<string, unknown> = {}) {
  return {data, isLoading: false, isError: false, refetch: mocks.refetch, ...overrides};
}

describe('community leaderboard page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.session = {authed: true, checking: false};
    mocks.viewer = 'viewer-id';
    mocks.query = queryState(entries);
  });
  
  test('renders the podium, remaining positions and viewer summary from backend data', () => {
    render(<Ranking/>);
    
    expect(screen.getByLabelText('Sua posição atual')).toHaveTextContent('#2 de 4 jogadores');
    expect(screen.getByLabelText('Sua posição atual')).toHaveTextContent('30.0%');
    expect(screen.getByLabelText('Pódio do ranking')).toHaveTextContent('Ana');
    expect(screen.getByLabelText('Pódio do ranking')).toHaveTextContent('Beto (Você)');
    expect(screen.getByText('04')).toBeInTheDocument();
    expect(screen.getByText('Dani')).toBeInTheDocument();
    expect(screen.getByText('profile-menu')).toBeInTheDocument();
    expect(within(screen.getByRole('navigation', {name: 'Navegação principal'}))
      .getByRole('link', {name: /Lobby/})).toHaveAttribute('href', '/lobby');
  });
  
  test('uses a list without podium for fewer than three entries and singularizes one player', () => {
    mocks.query = queryState([entries[1]]);
    render(<Ranking/>);
    
    expect(screen.queryByLabelText('Pódio do ranking')).not.toBeInTheDocument();
    expect(screen.getByLabelText('Sua posição atual')).toHaveTextContent('#1 de 1 jogador');
    expect(screen.getByText('01')).toBeInTheDocument();
    expect(screen.getByText('Beto (Você)')).toBeInTheDocument();
  });
  
  test('handles loading, error retry and the empty ranking', () => {
    mocks.query = queryState([], {isLoading: true});
    const view = render(<Ranking/>);
    expect(screen.getByText(/Buscando o ranking da comunidade/)).toBeInTheDocument();
    
    mocks.query = queryState([], {isError: true});
    view.rerender(<Ranking/>);
    fireEvent.click(screen.getByRole('button', {name: 'Tentar novamente'}));
    expect(mocks.refetch).toHaveBeenCalledOnce();
    
    mocks.query = queryState([]);
    view.rerender(<Ranking/>);
    expect(screen.getByText(/Nenhum jogador pontuou ainda/)).toBeInTheDocument();
  });
  
  test('shows public navigation without private controls for anonymous visitors', () => {
    mocks.session = {authed: false, checking: false};
    mocks.viewer = '';
    mocks.query = queryState(entries.slice(0, 3));
    render(<Ranking/>);
    
    expect(screen.getByRole('link', {name: /Voltar/})).toHaveAttribute('href', '/');
    expect(screen.queryByText('profile-menu')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Sua posição atual')).not.toBeInTheDocument();
  });
});
