import {apiClient} from './client';

// #304: a player's own configurable wallet-alert thresholds. Separate from
// the `wallet_alerts` firing state on PlayerProfile (player.ts) — this is the
// *configuration*, that is the *result* of evaluating it against the current
// balance.
export interface WalletAlertPrefs {
  min_sandbox_balance: number;
  max_purchase_cents: number;
  updated_at?: string;
}

export const WALLET_ALERT_PREFS_KEY = ['player', 'wallet-alerts'] as const;

// A player who never configured anything gets a zeroed response (both
// thresholds disabled) rather than a 404 — see internal/api/v1/walletalert.go.
export async function getWalletAlertPrefs() {
  return (await apiClient.get<WalletAlertPrefs>('/v1.0/players/me/wallet-alerts', {silentError: true})).data;
}

export async function saveWalletAlertPrefs(input: {min_sandbox_balance: number; max_purchase_cents: number}) {
  return (await apiClient.put<WalletAlertPrefs>('/v1.0/players/me/wallet-alerts', input)).data;
}

export async function deleteWalletAlertPrefs() {
  await apiClient.delete('/v1.0/players/me/wallet-alerts');
}
