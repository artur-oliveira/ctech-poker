import {ApiError} from '@/lib/api/client';
import type {PresenceStatus, SocialInboxEvent, SocialPlayer, SocialRelationship} from '@/lib/api/social';
import {playerName} from '@/lib/utils';

// Every social query lives under the same root key, so a realtime push (or a
// reconnect, which replays nothing) can invalidate the whole surface with one
// call instead of listing every list it touched.
export const SOCIAL_ROOT_KEY = 'social';

export const SOCIAL_KEYS = {
  root: [SOCIAL_ROOT_KEY] as const,
  summary: [SOCIAL_ROOT_KEY, 'summary'] as const,
  friends: [SOCIAL_ROOT_KEY, 'friends'] as const,
  requests: (direction: 'incoming' | 'outgoing') => [SOCIAL_ROOT_KEY, 'requests', direction] as const,
  recent: [SOCIAL_ROOT_KEY, 'recent'] as const,
  blocked: [SOCIAL_ROOT_KEY, 'blocked'] as const,
  inbox: [SOCIAL_ROOT_KEY, 'inbox'] as const,
  relationships: (playerIds: string[]) => [SOCIAL_ROOT_KEY, 'relationships', [...playerIds].sort().join(',')] as const,
  relationship: (playerId: string) => [SOCIAL_ROOT_KEY, 'relationship', playerId] as const,
};

export const PRESENCE_LABELS: Record<PresenceStatus, string> = {
  online: 'Online',
  offline: 'Offline',
  in_table: 'Em uma mesa'
};

// Presence never carries the table, room code, blinds or balance — an
// `in_table` friend is only ever "em uma mesa".
export function presenceLabel(presence?: PresenceStatus) {
  return presence ? PRESENCE_LABELS[presence] : PRESENCE_LABELS.offline;
}

export const RELATIONSHIP_LABELS: Record<SocialRelationship, string> = {
  none: 'Sem conexão',
  outgoing: 'Solicitação enviada',
  incoming: 'Solicitação recebida',
  friend: 'Amigo'
};

/** Authoritative suppression set for the table surface: mute and block both
 * hide chat and reactions, and never anything else about the player. */
export function suppressedPlayerIds(players: SocialPlayer[] = []) {
  return new Set(players.filter(player => player.muted || player.blocked).map(player => player.player_id));
}

/** Inbox events carry only the actor's id; the lists already on screen are
 * where the names come from, so no extra request is needed to read the feed. */
export function nameResolver(...groups: SocialPlayer[][]) {
  const names = new Map<string, string>();
  for (const group of groups) {
    for (const player of group) {
      if (player.name) names.set(player.player_id, player.name);
    }
  }
  return (playerId: string) => playerName(playerId, undefined, names.get(playerId));
}

export const INVITE_TTL_MS = 15 * 60 * 1000;

export function inviteExpired(event: SocialInboxEvent, nowMs = Date.now()) {
  return Boolean(event.expires_at && event.expires_at <= nowMs);
}

/** An invite is only actionable while pending and unexpired; friend requests
 * are answered from the requests tab, so the inbox only ever links to them. */
export function inviteActionable(event: SocialInboxEvent, nowMs = Date.now()) {
  return event.type === 'table_invite' && event.status === 'pending' && !inviteExpired(event, nowMs);
}

export function socialEventCopy(event: SocialInboxEvent, actorName: string, nowMs = Date.now()) {
  if (event.type === 'friend_request') return `${actorName} quer ser seu amigo.`;
  if (event.type === 'friend_accepted') return `${actorName} aceitou sua solicitação de amizade.`;
  if (event.status === 'accepted') return `Você aceitou o convite de ${actorName}.`;
  if (event.status === 'declined') return `Você recusou o convite de ${actorName}.`;
  if (inviteExpired(event, nowMs)) return `O convite de ${actorName} para uma mesa expirou.`;
  return `${actorName} te convidou para uma mesa.`;
}

// Keyed by the stable Problem Details `type`, never by HTTP status or message:
// relationship_conflict deliberately reads the same whether the cause was a
// stale state or the other player blocking the viewer.
const SOCIAL_PROBLEM_COPY: Record<string, string> = {
  '/problems/social-disabled': 'Os recursos sociais estão temporariamente indisponíveis.',
  '/problems/friend-limit-reached': 'Você atingiu o limite de amigos.',
  '/problems/request-limit-reached': 'Você tem muitas solicitações pendentes. Responda algumas antes de enviar outra.',
  '/problems/relationship-conflict': 'Não foi possível concluir essa ação agora.',
  '/problems/invite-expired': 'Esse convite expirou.',
  '/problems/invite-already-pending': 'Já existe um convite pendente para esse amigo nesta mesa.',
  '/problems/table-full': 'A mesa está cheia.',
  '/problems/room-closed': 'Essa sala não está mais disponível.',
  '/problems/report-rate-limited': 'Você enviou muitas denúncias em pouco tempo. Tente novamente mais tarde.',
  '/problems/friend-code-unavailable': 'Esse código de amizade está indisponível. Tente novamente mais tarde.'
};

export function socialErrorMessage(error: unknown) {
  const type = error instanceof ApiError ? error.problem?.type : undefined;
  if (type && SOCIAL_PROBLEM_COPY[type]) return SOCIAL_PROBLEM_COPY[type];
  if (error instanceof ApiError && error.status === 404) return 'Jogador não encontrado.';
  return 'Não foi possível concluir essa ação. Tente novamente.';
}

const DAY_MS = 24 * 60 * 60 * 1000;

/** Recent players are capped at 90 days server-side, so this only ever has to
 * describe a window of about three months. */
export function lastPlayedLabel(lastPlayedAt?: number, nowMs = Date.now()) {
  if (!lastPlayedAt) return '';
  const days = Math.floor((nowMs - lastPlayedAt) / DAY_MS);
  if (days <= 0) return 'Jogaram hoje';
  if (days === 1) return 'Jogaram ontem';
  return `Jogaram há ${days} dias`;
}
