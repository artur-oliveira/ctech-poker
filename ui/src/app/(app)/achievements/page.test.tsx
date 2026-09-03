import {fireEvent, render, screen, within} from '@testing-library/react';
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

// Wraps a flat [{key, count}] fixture (the old paginated shape) into the
// summary endpoint's {achievements: [{key, progress}]} shape, so existing
// test fixtures below only needed a rename, not a rewrite.
function summaryOf(counts: { key: string; count: number }[]) {
  return {achievements: counts.map(({key, count}) => ({key, progress: count}))};
}
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
    mocks.mine = queryState(summaryOf([
      {key: 'wins', count: 5},
      {key: 'bluff', count: 2},
      {key: 'all_in', count: 0},
    ]));
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
    const nav = screen.getByRole('navigation', {name: 'Navegação principal'});
    expect(within(nav).getByRole('link', {name: /Lobby/})).toHaveAttribute('href', '/lobby');
    expect(screen.getByText('7', {selector: '.stat-value'})).toHaveTextContent('7 / 20');
    expect(screen.getByText('2', {selector: '.stat-value'})).toHaveTextContent('2 / 4');
    expect(screen.getByText('1', {selector: '.stat-value'})).toBeInTheDocument();
    expect(screen.getByText('35%')).toBeInTheDocument();
    expect(screen.getByText('Sua próxima estrela')).toBeInTheDocument();
    expect(screen.getByText(/Faltam 1 para o nível 3/)).toBeInTheDocument();
    expect(screen.getByText('hands_played')).toHaveAttribute('data-count', '0');
  });
  
  test('keeps the heading tree intact and animates mastery without touching layout', () => {
    render(<Achievements/>);
    // The cards are h3s: without an h2 for the catalogue the page jumped H1->H3.
    expect(screen.getByRole('heading', {level: 1})).toHaveTextContent('Conquistas');
    expect(screen.getByRole('heading', {level: 2, name: 'Catálogo de conquistas'})).toBeInTheDocument();

    const overview = screen.getByRole('progressbar', {name: 'Maestria geral'});
    const rate = Number(overview.getAttribute('aria-valuenow'));
    expect(overview.firstElementChild).toHaveStyle({'--fill': String(rate / 100)});
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
    mocks.mine = queryState(summaryOf([]));
    const view = render(<Achievements/>);
    fireEvent.click(screen.getByRole('button', {name: 'Completas (0)'}));
    expect(screen.getByText('Nenhuma conquista nesta categoria.')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: 'Ver todas'}));
    expect(screen.getAllByTestId('achievement')).toHaveLength(4);
    view.unmount();
  });
  

  test('hides achievements that belong to the other wallet mode', () => {
    mocks.catalog = queryState([...catalog,
      {key: 'real_money_earned', metric: 'real', tiers},
      {key: 'sandbox_chips_earned', metric: 'sandbox', tiers},
    ]);
    render(<Achievements/>);
    const keys = screen.getAllByTestId('achievement').map(node => node.textContent);
    expect(keys).toContain('sandbox_chips_earned');
    expect(keys).not.toContain('real_money_earned');
  });

  test('reveals a secret achievement only once its first tier is reached', () => {
    mocks.catalog = queryState([...catalog, {key: 'secret_one', metric: 'secret', tiers, secret: true}]);
    const view = render(<Achievements/>);
    expect(screen.getAllByTestId('achievement').map(node => node.textContent)).not.toContain('secret_one');

    mocks.mine = queryState(summaryOf([{key: 'secret_one', count: 3}]));
    view.rerender(<Achievements/>);
    expect(screen.getAllByTestId('achievement').map(node => node.textContent)).toContain('secret_one');
  });

  test('points at the milestone the player is closest to finishing', () => {
    render(<Achievements/>);
    const milestone = document.querySelector('.achievement-next-star');
    expect(milestone).toBeInTheDocument();
    expect(milestone).toHaveTextContent(/Faltam \d+ para o nível \d/);
  });

  test('drops the milestone hint once every visible achievement is maxed out', () => {
    mocks.catalog = queryState([{key: 'wins', metric: 'wins', tiers}]);
    mocks.mine = queryState(summaryOf([{key: 'wins', count: 99}]));
    render(<Achievements/>);
    expect(document.querySelector('.achievement-next-star')).toBeNull();
    expect(screen.getByRole('button', {name: 'Completas (1)'})).toBeInTheDocument();
  });

  test('reports a zero completion rate when the catalog has no stars to earn', () => {
    mocks.catalog = queryState([]);
    mocks.mine = queryState(summaryOf([]));
    render(<Achievements/>);
    expect(screen.getByRole('button', {name: 'Todas (0)'})).toBeInTheDocument();
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
  // ── #119: recency rail, recency sort, arrival celebration ───────────────
  const dated = (key: string, progress: number, stars: number, unlockedAt?: string) =>
    ({key, progress, stars, unlocked: stars > 0, unlocked_at: unlockedAt});

  test('rails the most recent unlocks and offers a recency sort', () => {
    mocks.mine = queryState({achievements: [
      dated('wins', 5, 3, '2026-09-01T12:00:00Z'),
      dated('bluff', 2, 1, '2026-09-02T12:00:00Z'),
      dated('all_in', 0, 0),
    ]});
    render(<Achievements/>);

    const rail = screen.getByRole('region', {name: /Recém-desbloqueadas/});
    // Newest first, and the never-unlocked key is not on the rail at all.
    expect(within(rail).getAllByRole('listitem')).toHaveLength(2);
    expect(within(rail).getAllByRole('listitem')[0]).toHaveTextContent('1 estrela');

    // The sort reorders the same catalogue instead of narrowing it.
    const before = screen.getAllByTestId('achievement').map(card => card.textContent);
    expect(before).toHaveLength(4);
    fireEvent.click(screen.getByRole('button', {name: 'Mais recentes'}));
    expect(screen.getAllByTestId('achievement').map(card => card.textContent))
      .toEqual(['bluff', 'wins', ...before.filter(key => key !== 'bluff' && key !== 'wins')]);

    fireEvent.click(screen.getByRole('button', {name: 'Ordem do catálogo'}));
    expect(screen.getAllByTestId('achievement').map(card => card.textContent)).toEqual(before);
  });

  test('hides the rail and the sort when nothing carries a timestamp', () => {
    render(<Achievements/>);
    expect(screen.queryByRole('region', {name: /Recém-desbloqueadas/})).not.toBeInTheDocument();
    expect(screen.queryByRole('button', {name: 'Mais recentes'})).not.toBeInTheDocument();
  });

  test('celebrates a table unlock once, then never again in that tab', () => {
    window.sessionStorage.setItem('ctech-poker:achievement-arrival', 'wins');
    mocks.mine = queryState({achievements: [dated('wins', 5, 3, '2026-09-01T12:00:00Z')]});

    const first = render(<Achievements/>);
    expect(screen.getByRole('status')).toHaveTextContent(/entrou na sua coleção agora/);
    first.unmount();

    render(<Achievements/>);
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });
});
