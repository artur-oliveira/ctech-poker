import {beforeEach, describe, expect, test, vi} from 'vitest';
import {getAchievementCatalog, getMyAchievements, getMyAchievementsSummary} from './achievements';
import {leaderboard, myRank, remainingTime, spin} from './gamification';
import {createHandShare, getHandShare, revokeHandShare} from './handShares';
import {acceptPokerTerms, getHand, getHands, getMe, getProfileShowcase, getSessions, updateMe,} from './player';
import {getPlayerNotes, savePlayerNote} from './playerNotes';
import {getTodayHighlight} from './highlights';
import {getMyPokerStats} from './pokerStats';
import {createRoom, getRoom, getSeated, joinRoom, leaveRoom, listAllRooms, listRooms, listStakes, MAX_ROOM_LIST_PAGES} from './rooms';
import {getHandHistory} from './table';
import {
  createReactionPurchase, getReactionPurchase, listReactionCatalog, listReactionPurchases,
  refundReactionPurchase
} from './reactionPurchases';

const client = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  delete: vi.fn(),
}));
vi.mock('./client', () => ({apiClient: client}));

describe('API domain modules', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    client.get.mockResolvedValue({data: {data: ['item'], stakes: ['stake'], value: true}});
    client.post.mockResolvedValue({data: {value: true}});
    client.delete.mockResolvedValue({data: undefined});
    vi.spyOn(globalThis.crypto, 'randomUUID').mockReturnValue('idem-key');
  });
  
  test('maps achievement and gamification calls to their backend contracts', async () => {
    const summary = {mode: 'real', totals: {revealed: 1, unlocked: 1, completed: 0, stars: 1, max_stars: 5}, achievements: [{key: 'wins'}]};
    client.get
      .mockResolvedValueOnce({data: ['catalog']})
      .mockResolvedValueOnce({data: {data: ['progress']}})
      .mockResolvedValueOnce({data: summary})
      .mockResolvedValueOnce({data: {data: ['leader']}})
      .mockResolvedValueOnce({data: {remaining_time_seconds: 5}})
      .mockResolvedValueOnce({data: {ranked: true, rank: 12, total: 480, entry: {player_id: 'me'}}});
    client.post.mockResolvedValueOnce({data: {amount: 100, remaining_time_seconds: 60}});

    await expect(getAchievementCatalog()).resolves.toEqual(['catalog']);
    await expect(getMyAchievements('real', 'next')).resolves.toEqual(['progress']);
    await expect(getMyAchievementsSummary('real')).resolves.toEqual(summary);
    await expect(leaderboard('real', 'rank-next')).resolves.toEqual(['leader']);
    await expect(spin()).resolves.toEqual({amount: 100, remaining_time_seconds: 60});
    await expect(remainingTime()).resolves.toEqual({remaining_time_seconds: 5});
    await expect(myRank('real')).resolves.toEqual({ranked: true, rank: 12, total: 480, entry: {player_id: 'me'}});

    expect(client.get).toHaveBeenNthCalledWith(2, '/v1.0/players/me/achievements', {
      params: {mode: 'real', cursor: 'next'}, silentError: true,
    });
    expect(client.get).toHaveBeenNthCalledWith(3, '/v1.0/players/me/achievements/summary', {
      params: {mode: 'real'}, silentError: true,
    });
    expect(client.post).toHaveBeenCalledWith('/v1.0/sandbox-credits', {}, {silentError: true});
    expect(client.get).toHaveBeenCalledWith('/v1.0/leaderboard/me', {params: {mode: 'real'}});
  });
  
  test('encodes identifiers for player profile, hands and public shares', async () => {
    client.get
      .mockResolvedValueOnce({data: {user_id: 'me'}})
      .mockResolvedValueOnce({data: {player_id: 'a/b'}})
      .mockResolvedValueOnce({data: {data: ['session']}})
      .mockResolvedValueOnce({data: {data: ['hand']}})
      .mockResolvedValueOnce({data: {hand_id: 'h/1'}})
      .mockResolvedValueOnce({data: {token: 'share'}})
      .mockResolvedValueOnce({data: {hands: 10}});
    client.post
      .mockResolvedValueOnce({data: {accepted: true}})
      .mockResolvedValueOnce({data: {name: 'Novo'}})
      .mockResolvedValueOnce({data: {token: 'created'}});
    
    await getMe();
    await acceptPokerTerms();
    await updateMe({name: 'Novo'});
    await getProfileShowcase('a/b');
    await expect(getSessions('older')).resolves.toEqual(['session']);
    await expect(getHands({cursor: 'more', tableId: 'table-1'})).resolves.toEqual({data: ['hand']});
    await getHand('h/1');
    await createHandShare('h/1', {kind: 'brag', include_hero_cards: true, expiry_days: 7});
    await getHandShare('a/b');
    await revokeHandShare('a/b');
    await getMyPokerStats();
    
    expect(client.get).toHaveBeenCalledWith('/v1.0/players/a%2Fb/showcase', {silentError: true});
    expect(client.get).toHaveBeenCalledWith('/v1.0/players/me/hand/h%2F1', {
      params: {mode: 'sandbox'}, silentError: true
    });
    expect(client.post).toHaveBeenCalledWith(
      '/v1.0/players/me/hand/h%2F1/share',
      {kind: 'brag', include_hero_cards: true, expiry_days: 7, mode: 'sandbox'},
    );
    expect(client.delete).toHaveBeenCalledWith('/v1.0/players/me/hand-shares/a%2Fb');
  });
  
  test('covers room listing, creation, seat lifecycle and idempotency keys', async () => {
    client.get
      .mockResolvedValueOnce({data: {data: ['room']}})
      .mockResolvedValueOnce({data: {stakes: ['1/2']}})
      .mockResolvedValueOnce({data: {id: 'table-1'}})
      .mockResolvedValueOnce({data: {seated: true, stack: 500}});
    client.post
      .mockResolvedValueOnce({data: {id: 'created'}})
      .mockResolvedValueOnce({data: undefined})
      .mockResolvedValueOnce({data: {amount: 500}});
    
    await expect(listRooms('next', 'sandbox')).resolves.toEqual(['room']);
    expect(client.get).toHaveBeenCalledWith('/v1.0/rooms', {params: {cursor: 'next', currency_mode: 'sandbox'}});
    await expect(listStakes('real')).resolves.toEqual(['1/2']);
    await getRoom('table-1');
    await createRoom({
      visibility: 'private',
      small_blind: 1,
      big_blind: 2,
      max_seats: 6,
      buy_in_min: 100,
      buy_in_max: 1000,
    });
    await joinRoom('table-1', 500, 'secret');
    await expect(getSeated('table-1')).resolves.toEqual({seated: true, stack: 500});
    await expect(leaveRoom('table-1')).resolves.toEqual({amount: 500});
    await joinRoom('table-1', 500, undefined, true);

    expect(client.post).toHaveBeenCalledWith('/v1.0/rooms/table-1/join', {
      amount: 500, share_code: 'secret', idem_key: 'idem-key',
    }, {silentError: true});
    expect(client.post).toHaveBeenCalledWith('/v1.0/rooms/table-1/join', {
      amount: 500, share_code: undefined, auto_rebuy: true, idem_key: 'idem-key',
    }, {silentError: true});
    expect(client.post).toHaveBeenCalledWith('/v1.0/rooms/table-1/leave', {
      idem_key: 'idem-key',
    }, {silentError: true});
  });
  
  test('listAllRooms walks the cursor across every page, scoped to the requested currency mode', async () => {
    client.get
      .mockResolvedValueOnce({
        data: {data: [{id: 'r1'}], has_next: true, next_cursor: 'p2', has_previous: false, previous_cursor: null},
      })
      .mockResolvedValueOnce({
        data: {data: [{id: 'r2'}], has_next: true, next_cursor: 'p3', has_previous: true, previous_cursor: 'p1'},
      })
      .mockResolvedValueOnce({
        data: {data: [{id: 'r3'}], has_next: false, next_cursor: null, has_previous: true, previous_cursor: 'p2'},
      });

    await expect(listAllRooms('sandbox')).resolves.toEqual([{id: 'r1'}, {id: 'r2'}, {id: 'r3'}]);

    expect(client.get).toHaveBeenNthCalledWith(1, '/v1.0/rooms', {params: {cursor: undefined, currency_mode: 'sandbox'}});
    expect(client.get).toHaveBeenNthCalledWith(2, '/v1.0/rooms', {params: {cursor: 'p2', currency_mode: 'sandbox'}});
    expect(client.get).toHaveBeenNthCalledWith(3, '/v1.0/rooms', {params: {cursor: 'p3', currency_mode: 'sandbox'}});
  });

  test('listAllRooms stops at the page cap instead of hanging when the server never terminates the cursor', async () => {
    client.get.mockResolvedValue({
      data: {data: [{id: 'r'}], has_next: true, next_cursor: 'more', has_previous: true, previous_cursor: 'more'},
    });

    const rooms = await listAllRooms('sandbox');

    expect(rooms).toHaveLength(MAX_ROOM_LIST_PAGES);
    expect(client.get).toHaveBeenCalledTimes(MAX_ROOM_LIST_PAGES);
  });

  test('covers notes and hand-history endpoints with encoded opponent ids', async () => {
    client.get
      .mockResolvedValueOnce({data: {data: ['note']}})
      .mockResolvedValueOnce({data: {hand_id: 'hand-1'}})
      .mockResolvedValueOnce({data: {table_id: 'table/1', pot: 500}});
    client.post.mockResolvedValueOnce({data: {opponent_id: 'a/b'}});

    await expect(getPlayerNotes()).resolves.toEqual(['note']);
    await savePlayerNote('a/b', {tag: 'red', note: 'agressivo'});
    await getHandHistory('table-1', 'hand-1');
    await expect(getTodayHighlight('table/1')).resolves.toEqual({table_id: 'table/1', pot: 500});

    expect(client.post).toHaveBeenCalledWith(
      '/v1.0/players/me/notes/a%2Fb',
      {tag: 'red', note: 'agressivo'},
    );
    expect(client.get).toHaveBeenCalledWith(
      '/v1.0/tables/table-1/hands/hand-1/history',
      {silentError: true},
    );
    expect(client.get).toHaveBeenCalledWith(
      '/v1.0/rooms/table%2F1/highlights/today',
      {silentError: true},
    );
  });

  test('maps premium reaction catalog, purchase, refresh and refund contracts', async () => {
    client.get
      .mockResolvedValueOnce({data: [{id: 'cold', premium: true}]})
      .mockResolvedValueOnce({data: [{purchase_id: 'rp-1'}]})
      .mockResolvedValueOnce({data: {purchase_id: 'rp/1', status: 'confirmed'}});
    client.post
      .mockResolvedValueOnce({data: {purchase_id: 'rp-1', status: 'pending'}})
      .mockResolvedValueOnce({data: {purchase_id: 'rp/1', status: 'refunded'}});

    await listReactionCatalog();
    await listReactionPurchases();
    await createReactionPurchase('cold', 'pix');
    await getReactionPurchase('rp/1');
    await refundReactionPurchase('rp/1');

    expect(client.post).toHaveBeenCalledWith('/v1.0/wallet/reaction-purchase/', {
      reaction_id: 'cold', method: 'pix', idem_key: 'idem-key'
    }, {silentError: true});
    expect(client.get).toHaveBeenCalledWith('/v1.0/wallet/reaction-purchase/rp%2F1', {silentError: true});
    expect(client.post).toHaveBeenCalledWith('/v1.0/wallet/reaction-purchase/rp%2F1/refund', {
      idem_key: 'idem-key'
    }, {silentError: true});
  });
});
