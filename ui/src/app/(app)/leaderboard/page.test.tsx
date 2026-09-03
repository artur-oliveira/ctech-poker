import {fireEvent, render, screen, within} from '@testing-library/react';
import {expectNoAxeViolations} from '@/test/axe';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import type {Entry, MyRank} from '@/lib/api/gamification';
import Ranking from './page';

const mocks = vi.hoisted(() => ({
  session: {authed: true, checking: false},
  viewer: 'viewer-id',
  boardQuery: {} as Record<string, unknown>,
  rankQuery: {} as Record<string, unknown>,
  refetch: vi.fn(),
}));

vi.mock('@/lib/auth/session', () => ({useOptionalSession: () => mocks.session}));
vi.mock('@/lib/utils', async (importOriginal) => ({
  ...await importOriginal<typeof import('@/lib/utils')>(),
  getViewerId: () => mocks.viewer,
  playerName: (id: string, viewer: string, name?: string) => name || (id === viewer ? 'Você' : id),
}));
vi.mock('@tanstack/react-query', () => ({
  useQuery: ({queryKey}: {queryKey: unknown[]}) =>
    queryKey[0] === 'leaderboard-me' ? mocks.rankQuery : mocks.boardQuery,
}));
vi.mock('@/components/lobby/ProfileMenu', () => ({ProfileMenu: () => <div>profile-menu</div>}));

const entries: Entry[] = [
  {player_id: 'first', player_name: 'Ana', hands_played: 100, hands_won: 40, win_rate: .4},
  {player_id: 'viewer-id', player_name: 'Beto', hands_played: 80, hands_won: 24, win_rate: .3},
  {player_id: 'third', player_name: 'Caio', hands_played: 70, hands_won: 14, win_rate: .2},
  {player_id: 'fourth', player_name: 'Dani', hands_played: 50, hands_won: 5, win_rate: .1},
];

function boardState(data: Entry[] = [], overrides: Record<string, unknown> = {}) {
  return {data, isLoading: false, isError: false, refetch: mocks.refetch, ...overrides};
}

function rankState(data?: MyRank, overrides: Record<string, unknown> = {}) {
  return {data, isLoading: false, isError: false, ...overrides};
}

describe('community leaderboard page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.session = {authed: true, checking: false};
    mocks.viewer = 'viewer-id';
    mocks.boardQuery = boardState(entries);
    mocks.rankQuery = rankState({ranked: true, rank: 2, total: 4, entry: entries[1]});
  });

  test('renders the podium, remaining positions and viewer summary from the my-rank endpoint', () => {
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
    mocks.boardQuery = boardState([entries[1]]);
    mocks.rankQuery = rankState({ranked: true, rank: 1, total: 1, entry: entries[1]});
    render(<Ranking/>);

    expect(screen.queryByLabelText('Pódio do ranking')).not.toBeInTheDocument();
    expect(screen.getByLabelText('Sua posição atual')).toHaveTextContent('#1 de 1 jogador');
    expect(screen.getByText('01')).toBeInTheDocument();
    expect(screen.getByText('Beto (Você)')).toBeInTheDocument();
  });

  test('shows an unranked hint when the player has no stats row for this mode yet', () => {
    mocks.rankQuery = rankState({ranked: false});
    render(<Ranking/>);

    expect(screen.getByLabelText('Sua posição atual')).toHaveTextContent('Ainda sem ranking');
    expect(screen.getByLabelText('Sua posição atual')).toHaveTextContent('Jogue uma mão');
  });

  test('handles loading, error retry and the empty ranking', () => {
    mocks.boardQuery = boardState([], {isLoading: true});
    const view = render(<Ranking/>);
    expect(screen.getByText(/Buscando o ranking da comunidade/)).toBeInTheDocument();

    mocks.boardQuery = boardState([], {isError: true});
    view.rerender(<Ranking/>);
    expect(screen.getByRole('heading', {level: 1}))
      .toHaveTextContent(/Não foi possível carregar o ranking/);
    fireEvent.click(screen.getByRole('button', {name: 'Tentar novamente'}));
    expect(mocks.refetch).toHaveBeenCalledOnce();

    mocks.boardQuery = boardState([]);
    view.rerender(<Ranking/>);
    expect(screen.getByText(/Nenhum jogador pontuou ainda/)).toBeInTheDocument();
  });

  test('window-virtualizes a long board instead of rendering every row', () => {
    const many: Entry[] = Array.from({length: 60}, (_, i) => ({
      player_id: `p-${i}`, player_name: `Jogador ${i}`,
      hands_played: 60 - i, hands_won: 30 - i / 2, win_rate: 0.3,
    }));
    mocks.boardQuery = boardState(many);
    mocks.rankQuery = rankState({ranked: false});
    render(<Ranking/>);

    const list = screen.getByRole('list', {name: /Ranking, posições 4 em diante/});
    const rows = within(list).getAllByRole('listitem');
    expect(rows.length).toBeGreaterThan(0);
    expect(rows.length).toBeLessThan(57);
    expect(rows[0]).toHaveAttribute('aria-setsize', '57');
  });

  test('shows public navigation without private controls for anonymous visitors', () => {
    mocks.session = {authed: false, checking: false};
    mocks.viewer = '';
    mocks.boardQuery = boardState(entries.slice(0, 3));
    mocks.rankQuery = rankState(undefined);
    render(<Ranking/>);

    expect(screen.getByRole('link', {name: /Voltar/})).toHaveAttribute('href', '/');
    expect(screen.queryByText('profile-menu')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Sua posição atual')).not.toBeInTheDocument();
  });

  // Issue #60: an automated floor under the a11y intent in ui/CLAUDE.md — a new
  // serious or critical axe violation on this route fails CI.
  test('is axe-clean', async () => {
    const {container} = render(<Ranking/>);
    await expectNoAxeViolations(container);
  });

});
