import {apiClient} from './client';

/** Mirrors `chatprefs.MaxExtraWords`/`MaxWordLength` (api/internal/chatprefs/store.go). */
export const CHAT_PREFS_MAX_WORDS = 50;
export const CHAT_PREFS_MAX_WORD_LENGTH = 32;

export interface ChatPreferences {
  extra_words: string[];
}

export const CHAT_PREFS_KEY = ['chat-prefs'] as const;

/** The player's personal chat-filter addition (#327) — words masked only in
 * chat that player themselves reads, on top of the table-wide floor. */
export async function getChatPrefs() {
  return (await apiClient.get<ChatPreferences>('/v1.0/players/me/chat-prefs/', {silentError: true})).data;
}

export async function saveChatPrefs(extraWords: string[]) {
  return (await apiClient.put<ChatPreferences>('/v1.0/players/me/chat-prefs/', {extra_words: extraWords})).data;
}
