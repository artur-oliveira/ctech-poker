import type {Page} from './client';
import {apiClient} from './client';
import type {DeckVariantId} from '../cardVariants';
import type {PlaystyleBadge} from '../playstyle';
import type {TableThemeId} from '../tablePreferences';

export type WalletMode = 'sandbox' | 'real';

export interface PlayerProfile {
  user_id: string;
  name?: string;
  avatar_url?: string;
  wallet_mode: WalletMode;
  // Stable, shareable identifier for friend requests (PKR-XXXX-XXXX-XXXX).
  // Older profiles get one lazily on their next load, so it can be absent.
  friend_code?: string;
  poker_terms_accepted: boolean;
  poker_terms_accepted_at?: string;
  game_balance?: number;
  sandbox_balance?: number;
  // Not sent by the backend yet, reserved so the deck color variant can be
  // wired in without another PlayerProfile shape change.
  deck_variant?: DeckVariantId;
  table_theme?: TableThemeId;
  showcase_public: boolean;
  playstyle_public: boolean;
  featured_achievements?: string[];
  favorite_reactions?: string[];
  playstyle?: PlaystyleBadge[];
}

export async function getMe() {
  return (await apiClient.get<PlayerProfile>('/v1.0/players/me', {silentError: true})).data;
}

export async function acceptPokerTerms() {
  return (await apiClient.post<PlayerProfile>('/v1.0/players/me/terms/accept', {}, {silentError: true})).data;
}

export async function updateMe(input: {
  name?: string;
  wallet_mode?: WalletMode;
  deck_variant?: DeckVariantId;
  table_theme?: TableThemeId;
  showcase_public?: boolean;
  playstyle_public?: boolean;
  featured_achievements?: string[];
  favorite_reactions?: string[];
}) {
  return (await apiClient.post<PlayerProfile>('/v1.0/players/me', input, {silentError: false})).data;
}

export interface ProfileShowcase {
  player_id: string;
  name?: string;
  avatar_url?: string;
  featured_achievements: Array<{ key: string; count: number }>;
  playstyle?: PlaystyleBadge[];
  best_hand?: Pick<HandItem, 'hand_id' | 'table_id' | 'net_change' | 'ended_at' | 'board' | 'hole_cards'>;
}

export async function getProfileShowcase(playerId: string) {
  return (await apiClient.get<ProfileShowcase>(
    `/v1.0/players/${encodeURIComponent(playerId)}/showcase`,
    {silentError: true}
  )).data;
}

export interface MatchupStats {
  hands_together: number;
  viewer_wins: number;
  opponent_wins: number;
  ties: number;
  heads_up_hands_together: number;
  net_change_viewer: number;
}

export async function getMatchupStats(opponentId: string) {
  return (await apiClient.get<MatchupStats>(
    `/v1.0/players/me/matchups/${encodeURIComponent(opponentId)}`,
    {silentError: true}
  )).data;
}

export interface PlayerSession {
  table_id: string;
  buyin_amount: number;
  cashout_amount: number;
  net_pnl: number;
  joined_at: number;
  ended_at: number;
}

// Most-recent-first (server sorts descending): sessions[0].ended_at === 0
// means that table is still the player's open seat. cursor pages backward
// through history; omit it for the first (most recent) page.
export async function getSessions(cursor?: string) {
  return (await apiClient.get<Page<PlayerSession>>('/v1.0/players/me/sessions', {
    params: {cursor}, silentError: true
  })).data.data;
}

export type HandOutcome = 'won' | 'lost' | 'tied';

export interface OpponentSummary {
  player_id: string;
  name?: string;
  avatar_url?: string;
  hole_cards?: string[];
  won?: boolean;
}

export interface HandItem {
  pk: string;
  sk: string;
  table_id: string;
  hand_id: string;
  outcome: HandOutcome;
  net_change: number;
  ended_at: number;
  board?: string[];
  board_two?: string[];
  hole_cards?: string[];
  opponents?: OpponentSummary[];
  server_seed?: string;
  commit_hash?: string;
  // Per-position deck proof, present when the hand ended without a full
  // showdown so the seed had to stay secret: revealed positions carry their
  // card + salt, the rest only their committed hash, and together they still
  // recompute root_commit_hash.
  root_commit_hash?: string;
  revealed_card_salts?: Record<number, { card: string; salt_hex: string }>;
  unrevealed_card_hashes?: Record<number, string>;
}

// Most-recent-first (server sorts descending), capped at 50 per page.
// `tableId` scopes it to one table's hands (e.g. the live table's own
// "last winners" strip) instead of the viewer's whole history.
// Returns the whole envelope, not just the items: the history page pages
// through it (has_next / next_cursor) instead of showing one page as if it
// were the complete history.
export async function getHands({cursor, tableId, mode = 'sandbox'}: {
  cursor?: string; tableId?: string; mode?: WalletMode
} = {}) {
  return (await apiClient.get<Page<HandItem>>('/v1.0/players/me/hands', {
    params: {cursor, table_id: tableId, mode}, silentError: true
  })).data;
}

export async function getHand(handId: string, mode: WalletMode = 'sandbox') {
  return (await apiClient.get<HandItem>(`/v1.0/players/me/hand/${encodeURIComponent(handId)}`, {
    params: {mode}, silentError: true
  })).data;
}
