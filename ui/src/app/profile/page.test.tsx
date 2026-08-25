import {fireEvent, render, screen} from '@testing-library/react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import type {ProfileShowcase} from '@/lib/api/player';
import ProfilePage from './page';

const mocks = vi.hoisted(() => ({
  playerID: 'player-42',
  session: {authed: true, checking: false},
  query: {} as Record<string, unknown>,
  relationshipQuery: {} as Record<string, unknown>,
  matchupQuery: {} as Record<string, unknown>,
  queryOptions: undefined as unknown,
  invalidateQueries: vi.fn(),
}));

vi.mock('next/navigation', () => ({
  useSearchParams: () => ({get: (key: string) => key === 'id' ? mocks.playerID : null}),
}));
vi.mock('@/lib/auth/session', () => ({useOptionalSession: () => mocks.session}));
vi.mock('@tanstack/react-query', () => ({
  useQuery: (options: unknown) => {
    const key = (options as {queryKey: string[]}).queryKey;
    if (key[1] === 'relationship') return mocks.relationshipQuery;
    if (key[0] === 'profile-matchup') return mocks.matchupQuery;
    mocks.queryOptions = options;
    return mocks.query;
  },
  useQueryClient: () => ({invalidateQueries: mocks.invalidateQueries}),
}));
vi.mock('@/components/lobby/ProfileMenu', () => ({ProfileMenu: () => <div>profile-menu</div>}));
vi.mock('@/components/social/PeopleNavBadge', () => ({PeopleNavBadge: () => null}));
vi.mock('@/components/table/PlayingCard', () => ({
  PlayingCard: ({card}: { card: string }) => <span data-testid="card">{card}</span>,
}));

function queryState(data?: ProfileShowcase, overrides: Record<string, unknown> = {}) {
  return {data, isLoading: false, isError: false, ...overrides};
}

describe('public player profile page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.playerID = 'player-42';
    mocks.session = {authed: true, checking: false};
    mocks.relationshipQuery = {data: undefined, isLoading: false, isError: true};
    mocks.matchupQuery = {data: undefined, isLoading: false, isError: true};
    mocks.query = queryState({
      player_id: 'player-42',
      name: 'Ás da Mesa',
      playstyle: [{key: 'selective'}],
      featured_achievements: [{key: 'wins', count: 1234}],
      best_hand: {
        hand_id: 'hand-1',
        table_id: 'table-1',
        net_change: 2500,
        ended_at: 1_700_000_000,
        hole_cards: ['AH', 'KH'],
        board: ['QH', 'JH', 'TH'],
      },
    });
  });
  
  test('loads the requested showcase and renders its achievements and best hand', () => {
    render(<ProfilePage/>);
    
    expect(mocks.queryOptions).toMatchObject({
      queryKey: ['profile-showcase', 'player-42'],
      enabled: true,
    });
    expect(screen.getByRole('heading', {name: 'Ás da Mesa'})).toBeInTheDocument();
    expect(screen.getByText('1.234 registradas')).toBeInTheDocument();
    expect(screen.getByText('+2.500 fichas')).toBeInTheDocument();
    const playstyle = screen.getByText('Seletivo');
    expect(screen.getByText('VPIP de até 22%')).not.toBeVisible();
    fireEvent.click(playstyle);
    expect(screen.getByText('VPIP de até 22%')).toBeVisible();
    expect(screen.getAllByTestId('card').map(card => card.textContent)).toEqual(['AH', 'KH']);
    expect(screen.getByText('profile-menu')).toBeInTheDocument();
    expect(screen.getByRole('navigation', {name: 'Navegação rápida'})).toBeInTheDocument();
    expect(screen.getAllByRole('link', {name: 'Lobby'}).some(link => link.getAttribute('href') === '/lobby')).toBe(true);
  });
  
  test('disables the request without an id and exposes the public navigation', () => {
    mocks.playerID = '';
    mocks.session = {authed: false, checking: false};
    mocks.query = queryState(undefined, {isError: true});
    render(<ProfilePage/>);
    
    expect(mocks.queryOptions).toMatchObject({
      queryKey: ['profile-showcase', ''],
      enabled: false,
    });
    expect(screen.getByRole('heading', {name: 'Vitrine indisponível'})).toBeInTheDocument();
    expect(screen.getByRole('link', {name: 'Voltar'})).toHaveAttribute('href', '/');
    expect(screen.getByRole('button', {name: 'Ir para o Lobby'})).toHaveAttribute('href', '/lobby');
    expect(screen.queryByText('profile-menu')).not.toBeInTheDocument();
  });
  
  test('handles loading and an unavailable backend response', () => {
    mocks.query = queryState(undefined, {isLoading: true});
    const view = render(<ProfilePage/>);
    expect(screen.getByText(/Carregando vitrine do jogador/)).toBeInTheDocument();
    
    mocks.query = queryState(undefined);
    view.rerender(<ProfilePage/>);
    expect(screen.getByText('Este perfil não existe ou não foi encontrado.')).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Ir para o Lobby'})).toHaveAttribute('href', '/lobby');
  });
  
  test('uses safe fallbacks when optional showcase content is absent', () => {
    mocks.query = queryState({
      player_id: 'player-42',
      featured_achievements: [],
    });
    render(<ProfilePage/>);
    
    expect(screen.getByRole('heading', {name: 'Jogador'})).toBeInTheDocument();
    expect(screen.getByText('Nenhuma conquista selecionada para exibição.')).toBeInTheDocument();
    expect(screen.getByText('Nenhuma vitória recente registrada nesta vitrine.')).toBeInTheDocument();
  });
  test('offers the shared player menu only for someone else profile', async () => {
    const {rerender} = render(<ProfilePage/>);
    expect(screen.queryByRole('button', {name: /Ações para/})).not.toBeInTheDocument();

    mocks.relationshipQuery = {
      data: {player_id: 'player-42', name: 'Ás da Mesa', relationship: 'none', muted: false, blocked: false},
      isLoading: false, isError: false,
    };
    rerender(<ProfilePage/>);
    expect(screen.getByRole('button', {name: 'Ações para Ás da Mesa'})).toBeInTheDocument();
  });

  test('shows the head-to-head card once the pair has shared hands', () => {
    mocks.matchupQuery = {
      data: {
        hands_together: 12,
        viewer_wins: 7,
        opponent_wins: 5,
        ties: 0,
        heads_up_hands_together: 4,
        net_change_viewer: 380
      },
      isLoading: false, isError: false,
    };
    render(<ProfilePage/>);
    expect(screen.getByText(/12 mãos juntos/)).toBeInTheDocument();
    expect(screen.getByText(/você venceu 7/)).toBeInTheDocument();
    expect(screen.getByText(/Ás da Mesa venceu 5/)).toBeInTheDocument();
  });

  test('hides the head-to-head card for a pair that never shared a table', () => {
    mocks.matchupQuery = {
      data: {
        hands_together: 0,
        viewer_wins: 0,
        opponent_wins: 0,
        ties: 0,
        heads_up_hands_together: 0,
        net_change_viewer: 0
      },
      isLoading: false, isError: false,
    };
    render(<ProfilePage/>);
    expect(screen.queryByText(/mãos juntos/)).not.toBeInTheDocument();
  });

});
