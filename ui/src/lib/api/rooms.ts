import type {Page} from './client';
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
  turn_timeout_seconds?: number;
  // Persisted by the table actor as players join/leave (never computed live
  // from tablemanager). This is how the lobby knows a table has a free seat.
  seats_taken: number;
  // Present only for a private room's own creator (the server strips both
  // from every other viewer's response).
  share_code?: string;
  created_by?: string;
  // Fixed real-money table-entry fee (BRL cents), charged on every seat-entry
  // (join/rebuy), zero/absent for sandbox rooms. Set once at room creation,
  // never a function of the pot (see docs/plans/2026-07-25-realmoney-fixed-fee-and-sandbox-rake.md).
  entry_fee_cents?: number;
  run_it_twice_enabled?: boolean;
}

export interface Stake {
  small_blind: number;
  big_blind: number;
  fee_cents?: number;
}

async function fetchRoomsPage(cursor?: string, currencyMode?: 'sandbox' | 'real') {
  return (await apiClient.get<Page<Room>>('/v1.0/rooms', {
    params: {cursor, currency_mode: currencyMode},
  })).data;
}

export async function listRooms(cursor?: string, currencyMode?: 'sandbox' | 'real') {
  return (await fetchRoomsPage(cursor, currencyMode)).data;
}

// One lobby tile's availability, aggregated server-side over the whole public
// directory. The lobby renders these instead of paginating every room itself
// (#205): the count it shows is the server's, and the room a click lands in is
// resolved by joinOrCreateRoom below, never picked from a list here.
export interface RoomBucket {
  small_blind: number;
  big_blind: number;
  max_seats: number;
  currency_mode: string;
  rooms: number;
  open_rooms: number;
  seats_taken: number;
  seats_available: number;
}

export async function listRoomBuckets(currencyMode: 'sandbox' | 'real' = 'sandbox') {
  return (await apiClient.get<{ data: RoomBucket[] }>('/v1.0/rooms/buckets', {
    params: {currency_mode: currencyMode},
  })).data.data;
}

export interface JoinOrCreateInput {
  small_blind: number;
  big_blind: number;
  max_seats: number;
  amount: number;
  currency_mode?: 'sandbox' | 'real';
  auto_rebuy?: boolean;
  // Stable per click (a retry of the same click must re-seat at the same
  // table, not buy a second seat in a sibling one). The caller owns it, so a
  // retry can reuse the key it already sent.
  idem_key: string;
}

// The single server-resolved entry mutation: the server picks (or opens) the
// table inside the bucket and seats the player in one round trip, so a lost
// last-seat race falls through to another table without the client walking
// candidates or re-reading the lobby (#205, backend #76).
export async function joinOrCreateRoom(input: JoinOrCreateInput) {
  return (await apiClient.post<{ room_id: string; created: boolean }>(
    '/v1.0/rooms/join-or-create', input, {silentError: true},
  )).data;
}

export async function listStakes(currencyMode: 'sandbox' | 'real' = 'sandbox') {
  return (await apiClient.get<{ stakes: Stake[] }>('/v1.0/rooms/stakes', {
    params: {currency_mode: currencyMode},
    silentError: true
  })).data.stakes;
}

export async function getRoom(id: string) {
  return (await apiClient.get<Room>(`/v1.0/rooms/${id}`)).data;
}

export async function createRoom(input: Omit<Room, 'room_id' | 'id' | 'currency_mode' | 'status' | 'seats_taken'> & {
  currency_mode?: 'sandbox' | 'real'
}) {
  return (await apiClient.post<Room>('/v1.0/rooms', input, {silentError: true})).data;
}

export async function joinRoom(id: string, amount: number, shareCode?: string, autoRebuy?: boolean) {
  // idem_key must be fresh per buy-in click (a rejoin/rebuy is a distinct
  // debit) but stable across a single click's own network retries. The
  // server derives its wallet idempotency key from this, so leaving it out
  // makes every buy-in for this player+room collide on the same key.
  await apiClient.post(
    `/v1.0/rooms/${id}/join`,
    {amount, share_code: shareCode || undefined, auto_rebuy: autoRebuy || undefined, idem_key: crypto.randomUUID()},
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
