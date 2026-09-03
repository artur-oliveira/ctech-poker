import {apiClient} from './client';

export interface SeatView {
  player_id: string;
  name?: string;
  avatar_url?: string;
  playstyle_badge?: string;
  run_it_twice?: boolean;
  auto_rebuy?: boolean;
  connection_state?: 'connected' | 'disconnected';
  stack: number;
  state: string;
  // True only when this seat belongs to the hand identified by `hand_id`.
  // `state: active` alone can mean the player is waiting for the next deal.
  dealt_in?: boolean;
  // Optional during the API-first rollout; absent snapshots use state-based
  // compatibility behavior without falsely marking every player paused.
  ready?: boolean;
  // The player asked to leave (request_exit) and will be removed once no
  // longer dealt into the current hand; cancelable via cancel_exit.
  pending_exit?: boolean;
  contributed: number;
  hole_cards?: string[];
  hole_cards_revealed?: boolean[];
  stack_at_hand_start?: number;
  equity?: number;
  hand_category?: string
  hand_score?: number
  time_bank_ms?: number
  // Running per-table win/loss streak: positive is consecutive wins,
  // negative is consecutive losses, absent/zero means no badge.
  current_streak?: number
}

export type PokerAction = 'fold' | 'check' | 'call' | 'raise'
export type ActionPreselection = 'check_fold' | 'fold' | 'call' | 'call_any' | 'all_in'

export type ChatMessage = { id: string; player_id: string; message: string; timestamp: number }
export type PersistedReaction = {
  id: string;
  player_id: string;
  reaction_id: string;
  target_player_id?: string;
  timestamp: number;
  expires_at: number;
}

export interface LegalActionState {
  actions: PokerAction[];
  call_amount?: number;
  min_raise_to?: number;
  max_raise_to?: number;
  step?: number;
  current_contribution?: number;
  current_bet?: number;
  one_third_pot_raise_to?: number;
  half_pot_raise_to?: number;
  two_thirds_pot_raise_to?: number;
  pot_raise_to?: number
}

export interface PotView {
  amount: number;
  eligible_player_ids: string[]
}

export interface PotResultView {
  amount: number;
  payout_amount: number;
  eligible_player_ids: string[];
  winner_player_ids: string[];
  payouts?: Record<string, number>;
  refund?: boolean
  runout?: number
}

export interface WinnerCardsRequest {
  requester_id: string;
  requester_name?: string;
  winner_id: string;
  winner_name?: string;
  fee: number;
  expires_at_unix_ms: number;
}

export interface TableSnapshot {
  stage: string;
  board: string[];
  board_two?: string[];
  board_split_at?: number;
  seats: SeatView[];
  current_player_id?: string;
  legal_actions?: LegalActionState;
  payouts?: Record<string, number>;
  // Who actually won a contested pot, as opposed to merely appearing in
  // `payouts`: an uncalled all-in's excess or an orphaned side-pot refund
  // also lands in `payouts` without being a win. Use this for win UI, not
  // `payouts[id] > 0`.
  winners?: string[];
  rake?: number;
  action_deadline_unix_ms?: number;
  action_base_deadline_unix_ms?: number;
  chat_messages?: ChatMessage[];
  reactions?: PersistedReaction[];
  action_preselection?: ActionPreselection;
  action_preselection_amount?: number;
  prospective_call_amount?: number;
  next_hand_unix_ms?: number;
  idle_removal_unix_ms?: number;
  won_without_showdown?: boolean;
  // Absent before the first hand's dealer is drawn (mirrors the API's
  // omitempty). Heads-up play has the dealer double as the small blind.
  dealer_player_id?: string;
  small_blind_player_id?: string;
  big_blind_player_id?: string
  snapshot_version?: number;
  pots?: PotView[];
  pot_results?: PotResultView[];
  pending_winner_cards?: WinnerCardsRequest;
  protocol_version?: number;
  hand_id?: string;
  shuffle_commit_hash?: string;
  shuffle_server_seed_hex?: string;
  runout_cards?: string[];
  root_commit_hash?: string;
  revealed_card_salts?: Record<number, { card: string; salt_hex: string }>;
  unrevealed_card_hashes?: Record<number, string>;
}

export type ServerMessage = {
  type: string;
  snapshot?: TableSnapshot;
  key?: string;
  stars?: number;
  player_id?: string;
  message?: string;
  code?: string;
  // Settled stack on a "removed" frame (also reused for payment_received /
  // sandbox_purchase_update credits — see proto/poker.proto's amount field).
  amount?: number;
  action_id?: string;
  snapshot_version?: number;
  equity?: number
  reaction_id?: string;
  target_player_id?: string
}

export type Action = (
  'post_big_blind' | 'escalate_blinds' | 'not_ready' | 'ready' | 'sit_out' | 'show_cards' | 'disconnect_sit_out' | 'join' |
  'leave' | 'keep_seat' | 'next_hand' | 'runout_step' | 'check' | 'fold' | 'call' | 'bet' | 'raise' | 'all_in' | 'won' | 'tie'
  | 'lost' | 'peek_cards' | 'request_exit' | 'set_run_it_twice' | 'chat' | 'reaction' | 'set_identity'
  )

export interface HandHistoryAction {
  seq: number;
  player_id: string;
  action: Action;
  amount: number;
  timestamp: number; // unix millis
  // Only on `reaction` rows: which emoji was thrown, and at whom when the
  // reaction is a targeted one (see TABLE_REACTIONS in lib/reactions.ts).
  reaction_id?: string;
  target_player_id?: string;
  frame?: ReplayFrame
}

export interface ReplaySeat {
  player_id: string;
  name?: string;
  stack: number;
  state: string;
  contributed: number;
  dealt_in: boolean
}

export interface ReplayFrame {
  stage: string;
  board?: string[];
  board_two?: string[];
  board_split_at?: number;
  seats?: ReplaySeat[];
  current_player_id?: string;
  dealer_player_id?: string;
  small_blind_player_id?: string;
  big_blind_player_id?: string;
  pot: number;
  payouts?: Record<string, number>;
  winners?: string[]
}

export interface HandHistory {
  table_id: string;
  hand_id: string;
  actions: HandHistoryAction[];
}

export async function getHandHistory(tableId: string, handId: string) {
  return (await apiClient.get<HandHistory>(`/v1.0/tables/${tableId}/hands/${handId}/history`, {silentError: true})).data;
}
