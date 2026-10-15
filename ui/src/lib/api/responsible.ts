import {apiClient} from './client';

export const RESPONSIBLE_KEY = ['responsible'] as const;

/** The addendum the player accepts to play for real money. The text itself lives on the wallet's site. */
export const GAMBLING_ADDENDUM_URL = 'https://wallet.aoctech.app/gambling-addendum';

export type ExclusionPeriod = '30d' | '90d' | 'indefinite';

export interface GameLimits {
  daily: number;
  weekly: number;
  monthly: number;
  pending?: {daily: number; weekly: number; monthly: number; applies_at: string};
}

export interface SelfExclusion {
  period: ExclusionPeriod;
  requested_at: string;
  until?: string;
}

/** Mirrors api/internal/api/v1/responsible.go. Money is integer centavos. */
export interface ResponsibleStatus {
  addendum_version: string;
  accepted: boolean;
  limits: GameLimits | null;
  usage: {
    daily: number; weekly: number; monthly: number;
    day_resets_at: string; week_resets_at: string; month_resets_at: string;
  };
  excluded: SelfExclusion | null;
}

const BASE = '/v1.0/players/me/responsible';

export async function getResponsible() {
  return (await apiClient.get<ResponsibleStatus>(`${BASE}/`, {silentError: true})).data;
}

export async function acceptGamblingAddendum() {
  await apiClient.post(`${BASE}/accept`);
}

export async function saveLimits(limits: {daily: number; weekly: number; monthly: number}) {
  return (await apiClient.put<GameLimits>(`${BASE}/limits`, limits)).data;
}

export async function cancelPendingLimits() {
  return (await apiClient.delete<GameLimits>(`${BASE}/limits/pending`)).data;
}

export async function selfExclude(period: ExclusionPeriod) {
  return (await apiClient.post<SelfExclusion>(`${BASE}/self-exclusion`, {period})).data;
}

export async function revokeSelfExclusion() {
  await apiClient.delete(`${BASE}/self-exclusion`);
}
