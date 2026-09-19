import {apiClient} from './client';

/** Mirrors `botcheck.MaxContestReasonLength` (api/internal/botcheck/contest.go). */
export const BOT_CHECK_CONTEST_MAX_REASON_LENGTH = 500;

export type BotCheckContestStatus = 'pending';

export interface BotCheckContest {
  contest_id: string;
  table_id?: string;
  reason?: string;
  status: BotCheckContestStatus;
  created_at: string;
}

export const BOT_CHECK_CONTESTS_KEY = ['bot-check-contests'] as const;

/** The player's own auditable false-positive claims (#322) — a separate,
 * append-only trail. Filing one never re-checks or accepts a Turnstile token;
 * it only records the claim for later review. */
export async function listBotCheckContests() {
  return (await apiClient.get<{ data: BotCheckContest[] }>(
    '/v1.0/players/me/bot-challenge/contests/', {silentError: true}
  )).data.data;
}

export async function fileBotCheckContest(input: { tableId?: string; reason?: string }) {
  return (await apiClient.post<BotCheckContest>('/v1.0/players/me/bot-challenge/contests/', {
    table_id: input.tableId, reason: input.reason,
  })).data;
}
