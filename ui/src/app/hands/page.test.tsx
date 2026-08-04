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

});
