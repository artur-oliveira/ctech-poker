import {apiClient} from '@/lib/api/client';
import type {HandHistoryAction} from '@/lib/api/table';
import type {HandOutcome, WalletMode} from '@/lib/api/player';

export interface PublicHandShare {
  token: string;
  kind: 'brag' | 'bad_beat';
  outcome: HandOutcome;
  net_change: number;
  ended_at: number;
  // Blind level in force for the shared hand (backend issue #75). Absent on
  // shares created before it; HandReplayer then derives it from the timeline.
  small_blind?: number;
  big_blind?: number;
  board?: string[];
  hero_cards?: string[];
  opponents?: Array<{ alias: string; hole_cards?: string[]; won?: boolean }>;
  actions?: HandHistoryAction[];
  created_at: number;
  expires_at: number;
}

// One row of `GET /v1.0/players/me/hand-shares` (#77): enough to recognize a
// link and revoke it, without the replay projection the public payload carries.
export interface HandShareSummary {
  token: string;
  kind: 'brag' | 'bad_beat';
  outcome: HandOutcome;
  net_change: number;
  created_at: number;
  expires_at: number;
}

export const HAND_SHARES_QUERY_KEY = ['hand-shares', 'mine'] as const;

/** The caller's own live shares. Scoped server-side to the bearer's player id. */
export async function listMyHandShares() {
  return (await apiClient.get<{ data: HandShareSummary[] }>(
    '/v1.0/players/me/hand-shares', {silentError: true}
  )).data.data;
}

export async function createHandShare(handId: string, input: {
  kind: 'brag' | 'bad_beat';
  include_hero_cards: boolean;
  expiry_days: number;
}, mode: WalletMode = 'sandbox') {
  return (await apiClient.post<PublicHandShare>(
    `/v1.0/players/me/hand/${encodeURIComponent(handId)}/share`, {...input, mode}
  )).data;
}

export async function getHandShare(token: string) {
  return (await apiClient.get<PublicHandShare>(
    `/v1.0/hand-shares/${encodeURIComponent(token)}`, {silentError: true}
  )).data;
}

export async function revokeHandShare(token: string) {
  await apiClient.delete(`/v1.0/players/me/hand-shares/${encodeURIComponent(token)}`);
}
