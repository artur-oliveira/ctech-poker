import {render, screen} from '@testing-library/react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import type {HandItem} from '@/lib/api/player';

const mocks = vi.hoisted(() => ({
  params: new Map<string, string>(),
  query: vi.fn(),
  viewerId: vi.fn(() => 'viewer'),
}));

vi.mock('next/navigation', () => ({useSearchParams: () => ({get: (key: string) => mocks.params.get(key) ?? null})}));
vi.mock('@tanstack/react-query', () => ({useQuery: mocks.query}));
vi.mock('@/lib/utils', async importOriginal => {
  const actual = await importOriginal<typeof import('@/lib/utils')>();
  return {...actual, getViewerId: mocks.viewerId};
});
vi.mock('@/components/TermsGate', () => ({TermsGate: ({children}: {children: React.ReactNode}) => children}));
vi.mock('@/components/table/PlayingCard', () => ({
  PlayingCard: ({card}: {card: string}) => <span data-testid="card">{card}</span>,
}));
vi.mock('@/components/hands/OutcomeBadge', () => ({
  OutcomeBadge: ({outcome}: {outcome: string}) => <span>outcome:{outcome}</span>,
}));
vi.mock('@/components/hands/ActionTimeline', () => ({
  ActionTimeline: ({actions, resolveName}: {
    actions: Array<{seq: number; player_id: string}>,
    resolveName: (id: string) => string,
  }) => <div data-testid="timeline">{actions.map(a => `${a.seq}:${resolveName(a.player_id)}`).join('|')}</div>,
}));
vi.mock('@/components/hands/DeckReveal', () => ({
  DeckReveal: ({serverSeed}: {serverSeed: string}) => <div>proof:{serverSeed}</div>,
}));
vi.mock('@/components/hands/HandExportButton', () => ({HandExportButton: () => <button>exportar</button>}));
vi.mock('@/components/hands/ShareHandDialog', () => ({ShareHandDialog: () => <button>compartilhar</button>}));

import HandHistoryPage from './page';

const hand: HandItem = {
  pk: 'viewer', sk: 'h1', table_id: 'table-123456', hand_id: 'h1', outcome: 'won',
  net_change: 500, ended_at: 1_700_000_000_000,
  hole_cards: ['AH', 'KH'], board: ['QH', 'JH', 'TH', '2C', '3D'],
  opponents: [
    {player_id: 'p2', name: 'Bia', hole_cards: ['AS', 'AD'], won: false},
    {player_id: 'p3', won: true},
  ],
  server_seed: 'seed-1', commit_hash: 'commit-1',
};

function queryState({
  handData = hand,
  handLoading = false,
  handError = false,
  historyData = {actions: [
    {seq: 2, player_id: 'p2', action: 'check', amount: 0, timestamp: 200, frame: {stage: 'flop', pot: 10}},
    {seq: 1, player_id: 'viewer', action: 'call', amount: 10, timestamp: 100},
  ]},
  historyLoading = false,
  historyError = false,
}: Record<string, unknown> = {}) {
  mocks.query.mockImplementation(({queryKey}: {queryKey: string[]}) =>
    queryKey[0] === 'hand'
      ? {data: handData, isLoading: handLoading, isError: handError}
      : {data: historyData, isLoading: historyLoading, isError: historyError}
  );
}

describe('hand detail page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.params = new Map([['table_id', 'table/one'], ['hand_id', 'hand one']]);
    queryState();
  });

  test('rejects incomplete links without executing enabled queries', () => {
    mocks.params = new Map();
    render(<HandHistoryPage/>);
    expect(screen.getByText('Link de mão inválido ou incompleto.')).toBeInTheDocument();
    expect(mocks.query).toHaveBeenCalledWith(expect.objectContaining({enabled: false}));
  });

  test('renders loading and missing-hand errors', () => {
    queryState({handLoading: true});
    const view = render(<HandHistoryPage/>);
    expect(screen.getByText(/Carregando detalhes/)).toBeInTheDocument();
    queryState({handData: undefined, handError: true});
    view.rerender(<HandHistoryPage/>);
    expect(screen.getByText(/conta correta/)).toBeInTheDocument();
  });

  test('integrates hand and chronologically sorted history responses', () => {
    render(<HandHistoryPage/>);
    expect(screen.getByText('outcome:won')).toBeInTheDocument();
    expect(screen.getByText('+500 fichas')).toBeInTheDocument();
    expect(screen.getAllByText('Royal flush').length).toBeGreaterThan(0);
    expect(screen.getByText('Cartas não reveladas')).toBeInTheDocument();
    expect(screen.getByTestId('timeline')).toHaveTextContent('1:Você|2:Bia');
    expect(screen.getByText('proof:seed-1')).toBeInTheDocument();
    expect(screen.getByRole('button', {name: /Assistir replay/})).toHaveAttribute(
      'href', '/hands/replay?table_id=table%2Fone&hand_id=hand%20one'
    );
    expect(screen.getByRole('button', {name: 'exportar'})).toBeInTheDocument();
  });

  test('shows independent history error and unavailable fairness proof', () => {
    queryState({
      handData: {...hand, server_seed: undefined, commit_hash: undefined},
      historyError: true,
    });
    render(<HandHistoryPage/>);
    expect(screen.getByText(/sequência de ações/)).toBeInTheDocument();
    expect(screen.getByText(/Prova de integridade criptográfica indisponível/)).toBeInTheDocument();
    expect(screen.queryByRole('button', {name: 'exportar'})).not.toBeInTheDocument();
  });
});
