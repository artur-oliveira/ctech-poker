import {apiClient} from './client';

export interface PokerStats {
  hands: number;
  vpip_hands: number;
  pfr_hands: number;
  three_bet_hands: number;
  three_bet_chances: number;
  vpip_rate: number;
  pfr_rate: number;
  three_bet_rate: number;
}

export async function getMyPokerStats() {
  return (await apiClient.get<PokerStats>('/v1.0/players/me/poker-stats', {
    silentError: true
  })).data;
}
