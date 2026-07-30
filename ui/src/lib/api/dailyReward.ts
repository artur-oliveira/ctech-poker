import {apiClient} from './client';

export interface DailyRewardCooldown {
  remaining_time_seconds: number;
}

export interface DailyRewardSpinResult {
  amount: number;
  remaining_time_seconds: number;
}

export async function getCooldown() {
  return (await apiClient.get<DailyRewardCooldown>('/v1.0/sandbox-credits/')).data;
}

export async function spin() {
  return (await apiClient.post<DailyRewardSpinResult>('/v1.0/sandbox-credits/', undefined, {silentError: true})).data;
}
