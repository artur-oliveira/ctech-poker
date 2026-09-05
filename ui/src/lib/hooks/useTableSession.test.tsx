import type {ReactNode} from 'react';
import {renderHook, waitFor} from '@testing-library/react';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {useTableProgressiveSession, useTableSession} from './useTableSession';

const api = vi.hoisted(() => ({
  getRoom: vi.fn(),
  getSeated: vi.fn(),
  getHands: vi.fn(),
  getSessions: vi.fn(),
  getMe: vi.fn(),
  getPlayerNotes: vi.fn(),
  listReactionCatalog: vi.fn(),
  listReactionPurchases: vi.fn(),
}));

vi.mock('@/lib/api/rooms', () => ({getRoom: api.getRoom, getSeated: api.getSeated}));
vi.mock('@/lib/api/player', () => ({
  getHands: api.getHands, getSessions: api.getSessions, getMe: api.getMe,
}));
vi.mock('@/lib/api/playerNotes', () => ({
  getPlayerNotes: api.getPlayerNotes,
  PLAYER_NOTES_KEY: (ids: string[]) => ['player-notes', [...ids].sort().join(',')],
}));
vi.mock('@/lib/api/reactionPurchases', () => ({
  listReactionCatalog: api.listReactionCatalog,
  listReactionPurchases: api.listReactionPurchases,
  REACTION_PURCHASE_FIRST_PAGE_KEY: ['wallet', 'reaction-purchases', 'first-page'],
}));
vi.mock('next/navigation', () => ({useRouter: () => ({push: vi.fn()})}));

const ID = '01J0000000000000000000000A';
let client: QueryClient;

function wrapper({children}: { children: ReactNode }) {
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

/** The page's own composition: the critical reads, then the progressive ones
 *  gated on the first snapshot and on the reactions panel. */
function useTableReads({valid = true, seeded = false, reactionsOpen = false, opponentIds = ['seat-a']} = {}) {
  const core = useTableSession(ID, valid);
  return useTableProgressiveSession(core, {id: ID, seeded, reactionsOpen, opponentIds});
}

function progressiveCalls() {
  return [api.getHands, api.getSessions, api.getMe, api.getPlayerNotes,
    api.listReactionCatalog, api.listReactionPurchases].reduce((n, fn) => n + fn.mock.calls.length, 0);
}

describe('useTableSession request budget', () => {
  beforeEach(() => {
    api.getRoom.mockReset().mockResolvedValue({room_id: ID, big_blind: 25});
    api.getSeated.mockReset().mockResolvedValue({seated: true, stack: 500});
    api.getHands.mockReset().mockResolvedValue({data: []});
    api.getSessions.mockReset().mockResolvedValue([{table_id: ID, ended_at: 0, joined_at: 1, buyin_amount: 500}]);
    api.getMe.mockReset().mockResolvedValue({player_id: 'me'});
    api.getPlayerNotes.mockReset().mockResolvedValue([]);
    api.listReactionCatalog.mockReset().mockResolvedValue([]);
    api.listReactionPurchases.mockReset().mockResolvedValue({data: []});
    client = new QueryClient({defaultOptions: {queries: {retry: false}}});
  });

  test('an invalid room id reads nothing at all', () => {
    renderHook(() => useTableReads({valid: false}), {wrapper});
    expect(api.getRoom).not.toHaveBeenCalled();
    expect(api.getSeated).not.toHaveBeenCalled();
    expect(progressiveCalls()).toBe(0);
  });

  test('a visitor who is not seated pays only the room and the seat check', async () => {
    api.getSeated.mockResolvedValue({seated: false, stack: 0});
    const {result} = renderHook(() => useTableReads(), {wrapper});
    await waitFor(() => expect(result.current.seated).toBe(false));
    expect(api.getRoom).toHaveBeenCalledTimes(1);
    expect(api.getSeated).toHaveBeenCalledTimes(1);
    expect(progressiveCalls()).toBe(0);
  });

  test('a seated player waiting on the first snapshot pays nothing extra', async () => {
    const {result} = renderHook(() => useTableReads(), {wrapper});
    await waitFor(() => expect(result.current.seated).toBe(true));
    expect(progressiveCalls()).toBe(0);
  });

  test('the first snapshot arms the progressive reads, but not the reaction ones', async () => {
    const {result, rerender} = renderHook(props => useTableReads(props),
      {wrapper, initialProps: {seeded: false}});
    await waitFor(() => expect(result.current.seated).toBe(true));
    rerender({seeded: true});
    await waitFor(() => expect(result.current.openSession?.buyin_amount).toBe(500));
    expect(api.getHands).toHaveBeenCalledTimes(1);
    expect(api.getSessions).toHaveBeenCalledTimes(1);
    expect(api.getPlayerNotes).toHaveBeenCalledTimes(1);
    expect(api.getMe).toHaveBeenCalledTimes(1);
    expect(api.listReactionCatalog).not.toHaveBeenCalled();
    expect(api.listReactionPurchases).not.toHaveBeenCalled();
  });

  test('opening the reactions panel is what reads the catalog and the purchase page', async () => {
    const {result, rerender} = renderHook(props => useTableReads(props),
      {wrapper, initialProps: {seeded: true, reactionsOpen: false}});
    await waitFor(() => expect(api.getMe).toHaveBeenCalledTimes(1));
    expect(api.listReactionCatalog).not.toHaveBeenCalled();
    rerender({seeded: true, reactionsOpen: true});
    await waitFor(() => expect(result.current.reactionCatalogLoading).toBe(false));
    expect(api.listReactionCatalog).toHaveBeenCalledTimes(1);
    expect(api.listReactionPurchases).toHaveBeenCalledTimes(1);
    // The latch never disarms: closing the panel keeps the cached answer
    // instead of spending the two reads again on the next open.
    rerender({seeded: true, reactionsOpen: false});
    rerender({seeded: true, reactionsOpen: true});
    expect(api.listReactionCatalog).toHaveBeenCalledTimes(1);
    expect(api.listReactionPurchases).toHaveBeenCalledTimes(1);
  });

  test('private notes are read for the seated opponents only, and re-read when the seats change', async () => {
    const {rerender} = renderHook(props => useTableReads(props),
      {wrapper, initialProps: {seeded: true, opponentIds: ['b', 'a']}});
    await waitFor(() => expect(api.getPlayerNotes).toHaveBeenCalledTimes(1));
    expect(api.getPlayerNotes).toHaveBeenCalledWith(['b', 'a']);
    // Same seats in a different order is the same answer: the key is sorted,
    // so it must not cost a second read.
    rerender({seeded: true, opponentIds: ['a', 'b']});
    expect(api.getPlayerNotes).toHaveBeenCalledTimes(1);
    // A new player sitting down is a different question, and gets asked.
    rerender({seeded: true, opponentIds: ['a', 'b', 'c']});
    await waitFor(() => expect(api.getPlayerNotes).toHaveBeenCalledTimes(2));
    expect(api.getPlayerNotes).toHaveBeenLastCalledWith(['a', 'b', 'c']);
  });

  test('an empty table costs no note read at all', async () => {
    renderHook(() => useTableReads({seeded: true, opponentIds: []}), {wrapper});
    await waitFor(() => expect(api.getMe).toHaveBeenCalledTimes(1));
    expect(api.getPlayerNotes).not.toHaveBeenCalled();
  });

  test('a reconnect that drops the snapshot does not re-run the bootstrap', async () => {
    const {rerender} = renderHook(props => useTableReads(props),
      {wrapper, initialProps: {seeded: true}});
    await waitFor(() => expect(api.getMe).toHaveBeenCalledTimes(1));
    const spent = progressiveCalls();
    rerender({seeded: false});
    rerender({seeded: true});
    expect(progressiveCalls()).toBe(spent);
  });
});
