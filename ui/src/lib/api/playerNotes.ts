import {apiClient} from './client';

export const PLAYER_NOTE_TAGS = ['red', 'orange', 'yellow', 'green', 'blue', 'purple'] as const;
export type PlayerNoteTag = typeof PLAYER_NOTE_TAGS[number];

export interface PlayerNote {
  opponent_id: string;
  tag?: PlayerNoteTag;
  note?: string;
  updated_at: string;
}

/** Query key for a scoped note read. The opponent set is part of the key —
 * sorted, so seat order never forks the cache — because the answer is only
 * valid for the players that were asked about (#209). */
export const PLAYER_NOTES_KEY = (opponentIds: string[]) =>
  ['player-notes', [...opponentIds].sort().join(',')] as const;

/** Reads only the notes for the players currently on screen: the seats at a
 * table, or the players in one hand. The unscoped route still exists for a
 * future notes-management screen, but no screen loads a player's whole note
 * history to render at most nine of them. */
export async function getPlayerNotes(opponentIds: string[]) {
  if (!opponentIds.length) return [];
  return (await apiClient.get<{ data: PlayerNote[] }>('/v1.0/players/me/notes/', {
    params: {opponent_ids: opponentIds.join(',')}, silentError: true
  })).data.data;
}

export async function savePlayerNote(opponentId: string, input: { tag?: PlayerNoteTag; note?: string }) {
  return (await apiClient.post<PlayerNote | { deleted: true }>(
    `/v1.0/players/me/notes/${encodeURIComponent(opponentId)}`,
    input
  )).data;
}
