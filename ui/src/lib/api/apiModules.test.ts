import {beforeEach, describe, expect, test, vi} from 'vitest';
import {getAchievementCatalog, getMyAchievements, getMyAchievementsSummary} from './achievements';
import {leaderboard, remainingTime, spin} from './gamification';
import {createHandShare, getHandShare, revokeHandShare} from './handShares';
import {acceptPokerTerms, getHand, getHands, getMe, getProfileShowcase, getSessions, updateMe,} from './player';
import {getPlayerNotes, savePlayerNote} from './playerNotes';
import {getTodayHighlight} from './highlights';
import {getMyPokerStats} from './pokerStats';
import {createRoom, getRoom, getSeated, joinRoom, leaveRoom, listRooms, listStakes} from './rooms';
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
      .mockResolvedValueOnce({data: {remaining_time_seconds: 5}});
    client.post.mockResolvedValueOnce({data: {amount: 100, remaining_time_seconds: 60}});

    await expect(getAchievementCatalog()).resolves.toEqual(['catalog']);
    await expect(getMyAchievements('real', 'next')).resolves.toEqual(['progress']);
    await expect(getMyAchievementsSummary('real')).resolves.toEqual(summary);
    await expect(leaderboard('real', 'rank-next')).resolves.toEqual(['leader']);
    await expect(spin()).resolves.toEqual({amount: 100, remaining_time_seconds: 60});
    await expect(remainingTime()).resolves.toEqual({remaining_time_seconds: 5});

    expect(client.get).toHaveBeenNthCalledWith(2, '/v1.0/players/me/achievements', {
      params: {mode: 'real', cursor: 'next'}, silentError: true,
    });
    expect(client.get).toHaveBeenNthCalledWith(3, '/v1.0/players/me/achievements/summary', {
      params: {mode: 'real'}, silentError: true,
    });
    expect(client.post).toHaveBeenCalledWith('/v1.0/sandbox-credits', {}, {silentError: true});
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
    
    await expect(listRooms('next')).resolves.toEqual(['room']);
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
