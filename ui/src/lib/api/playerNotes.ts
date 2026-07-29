import {apiClient} from './client';

export const PLAYER_NOTE_TAGS = ['red', 'orange', 'yellow', 'green', 'blue', 'purple'] as const;
export type PlayerNoteTag = typeof PLAYER_NOTE_TAGS[number];

export interface PlayerNote {
  opponent_id: string;
  tag?: PlayerNoteTag;
  note?: string;
  updated_at: string;
}

export async function getPlayerNotes() {
  return (await apiClient.get<{ data: PlayerNote[] }>('/v1.0/players/me/notes/', {silentError: true})).data.data;
}

export async function savePlayerNote(opponentId: string, input: { tag?: PlayerNoteTag; note?: string }) {
  return (await apiClient.post<PlayerNote | { deleted: true }>(
    `/v1.0/players/me/notes/${encodeURIComponent(opponentId)}`,
    input
  )).data;
}
