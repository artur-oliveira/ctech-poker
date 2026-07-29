import {fireEvent, render, screen} from '@testing-library/react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import type {ProfileShowcase} from '@/lib/api/player';
import ProfilePage from './page';

const mocks = vi.hoisted(() => ({
  playerID: 'player-42',
  session: {authed: true, checking: false},
  query: {} as Record<string, unknown>,
  queryOptions: undefined as unknown,
}));

vi.mock('next/navigation', () => ({
  useSearchParams: () => ({get: (key: string) => key === 'id' ? mocks.playerID : null}),
}));
vi.mock('@/lib/auth/session', () => ({useOptionalSession: () => mocks.session}));
vi.mock('@tanstack/react-query', () => ({
  useQuery: (options: unknown) => {
    mocks.queryOptions = options;
    return mocks.query;
  },
}));
vi.mock('@/components/lobby/ProfileMenu', () => ({ProfileMenu: () => <div>profile-menu</div>}));
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
      retry: false,
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
    expect(screen.getByRole('link', {name: /Lobby/})).toHaveAttribute('href', '/lobby');
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
    expect(screen.getByRole('link', {name: /Lobby/})).toHaveAttribute('href', '/lobby');
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
});
