import {apiClient} from './client';

/** Mirrors go-common pushsvc (`config`, `subscriptions`) and `pokerpush.Preferences` (api/internal/pokerpush). */

export interface PushConfig {
  /** False: this installation has no VAPID key; the screen says so and subscribing answers 503. */
  enabled: boolean;
  /** VAPID public key, base64url (uncompressed P-256 point). Absent when disabled. */
  public_key?: string;
  /** Short key id; the client resubscribes when it changes. */
  vapid_kid?: string;
  max_subscriptions: number;
}

export interface PushDevice {
  id: string;
  /** Coarse label such as "Chrome no Android". */
  device: string;
  created_at?: string;
  last_seen_at?: string;
}

export interface PushDeviceList {
  data: PushDevice[];
  max_subscriptions: number;
}

export interface PushSubscriptionBody {
  endpoint: string;
  keys: {p256dh: string; auth: string};
}

export interface PushPreferences {
  streak_reminder: {enabled: boolean; time: string};
  come_play: {enabled: boolean};
}

export const PUSH_CONFIG_KEY = ['push', 'config'] as const;
export const PUSH_DEVICES_KEY = ['push', 'devices'] as const;
export const PUSH_PREFERENCES_KEY = ['push', 'preferences'] as const;

export async function getPushConfig() {
  return (await apiClient.get<PushConfig>('/v1.0/push/config', {silentError: true})).data;
}

export async function listPushDevices() {
  return (await apiClient.get<PushDeviceList>('/v1.0/push/subscriptions', {silentError: true})).data;
}

export async function putPushSubscription(body: PushSubscriptionBody) {
  return (await apiClient.put<PushDevice>('/v1.0/push/subscriptions', body, {silentError: true})).data;
}

export async function deletePushDevice(id: string) {
  await apiClient.delete(`/v1.0/push/subscriptions/${encodeURIComponent(id)}`, {silentError: true});
}

export async function getPushPreferences() {
  return (await apiClient.get<PushPreferences>('/v1.0/push/preferences', {silentError: true})).data;
}

export async function putPushPreferences(preferences: PushPreferences) {
  return (await apiClient.put<PushPreferences>('/v1.0/push/preferences', preferences)).data;
}

export async function sendPushTest() {
  await apiClient.post('/v1.0/push/test', {});
}
