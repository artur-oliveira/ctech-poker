import {apiClient} from './client';

/** One slot of the 30-day streak trail, as the server computes it. */
export interface DailyStreakDay {
  day: number;
  amount: number;
  milestone: boolean;
  claimed: boolean;
  today: boolean;
}

/** `remaining_time_seconds` is the field the cooldown has always carried; the
 * streak fields are additive (#293), so an older client keeps working. */
export interface DailyRewardStatus {
  remaining_time_seconds: number;
  current_streak: number;
  best_streak: number;
  total_claims: number;
  cycle_day: number;
  cycle_length: number;
  protection_available: boolean;
  protection_used_day?: string;
  claimed_today: boolean;
  streak_at_risk: boolean;
  days: DailyStreakDay[];
}

export interface DailyRewardSpinResult extends DailyRewardStatus {
  amount: number;
}

export async function getCooldown() {
  return (await apiClient.get<DailyRewardStatus>('/v1.0/sandbox-credits/')).data;
}

export async function spin() {
  return (await apiClient.post<DailyRewardSpinResult>('/v1.0/sandbox-credits/', undefined, {silentError: true})).data;
}
