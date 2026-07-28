import {render, screen} from '@testing-library/react';
import {beforeEach, describe, expect, test, vi} from 'vitest';

const mocks = vi.hoisted(() => ({
  token: 'share-token',
  query: {} as Record<string, unknown>,
  options: undefined as unknown,
}));

vi.mock('next/navigation', () => ({
  useSearchParams: () => ({get: (key: string) => key === 'id' ? mocks.token : null}),
}));
vi.mock('@tanstack/react-query', () => ({
  useQuery: (options: unknown) => {
    mocks.options = options;
    return mocks.query;
  },
}));
vi.mock('@/components/table/PlayingCard', () => ({
  PlayingCard: ({card}: {card: string}) => <span data-testid="card">{card}</span>,
}));
vi.mock('@/components/hands/HandReplayer', () => ({
  HandReplayer: ({viewerId}: {viewerId: string}) => <div data-testid="replayer">{viewerId}</div>,
}));

import SharedHandPage from './page';

describe('shared hand page', () => {
  beforeEach(() => {
    mocks.token = 'share-token';
    mocks.query = {isLoading: true, isError: false};
  });

  test('configures and presents the loading query', () => {
    render(<SharedHandPage/>);
    expect(mocks.options).toMatchObject({
      queryKey: ['hand-share', 'share-token'],
      enabled: true,
      retry: false,
    });
    expect(screen.getByText(/Carregando mão compartilhada/)).toBeInTheDocument();
  });

  test('rejects absent and expired share links', () => {
    mocks.token = '';
    mocks.query = {isLoading: false, isError: false};
    const view = render(<SharedHandPage/>);
    expect(mocks.options).toMatchObject({enabled: false});
    expect(screen.getByRole('heading', {name: 'Link indisponível'})).toBeInTheDocument();

    mocks.token = 'expired';
    mocks.query = {isLoading: false, isError: true};
    view.rerender(<SharedHandPage/>);
    expect(screen.getByText(/revogada ou ter expirado/)).toBeInTheDocument();
  });

  test('renders a bad beat, anonymized cards and action replay', () => {
    mocks.query = {
      isLoading: false,
      isError: false,
      data: {
        token: 'share-token',
        kind: 'bad_beat',
        outcome: 'lost',
        net_change: -1250,
        ended_at: 1,
        expires_at: Date.UTC(2026, 7, 2),
        created_at: 1,
        hero_cards: ['AH', 'KH'],
        board: ['QH', 'JH', 'TH'],
        opponents: [{alias: 'Vilão 1', hole_cards: ['AS', 'AD'], won: true}],
        actions: [{frame: {stage: 'flop'}}],
      },
    };
    render(<SharedHandPage/>);
    expect(screen.getByText('Bad Beat Compartilhado')).toBeInTheDocument();
    expect(screen.getByRole('heading', {name: /-1.250 fichas/})).toBeInTheDocument();
    expect(screen.getAllByTestId('card')).toHaveLength(5);
    expect(screen.getByTestId('replayer')).toHaveTextContent('hero');
  });

  test('supports a positive hand with hidden cards and no replay', () => {
    mocks.query = {
      isLoading: false,
      isError: false,
      data: {
        token: 'share-token', kind: 'brag', outcome: 'won', net_change: 500,
        ended_at: 1, expires_at: Date.UTC(2026, 7, 2), created_at: 1,
        actions: [],
      },
    };
    render(<SharedHandPage/>);
    expect(screen.getByRole('heading', {name: /\+500 fichas/})).toBeInTheDocument();
    expect(screen.getByText('Cartas ocultas')).toBeInTheDocument();
    expect(screen.queryByTestId('replayer')).not.toBeInTheDocument();
  });
});
