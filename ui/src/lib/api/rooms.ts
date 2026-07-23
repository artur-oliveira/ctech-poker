import {apiClient} from './client';

export interface Room {
  room_id?: string;
  id?: string;
  visibility: 'public' | 'private';
  currency_mode: string;
  small_blind: number;
  big_blind: number;
  max_seats: number;
  buy_in_min: number;
  buy_in_max: number;
  status: string;
  // Persisted by the table actor as players join/leave (never computed live
  // from tablemanager) — how the lobby knows a table has a free seat.
  seats_taken: number;
  // Present only for a private room's own creator (the server strips both
  // from every other viewer's response).
  share_code?: string;
  created_by?: string
}

export interface Stake {
  small_blind: number;
  big_blind: number
}

export async function listRooms() {
  return (await apiClient.get<Room[]>('/v1.0/rooms')).data;
}

export async function listStakes() {
  return (await apiClient.get<{ stakes: Stake[] }>('/v1.0/rooms/stakes')).data.stakes;
}

export async function getRoom(id: string) {
  return (await apiClient.get<Room>(`/v1.0/rooms/${id}`)).data;
}

export async function createRoom(input: Omit<Room, 'room_id' | 'id' | 'currency_mode' | 'status' | 'seats_taken'>) {
  return (await apiClient.post<Room>('/v1.0/rooms', input, {silentError: true})).data;
}

export async function joinRoom(id: string, amount: number, shareCode?: string) {
  // idem_key must be fresh per buy-in click (a rejoin/rebuy is a distinct
  // debit) but stable across a single click's own network retries — the
  // server derives its wallet idempotency key from this, so leaving it out
  // makes every buy-in for this player+room collide on the same key.
  await apiClient.post(
    `/v1.0/rooms/${id}/join`,
    {amount, share_code: shareCode || undefined, idem_key: crypto.randomUUID()},
    {silentError: true},
  );
}

export interface SeatedStatus {
  seated: boolean;
  stack: number;
}

export async function getSeated(id: string) {
  return (await apiClient.get<SeatedStatus>(`/v1.0/rooms/${id}/seated`)).data;
}

export async function leaveRoom(id: string) {
  // idem_key fresh per cash-out click, same reasoning as joinRoom above.
  return (await apiClient.post<{ amount: number }>(
    `/v1.0/rooms/${id}/leave`,
    {idem_key: crypto.randomUUID()},
    {silentError: true},
  )).data;
}
