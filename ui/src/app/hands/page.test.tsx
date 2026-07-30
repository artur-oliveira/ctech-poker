import {act, fireEvent, render, screen} from '@testing-library/react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import type {HandItem} from '@/lib/api/player';
import type {Page} from '@/lib/api/client';
import HandsHistory from './page';

const mocks = vi.hoisted(() => ({
  query: vi.fn(),
  refetch: vi.fn(),
  fetchNextPage: vi.fn(),
}));

vi.mock('@tanstack/react-query', () => ({useInfiniteQuery: mocks.query}));
vi.mock('@/components/TermsGate', () => ({TermsGate: ({children}: { children: React.ReactNode }) => children}));
vi.mock('@/components/lobby/ProfileMenu', () => ({ProfileMenu: () => <div>profile-menu</div>}));
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
  });
  
  test('summarizes backend outcomes and renders safe hand links and incomplete cards', () => {
    render(<HandsHistory/>);
    expect(screen.getByText('3', {selector: '.stat-value'})).toBeInTheDocument();
    expect(screen.getByText('+800')).toBeInTheDocument();
    expect(screen.getByText(/67%/)).toHaveTextContent('67% (2V / 1D)');
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
  
  test('filters wins, losses and restores all results from an empty filter', () => {
    const {rerender} = render(<HandsHistory/>);
    fireEvent.click(screen.getByRole('button', {name: 'Vitórias (2)'}));
    expect(screen.getByText('won')).toBeInTheDocument();
    expect(screen.getByText('tied')).toBeInTheDocument();
    expect(screen.queryByText('lost')).not.toBeInTheDocument();
    
    fireEvent.click(screen.getByRole('button', {name: 'Derrotas (1)'}));
    expect(screen.getByText('lost')).toBeInTheDocument();
    
    mocks.query.mockReturnValue(queryResult([pageOf([hands[0]])]));
    rerender(<HandsHistory/>);
    expect(screen.getByText(/Nenhuma mão encontrada neste filtro/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: 'Ver todas'}));
    expect(screen.getByText('won')).toBeInTheDocument();
  });
  
  test('handles loading, failure with retry, and an empty account', () => {
    mocks.query.mockReturnValueOnce(queryResult([], {isLoading: true}));
    const view = render(<HandsHistory/>);
    expect(screen.getByText(/Buscando seu histórico/)).toBeInTheDocument();
    
    mocks.query.mockReturnValueOnce(queryResult([], {isError: true}));
    view.rerender(<HandsHistory/>);
    fireEvent.click(screen.getByRole('button', {name: 'Tentar novamente'}));
    expect(mocks.refetch).toHaveBeenCalledOnce();
    
    mocks.query.mockReturnValueOnce(queryResult([pageOf([])]));
    view.rerender(<HandsHistory/>);
    expect(screen.getByText(/ainda não jogou nenhuma mão/)).toBeInTheDocument();
  });
  
  test('marks counts as partial, appends pages and loads more on scroll or click', () => {
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
    expect(screen.getByRole('button', {name: 'Todas (3+)'})).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Vitórias (2+)'})).toBeInTheDocument();
    expect(screen.getByText('3+', {selector: '.stat-value'})).toBeInTheDocument();
    
    act(() => observed[0]([{isIntersecting: true} as IntersectionObserverEntry], {} as IntersectionObserver));
    expect(mocks.fetchNextPage).toHaveBeenCalledOnce();
    
    fireEvent.click(screen.getByRole('button', {name: 'Carregar mais mãos'}));
    expect(mocks.fetchNextPage).toHaveBeenCalledTimes(2);
    
    const secondPage: HandItem[] = [{...hands[0], hand_id: 'h4', sk: 'hand#4', net_change: 300}];
    mocks.query.mockReturnValue(queryResult([pageOf(hands, true), pageOf(secondPage)]));
    view.rerender(<HandsHistory/>);
    expect(screen.getAllByText('won')).toHaveLength(2);
    expect(screen.getByRole('button', {name: 'Todas (4)'})).toBeInTheDocument();
    expect(screen.queryByRole('button', {name: /Carregar mais/})).not.toBeInTheDocument();
  });
});
