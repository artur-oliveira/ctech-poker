import {act, fireEvent, render, screen} from '@testing-library/react';
import {expectNoAxeViolations} from '@/test/axe';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import type {HandItem} from '@/lib/api/player';
import type {Page} from '@/lib/api/client';
import HandsHistory from './page';

const mocks = vi.hoisted(() => ({
  query: vi.fn(),
  lifetime: vi.fn(),
  refetch: vi.fn(),
  fetchNextPage: vi.fn(),
}));

// `useQuery` is the lifetime-totals read (#115); the shared-links panel below
// is its own suite, so it is stubbed out rather than given a query client.
vi.mock('@tanstack/react-query', () => ({useInfiniteQuery: mocks.query, useQuery: mocks.lifetime}));
vi.mock('@/components/hands/MyHandSharesPanel', () => ({MyHandSharesPanel: () => <div>hand-shares-panel</div>}));
vi.mock('@/components/TermsGate', () => ({TermsGate: ({children}: { children: React.ReactNode }) => children}));
vi.mock('@/components/lobby/ProfileMenu', () => ({ProfileMenu: () => <div>profile-menu</div>}));
vi.mock('@/lib/hooks/useSocialUnread', () => ({useSocialUnread: () => 0}));
vi.mock('@/components/social/PeopleNavBadge', () => ({PeopleNavBadge: () => <span>people-badge</span>}));
vi.mock('@/components/table/PlayingCard', () => ({
  PlayingCard: ({card}: { card: string }) => <span data-testid="card">{card}</span>,
}));
vi.mock('@/components/hands/OutcomeBadge', () => ({
  OutcomeBadge: ({outcome}: { outcome: string }) => <span>{outcome}</span>,
}));

const hands: HandItem[] = [
  {
    pk: 'p1', sk: 'hand#1', table_id: 'table-one', hand_id: 'h1', outcome: 'won',
    net_change: 1200, ended_at: 1_700_000_000_000,
    hole_cards: ['AH', 'KH'], board: ['QH', 'JH', 'TH', '2C', '3D'],
    server_seed: '1234567890abcdef',
  },
  {
    pk: 'p1', sk: 'hand#2', table_id: 'table-two', hand_id: 'h2', outcome: 'lost',
    net_change: -400, ended_at: 1_699_000_000_000, hole_cards: ['2C'], board: ['AS'],
  },
  {
    pk: 'p1', sk: 'hand#3', table_id: 'table-three', hand_id: 'h3', outcome: 'tied',
    net_change: 0, ended_at: 1_698_000_000_000,
  },
];

function pageOf(items: HandItem[], hasNext = false): Page<HandItem> {
  return {
    data: items,
    has_next: hasNext,
    next_cursor: hasNext ? 'next' : null,
    has_previous: false,
    previous_cursor: null,
  };
}

function queryResult(pages: Page<HandItem>[], overrides: Record<string, unknown> = {}) {
  const last = pages[pages.length - 1];
  return {
    data: {pages, pageParams: pages.map(() => undefined)},
    isLoading: false,
    isError: false,
    refetch: mocks.refetch,
    fetchNextPage: mocks.fetchNextPage,
    hasNextPage: Boolean(last?.has_next),
    isFetchingNextPage: false,
    ...overrides,
  };
}

describe('hands list page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.query.mockReturnValue(queryResult([pageOf(hands)]));
    mocks.lifetime.mockReturnValue({data: undefined, isLoading: false, isError: false});
  });
  
  test('summarizes backend outcomes and renders safe hand links and incomplete cards', () => {
    render(<HandsHistory/>);
    expect(screen.getByRole('button', {name: 'Dinheiro real · Indisponível'})).toBeDisabled();
    expect(screen.queryByLabelText('ID da mão')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Ordenar')).not.toBeInTheDocument();
    expect(screen.queryByRole('group', {name: 'Resultado da mão'})).not.toBeInTheDocument();
    expect(screen.getByText('3', {selector: '.stat-value'})).toBeInTheDocument();
    expect(screen.getByText('+800')).toBeInTheDocument();
    expect(screen.getByText(/33%/)).toHaveTextContent('33% (1V · 1E · 1D)');
    expect(screen.getByText(/Royal flush/)).toBeInTheDocument();
    // 3 hole cards in the fixtures + 5 board positions per row: undealt board
    // positions now render a card back instead of an empty outline.
    expect(screen.getAllByTestId('card')).toHaveLength(3 + 15);
    expect(screen.getByRole('link', {name: /won/})).toHaveAttribute(
      'href', '/hands/history?table_id=table-one&hand_id=h1&mode=sandbox'
    );
    expect(screen.getByTitle('1234567890abcdef')).toHaveTextContent('seed 12345678…');
    expect(screen.queryByRole('button', {name: /Carregar mais/})).not.toBeInTheDocument();
  });
  
  test('handles loading, failure with retry, and an empty account', () => {
    mocks.query.mockReturnValueOnce(queryResult([], {isLoading: true}));
    const view = render(<HandsHistory/>);
    expect(screen.getByText(/Reunindo cartas, resultados e provas/)).toBeInTheDocument();
    
    mocks.query.mockReturnValueOnce(queryResult([], {isError: true}));
    view.rerender(<HandsHistory/>);
    fireEvent.click(screen.getByRole('button', {name: 'Tentar novamente'}));
    expect(mocks.refetch).toHaveBeenCalledOnce();
    
    mocks.query.mockReturnValueOnce(queryResult([pageOf([])]));
    view.rerender(<HandsHistory/>);
    expect(screen.getByText(/Sua primeira mão começa no lobby/)).toBeInTheDocument();
    expect(screen.getByRole('button', {name: /Encontrar uma mesa/})).toHaveAttribute('href', '/lobby');
  });
  

  test('signs the loaded balance and reports a losing run as such', () => {
    const {container, rerender} = render(<HandsHistory/>);
    expect(container.querySelector('.stat-value.gain')).toHaveTextContent('+800');

    mocks.query.mockReturnValue(queryResult([pageOf([{...hands[1], net_change: -400}])]));
    rerender(<HandsHistory/>);
    expect(container.querySelector('.stat-value.loss')).toHaveTextContent('-400');

    mocks.query.mockReturnValue(queryResult([pageOf([{...hands[2], net_change: 0}])]));
    rerender(<HandsHistory/>);
    expect(container.querySelector('.stat-value.gain')).toBeNull();
    expect(container.querySelector('.stat-value.loss')).toBeNull();
  });

  test('announces the fetch of the next page while it is in flight', () => {
    vi.stubGlobal('IntersectionObserver', class {
      observe = vi.fn();
      unobserve = vi.fn();
      disconnect = vi.fn();
      takeRecords = vi.fn(() => []);
    });
    mocks.query.mockReturnValue(queryResult([pageOf(hands, true)], {isFetchingNextPage: true}));
    render(<HandsHistory/>);
    expect(screen.getByRole('button', {name: 'Carregando mais mãos…'})).toBeDisabled();
    expect(screen.getByRole('status', {name: 'Carregando mais mãos'})).toBeInTheDocument();
  });

  test('stops paginating when the API reports another page but no cursor', () => {
    mocks.query.mockReturnValue(queryResult([{
      data: hands, has_next: true, next_cursor: null, has_previous: false, previous_cursor: null,
    }], {hasNextPage: false}));
    render(<HandsHistory/>);
    expect(screen.queryByRole('button', {name: /Carregar mais/})).not.toBeInTheDocument();
  });

  test('reports loaded counts, appends API pages and loads more on scroll or click', () => {
    const observed: IntersectionObserverCallback[] = [];
    vi.stubGlobal('IntersectionObserver', class {
      observe = vi.fn();
      unobserve = vi.fn();
      disconnect = vi.fn();
      takeRecords = vi.fn(() => []);
      
      constructor(callback: IntersectionObserverCallback) {
        observed.push(callback);
      }
    });
    
    mocks.query.mockReturnValue(queryResult([pageOf(hands, true)]));
    const view = render(<HandsHistory/>);
    expect(screen.getByText('3', {selector: '.stat-value'})).toBeInTheDocument();
    
    act(() => observed[0]([{isIntersecting: true} as IntersectionObserverEntry], {} as IntersectionObserver));
    expect(mocks.fetchNextPage).toHaveBeenCalledOnce();
    
    fireEvent.click(screen.getByRole('button', {name: 'Carregar mais mãos'}));
    expect(mocks.fetchNextPage).toHaveBeenCalledTimes(2);
    
    const secondPage: HandItem[] = [{...hands[0], hand_id: 'h4', sk: 'hand#4', net_change: 300}];
    mocks.query.mockReturnValue(queryResult([pageOf(hands, true), pageOf(secondPage)]));
    view.rerender(<HandsHistory/>);
    expect(screen.getAllByText('won')).toHaveLength(2);
    expect(screen.getByText('4', {selector: '.stat-value'})).toBeInTheDocument();
    expect(screen.queryByRole('button', {name: /Carregar mais/})).not.toBeInTheDocument();
  });

  test('keeps a 500-hand history bounded to the visible DOM window', () => {
    const manyHands = Array.from({length: 500}, (_, index) => ({
      ...hands[index % hands.length],
      hand_id: `history-${index}`,
      sk: `hand#${index}`,
    }));
    mocks.query.mockReturnValue(queryResult([pageOf(manyHands)]));

    const {container} = render(<HandsHistory/>);

    // #115: the rows are now interleaved with real day headings, so the list
    // roles are gone and the named region carries the count instead.
    expect(screen.getByRole('region', {name: /^500 mãos nesta lista/})).toBeInTheDocument();
    expect(container.querySelectorAll('.hand-row').length).toBeGreaterThan(0);
    expect(container.querySelectorAll('.hand-row').length).toBeLessThan(20);
    // Only the window is mounted; the day headers ride along inside it.
    expect(container.querySelectorAll('.hands-day-header').length).toBeGreaterThan(0);
    expect(container.querySelector('.hands-day-pinned')).toBeInTheDocument();
  });

  test('filters to only wins over the loaded pages, without refetching', () => {
    render(<HandsHistory/>);
    mocks.query.mockClear();

    fireEvent.click(screen.getByRole('button', {name: 'Só vitórias'}));

    expect(screen.getAllByText('won')).toHaveLength(1);
    expect(screen.queryByText('lost')).not.toBeInTheDocument();
    // The whole point of client-side filtering: no new request.
    expect(mocks.fetchNextPage).not.toHaveBeenCalled();
    // The subset roll-up follows the filter, not the loaded page count.
    expect(screen.getByText('1', {selector: '.stat-value'})).toBeInTheDocument();
  });

  test('filters to a single table and offers a way back out of an empty result', () => {
    render(<HandsHistory/>);
    fireEvent.click(screen.getByRole('button', {name: /Mesa .*table-one/i}));
    expect(screen.getAllByText('won')).toHaveLength(1);

    fireEvent.click(screen.getByRole('button', {name: 'Só derrotas'}));
    expect(screen.getByText('Nenhuma mão com esse filtro')).toBeInTheDocument();
    expect(screen.queryByRole('button', {name: /Carregar mais/})).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', {name: 'Limpar filtros'}));
    expect(screen.getAllByText('won')).toHaveLength(1);
    expect(screen.getByText('lost')).toBeInTheDocument();
  });

  test('groups the rows by day with a pinned header for the day in view', () => {
    const {container} = render(<HandsHistory/>);
    // Three hands, three distinct days in the fixture.
    expect(container.querySelectorAll('.hands-day-header')).toHaveLength(3);
    expect(container.querySelector('.hands-day-pinned')?.textContent)
      .toBe(container.querySelector('.hands-day-header')?.firstChild?.textContent);
  });

  test('states the lifetime totals beside the loaded subset, and stays quiet when unranked', () => {
    mocks.lifetime.mockReturnValue({
      data: {ranked: true, rank: 3, total: 40, entry: {player_id: 'p1', hands_played: 12_500, hands_won: 4_000, win_rate: 0.32}},
      isLoading: false, isError: false,
    });
    const {rerender} = render(<HandsHistory/>);
    expect(screen.getByText(/12\.500 mãos/)).toBeInTheDocument();
    expect(screen.getByText(/4\.000 vitórias/)).toBeInTheDocument();
    expect(screen.getByText(/\(32%\)/)).toBeInTheDocument();

    mocks.lifetime.mockReturnValue({data: {ranked: false}, isLoading: false, isError: false});
    rerender(<HandsHistory/>);
    expect(screen.queryByText(/Desde o início/)).not.toBeInTheDocument();
  });

  test('shows the blind level of a hand only when the record carries one', () => {
    mocks.query.mockReturnValue(queryResult([pageOf([
      {...hands[0], small_blind: 25, big_blind: 50},
      {...hands[1]},
    ])]));
    const {container} = render(<HandsHistory/>);
    const blinds = container.querySelectorAll('.hand-row-blinds');
    expect(blinds).toHaveLength(1);
    expect(blinds[0].textContent).toBe('25/50');
  });


  // Issue #60: an automated floor under the a11y intent in ui/CLAUDE.md — a new
  // serious or critical axe violation on this route fails CI.
  test('is axe-clean', async () => {
    const {container} = render(<HandsHistory/>);
    await expectNoAxeViolations(container);
  });

});
