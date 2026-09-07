import type {Page} from './client';
import {apiClient} from './client';
import type {DeckVariantId} from '../cardVariants';
import type {PlaystyleBadge} from '../playstyle';
import type {TableThemeId} from '../tablePreferences';

export type WalletMode = 'sandbox' | 'real';

// Showcase layout (#335). Achievements is reorderable but never hideable —
// it already has its own "no achievement selected" empty copy, so hiding it
// entirely would just duplicate that with less explanation. Best hand and
// matchup can be reordered AND hidden.
export type ShowcaseSectionId = 'achievements' | 'best_hand' | 'matchup';
export interface ShowcaseLayout {
  order: ShowcaseSectionId[];
  hidden: ShowcaseSectionId[];
}
export const DEFAULT_SHOWCASE_ORDER: ShowcaseSectionId[] = ['achievements', 'best_hand', 'matchup'];
const DEFAULT_SHOWCASE_LAYOUT: ShowcaseLayout = {order: DEFAULT_SHOWCASE_ORDER, hidden: []};

/** Guards against a stale/partial stored layout (an older app version, or a
 * profile that predates this field) — always returns every known section
 * exactly once, in a valid order, with nothing but 'best_hand'/'matchup' ever
 * hidden. */
export function normalizeShowcaseLayout(layout?: ShowcaseLayout): ShowcaseLayout {
  if (!layout) return DEFAULT_SHOWCASE_LAYOUT;
  const known = new Set(DEFAULT_SHOWCASE_ORDER);
  // order/hidden can arrive null or absent from an older profile row, not just
  // the whole object — coerce each to an array before filtering.
  const order = (Array.isArray(layout.order) ? layout.order : []).filter(id => known.has(id));
  for (const id of DEFAULT_SHOWCASE_ORDER) if (!order.includes(id)) order.push(id);
  const hidden = (Array.isArray(layout.hidden) ? layout.hidden : []).filter(id => id !== 'achievements' && known.has(id));
  return {order, hidden};
}

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
  table_public: boolean;
  playstyle_public: boolean;
  featured_achievements?: string[];
  favorite_reactions?: string[];
  favorite_bet_presets?: string[];
  showcase_layout?: ShowcaseLayout;
  playstyle?: PlaystyleBadge[];
}

// Canonical source of truth for a player's wallet balances. Both the sandbox
// balance (`sandbox_balance`) and the real-money balance (`game_balance`) live
// on the `['player','me']` profile — there is no separate balance query. Every
// balance-moving path (PIX purchase confirm, daily reward, wallet socket
// frames) must invalidate THIS key. Do not reintroduce a `['wallet','balance']`
// key: nothing reads it and a widget wired to it would silently go stale.
export const BALANCE_QUERY_KEY = ['player', 'me'] as const;

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
  table_public?: boolean;
  playstyle_public?: boolean;
  featured_achievements?: string[];
  favorite_reactions?: string[];
  favorite_bet_presets?: string[];
  showcase_layout?: ShowcaseLayout;
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
  // Not sent by the backend yet — ProfileContent falls back to the legacy
  // fixed order (achievements, best hand, matchup) with everything visible
  // when absent, so an older server response degrades to today's behavior
  // rather than to a broken layout.
  showcase_layout?: ShowcaseLayout;
  // #330. Absent on an older server, and absent on a private profile (the
  // whole showcase 404s there) — never a separate opt-in.
  member_since?: string;
  milestones?: ProfileMilestone[];
}

/** A derived longevity/volume/ranking mark on a public showcase (#330).
 * `value` carries the figure the mark was earned with — days for tenure,
 * hands for volume, the rank itself for ranking. */
export interface ProfileMilestone {
  key: string;
  category: 'tenure' | 'volume' | 'ranking';
  value: number;
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
  // Epoch milliseconds, never seconds — see handEndedAtMs below (#74).
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
  // Epoch milliseconds, never seconds, on every hand endpoint that emits
  // this field (list, by-id, and ProfileShowcase.best_hand) — see
  // handEndedAtMs below. Do not add a per-call-site unit heuristic; a
  // `< 1e12 ? *1000 : ended_at` guess is exactly the bug #74 removed.
  ended_at: number;
  // Blind level in force for this hand, not the room's current one. Added by
  // backend issue #75; both absent on hands logged before it, where
  // HandReplayer derives the level from the `post_big_blind` action instead.
  // Zero/absent means unknown — never substitute a default.
  small_blind?: number;
  big_blind?: number;
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

// Single point of truth for turning a hand's `ended_at` into the epoch
// milliseconds `Date` expects. Every hand-bearing endpoint (getHands,
// getHand, and ProfileShowcase.best_hand) already returns ms, so this is
// intentionally a passthrough — its purpose is to be the one call site a
// future unit regression on any single endpoint gets fixed at, instead of a
// `< 1e12 ? *1000 : ended_at` heuristic creeping back into a page (#74).
export function handEndedAtMs(endedAt: number): number {
  return endedAt;
}

export interface HandRevealCheck {
  fee: number;
  already_paid: boolean;
  cards?: [string, string];
}

// GET .../reveal-winner always returns 200 with the fee once an archive
// exists for handId (a sandbox hand that ended without a showdown with
// exactly one winner) — 404 means no archive exists at all (showdown hand,
// real-money hand, split pot, or a hand that predates this feature).
export async function getHandRevealWinner(handId: string) {
  return (await apiClient.get<HandRevealCheck>(
    `/v1.0/players/me/hands/${encodeURIComponent(handId)}/reveal-winner`, {silentError: true}
  )).data;
}

export async function revealHandWinner(handId: string) {
  return (await apiClient.post<{ cards: [string, string] }>(
    `/v1.0/players/me/hands/${encodeURIComponent(handId)}/reveal-winner`, {}, {silentError: true}
  )).data;
}
