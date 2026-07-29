import {render, screen} from '@testing-library/react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import ReplayPage from './page';

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
vi.mock('@/components/TermsGate', () => ({TermsGate: ({children}: { children: React.ReactNode }) => children}));
vi.mock('@/components/hands/HandReplayer', () => ({
  HandReplayer: ({actions, viewerId}: { actions: Array<{ seq: number }>; viewerId: string }) =>
    <div data-testid="replayer">{viewerId}:{actions.map(a => a.seq).join(',')}</div>,
}));

function queryState(handState = {}, historyState = {}) {
  mocks.query.mockImplementation(({queryKey}: { queryKey: string[] }) =>
    queryKey[0] === 'hand'
      ? {data: {hand_id: 'h1'}, isLoading: false, isError: false, ...handState}
      : {data: {actions: []}, isLoading: false, isError: false, ...historyState}
  );
}

describe('hand replay page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.params = new Map([['table_id', 'table/1'], ['hand_id', 'hand 1']]);
    queryState();
  });
  
  test('rejects incomplete replay links and disables their queries', () => {
    mocks.params = new Map([['table_id', 'table-1']]);
    render(<ReplayPage/>);
    expect(screen.getByRole('heading', {name: 'Replay inválido'})).toBeInTheDocument();
    expect(mocks.query).toHaveBeenCalledWith(expect.objectContaining({enabled: false}));
  });
  
  test('waits for both backend resources and handles either failure', () => {
    queryState({}, {isLoading: true});
    const view = render(<ReplayPage/>);
    expect(screen.getByText(/Preparando a mesa/)).toBeInTheDocument();
    queryState({}, {isLoading: false, isError: true});
    view.rerender(<ReplayPage/>);
    expect(screen.getByRole('heading', {name: 'Não foi possível carregar o replay'})).toBeInTheDocument();
  });
  
  test('sorts backend actions and passes viewer identity to the engine', () => {
    queryState({}, {
      data: {
        actions: [
          {seq: 3, timestamp: 300}, {seq: 1, timestamp: 100}, {seq: 2, timestamp: 200},
        ]
      }
    });
    render(<ReplayPage/>);
    expect(screen.getByTestId('replayer')).toHaveTextContent('viewer:1,2,3');
    expect(screen.getByRole('link', {name: /Voltar para Detalhes/})).toHaveAttribute(
      'href', '/hands/history?table_id=table%2F1&hand_id=hand%201&mode=sandbox'
    );
  });
  
  test('handles a successful response with no persisted action frames', () => {
    render(<ReplayPage/>);
    expect(screen.getByTestId('replayer')).toHaveTextContent('viewer:');
  });
});
