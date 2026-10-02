import {act, fireEvent, render, screen} from '@testing-library/react';
import {expectNoAxeViolations} from '@/test/axe';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import type {HandItem} from '@/lib/api/player';
import type {Page} from '@/lib/api/client';
import HandsHistory from './page';

const mocks = vi.hoisted(() => ({
  query: vi.fn(),
  lifetime: vi.fn(),
  collections: vi.fn(),
  refetch: vi.fn(),
  fetchNextPage: vi.fn(),
}));

// `useQuery` backs two independent reads (lifetime totals #115 and
// collections #347); routed by queryKey so each test can configure only the
// one it cares about. The shared-links panel below is its
// own suite, so it is stubbed out rather than given a query client.
vi.mock('@tanstack/react-query', () => ({
  useInfiniteQuery: mocks.query,
  useQuery: ({queryKey}: { queryKey: readonly unknown[] }) => {
    if (queryKey[0] === 'leaderboard') return mocks.lifetime();
    if (queryKey[0] === 'hand-collections') return mocks.collections();
    return {data: undefined, isLoading: false, isError: false};
  },
}));
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
    mocks.collections.mockReturnValue({data: [], isLoading: false, isError: false});
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

  test('stops auto-paginating while a collection is shown', () => {
    const instances: {disconnect: ReturnType<typeof vi.fn>}[] = [];
    vi.stubGlobal('IntersectionObserver', class {
      observe = vi.fn();
      unobserve = vi.fn();
      disconnect = vi.fn();
      takeRecords = vi.fn(() => []);

      constructor() {
        instances.push(this);
      }
    });
    mocks.collections.mockReturnValue({
      data: [{hand_id: 'h1', review_marked: false, collections: ['Estudar depois']}],
      isLoading: false, isError: false,
    });

    mocks.query.mockReturnValue(queryResult([pageOf(hands, true)]));
    render(<HandsHistory/>);
    expect(instances).toHaveLength(1);

    // A collection is short, so the sentinel never leaves the viewport:
    // auto-loading here downloads the whole history in one cascade. The
    // observer is torn down and not replaced.
    fireEvent.change(screen.getByLabelText('Mostrar'), {target: {value: 'Estudar depois'}});
    expect(instances[0].disconnect).toHaveBeenCalled();
    expect(instances).toHaveLength(1);
    expect(mocks.fetchNextPage).not.toHaveBeenCalled();

    // The explicit button still works under a collection.
    fireEvent.click(screen.getByRole('button', {name: 'Carregar mais mãos'}));
    expect(mocks.fetchNextPage).toHaveBeenCalledOnce();
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

  test('offers no client-side outcome/table filters and no saved filters', () => {
    render(<HandsHistory/>);
    // They only ever filtered the pages loaded so far, so their counts lied.
    expect(screen.queryByRole('button', {name: 'Só vitórias'})).not.toBeInTheDocument();
    expect(screen.queryByRole('button', {name: /Mesa /})).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Salvar filtro atual como')).not.toBeInTheDocument();
    // With no collections there is nothing to pick, so no picker either.
    expect(screen.queryByLabelText('Mostrar')).not.toBeInTheDocument();
  });

  test('renders the shared-links panel above the infinite list, not after it', () => {
    mocks.query.mockReturnValue(queryResult([pageOf(hands, true)]));
    render(<HandsHistory/>);
    const panel = screen.getByText('hand-shares-panel');
    const list = screen.getByRole('region', {name: /mãos nesta lista/});
    const more = screen.getByRole('button', {name: 'Carregar mais mãos'});
    expect(panel.compareDocumentPosition(list) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(panel.compareDocumentPosition(more) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
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


  // #296: this hand's own recorded variant, not standard, decides how a
  // client-recomputed category is labeled. A-6-7-8-9 is a straight only
  // under short-deck (there is no rank Two-Five to make any standard
  // low straight), so a hand recorded as 'short_deck' must show
  // "Sequência" here instead of the "high card" a standard recompute
  // would give the same seven cards.
  test('categorizes a hand by its own recorded variant, not standard rules', () => {
    mocks.query.mockReturnValue(queryResult([pageOf([
      {...hands[0], hole_cards: ['AC', '6D'], board: ['7H', '8S', '9C', 'KH', 'JD'], variant: 'short_deck'},
    ])]));
    render(<HandsHistory/>);
    expect(screen.getByText(/Sequência/)).toBeInTheDocument();
  });

  test('the Mostrar picker narrows the list to a collection, including the review-marker one', () => {
    mocks.collections.mockReturnValue({
      data: [
        {hand_id: 'h1', review_marked: true, collections: ['Estudar depois']},
        {hand_id: 'h2', review_marked: false, collections: ['Estudar depois']},
      ],
      isLoading: false, isError: false,
    });
    render(<HandsHistory/>);
    const picker = screen.getByLabelText('Mostrar');
    expect(picker).toHaveValue('');
    expect(screen.getByText('tied')).toBeInTheDocument();

    // The review-marker collection is offered first.
    const options = screen.getAllByRole('option').map(option => option.textContent);
    expect(options).toEqual(['Todas as mãos', 'Marcadas para revisar', 'Estudar depois']);

    fireEvent.change(picker, {target: {value: '__review__'}});
    expect(screen.getAllByText('won')).toHaveLength(1);
    expect(screen.queryByText('lost')).not.toBeInTheDocument();

    fireEvent.change(picker, {target: {value: 'Estudar depois'}});
    expect(screen.getAllByText('won')).toHaveLength(1);
    expect(screen.getByText('lost')).toBeInTheDocument();
    expect(screen.queryByText('tied')).not.toBeInTheDocument();
  });

  test('a collection with none of its hands loaded says so honestly and offers the way back', () => {
    mocks.collections.mockReturnValue({
      data: [{hand_id: 'older', review_marked: false, collections: ['Antigas']}],
      isLoading: false, isError: false,
    });
    mocks.query.mockReturnValue(queryResult([pageOf(hands, true)]));
    render(<HandsHistory/>);
    fireEvent.change(screen.getByLabelText('Mostrar'), {target: {value: 'Antigas'}});

    expect(screen.getByText(/Entre as 3 mãos mais recentes/)).toBeInTheDocument();
    // Loading more stays possible even though nothing is visible yet.
    expect(screen.getByRole('button', {name: 'Carregar mais mãos'})).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', {name: 'Ver todas as mãos'}));
    expect(screen.getByLabelText('Mostrar')).toHaveValue('');
    expect(screen.getByText('tied')).toBeInTheDocument();
  });

  // Issue #60: an automated floor under the a11y intent in ui/CLAUDE.md — a new
  // serious or critical axe violation on this route fails CI.
  test('is axe-clean', async () => {
    const {container} = render(<HandsHistory/>);
    await expectNoAxeViolations(container);
  });

});
