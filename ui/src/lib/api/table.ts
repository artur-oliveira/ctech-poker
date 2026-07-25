import {apiClient} from './client';

export interface SeatView {
  player_id: string;
  name?: string;
  stack: number;
  state: string;
  contributed: number;
  hole_cards?: string[];
  equity?: number;
  hand_category?: string
}

export type PokerAction = 'fold' | 'check' | 'call' | 'raise'

export interface LegalActionState {
  actions: PokerAction[];
  call_amount?: number;
  min_raise_to?: number;
  max_raise_to?: number;
  step?: number
}

export interface TableSnapshot {
  stage: string;
  board: string[];
  seats: SeatView[];
  current_player_id?: string;
  legal_actions?: LegalActionState;
  payouts?: Record<string, number>;
  // Who actually won a contested pot, as opposed to merely appearing in
  // `payouts` — an uncalled all-in's excess or an orphaned side-pot refund
  // also lands in `payouts` without being a win. Use this for win UI, not
  // `payouts[id] > 0`.
  winners?: string[];
  rake?: number;
  action_deadline_unix_ms?: number;
  next_hand_unix_ms?: number;
  won_without_showdown?: boolean;
  // Absent before the first hand's dealer is drawn (mirrors the API's
  // omitempty). Heads-up play has the dealer double as the small blind.
  dealer_player_id?: string;
  small_blind_player_id?: string;
  big_blind_player_id?: string
}

export type ServerMessage = {
  type: string;
  snapshot?: TableSnapshot;
  key?: string;
  stars?: number;
  player_id?: string;
  message?: string;
  code?: string;
  action_id?: string
}

export interface HandHistoryAction {
  seq: number;
  player_id: string;
  action: string;
  amount: number;
  timestamp: number; // unix millis
}

export interface HandHistory {
  table_id: string;
  hand_id: string;
  actions: HandHistoryAction[];
}

export async function getHandHistory(tableId: string, handId: string) {
  return (await apiClient.get<HandHistory>(`/v1.0/tables/${tableId}/hands/${handId}/history`, {silentError: true})).data;
}
