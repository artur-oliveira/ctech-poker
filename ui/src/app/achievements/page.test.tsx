import {fireEvent, render, screen} from '@testing-library/react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import type {Achievement} from '@/lib/api/achievements';
import Achievements from './page';

const mocks = vi.hoisted(() => ({
  session: {authed: true, checking: false},
  catalog: {} as Record<string, unknown>,
  mine: {} as Record<string, unknown>,
  catalogRefetch: vi.fn(),
}));

vi.mock('@/lib/auth/session', () => ({useOptionalSession: () => mocks.session}));
vi.mock('@tanstack/react-query', () => ({
  useQuery: ({queryKey}: { queryKey: string[] }) =>
    queryKey[1] === 'catalog' ? mocks.catalog : mocks.mine,
}));
vi.mock('@/components/lobby/ProfileMenu', () => ({ProfileMenu: () => <div>profile-menu</div>}));
vi.mock('@/components/achievements/AchievementCard', () => ({
  AchievementCard: ({achievement, count}: { achievement: Achievement; count?: number }) =>
    <article data-testid="achievement" data-count={count === undefined ? 'unknown' : count}>
      {achievement.key}
    </article>,
}));

const tiers = [1, 2, 3, 4, 5].map((threshold, index) => ({stars: index + 1, threshold}));
const catalog: Achievement[] = [
  {key: 'wins', metric: 'wins', tiers},
  {key: 'bluff', metric: 'bluff', tiers},
  {key: 'all_in', metric: 'all_in', tiers},
  {key: 'hands_played', metric: 'hands', tiers},
];

function queryState(data: unknown, overrides: Record<string, unknown> = {}) {
  return {data, isLoading: false, isError: false, refetch: vi.fn(), ...overrides};
}

describe('achievements page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.session = {authed: true, checking: false};
    mocks.catalog = queryState(catalog, {refetch: mocks.catalogRefetch});
    mocks.mine = queryState([
      {key: 'wins', count: 5},
      {key: 'bluff', count: 2},
      {key: 'all_in', count: 0},
    ]);
  });
  
  test('keeps the session loading screen isolated from page content', () => {
    mocks.session = {authed: false, checking: true};
    render(<Achievements/>);
    expect(screen.getByText(/Carregando conquistas/)).toBeInTheDocument();
    expect(screen.queryByRole('heading', {name: 'Conquistas'})).not.toBeInTheDocument();
  });
  
  test('renders a public catalog with neutral progress and public navigation', () => {
    mocks.session = {authed: false, checking: false};
    mocks.mine = queryState(undefined, {isLoading: true});
    render(<Achievements/>);
    
    expect(screen.getByRole('link', {name: /Voltar/})).toHaveAttribute('href', '/');
    expect(screen.getByText(/Entre com sua conta CTech/)).toBeInTheDocument();
    expect(screen.queryByText('profile-menu')).not.toBeInTheDocument();
    expect(screen.queryByRole('tablist', {name: 'Filtro de conquistas'})).not.toBeInTheDocument();
    expect(screen.getAllByTestId('achievement')).toHaveLength(4);
    screen.getAllByTestId('achievement').forEach(card =>
      expect(card).toHaveAttribute('data-count', 'unknown')
    );
  });
  
  test('calculates player statistics and passes zero for absent backend counters', () => {
    render(<Achievements/>);
    
    expect(screen.getByText('profile-menu')).toBeInTheDocument();
    expect(screen.getByRole('link', {name: /Lobby/})).toHaveAttribute('href', '/lobby');
    expect(screen.getByText('7', {selector: '.stat-value'})).toHaveTextContent('7 / 20');
    expect(screen.getByText('2', {selector: '.stat-value'})).toHaveTextContent('2 / 4');
    expect(screen.getByText('1', {selector: '.stat-value'})).toBeInTheDocument();
    expect(screen.getByText('35%')).toBeInTheDocument();
    expect(screen.getByText('hands_played')).toHaveAttribute('data-count', '0');
  });
  
  test('filters unlocked, in-progress and completed achievements and restores an empty filter', () => {
    const populated = render(<Achievements/>);
    
    fireEvent.click(screen.getByRole('button', {name: 'Desbloqueadas (2)'}));
    expect(screen.getAllByTestId('achievement')).toHaveLength(2);
    expect(screen.queryByText('all_in')).not.toBeInTheDocument();
    
    fireEvent.click(screen.getByRole('button', {name: 'Em progresso (1)'}));
    expect(screen.getByText('bluff')).toBeInTheDocument();
    expect(screen.queryByText('wins')).not.toBeInTheDocument();
    
    fireEvent.click(screen.getByRole('button', {name: 'Completas (1)'}));
    expect(screen.getByText('wins')).toBeInTheDocument();
    
    populated.unmount();
    mocks.mine = queryState([]);
    const view = render(<Achievements/>);
    fireEvent.click(screen.getByRole('button', {name: 'Completas (0)'}));
    expect(screen.getByText('Nenhuma conquista nesta categoria.')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: 'Ver todas'}));
    expect(screen.getAllByTestId('achievement')).toHaveLength(4);
    view.unmount();
  });
  
  test('handles catalog loading and failure with retry', () => {
    mocks.catalog = queryState(undefined, {isLoading: true, refetch: mocks.catalogRefetch});
    const view = render(<Achievements/>);
    expect(screen.getByText(/Carregando catálogo/)).toBeInTheDocument();
    
    mocks.catalog = queryState(undefined, {isError: true, refetch: mocks.catalogRefetch});
    view.rerender(<Achievements/>);
    expect(screen.getByText(/Não foi possível carregar o catálogo/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: 'Tentar novamente'}));
    expect(mocks.catalogRefetch).toHaveBeenCalledOnce();
  });
  
  test('keeps the catalog usable when only player progress fails or is loading', () => {
    mocks.mine = queryState(undefined, {isError: true});
    const view = render(<Achievements/>);
    expect(screen.getByText(/Não foi possível carregar seu progresso/)).toBeInTheDocument();
    expect(screen.getAllByTestId('achievement')).toHaveLength(4);
    expect(screen.queryByRole('tablist', {name: 'Filtro de conquistas'})).not.toBeInTheDocument();
    
    mocks.mine = queryState(undefined, {isLoading: true});
    view.rerender(<Achievements/>);
    expect(screen.queryByText(/Não foi possível carregar seu progresso/)).not.toBeInTheDocument();
    screen.getAllByTestId('achievement').forEach(card =>
      expect(card).toHaveAttribute('data-count', 'unknown')
    );
  });
});
