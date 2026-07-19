export interface SeatView {
  player_id: string;
  stack: number;
  state: string;
  contributed: number;
  hole_cards?: string[];
  equity?: number
}

export interface TableSnapshot {
  stage: string;
  board: string[];
  seats: SeatView[];
  payouts?: Record<string, number>;
  rake?: number
}

export type ServerMessage = {
  type: string;
  snapshot?: TableSnapshot;
  key?: string;
  stars?: number;
  player_id?: string;
  message?: string
}
