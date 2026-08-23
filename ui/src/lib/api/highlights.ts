import {apiClient} from './client';

export interface RevealedHand {
  player_id: string;
  name?: string;
  hole_cards: string[];
}

export interface TableHighlight {
  table_id: string;
  date: string;
  hand_id: string;
  pot: number;
  board?: string[];
  revealed?: RevealedHand[];
  recorded_at: number;
}

// 404 both for "no highlight recorded yet today" and "you were never at this
// table" — the caller doesn't need to tell them apart, both render nothing.
export async function getTodayHighlight(tableId: string) {
  return (await apiClient.get<TableHighlight>(
    `/v1.0/rooms/${encodeURIComponent(tableId)}/highlights/today`, {silentError: true}
  )).data;
}
