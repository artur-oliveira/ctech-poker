import type {ReactNode} from 'react';
import {renderHook, waitFor} from '@testing-library/react';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import type {PlayerSession} from '@/lib/api/player';
import {useTableRemoval, type TableRemoval} from './useTableSession';

const mocks = vi.hoisted(() => ({push: vi.fn(), pushNotification: vi.fn()}));
vi.mock('next/navigation', () => ({useRouter: () => ({push: mocks.push})}));
vi.mock('@/lib/notify', () => ({pushNotification: mocks.pushNotification}));

const TABLE = 'table-1';
const openSession: PlayerSession = {
  table_id: TABLE, buyin_amount: 500, cashout_amount: 0, net_pnl: 0,
  joined_at: 1_700_000_000_000, ended_at: 0,
};

let client: QueryClient;

function wrapper({children}: {children: ReactNode}) {
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

function renderRemoval(props: Partial<Parameters<typeof useTableRemoval>[0]> = {}) {
  return renderHook((overrides: Partial<Parameters<typeof useTableRemoval>[0]>) => useTableRemoval({
    id: TABLE, removed: null, terminalError: null, sessions: [], sessionsLoading: false,
    ...props, ...overrides,
  }), {wrapper, initialProps: {}});
}

describe('useTableRemoval', () => {
  beforeEach(() => {
    mocks.push.mockReset();
    mocks.pushNotification.mockReset();
    client = new QueryClient({defaultOptions: {queries: {retry: false}}});
  });

  test('waits for the sessions query before building the leave recap', async () => {
    // "Not dealt in, instant leave" right after sitting down: the server
    // answers before the sessions request settles.
    const removed: TableRemoval = {code: 'exit_requested', amount: 420};
    const {result, rerender} = renderRemoval({removed, sessionsLoading: true});

    expect(result.current.sessionRecap).toBeNull();
    expect(mocks.pushNotification).not.toHaveBeenCalled();

    rerender({removed, sessions: [openSession], sessionsLoading: false});
    await waitFor(() => expect(result.current.sessionRecap).toEqual({
      joinedAt: openSession.joined_at, buyIn: 500, finalStack: 420,
    }));
    expect(mocks.pushNotification).toHaveBeenCalledTimes(1);
    expect(client.getQueryData(['seated', TABLE])).toEqual({seated: false, stack: 0});
  });

  test('handles the same removal exactly once, however many renders follow', async () => {
    const removed: TableRemoval = {code: 'exit_requested', amount: 420};
    const {result, rerender} = renderRemoval({removed, sessions: [openSession]});
    await waitFor(() => expect(result.current.sessionRecap).not.toBeNull());

    rerender({removed, sessions: [openSession, {...openSession, table_id: 'other'}]});
    rerender({removed, sessions: [openSession]});
    expect(mocks.pushNotification).toHaveBeenCalledTimes(1);
    expect(mocks.push).not.toHaveBeenCalled();
  });

  test('falls back to a zero buy-in only when the settled sessions hold no open seat', async () => {
    const removed: TableRemoval = {code: 'exit_requested', amount: 90};
    const {result} = renderRemoval({removed, sessions: [{...openSession, ended_at: 1}]});

    await waitFor(() => expect(result.current.sessionRecap).toMatchObject({buyIn: 0, finalStack: 90}));
  });

  test('an idle or disconnect kick notifies and returns to the lobby without a recap', () => {
    const {result} = renderRemoval({removed: {code: 'idle'}, sessionsLoading: true});

    expect(result.current.sessionRecap).toBeNull();
    expect(mocks.pushNotification)
      .toHaveBeenCalledWith('Você foi removido da mesa por inatividade.', 'info');
    expect(mocks.push).toHaveBeenCalledWith('/lobby');
  });

  test('an unknown removal reason still explains itself', () => {
    renderRemoval({removed: {code: 'something_new'}});
    expect(mocks.pushNotification).toHaveBeenCalledWith('Você foi removido da mesa.', 'info');
  });

  test('a terminal error clears the seat and returns to the lobby', () => {
    renderRemoval({terminalError: 'forbidden'});
    expect(mocks.pushNotification).toHaveBeenCalledWith('Você não tem acesso a esta mesa.', 'info');
    expect(mocks.push).toHaveBeenCalledWith('/lobby');

    mocks.pushNotification.mockReset();
    renderRemoval({terminalError: 'gone'});
    expect(mocks.pushNotification).toHaveBeenCalledWith('Essa sala não está mais disponível.', 'info');
  });

  test('closing the recap drops the seat and returns to the lobby', () => {
    const {result} = renderRemoval();
    result.current.closeRecap();

    expect(client.getQueryData(['seated', TABLE])).toEqual({seated: false, stack: 0});
    expect(mocks.push).toHaveBeenCalledWith('/lobby');
  });
});
