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
  status: string
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

export async function createRoom(input: Omit<Room, 'room_id' | 'id' | 'currency_mode' | 'status'>) {
  return (await apiClient.post<Room>('/v1.0/rooms', input, {silentError: true})).data;
}

export async function joinRoom(id: string, amount: number) {
  await apiClient.post(`/v1.0/rooms/${id}/join`, {amount}, {silentError: true});
}
