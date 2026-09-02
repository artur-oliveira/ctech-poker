import type {Page} from './client';
import {apiClient} from './client';
import type {Room} from './rooms';

export type SocialRelationship = 'none' | 'outgoing' | 'incoming' | 'friend';
export type PresenceStatus = 'online' | 'offline' | 'in_table';
export type SocialEventType = 'friend_request' | 'friend_accepted' | 'table_invite';
export type SocialEventStatus = 'pending' | 'accepted' | 'declined';

/** Minimal social DTO. The server never publishes whether the *other* player
 * blocked the viewer, so there is deliberately no `blocked_by_other` here. */
export interface SocialPlayer {
  player_id: string;
  name?: string;
  avatar_url?: string;
  friend_code?: string;
  relationship: SocialRelationship;
  muted: boolean;
  blocked: boolean;
  presence?: PresenceStatus;
  last_played_at?: number;
  hands_together?: number;
  /** Present only for a friend who opted in and is sitting at a joinable
   * public table. The join flow revalidates everything; this is a shortcut. */
  room_id?: string;
}

export interface SocialInboxEvent {
  event_id: string;
  type: SocialEventType;
  actor_id: string;
  /** Resolved server-side from actor_id via a single batch lookup per feed
   * page (#73) — present for any actor with a profile, not just one already
   * loaded into the friends/requests lists. Empty when the profile can't be
   * resolved (e.g. deleted); callers should fall back to a placeholder. */
  actor_name?: string;
  actor_avatar_url?: string;
  status: SocialEventStatus;
  room_id?: string;
  unread: boolean;
  created_at: number;
  expires_at?: number;
}

export const REPORT_CATEGORIES = ['harassment', 'hate', 'spam', 'cheating', 'inappropriate_profile', 'other'] as const;
export type ReportCategory = typeof REPORT_CATEGORIES[number];
export type ReportSurface = 'table_chat' | 'table_reaction' | 'table_behavior' | 'profile' | 'recent_player';

const SOCIAL = '/v1.0/social';

// Every social mutation is idempotent by contract: the server derives the
// event id from actor+target+operation+key, so a retried click cannot create a
// second inbox entry or a second report.
// silentError: the curated pt-BR copy comes from socialErrorMessage (see
// lib/social.ts), so the generic interceptor toast would only duplicate it.
function idempotent() {
  return {headers: {'Idempotency-Key': crypto.randomUUID()}, silentError: true};
}

function playerPath(base: string, playerId: string) {
  return `${SOCIAL}${base}/${encodeURIComponent(playerId)}`;
}

export async function getSocialSummary() {
  return (await apiClient.get<{unread_count: number}>(`${SOCIAL}/summary`, {silentError: true})).data;
}

export async function listFriends(cursor?: string) {
  return (await apiClient.get<Page<SocialPlayer>>(`${SOCIAL}/friends`, {params: {cursor}, silentError: true})).data;
}

export async function listFriendRequests(direction: 'incoming' | 'outgoing', cursor?: string) {
  return (await apiClient.get<Page<SocialPlayer>>(`${SOCIAL}/friend-requests`, {
    params: {direction, cursor}, silentError: true
  })).data;
}

export async function listRecentPlayers(cursor?: string) {
  return (await apiClient.get<Page<SocialPlayer>>(`${SOCIAL}/recent`, {params: {cursor}, silentError: true})).data;
}

export async function listBlockedPlayers(cursor?: string) {
  return (await apiClient.get<Page<SocialPlayer>>(`${SOCIAL}/blocked`, {params: {cursor}, silentError: true})).data;
}

export async function listSocialInbox(cursor?: string) {
  return (await apiClient.get<Page<SocialInboxEvent>>(`${SOCIAL}/inbox`, {
    params: {cursor}, silentError: true
  })).data;
}

export async function markInboxRead(eventIds: string[]) {
  await apiClient.post(`${SOCIAL}/inbox/read`, {event_ids: eventIds}, idempotent());
}

export async function lookupFriendCode(code: string) {
  return (await apiClient.get<SocialPlayer>(`${SOCIAL}/lookup/${encodeURIComponent(code)}`, {
    silentError: true
  })).data;
}

export async function getRelationship(playerId: string) {
  return (await apiClient.get<SocialPlayer>(playerPath('/relationships', playerId), {silentError: true})).data;
}

/** One request for a whole seat list: the table needs the mute/block state of
 * every seated player before chat and reactions reach React state. */
export async function getRelationships(playerIds: string[]) {
  if (!playerIds.length) return [];
  return (await apiClient.get<{data: SocialPlayer[]}>(`${SOCIAL}/relationships`, {
    params: {player_ids: playerIds.join(',')}, silentError: true
  })).data.data;
}

export async function sendFriendRequest(target: {player_id?: string; friend_code?: string}) {
  return (await apiClient.post<SocialPlayer>(`${SOCIAL}/friend-requests`, {
    target_player_id: target.player_id, friend_code: target.friend_code
  }, idempotent())).data;
}

export async function acceptFriendRequest(playerId: string) {
  return (await apiClient.post<SocialPlayer>(`${playerPath('/friend-requests', playerId)}/accept`, {},
    idempotent())).data;
}

export async function declineFriendRequest(playerId: string) {
  await apiClient.post(`${playerPath('/friend-requests', playerId)}/decline`, {}, idempotent());
}

export async function cancelFriendRequest(playerId: string) {
  await apiClient.delete(playerPath('/friend-requests', playerId), idempotent());
}

export async function removeFriend(playerId: string) {
  await apiClient.delete(playerPath('/friends', playerId), idempotent());
}

export async function mutePlayer(playerId: string) {
  return (await apiClient.put<SocialPlayer>(playerPath('/mutes', playerId), {}, idempotent())).data;
}

export async function unmutePlayer(playerId: string) {
  await apiClient.delete(playerPath('/mutes', playerId), idempotent());
}

export async function blockPlayer(playerId: string) {
  return (await apiClient.put<SocialPlayer>(playerPath('/blocks', playerId), {}, idempotent())).data;
}

export async function unblockPlayer(playerId: string) {
  await apiClient.delete(playerPath('/blocks', playerId), idempotent());
}

export async function sendTableInvite(targetPlayerId: string, roomId: string) {
  return (await apiClient.post<SocialInboxEvent>(`${SOCIAL}/table-invites`, {
    target_player_id: targetPlayerId, room_id: roomId
  }, idempotent())).data;
}

export async function acceptTableInvite(eventId: string) {
  return (await apiClient.post<{event: SocialInboxEvent; room: Room}>(
    `${SOCIAL}/table-invites/${encodeURIComponent(eventId)}/accept`, {}, idempotent())).data;
}

export async function declineTableInvite(eventId: string) {
  await apiClient.post(`${SOCIAL}/table-invites/${encodeURIComponent(eventId)}/decline`, {}, idempotent());
}

export async function reportPlayer(input: {
  target_player_id: string;
  category: ReportCategory;
  surface: ReportSurface;
  table_id?: string;
  hand_id?: string;
  action_id?: string;
  details?: string;
}) {
  return (await apiClient.post<{report_id: string; status: string}>(`${SOCIAL}/reports`, input, idempotent())).data;
}
