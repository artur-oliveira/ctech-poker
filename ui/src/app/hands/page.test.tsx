import {fireEvent, render, screen} from '@testing-library/react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import type {HandItem} from '@/lib/api/player';

const mocks = vi.hoisted(() => ({
  query: vi.fn(),
  refetch: vi.fn(),
}));

vi.mock('@tanstack/react-query', () => ({useQuery: mocks.query}));
vi.mock('@/components/TermsGate', () => ({TermsGate: ({children}: {children: React.ReactNode}) => children}));
vi.mock('@/components/lobby/ProfileMenu', () => ({ProfileMenu: () => <div>profile-menu</div>}));
vi.mock('@/components/table/PlayingCard', () => ({
  PlayingCard: ({card}: {card: string}) => <span data-testid="card">{card}</span>,
}));
vi.mock('@/components/hands/OutcomeBadge', () => ({
  OutcomeBadge: ({outcome}: {outcome: string}) => <span>{outcome}</span>,
}));

import HandsHistory from './page';

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

describe('hands list page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.query.mockReturnValue({data: hands, isLoading: false, isError: false, refetch: mocks.refetch});
  });

  test('summarizes backend outcomes and renders safe hand links and incomplete cards', () => {
    render(<HandsHistory/>);
    expect(screen.getByText('3', {selector: '.stat-value'})).toBeInTheDocument();
    expect(screen.getByText('+800')).toBeInTheDocument();
    expect(screen.getByText(/67%/)).toHaveTextContent('67% (2V / 1D)');
    expect(screen.getByText(/Royal flush/)).toBeInTheDocument();
    expect(screen.getAllByTestId('card')).toHaveLength(9);
    expect(screen.getByRole('link', {name: /won/})).toHaveAttribute(
      'href', '/hands/history?table_id=table-one&hand_id=hand%231'
    );
    expect(screen.getByTitle('1234567890abcdef')).toHaveTextContent('seed 12345678…');
  });

  test('filters wins, losses and restores all results from an empty filter', () => {
    const {rerender} = render(<HandsHistory/>);
    fireEvent.click(screen.getByRole('tab', {name: 'Vitórias (2)'}));
    expect(screen.getByText('won')).toBeInTheDocument();
    expect(screen.getByText('tied')).toBeInTheDocument();
    expect(screen.queryByText('lost')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('tab', {name: 'Derrotas (1)'}));
    expect(screen.getByText('lost')).toBeInTheDocument();

    mocks.query.mockReturnValue({
      data: [hands[0]], isLoading: false, isError: false, refetch: mocks.refetch,
    });
    rerender(<HandsHistory/>);
    expect(screen.getByText('Nenhuma mão encontrada neste filtro.')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: 'Ver todas'}));
    expect(screen.getByText('won')).toBeInTheDocument();
  });

  test('handles loading, failure with retry, and an empty account', () => {
    mocks.query.mockReturnValueOnce({data: [], isLoading: true, isError: false, refetch: mocks.refetch});
    const view = render(<HandsHistory/>);
    expect(screen.getByText(/Buscando seu histórico/)).toBeInTheDocument();

    mocks.query.mockReturnValueOnce({data: [], isLoading: false, isError: true, refetch: mocks.refetch});
    view.rerender(<HandsHistory/>);
    fireEvent.click(screen.getByRole('button', {name: 'Tentar novamente'}));
    expect(mocks.refetch).toHaveBeenCalledOnce();

    mocks.query.mockReturnValueOnce({data: [], isLoading: false, isError: false, refetch: mocks.refetch});
    view.rerender(<HandsHistory/>);
    expect(screen.getByText(/ainda não jogou nenhuma mão/)).toBeInTheDocument();
  });
});
