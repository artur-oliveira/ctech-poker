import {beforeEach, describe, expect, test, vi} from 'vitest';
import {
  acceptFriendRequest, acceptTableInvite, blockPlayer, cancelFriendRequest, declineFriendRequest, declineTableInvite,
  getRelationship, getRelationships, getSocialSummary, listBlockedPlayers, listFriendRequests, listFriends,
  listRecentPlayers, listSocialInbox, lookupFriendCode, markInboxRead, mutePlayer, removeFriend, reportPlayer,
  sendFriendRequest, sendTableInvite, unblockPlayer, unmutePlayer
} from './social';

const client = vi.hoisted(() => ({get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn()}));
vi.mock('./client', () => ({apiClient: client}));

describe('social API module', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    client.get.mockResolvedValue({data: {data: ['row'], unread_count: 2}});
    client.post.mockResolvedValue({data: {ok: true}});
    client.put.mockResolvedValue({data: {ok: true}});
    client.delete.mockResolvedValue({data: undefined});
    vi.spyOn(globalThis.crypto, 'randomUUID').mockReturnValue('11111111-1111-4111-8111-111111111111');
  });

  test('reads every list through the cursor envelope', async () => {
    await expect(getSocialSummary()).resolves.toEqual({data: ['row'], unread_count: 2});
    await listFriends('cursor-1');
    await listFriendRequests('outgoing');
    await listRecentPlayers();
    await listBlockedPlayers();
    await listSocialInbox();

    expect(client.get).toHaveBeenNthCalledWith(2, '/v1.0/social/friends',
      {params: {cursor: 'cursor-1'}, silentError: true});
    expect(client.get).toHaveBeenNthCalledWith(3, '/v1.0/social/requests',
      {params: {direction: 'outgoing', cursor: undefined}, silentError: true});
    expect(client.get).toHaveBeenNthCalledWith(4, '/v1.0/social/recent',
      {params: {cursor: undefined}, silentError: true});
    expect(client.get).toHaveBeenNthCalledWith(5, '/v1.0/social/blocked',
      {params: {cursor: undefined}, silentError: true});
    expect(client.get).toHaveBeenNthCalledWith(6, '/v1.0/social/inbox',
      {params: {cursor: undefined}, silentError: true});
  });

  test('resolves a friend code and a relationship by exact identifier', async () => {
    await lookupFriendCode('pkr-1/2');
    await getRelationship('player/1');
    expect(client.get).toHaveBeenNthCalledWith(1, '/v1.0/social/lookup/pkr-1%2F2', {silentError: true});
    expect(client.get).toHaveBeenNthCalledWith(2, '/v1.0/social/relationships/player%2F1', {silentError: true});
  });

  test('batches a seat list into one relationship request and skips an empty one', async () => {
    await expect(getRelationships([])).resolves.toEqual([]);
    expect(client.get).not.toHaveBeenCalled();

    await expect(getRelationships(['a', 'b'])).resolves.toEqual(['row']);
    expect(client.get).toHaveBeenCalledWith('/v1.0/social/relationships',
      {params: {player_ids: 'a,b'}, silentError: true});
  });

  test('sends an idempotency key with every mutation', async () => {
    const idempotent = {headers: {'Idempotency-Key': '11111111-1111-4111-8111-111111111111'}, silentError: true};
    await sendFriendRequest({friend_code: 'PKR-1'});
    await acceptFriendRequest('p1');
    await declineFriendRequest('p1');
    await cancelFriendRequest('p1');
    await removeFriend('p1');
    await mutePlayer('p1');
    await unmutePlayer('p1');
    await blockPlayer('p1');
    await unblockPlayer('p1');
    await markInboxRead(['e1']);
    await sendTableInvite('p1', 'room-1');
    await acceptTableInvite('e1');
    await declineTableInvite('e1');
    await reportPlayer({target_player_id: 'p1', category: 'spam', surface: 'table_chat'});

    expect(client.post).toHaveBeenCalledWith('/v1.0/social/friend-requests',
      {target_player_id: undefined, friend_code: 'PKR-1'}, idempotent);
    expect(client.post).toHaveBeenCalledWith('/v1.0/social/friend-requests/p1/accept', {}, idempotent);
    expect(client.post).toHaveBeenCalledWith('/v1.0/social/friend-requests/p1/decline', {}, idempotent);
    expect(client.delete).toHaveBeenCalledWith('/v1.0/social/friend-requests/p1', idempotent);
    expect(client.delete).toHaveBeenCalledWith('/v1.0/social/friends/p1', idempotent);
    expect(client.put).toHaveBeenCalledWith('/v1.0/social/mutes/p1', {}, idempotent);
    expect(client.delete).toHaveBeenCalledWith('/v1.0/social/mutes/p1', idempotent);
    expect(client.put).toHaveBeenCalledWith('/v1.0/social/blocks/p1', {}, idempotent);
    expect(client.delete).toHaveBeenCalledWith('/v1.0/social/blocks/p1', idempotent);
    expect(client.post).toHaveBeenCalledWith('/v1.0/social/inbox/read', {event_ids: ['e1']}, idempotent);
    expect(client.post).toHaveBeenCalledWith('/v1.0/social/table-invites',
      {target_player_id: 'p1', room_id: 'room-1'}, idempotent);
    expect(client.post).toHaveBeenCalledWith('/v1.0/social/table-invites/e1/accept', {}, idempotent);
    expect(client.post).toHaveBeenCalledWith('/v1.0/social/table-invites/e1/decline', {}, idempotent);
    expect(client.post).toHaveBeenCalledWith('/v1.0/social/reports',
      {target_player_id: 'p1', category: 'spam', surface: 'table_chat'}, idempotent);
  });

  test('requests a friend by player id when no code is given', async () => {
    await sendFriendRequest({player_id: 'p2'});
    expect(client.post).toHaveBeenCalledWith('/v1.0/social/friend-requests',
      {target_player_id: 'p2', friend_code: undefined},
      {headers: {'Idempotency-Key': '11111111-1111-4111-8111-111111111111'}, silentError: true});
  });
});
