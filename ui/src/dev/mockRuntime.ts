// Development-only in-process API and realtime simulation. Production code
// may only reach this module through a USE_MOCK-guarded dynamic import.
import {AxiosError, type AxiosResponse, type InternalAxiosRequestConfig} from 'axios';
import type {Achievement, PlayerAchievementProgress, Tier} from '@/lib/api/achievements';
import type {HandItem} from '@/lib/api/player';
import type {DeckVariantId} from '@/lib/cardVariants';
import type {Room} from '@/lib/api/rooms';
import type {Page} from '@/lib/api/client';
import type {SandboxPurchase, SandboxSKU} from '@/lib/api/wallet';
import type {ReactionCatalogEntry, ReactionPurchase} from '@/lib/api/reactionPurchases';
import {TABLE_REACTIONS} from '@/lib/reactions';
import type {PlayerNote} from '@/lib/api/playerNotes';
import type {
  ActionPreselection,
  HandHistoryAction,
  LegalActionState,
  PokerAction,
  SeatView,
  ServerMessage,
  TableSnapshot
} from '@/lib/api/table';
import {DEFAULT_TURN_TIMEOUT_MS, NEXT_HAND_DELAY_MS} from '@/lib/gameTiming';
import {MOCK_PLAYER_ID, type MockScenario} from '@/lib/mockConfig';

export {MOCK_PLAYER_ID, type MockScenario} from '@/lib/mockConfig';
let mockPlayerNotes: PlayerNote[] = [{
  opponent_id: 'bia_sp',
  tag: 'purple',
  note: 'Defende bastante o big blind.',
  updated_at: new Date().toISOString()
}];

const ROOM_ID = '01ARZ3NDEKTSV4RRFFQ69G5FAV';
const rooms: Room[] = [
  {
    room_id: ROOM_ID,
    visibility: 'public',
    currency_mode: 'sandbox',
    small_blind: 25,
    big_blind: 50,
    max_seats: 9,
    buy_in_min: 1000,
    buy_in_max: 10000,
    status: 'playing',
    seats_taken: 6,
    run_it_twice_enabled: true
  },
  {
    room_id: '01BX5ZZKBKACTAV9WEVGEMMVRZ',
    visibility: 'public',
    currency_mode: 'sandbox',
    small_blind: 50,
    big_blind: 100,
    max_seats: 6,
    buy_in_min: 2000,
    buy_in_max: 20000,
    status: 'waiting',
    seats_taken: 2
  },
];

// Real seed/commit-hash pairs, precomputed offline by replaying deck.go's
// shuffleWithSeed/commitHash — so the /hand/history deck-verify grid actually
// matches in mock mode instead of always failing.
const mockHands: HandItem[] = [
  {
    pk: ROOM_ID,
    table_id: ROOM_ID,
    hand_id: 'hand_0003',
    sk: 'hand_0003',
    outcome: 'won',
    net_change: 4200,
    ended_at: Date.now() - 60_000 * 12,
    board: ['9c', '5c', '6h', '8h', '3h'],
    hole_cards: ['Kd', 'Kc'],
    opponents: [{player_id: 'bia_sp', name: 'Bia', hole_cards: ['7d', 'Jd']}],
    server_seed: '028e42b0ae6c416ea890ea7737dc16c4c354e4ac8ae5b5607f4470b79628620a',
    commit_hash: '0fe99d12a113b9a6c05bcd8323cb1d35cc1ab8714daec4c0e300f78247c60bbe'
  },
  {
    pk: ROOM_ID,
    table_id: ROOM_ID,
    hand_id: 'hand_0002',
    sk: 'hand_0002',
    outcome: 'lost',
    net_change: -1800,
    ended_at: Date.now() - 60_000 * 40,
    board: ['2c', '3d', '2d', 'Jd', 'Ad'],
    hole_cards: ['Qd', '3c'],
    opponents: [{player_id: 'leo_rio', name: 'Leo', hole_cards: ['Ac', '2s'], won: true}],
    server_seed: 'f0fcb9b6dcc37fda6e9ef08f1215eb56e6ac4b1a32a2f6ee535d33ba98e10ac0',
    commit_hash: '6ad2da62948a2414364a1faffde7caded33faf4d426db17f4819dedbfa803347'
  },
  {
    pk: '01BX5ZZKBKACTAV9WEVGEMMVRZ',
    table_id: '01BX5ZZKBKACTAV9WEVGEMMVRZ',
    hand_id: 'hand_0001',
    sk: 'hand_0001',
    outcome: 'tied',
    net_change: 0,
    ended_at: Date.now() - 60_000 * 90,
    board: ['9c', '2d', '9h', '8h', '9s'],
    hole_cards: ['Kc', '7d'],
    opponents: [{player_id: 'leo_rio', name: 'Leo', hole_cards: ['6s', '8c'], won: true}],
    server_seed: 'dc3e9e4d25dd4c612648a0eada3539b4912da6fbd0a79341df221e8f51657b96',
    commit_hash: '063062170463cd2c086f4603c9635f6fb67c7baa1ba0b8597d01966650c2c685'
  }
];

// Only the three hands above have replay data; these clones exist so the
// history page's cursor paging and infinite scroll are exercisable in mock
// mode. Each one keeps a real hand's `sk`, so opening it still resolves.
const MOCK_HANDS_PAGE = 8;
const mockHistory: HandItem[] = [
  ...mockHands,
  ...Array.from({length: 21}, (_, i) => {
    const base = mockHands[i % mockHands.length];
    return {
      ...base,
      hand_id: `${base.hand_id}_r${i}`,
      ended_at: base.ended_at - (i + 1) * 3_600_000,
      net_change: base.net_change === 0 ? 0 : base.net_change + (i % 5) * 130 * Math.sign(base.net_change)
    };
  })
];

// Each action lands a few seconds after the previous one, ending at the
// hand's own ended_at — mirrors how CommitAction stamps Timestamp in prod.
function timedActions(endedAtMs: number, actions: Omit<HandHistoryAction, 'timestamp'>[]): HandHistoryAction[] {
  const startMs = endedAtMs - actions.length * 4000;
  return actions.map((a, i) => ({...a, timestamp: startMs + i * 4000}));
}

function replayActions(
  hand: HandItem,
  streetEnds: [number, number, number],
  actions: Omit<HandHistoryAction, 'timestamp' | 'frame'>[]
): HandHistoryAction[] {
  const timed = timedActions(hand.ended_at, actions);
  const opponent = hand.opponents?.[0];
  const stacks: Record<string, number> = {[MOCK_PLAYER_ID]: 5000};
  if (opponent) stacks[opponent.player_id] = 5000;
  let pot = 0;
  return timed.map((action, index) => {
    if (action.amount > 0) {
      const paid = Math.min(stacks[action.player_id] || 0, action.amount);
      stacks[action.player_id] = (stacks[action.player_id] || 0) - paid;
      pot += paid;
    }
    const boardCount = index < streetEnds[0] ? 0 : index < streetEnds[1] ? 3 : index < streetEnds[2] ? 4 : 5;
    const final = index === timed.length - 1;
    return {
      ...action,
      frame: {
        stage: final ? 'complete' : boardCount === 0 ? 'pre_flop' : boardCount === 3 ? 'flop' : boardCount === 4 ? 'turn' : 'river',
        board: hand.board?.slice(0, boardCount),
        pot,
        current_player_id: timed[index + 1]?.player_id,
        winners: final ? hand.outcome === 'lost' ? hand.opponents?.filter(item => item.won).map(item => item.player_id) :
          [MOCK_PLAYER_ID] : undefined,
        seats: [
          {
            player_id: MOCK_PLAYER_ID, name: 'Ana', stack: stacks[MOCK_PLAYER_ID],
            state: action.action === 'fold' && action.player_id === MOCK_PLAYER_ID ? 'folded' : 'active',
            contributed: action.player_id === MOCK_PLAYER_ID ? action.amount : 0, dealt_in: true
          },
          ...(opponent ? [{
            player_id: opponent.player_id, name: opponent.name, stack: stacks[opponent.player_id],
            state: action.action === 'fold' && action.player_id === opponent.player_id ? 'folded' : 'active',
            contributed: action.player_id === opponent.player_id ? action.amount : 0, dealt_in: true
          }] : [])
        ]
      }
    };
  });
}

// Reactions carry no replay frame in prod either, so they are spliced in after
// the frames are built — the replayer buckets them onto the action they follow.
function withReactions(
  built: HandHistoryAction[],
  reactions: { after: number; player_id: string; reaction_id: string; target_player_id?: string }[]
): HandHistoryAction[] {
  const out = [...built];
  for (const [i, reaction] of reactions.entries()) {
    const anchor = out.find(action => action.seq === reaction.after);
    if (!anchor) continue;
    out.splice(out.indexOf(anchor) + 1, 0, {
      seq: 1000 + i, player_id: reaction.player_id, action: 'reaction', amount: 0,
      reaction_id: reaction.reaction_id, target_player_id: reaction.target_player_id,
      timestamp: anchor.timestamp + 1200
    });
  }
  return out;
}

const mockHandActions: Record<string, HandHistoryAction[]> = {
  hand_0003: withReactions(replayActions(mockHands[0], [2, 5, 7], [
    {seq: 1, player_id: 'bia_sp', action: 'raise', amount: 150},
    {seq: 2, player_id: MOCK_PLAYER_ID, action: 'call', amount: 150},
    {seq: 3, player_id: 'bia_sp', action: 'check', amount: 0},
    {seq: 4, player_id: MOCK_PLAYER_ID, action: 'raise', amount: 400},
    {seq: 5, player_id: 'bia_sp', action: 'call', amount: 400},
    {seq: 6, player_id: 'bia_sp', action: 'check', amount: 0},
    {seq: 7, player_id: MOCK_PLAYER_ID, action: 'raise', amount: 1200},
    {seq: 8, player_id: 'bia_sp', action: 'fold', amount: 0}
  ]), [
    {after: 4, player_id: 'bia_sp', reaction_id: 'wow'},
    {after: 7, player_id: MOCK_PLAYER_ID, reaction_id: 'chip', target_player_id: 'bia_sp'},
    {after: 8, player_id: 'bia_sp', reaction_id: 'cry'}
  ]),
  hand_0002: replayActions(mockHands[1], [2, 5, 7], [
    {seq: 1, player_id: MOCK_PLAYER_ID, action: 'raise', amount: 100},
    {seq: 2, player_id: 'leo_rio', action: 'call', amount: 100},
    {seq: 3, player_id: MOCK_PLAYER_ID, action: 'check', amount: 0},
    {seq: 4, player_id: 'leo_rio', action: 'raise', amount: 300},
    {seq: 5, player_id: MOCK_PLAYER_ID, action: 'call', amount: 300},
    {seq: 6, player_id: MOCK_PLAYER_ID, action: 'check', amount: 0},
    {seq: 7, player_id: 'leo_rio', action: 'raise', amount: 900},
    {seq: 8, player_id: MOCK_PLAYER_ID, action: 'call', amount: 900}
  ]),
  hand_0001: replayActions(mockHands[2], [2, 4, 5], [
    {seq: 1, player_id: 'leo_rio', action: 'check', amount: 0},
    {seq: 2, player_id: MOCK_PLAYER_ID, action: 'check', amount: 0},
    {seq: 3, player_id: 'leo_rio', action: 'raise', amount: 200},
    {seq: 4, player_id: MOCK_PLAYER_ID, action: 'call', amount: 200},
    {seq: 5, player_id: 'leo_rio', action: 'check', amount: 0},
    {seq: 6, player_id: MOCK_PLAYER_ID, action: 'check', amount: 0}
  ])
};

function sharedMockActions(actions: HandHistoryAction[]) {
  const aliases = new Map<string, string>([[MOCK_PLAYER_ID, 'hero']]);
  const alias = (id?: string) => {
    if (!id) return undefined;
    if (!aliases.has(id)) aliases.set(id, `player_${aliases.size}`);
    return aliases.get(id)!;
  };
  return actions.map(action => ({
    ...action,
    player_id: alias(action.player_id) || '',
    frame: action.frame ? {
      ...action.frame,
      current_player_id: alias(action.frame.current_player_id),
      dealer_player_id: alias(action.frame.dealer_player_id),
      small_blind_player_id: alias(action.frame.small_blind_player_id),
      big_blind_player_id: alias(action.frame.big_blind_player_id),
      winners: action.frame.winners?.map(id => alias(id)!),
      payouts: action.frame.payouts && Object.fromEntries(
        Object.entries(action.frame.payouts).map(([id, amount]) => [alias(id)!, amount])
      ),
      seats: action.frame.seats?.map(seat => {
        const playerId = alias(seat.player_id)!;
        return {...seat, player_id: playerId, name: playerId === 'hero' ? 'Você' : 'Jogador'};
      })
    } : undefined
  }));
}

const mockProfile = {
  user_id: MOCK_PLAYER_ID,
  name: 'Ana',
  friend_code: 'PKR-ANA1-2345-6789',
  wallet_mode: 'sandbox' as 'sandbox' | 'real',
  deck_variant: 'four-color' as DeckVariantId,
  poker_terms_accepted: true,
  showcase_public: true,
  playstyle_public: true,
  table_public: false,
  featured_achievements: ['wins', 'hands_played', 'bad_beat'] as string[],
  favorite_reactions: ['clap', 'cold', 'tomato'] as string[],
  game_balance: 12500,
  sandbox_balance: 1_250_000
};

const mockSandboxSkus: SandboxSKU[] = [
  {id: 'pack_1000', price_cents: 490, base_credits: 1000, bonus_percent: 0, total_credits: 1000},
  {id: 'pack_5000', price_cents: 1990, base_credits: 5000, bonus_percent: 10, total_credits: 5500},
  {id: 'pack_12000', price_cents: 3990, base_credits: 12000, bonus_percent: 20, total_credits: 14400},
  {id: 'pack_30000', price_cents: 7990, base_credits: 30000, bonus_percent: 25, total_credits: 37500},
];

type MockSandboxPurchase = SandboxPurchase & {
  resolves_at_ms: number;
  balance_applied: boolean;
  outcome: 'confirmed' | 'expired' | 'failed';
};

const mockSandboxPurchases: MockSandboxPurchase[] = [];

const mockReactionPrices: Record<string, { price_cents: number; price_fichas: number }> = {
  cold: {price_cents: 100, price_fichas: 100_000},
  fire: {price_cents: 100, price_fichas: 100_000},
  poop: {price_cents: 500, price_fichas: 500_000},
  rofl: {price_cents: 500, price_fichas: 500_000},
  knife: {price_cents: 500, price_fichas: 500_000},
  turtle: {price_cents: 500, price_fichas: 500_000},
};
const mockReactionCatalog: ReactionCatalogEntry[] = Object.keys(TABLE_REACTIONS).map(id => ({
  id, premium: id in mockReactionPrices, owned: !(id in mockReactionPrices), ...mockReactionPrices[id]
}));
type MockReactionPurchase = ReactionPurchase & {
  resolves_at_ms?: number;
  outcome?: 'confirmed' | 'expired' | 'failed'
};
const mockReactionPurchases: MockReactionPurchase[] = [];

function settleReactionPurchase(purchase: MockReactionPurchase) {
  if (purchase.status !== 'pending' || !purchase.resolves_at_ms || Date.now() < purchase.resolves_at_ms) return;
  purchase.status = purchase.outcome || 'confirmed';
  purchase.updated_at = new Date().toISOString();
}

function publicPurchase(purchase: MockSandboxPurchase): SandboxPurchase {
  const result = {...purchase} as Partial<MockSandboxPurchase>;
  delete result.resolves_at_ms;
  delete result.balance_applied;
  delete result.outcome;
  return result as SandboxPurchase;
}

function settlePurchase(purchase: MockSandboxPurchase) {
  if (purchase.status !== 'pending' || Date.now() < purchase.resolves_at_ms) return;
  purchase.status = purchase.outcome;
  purchase.updated_at = new Date().toISOString();
  if (purchase.status === 'confirmed' && !purchase.balance_applied) {
    mockProfile.sandbox_balance += purchase.total_credits || 0;
    purchase.balance_applied = true;
  }
}

// The mock Pix payload cannot be paid, but a QR-shaped SVG exercises the same
// image/layout path as production without adding a QR dependency to the SPA.
function mockQRCodeBase64(seed: string) {
  let hash = 2166136261;
  for (const char of seed) hash = Math.imul(hash ^ char.charCodeAt(0), 16777619);
  const finderCell = (x: number, y: number) => {
    const inTop = y < 7 && (x < 7 || x >= 22);
    const inBottom = y >= 22 && x < 7;
    if (!inTop && !inBottom) return null;
    const localX = x >= 22 ? x - 22 : x;
    const localY = y >= 22 ? y - 22 : y;
    return localX === 0 || localX === 6 || localY === 0 || localY === 6 ||
      (localX >= 2 && localX <= 4 && localY >= 2 && localY <= 4);
  };
  const cells: string[] = [];
  for (let y = 0; y < 29; y++) {
    for (let x = 0; x < 29; x++) {
      const finder = finderCell(x, y);
      hash = Math.imul(hash ^ (x + y * 29), 16777619);
      if (finder === true || (finder === null && (hash >>> 29) % 2 === 1)) {
        cells.push(`<rect x="${x + 2}" y="${y + 2}" width="1" height="1"/>`);
      }
    }
  }
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 33 33" shape-rendering="crispEdges"><rect width="33" height="33" fill="#fff"/><g fill="#120d0e">${cells.join('')}</g></svg>`;
  return btoa(svg);
}

// Mirrors api/internal/achievements/catalog.go — same keys, metrics and tier
// thresholds, so the mock exercises the exact shape the real catalog sends.
const commonTiers: Tier[] = [{stars: 1, threshold: 1}, {stars: 2, threshold: 10}, {
  stars: 3,
  threshold: 100
}, {stars: 4, threshold: 1000}, {stars: 5, threshold: 10000}];
const rareTiers: Tier[] = [{stars: 1, threshold: 1}, {stars: 2, threshold: 5}, {
  stars: 3,
  threshold: 25
}, {stars: 4, threshold: 100}, {stars: 5, threshold: 500}];
const categoryOrder = ['high_card', 'pair', 'two_pair', 'three_of_a_kind', 'straight', 'flush', 'full_house', 'four_of_a_kind', 'straight_flush', 'royal_flush'];
const achievementCatalog: Achievement[] = [
  {key: 'wins', metric: 'hand_won', tiers: commonTiers},
  {
    key: 'hands_played',
    metric: 'hand_played',
    tiers: [{stars: 1, threshold: 100}, {stars: 2, threshold: 1000}, {stars: 3, threshold: 10000}, {
      stars: 4,
      threshold: 50000
    }, {stars: 5, threshold: 100000}]
  },
  {key: 'comeback', metric: 'won_after_all_in', tiers: rareTiers},
  {key: 'bluff', metric: 'won_without_showdown_weaker_hand', tiers: rareTiers},
  {
    key: 'survivor',
    metric: 'hands_without_leaving',
    tiers: [{stars: 1, threshold: 50}, {stars: 2, threshold: 250}, {stars: 3, threshold: 1000}, {
      stars: 4,
      threshold: 5000
    }, {stars: 5, threshold: 25000}]
  },
  {key: 'looser', metric: 'hand_lost_at_showdown', tiers: commonTiers},
  {key: 'almost_winner', metric: 'hand_lost_same_category_as_winner', tiers: commonTiers},
  {key: 'tied', metric: 'hand_tied', tiers: commonTiers},
  {key: 'bad_beat', metric: 'hand_lost_with_trips_or_better', tiers: rareTiers},
  {key: 'cooler', metric: 'hand_lost_with_full_house_or_better', tiers: rareTiers},
  {key: 'cracked_aces', metric: 'pocket_aces_lost', tiers: rareTiers},
  {key: 'fallen_king', metric: 'pocket_kings_lost', tiers: rareTiers},
  {key: 'giant_slayer', metric: 'won_allin_vs_bigger_stack', tiers: rareTiers},
  {key: 'showdown_warrior', metric: 'reached_showdown', tiers: commonTiers},
  {key: 'all_in', metric: 'went_all_in', tiers: commonTiers},
  ...categoryOrder.map(category => ({
    key: `win_category_${category}`,
    metric: 'hand_won_with_category',
    tiers: category === 'royal_flush'
      ? [{stars: 1, threshold: 1}, {stars: 2, threshold: 5}, {stars: 3, threshold: 10}, {
        stars: 4,
        threshold: 25
      }, {stars: 5, threshold: 50}]
      : commonTiers
  }))
];

// Deliberately varied so dev can eyeball every star-fill state: 0 progress,
// mid-tier, and maxed-out.
const mockAchievementProgress: PlayerAchievementProgress[] = [
  {key: 'wins', count: 42},
  {key: 'hands_played', count: 1250},
  {key: 'comeback', count: 3},
  {key: 'survivor', count: 4200},
  {key: 'showdown_warrior', count: 10005},
  {key: 'looser', count: 60},
  {key: 'bad_beat', count: 100},
  {key: 'giant_slayer', count: 1},
  {key: 'win_category_flush', count: 12}
];

/* ponytail: 90s mock cooldown (real API uses 24h) so the countdown is testable in dev;
 * sessionStorage keeps it across reloads since the mock lives in the page */
const MOCK_CREDIT_COOLDOWN_S = 90;
const CREDIT_KEY = 'mock_next_credit_at';
let nextCreditAt = typeof window === 'undefined' ? 0 : Number(sessionStorage.getItem(CREDIT_KEY)) || 0;
const creditCooldown = () => {
  // sessionStorage is authoritative in the browser. Reading it on demand
  // also makes the mock resettable after a Next.js hot reload, where this
  // module's in-memory deadline can otherwise outlive the cleared storage.
  const deadline = typeof window === 'undefined'
    ? nextCreditAt
    : Number(sessionStorage.getItem(CREDIT_KEY)) || 0;
  return Math.max(0, Math.ceil((deadline - Date.now()) / 1000));
};

function ok<T>(data: T, config: InternalAxiosRequestConfig): AxiosResponse<T> {
  return {data, status: 200, statusText: 'OK', headers: {}, config};
}

/** Wraps a list in the standard single-page pagination envelope (mock data never overflows a page). */
function page<T>(items: T[]): Page<T> {
  return {data: items, has_next: false, next_cursor: null, has_previous: false, previous_cursor: null};
}

/** Mirrors the real API's problem-detail error shape (see api/internal/problem) for a given status. */
function fail(status: number, detail: string, config: InternalAxiosRequestConfig): never {
  throw new AxiosError('Mock request failed', String(status), config, undefined, {
    data: {detail}, status, statusText: 'Mock Error', headers: {}, config
  });
}

// Social fixtures: one of each state the People surface has to render (friend
// online, friend in a table, incoming request, outgoing request, blocked,
// recent stranger) plus a pending invite in the inbox.
interface MockSocialPlayer {
  player_id: string;
  name: string;
  friend_code?: string;
  relationship: 'none' | 'incoming' | 'outgoing' | 'friend';
  muted: boolean;
  blocked: boolean;
  presence?: 'online' | 'offline' | 'in_table';
  last_played_at?: number;
  hands_together?: number;
  room_id?: string;
}

const mockSocialPlayers: MockSocialPlayer[] = [
  {
    player_id: 'bia_sp', name: 'Bia', friend_code: 'PKR-BIA1-1111-2222', relationship: 'friend',
    muted: false, blocked: false, presence: 'online', last_played_at: Date.now() - 3_600_000, hands_together: 42
  },
  {
    player_id: 'leo_rio', name: 'Leo', friend_code: 'PKR-LEO1-3333-4444', relationship: 'friend',
    muted: true, blocked: false, presence: 'in_table', last_played_at: Date.now() - 172_800_000, hands_together: 17,
    room_id: ROOM_ID
  },
  {
    player_id: 'caio_bh', name: 'Caio', friend_code: 'PKR-CAIO-5555-6666', relationship: 'incoming',
    muted: false, blocked: false
  },
  {
    player_id: 'duda_poa', name: 'Duda', friend_code: 'PKR-DUDA-7777-8888', relationship: 'outgoing',
    muted: false, blocked: false
  },
  {
    player_id: 'spammer', name: 'Spammer', relationship: 'none', muted: true, blocked: true
  },
  {
    player_id: 'vic_rec', name: 'Vic', friend_code: 'PKR-VICR-9999-0000', relationship: 'none',
    muted: false, blocked: false, last_played_at: Date.now() - 86_400_000, hands_together: 5
  }
];

interface MockSocialEvent {
  event_id: string;
  type: 'friend_request' | 'friend_accepted' | 'table_invite';
  actor_id: string;
  status: 'pending' | 'accepted' | 'declined';
  room_id?: string;
  unread: boolean;
  created_at: number;
  expires_at?: number;
}

let mockSocialEvents: MockSocialEvent[] = [
  {
    event_id: 'evt-invite-1', type: 'table_invite', actor_id: 'bia_sp', status: 'pending',
    room_id: '01J8ZQ4T7M3E5K9V2B6C1D0AFG', unread: true, created_at: Date.now() - 60_000,
    expires_at: Date.now() + 840_000
  },
  {
    event_id: 'evt-request-1', type: 'friend_request', actor_id: 'caio_bh', status: 'pending',
    unread: true, created_at: Date.now() - 600_000
  },
  {
    event_id: 'evt-accepted-1', type: 'friend_accepted', actor_id: 'leo_rio', status: 'accepted',
    unread: false, created_at: Date.now() - 7_200_000
  }
];

function mockSocialPlayer(playerId: string) {
  return mockSocialPlayers.find(player => player.player_id === playerId);
}

function mockSocialUnread() {
  return mockSocialEvents.filter(event => event.unread).length;
}

// Set by MockTableService as the active hand progresses — lets the REST
// `leave` mock mirror the real engine's rule (hand.go: RemovePlayerForActor)
// that a player still dealt in (active/all-in, hand in progress) can't cash out.
let mockPlayerDealtIn = false;
let applyActiveMockRebuy: ((amount: number, autoRebuy: boolean) => void) | null = null;

function scenarioFromLocation(): MockScenario | null {
  if (typeof window === 'undefined') return null;
  return new URLSearchParams(window.location.search).get('scenario') as MockScenario | null;
}

function mockDelay() {
  if (typeof window === 'undefined') return 0;
  const value = Number(window.localStorage.getItem('ctech_poker_mock_delay') || 350);
  return Number.isFinite(value) ? Math.min(15000, Math.max(0, value)) : 350;
}

function forcedError(method: string, path: string) {
  if (typeof window === 'undefined') return undefined;
  const raw = window.localStorage.getItem('ctech_poker_mock_errors');
  if (!raw) return undefined;
  try {
    const rules = JSON.parse(raw) as Record<string, { status: number; body?: unknown }>;
    return rules[`${method} ${path}`] || rules[`* ${path}`] || rules[`${method} *`] || rules['* *'];
  } catch {
    return undefined;
  }
}

function generateNativeULID() {
  const ENCODING = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"; // Crockford's Base32
  let time = Date.now();

  // 1. Encode Timestamp (48 bits -> 10 characters)
  let timeChars = "";
  for (let i = 9; i >= 0; i--) {
    timeChars = ENCODING[time % 32] + timeChars;
    time = Math.floor(time / 32);
  }

  // 2. Encode Randomness (80 bits -> 16 characters) using Secure Crypto
  const randomBytes = new Uint8Array(16);
  crypto.getRandomValues(randomBytes);

  let randomChars = "";
  for (let i = 0; i < 16; i++) {
    randomChars += ENCODING[randomBytes[i] % 32];
  }

  return timeChars + randomChars;
}


function wait(ms: number) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

/** The social surface, split out of mockAdapter so the People flows (friend
 * code, requests, presence, blocks, invites, reports) can be walked in mock
 * mode without a backend. Returns null when the path is not a social one. */
function mockSocialRequest(method: string, path: string, body: Record<string, unknown>,
                           config: InternalAxiosRequestConfig): AxiosResponse | null {
  const idempotencyKey = (config.headers?.['Idempotency-Key'] || config.headers?.['idempotency-key']) as string | undefined;
  const mutating = method !== 'GET';
  if (mutating && !idempotencyKey) fail(400, 'Idempotency-Key is required', config);
  const target = path.match(/^\/v1\.0\/social\/(?:friend-requests|friends|mutes|blocks|relationships)\/(.+)$/);
  const targetId = target ? decodeURIComponent(target[1].replace(/\/(accept|decline)$/, '')) : '';
  const player = targetId ? mockSocialPlayer(targetId) : undefined;

  if (method === 'GET' && path === '/v1.0/social/summary') return ok({unread_count: mockSocialUnread()}, config);
  if (method === 'GET' && path === '/v1.0/social/friends') {
    return ok(page(mockSocialPlayers.filter(item => item.relationship === 'friend')), config);
  }
  if (method === 'GET' && path === '/v1.0/social/friend-requests') {
    const direction = (config.params as { direction?: string } | undefined)?.direction === 'outgoing'
      ? 'outgoing' : 'incoming';
    return ok(page(mockSocialPlayers.filter(item => item.relationship === direction)), config);
  }
  if (method === 'GET' && path === '/v1.0/social/blocked') {
    return ok(page(mockSocialPlayers.filter(item => item.blocked)), config);
  }
  if (method === 'GET' && path === '/v1.0/social/recent') {
    return ok(page(mockSocialPlayers.filter(item => item.last_played_at && !item.blocked)), config);
  }
  if (method === 'GET' && path === '/v1.0/social/inbox') return ok(page(mockSocialEvents), config);
  if (method === 'POST' && path === '/v1.0/social/inbox/read') {
    const ids = Array.isArray(body.event_ids) ? body.event_ids as string[] : [];
    mockSocialEvents = mockSocialEvents.map(event => ids.includes(event.event_id) ? {...event, unread: false} : event);
    return ok({}, config);
  }
  if (method === 'GET' && path === '/v1.0/social/relationships') {
    const ids = String((config.params as { player_ids?: string } | undefined)?.player_ids || '').split(',');
    return ok({data: mockSocialPlayers.filter(item => ids.includes(item.player_id))}, config);
  }
  const lookup = method === 'GET' ? path.match(/^\/v1\.0\/social\/lookup\/(.+)$/) : null;
  if (lookup) {
    const code = decodeURIComponent(lookup[1]).toUpperCase();
    const found = mockSocialPlayers.find(item => item.friend_code === code);
    if (!found) fail(404, 'friend code not found', config);
    return ok(found, config);
  }
  if (method === 'POST' && path === '/v1.0/social/friend-requests') {
    const found = body.friend_code
      ? mockSocialPlayers.find(item => item.friend_code === String(body.friend_code).toUpperCase())
      : mockSocialPlayer(String(body.target_player_id || ''));
    if (!found) fail(404, 'player not found', config);
    if (found.blocked) fail(409, 'the relationship cannot be changed', config);
    found.relationship = found.relationship === 'incoming' ? 'friend' : 'outgoing';
    return ok(found, config);
  }
  if (method === 'GET' && target) return player ? ok(player, config) : fail(404, 'player not found', config);
  if (!player && targetId) fail(404, 'player not found', config);
  if (player) {
    if (method === 'POST' && path.endsWith('/accept')) {
      player.relationship = 'friend';
      return ok(player, config);
    }
    if (method === 'POST' && path.endsWith('/decline')) {
      player.relationship = 'none';
      return ok({}, config);
    }
    if (method === 'DELETE') {
      if (path.includes('/mutes/')) player.muted = false;
      else if (path.includes('/blocks/')) player.blocked = false;
      else player.relationship = 'none';
      return ok({}, config);
    }
    if (method === 'PUT' && path.includes('/mutes/')) {
      player.muted = true;
      return ok(player, config);
    }
    if (method === 'PUT' && path.includes('/blocks/')) {
      player.muted = true;
      player.blocked = true;
      player.relationship = 'none';
      return ok(player, config);
    }
  }
  if (method === 'POST' && path === '/v1.0/social/table-invites') {
    const invited = mockSocialPlayer(String(body.target_player_id || ''));
    if (!invited || invited.relationship !== 'friend') fail(409, 'the social action cannot be completed', config);
    return ok({
      event_id: `evt-invite-${generateNativeULID()}`, type: 'table_invite', actor_id: MOCK_PLAYER_ID, status: 'pending',
      room_id: String(body.room_id || ''), unread: false, created_at: Date.now(), expires_at: Date.now() + 900_000
    }, config);
  }
  const invite = path.match(/^\/v1\.0\/social\/table-invites\/([^/]+)\/(accept|decline)$/);
  if (method === 'POST' && invite) {
    const event = mockSocialEvents.find(item => item.event_id === decodeURIComponent(invite[1]));
    if (!event) fail(404, 'social event not found', config);
    if (event.status !== 'pending' || (event.expires_at && event.expires_at <= Date.now())) {
      fail(409, 'the table invite has expired', config);
    }
    event.status = invite[2] === 'accept' ? 'accepted' : 'declined';
    event.unread = false;
    return ok(invite[2] === 'accept' ? {event, room: rooms[0]} : {}, config);
  }
  if (method === 'POST' && path === '/v1.0/social/reports') {
    if (!body.target_player_id || !body.category || !body.surface) fail(400, 'report or evidence is invalid', config);
    if (body.target_player_id === MOCK_PLAYER_ID) fail(400, 'report or evidence is invalid', config);
    return ok({report_id: `rep-${generateNativeULID()}`, status: 'open'}, config);
  }
  return null;
}

/** Axios adapter matching the REST surface used by Poker's UI. */
export async function mockAdapter(config: InternalAxiosRequestConfig): Promise<AxiosResponse> {
  const method = (config.method || 'get').toUpperCase();
  const path = (config.url || '').replace(/^https?:\/\/[^/]+/, '').split('?')[0];
  await wait(mockDelay());
  const rule = forcedError(method, path);
  if (rule) {
    if (rule.status === 0) throw new AxiosError('Network Error', AxiosError.ERR_NETWORK, config);
    fail(rule.status, (rule.body as { detail?: string })?.detail || 'Erro simulado', config);
  }
  const body = typeof config.data === 'string' ? JSON.parse(config.data || '{}') : (config.data || {});
  if (method === 'GET' && path === '/v1.0/players/me') return ok({...mockProfile}, config);
  if (method === 'GET' && path === '/v1.0/players/me/poker-stats') return ok({
    hands: 184,
    vpip_hands: 47,
    pfr_hands: 31,
    three_bet_hands: 9,
    three_bet_chances: 67,
    vpip_rate: 47 / 184,
    pfr_rate: 31 / 184,
    three_bet_rate: 9 / 67,
    playstyle: [{key: 'initiative'}]
  }, config);
  if (method === 'GET' && path === '/v1.0/players/me/sessions') return ok(page([]), config);
  if (method === 'GET' && path === '/v1.0/players/me/notes/') return ok({data: mockPlayerNotes}, config);
  const playerNoteMatch = method === 'POST' ? path.match(/^\/v1\.0\/players\/me\/notes\/([^/]+)$/) : null;
  if (playerNoteMatch) {
    const opponentId = decodeURIComponent(playerNoteMatch[1]);
    const tag = typeof body.tag === 'string' ? body.tag : '';
    const note = typeof body.note === 'string' ? body.note.trim() : '';
    if (opponentId === MOCK_PLAYER_ID) fail(400, 'opponent must be another player', config);
    if (note.length > 500) fail(400, 'note must have at most 500 characters', config);
    mockPlayerNotes = mockPlayerNotes.filter(item => item.opponent_id !== opponentId);
    if (!tag && !note) return ok({deleted: true}, config);
    const saved: PlayerNote = {
      opponent_id: opponentId,
      tag: tag || undefined,
      note,
      updated_at: new Date().toISOString()
    };
    mockPlayerNotes.push(saved);
    return ok(saved, config);
  }
  if (method === 'POST' && path === '/v1.0/players/me/terms/accept') return ok({
    ...mockProfile,
    poker_terms_accepted: true
  }, config);
  if (method === 'POST' && path === '/v1.0/players/me') {
    if (typeof body.name === 'string') {
      if (!body.name.trim()) fail(400, 'name must not be empty', config);
      mockProfile.name = body.name.trim();
    }
    if (typeof body.wallet_mode === 'string') {
      if (body.wallet_mode !== 'sandbox' && body.wallet_mode !== 'real') {
        fail(400, 'wallet_mode must be sandbox or real', config);
      }
      mockProfile.wallet_mode = body.wallet_mode;
    }
    if (typeof body.deck_variant === 'string') {
      if (!body.deck_variant.trim()) fail(400, 'deck_variant must not be empty', config);
      mockProfile.deck_variant = body.deck_variant;
    }
    if (typeof body.showcase_public === 'boolean') mockProfile.showcase_public = body.showcase_public;
    if (typeof body.playstyle_public === 'boolean') mockProfile.playstyle_public = body.playstyle_public;
    if (typeof body.table_public === 'boolean') mockProfile.table_public = body.table_public;
    if (Array.isArray(body.featured_achievements)) {
      if (body.featured_achievements.length > 3) fail(400, 'too many featured achievements', config);
      mockProfile.featured_achievements = [...body.featured_achievements];
    }
    if (Array.isArray(body.favorite_reactions)) {
      if (body.favorite_reactions.length > 3 || new Set(body.favorite_reactions).size !== body.favorite_reactions.length ||
        body.favorite_reactions.some((id: unknown) => typeof id !== 'string' || !(id in TABLE_REACTIONS))) {
        fail(400, 'invalid favorite reactions', config);
      }
      mockProfile.favorite_reactions = [...body.favorite_reactions];
    }
    return ok({...mockProfile}, config);
  }
  const showcaseMatch = method === 'GET' ? path.match(/^\/v1\.0\/players\/([^/]+)\/showcase$/) : null;
  if (showcaseMatch) {
    if (decodeURIComponent(showcaseMatch[1]) !== MOCK_PLAYER_ID || !mockProfile.showcase_public) {
      fail(404, 'profile showcase not found', config);
    }
    const counts = new Map(mockAchievementProgress.map(item => [item.key, item.count]));
    return ok({
      player_id: mockProfile.user_id,
      name: mockProfile.name,
      playstyle: mockProfile.playstyle_public ? [{key: 'initiative'}] : undefined,
      featured_achievements: mockProfile.featured_achievements.map(key => ({key, count: counts.get(key) || 0})),
      best_hand: mockHands[0]
    }, config);
  }
  if (method === 'GET' && /^\/v1\.0\/wallet\/sandbox-purchase\/skus\/?$/.test(path)) {
    return ok(page(mockSandboxSkus.map(sku => ({...sku}))), config);
  }
  if (method === 'GET' && /^\/v1\.0\/wallet\/sandbox-purchase\/?$/.test(path)) {
    mockSandboxPurchases.forEach(settlePurchase);
    return ok(page(mockSandboxPurchases.map(publicPurchase)), config);
  }
  if (method === 'POST' && /^\/v1\.0\/wallet\/sandbox-purchase\/?$/.test(path)) {
    const sku = mockSandboxSkus.find(item => item.id === body.sku);
    if (!sku) fail(400, 'unknown sandbox SKU', config);
    const outcomeValue = typeof window === 'undefined'
      ? 'confirmed'
      : window.localStorage.getItem('ctech_poker_mock_purchase_outcome');
    const outcome = outcomeValue === 'expired' || outcomeValue === 'failed' ? outcomeValue : 'confirmed';
    const now = Date.now();
    const purchase: MockSandboxPurchase = {
      player_id: MOCK_PLAYER_ID,
      purchase_id: `mock-pix-${crypto.randomUUID()}`,
      sku: sku.id,
      price_cents: sku.price_cents,
      base_credits: sku.base_credits,
      bonus_percent: sku.bonus_percent,
      total_credits: sku.total_credits,
      status: 'pending',
      pix_copia_e_cola: `00020126MOCK.CTECH.POKER.${sku.id}.${crypto.randomUUID()}`,
      qr_code_base64: mockQRCodeBase64(`${sku.id}.${body.idem_key || now}`),
      expires_at: new Date(now + (outcome === 'expired' ? 7000 : 120_000)).toISOString(),
      created_at: new Date(now).toISOString(),
      updated_at: new Date(now).toISOString(),
      resolves_at_ms: now + 7000,
      balance_applied: false,
      outcome,
    };
    mockSandboxPurchases.unshift(purchase);
    return ok(publicPurchase(purchase), config);
  }
  const sandboxPurchaseMatch = path.match(/^\/v1\.0\/wallet\/sandbox-purchase\/([^/]+)\/?$/);
  if (method === 'GET' && sandboxPurchaseMatch) {
    const purchase = mockSandboxPurchases.find(item => item.purchase_id === sandboxPurchaseMatch[1]);
    if (!purchase) fail(404, 'sandbox purchase not found', config);
    settlePurchase(purchase);
    return ok(publicPurchase(purchase), config);
  }
  const refundPurchaseMatch = path.match(/^\/v1\.0\/wallet\/sandbox-purchase\/([^/]+)\/refund\/?$/);
  if (method === 'POST' && refundPurchaseMatch) {
    const purchase = mockSandboxPurchases.find(item => item.purchase_id === refundPurchaseMatch[1]);
    if (!purchase) fail(404, 'sandbox purchase not found', config);
    settlePurchase(purchase);
    if (purchase.status !== 'confirmed') fail(409, 'only confirmed purchases can be refunded', config);
    purchase.status = 'refunded';
    purchase.updated_at = new Date().toISOString();
    if (purchase.balance_applied) {
      mockProfile.sandbox_balance = Math.max(0, mockProfile.sandbox_balance - (purchase.total_credits || 0));
      purchase.balance_applied = false;
    }
    return ok(publicPurchase(purchase), config);
  }
  if (method === 'GET' && /^\/v1\.0\/wallet\/reaction-purchase\/catalog\/?$/.test(path)) {
    // Ownership mirrors the real API: derived from the entitlement the server
    // holds, which in mock-land is "a confirmed purchase exists".
    return ok(page(mockReactionCatalog.map(entry => ({
      ...entry,
      owned: !entry.premium || mockReactionPurchases.some(
        item => item.reaction_id === entry.id && item.status === 'confirmed'),
    }))), config);
  }
  if (method === 'GET' && /^\/v1\.0\/wallet\/reaction-purchase\/?$/.test(path)) {
    mockReactionPurchases.forEach(settleReactionPurchase);
    return ok(page(mockReactionPurchases.map(item => ({...item}))), config);
  }
  if (method === 'POST' && /^\/v1\.0\/wallet\/reaction-purchase\/?$/.test(path)) {
    const prices = mockReactionPrices[body.reaction_id];
    if (!prices) fail(400, 'reaction is not premium', config);
    if (body.method !== 'pix' && body.method !== 'fichas') fail(400, 'invalid purchase method', config);
    mockReactionPurchases.forEach(settleReactionPurchase);
    if (mockReactionPurchases.some(item => item.reaction_id === body.reaction_id &&
      (item.status === 'confirmed' || item.status === 'pending' || item.status === 'processing'))) {
      fail(409, 'reaction already owned or pending', config);
    }
    const now = Date.now();
    const purchase: MockReactionPurchase = {
      player_id: MOCK_PLAYER_ID,
      purchase_id: `mock-reaction-${crypto.randomUUID()}`,
      reaction_id: body.reaction_id,
      method: body.method,
      ...prices,
      status: body.method === 'fichas' ? 'confirmed' : 'pending',
      created_at: new Date(now).toISOString(),
      updated_at: new Date(now).toISOString(),
    };
    if (body.method === 'fichas') {
      if (mockProfile.sandbox_balance < prices.price_fichas) fail(400, 'insufficient sandbox balance', config);
      mockProfile.sandbox_balance -= prices.price_fichas;
    } else {
      purchase.pix_copia_e_cola = `00020126MOCK.CTECH.POKER.REACTION.${body.reaction_id}.${crypto.randomUUID()}`;
      purchase.qr_code_base64 = mockQRCodeBase64(`${body.reaction_id}.${body.idem_key || now}`);
      purchase.expires_at = new Date(now + 120_000).toISOString();
      purchase.resolves_at_ms = now + 7000;
      purchase.outcome = 'confirmed';
    }
    mockReactionPurchases.unshift(purchase);
    return ok({...purchase}, config);
  }
  const reactionPurchaseMatch = path.match(/^\/v1\.0\/wallet\/reaction-purchase\/([^/]+)\/?$/);
  if (method === 'GET' && reactionPurchaseMatch) {
    const purchase = mockReactionPurchases.find(item => item.purchase_id === decodeURIComponent(reactionPurchaseMatch[1]));
    if (!purchase) fail(404, 'reaction purchase not found', config);
    settleReactionPurchase(purchase);
    return ok({...purchase}, config);
  }
  const reactionRefundMatch = path.match(/^\/v1\.0\/wallet\/reaction-purchase\/([^/]+)\/refund\/?$/);
  if (method === 'POST' && reactionRefundMatch) {
    const purchase = mockReactionPurchases.find(item => item.purchase_id === decodeURIComponent(reactionRefundMatch[1]));
    if (!purchase) fail(404, 'reaction purchase not found', config);
    settleReactionPurchase(purchase);
    if (purchase.status !== 'confirmed') fail(409, 'reaction purchase is not refundable', config);
    purchase.status = 'refunded';
    purchase.updated_at = new Date().toISOString();
    if (purchase.method === 'fichas') mockProfile.sandbox_balance += purchase.price_fichas || 0;
    return ok({...purchase}, config);
  }
  if (method === 'GET' && path === '/v1.0/rooms') return ok(page(rooms), config);
  // Checked before the generic single-segment room-id match below, since
  // "stakes" would otherwise itself match `/rooms/:id` and never reach here.
  if (method === 'GET' && path === '/v1.0/rooms/stakes') {
    // Mock mirrors REAL_MONEY_ENABLED=false: "real" 404s, same as prod with
    // the flag off, so CreateRoomDialog's real-money toggle stays hidden.
    if (config.params?.currency_mode === 'real') {
      fail(404, 'real-money mode is not available', config);
    }
    return ok({
      stakes: [
        {small_blind: 10, big_blind: 20},
        {small_blind: 25, big_blind: 50},
        {small_blind: 50, big_blind: 100},
        {small_blind: 100, big_blind: 200},
        {small_blind: 200, big_blind: 500},
        {small_blind: 500, big_blind: 1000},
        {small_blind: 1000, big_blind: 2000},
        {small_blind: 2500, big_blind: 5000},
        {small_blind: 5000, big_blind: 10000},
        {small_blind: 25000, big_blind: 50000},
        {small_blind: 50000, big_blind: 100000},
      ]
    }, config);
  }
  const roomMatch = method === 'GET' ? path.match(/^\/v1\.0\/rooms\/([^/]+)$/) : null;
  if (roomMatch) {
    const room = rooms.find(r => r.room_id === roomMatch[1]);
    if (!room) fail(404, 'room not found', config);
    return ok(room, config);
  }
  if (method === 'POST' && path === '/v1.0/rooms') {
    if (body.visibility !== 'public' && body.visibility !== 'private') fail(400, 'visibility must be public or private', config);
    if (!(body.small_blind > 0) || !(body.big_blind > body.small_blind)) fail(400, 'blinds must be positive and big_blind greater than small_blind', config);
    if (body.max_seats !== 6 && body.max_seats !== 9) fail(400, 'max_seats must be 6 or 9', config);
    if (!(body.buy_in_min > 0) || body.buy_in_max < body.buy_in_min || body.buy_in_min % body.big_blind !== 0 || body.buy_in_max % body.big_blind !== 0) {
      fail(400, 'buy-in limits must be ordered positive multiples of big_blind', config);
    }
    const room = {
      ...body,
      room_id: generateNativeULID(),
      currency_mode: 'sandbox',
      status: 'waiting',
      seats_taken: 0,
      created_by: MOCK_PLAYER_ID,
      ...(body.visibility === 'private' ? {share_code: crypto.randomUUID().slice(0, 6).toUpperCase()} : {})
    };
    rooms.unshift(room);
    return ok(room, config);
  }
  const joinMatch = method === 'POST' ? path.match(/^\/v1\.0\/rooms\/([^/]+)\/join$/) : null;
  if (joinMatch) {
    const room = rooms.find(r => r.room_id === joinMatch[1]);
    if (!room) fail(404, 'room not found', config);
    if (room.currency_mode !== 'sandbox') fail(400, 'unsupported currency mode', config);
    if (!(body.amount >= room.buy_in_min) || body.amount > room.buy_in_max || body.amount % room.big_blind !== 0) {
      fail(400, 'amount must be within range and a multiple of big_blind', config);
    }
    const isCreator = room.created_by === MOCK_PLAYER_ID;
    if (room.visibility === 'private' && !isCreator && body.share_code !== room.share_code) {
      fail(403, 'share code required to join a private room', config);
    }
    if (scenarioFromLocation() === 'rebuy') {
      applyActiveMockRebuy?.(Number(body.amount), Boolean(body.auto_rebuy));
    }
    return ok({}, config);
  }
  if (method === 'GET' && /^\/v1\.0\/rooms\/[^/]+\/seated$/.test(path)) {
    // Named scenes are intentional local-QA deep links. A fresh browser should
    // land on the state under test instead of requiring a throwaway buy-in.
    return ok({seated: Boolean(scenarioFromLocation()), stack: 4850}, config);
  }
  const leaveMatch = method === 'POST' ? path.match(/^\/v1\.0\/rooms\/([^/]+)\/leave$/) : null;
  if (leaveMatch) {
    if (!rooms.find(r => r.room_id === leaveMatch[1])) fail(404, 'room not found', config);
    if (mockPlayerDealtIn) fail(409, 'cannot remove player mid-hand while still dealt in', config);
    return ok({amount: 4850}, config);
  }
  if (method === 'GET' && path === '/v1.0/players/me/hands') {
    const tableId = config.params?.table_id;
    const all = tableId ? mockHistory.filter(h => h.table_id === tableId) : mockHistory;
    const start = Number(config.params?.cursor) || 0;
    const end = start + MOCK_HANDS_PAGE;
    return ok({
      data: all.slice(start, end),
      has_next: end < all.length,
      next_cursor: end < all.length ? String(end) : null,
      has_previous: start > 0,
      previous_cursor: start > 0 ? String(Math.max(0, start - MOCK_HANDS_PAGE)) : null
    }, config);
  }
  const createShareMatch = method === 'POST' ? path.match(/^\/v1\.0\/players\/me\/hand\/([^/]+)\/share$/) : null;
  if (createShareMatch) {
    const hand = mockHands.find(h => h.hand_id === createShareMatch[1]);
    if (!hand) fail(404, 'hand not found', config);
    return ok({
      token: 'mock-share-demo',
      kind: body.kind === 'bad_beat' ? 'bad_beat' : 'brag',
      outcome: hand.outcome,
      net_change: hand.net_change,
      ended_at: hand.ended_at,
      board: hand.board,
      hero_cards: body.include_hero_cards ? hand.hole_cards : undefined,
      opponents: hand.opponents?.map((opponent, index) => ({
        alias: `Jogador ${index + 1}`, hole_cards: opponent.hole_cards, won: opponent.won
      })),
      actions: sharedMockActions(mockHandActions[hand.hand_id] || []),
      created_at: Date.now(),
      expires_at: Date.now() + Number(body.expiry_days || 7) * 86_400_000
    }, config);
  }
  const publicShareMatch = method === 'GET' ? path.match(/^\/v1\.0\/hand-shares\/([^/]+)$/) : null;
  if (publicShareMatch) {
    if (publicShareMatch[1] !== 'mock-share-demo') fail(404, 'hand share not found', config);
    const hand = mockHands[0];
    return ok({
      token: 'mock-share-demo', kind: 'brag', outcome: hand.outcome, net_change: hand.net_change,
      ended_at: hand.ended_at, board: hand.board, hero_cards: hand.hole_cards,
      opponents: hand.opponents?.map((opponent, index) => ({
        alias: `Jogador ${index + 1}`, hole_cards: opponent.hole_cards, won: opponent.won
      })),
      actions: sharedMockActions(mockHandActions[hand.hand_id] || []),
      created_at: Date.now(), expires_at: Date.now() + 7 * 86_400_000
    }, config);
  }
  // getHand() calls the singular `/hand/{id}`; matching only the plural made
  // every /hands/history open on the error boundary in mock mode.
  const handMatch = method === 'GET' ? path.match(/^\/v1\.0\/players\/me\/hands?\/([^/]+)$/) : null;
  if (handMatch) {
    const hand = mockHands.find(h => h.hand_id === handMatch[1]);
    if (!hand) fail(404, 'hand not found', config);
    return ok(hand, config);
  }
  const handHistoryMatch = method === 'GET' ? path.match(/^\/v1\.0\/tables\/[^/]+\/hands\/([^/]+)\/history$/) : null;
  if (handHistoryMatch) {
    const hand = mockHands.find(h => h.hand_id === handHistoryMatch[1]);
    if (!hand) fail(404, 'hand not found', config);
    return ok({table_id: hand.table_id, hand_id: hand.hand_id, actions: mockHandActions[hand.hand_id] || []}, config);
  }
  if (path.startsWith('/v1.0/social')) {
    const social = mockSocialRequest(method, path, body, config);
    if (social) return social;
  }
  if (method === 'GET' && path === '/v1.0/leaderboard') return ok(page([
    {player_id: 'bia_sp', player_name: 'Bia', hands_played: 248, hands_won: 71, win_rate: .286},
    {player_id: MOCK_PLAYER_ID, player_name: mockProfile.name, hands_played: 184, hands_won: 49, win_rate: .266},
    {player_id: 'leo_rio', player_name: 'Leo', hands_played: 213, hands_won: 52, win_rate: .244},
  ]), config);
  if (method === 'GET' && path === '/v1.0/achievements') return ok(achievementCatalog, config);
  if (method === 'GET' && path === '/v1.0/players/me/achievements') return ok(page(mockAchievementProgress), config);
  if (method === 'GET' && /^\/v1\.0\/sandbox-credits\/?$/.test(path)) {
    return ok({remaining_time_seconds: creditCooldown()}, config);
  }
  if (method === 'POST' && /^\/v1\.0\/sandbox-credits\/?$/.test(path)) {
    if (creditCooldown() > 0) return ok({amount: 0, remaining_time_seconds: creditCooldown()}, config);
    nextCreditAt = Date.now() + MOCK_CREDIT_COOLDOWN_S * 1000;
    sessionStorage.setItem(CREDIT_KEY, String(nextCreditAt));
    mockProfile.sandbox_balance += 250;
    return ok({amount: 250, remaining_time_seconds: MOCK_CREDIT_COOLDOWN_S}, config);
  }
  if (method === 'GET' && /^\/v1\.0\/wallet\/cosmetic-purchase\/(deck|felt)\/catalog$/.test(path)) {
    const kind = path.includes('/deck/') ? 'deck' : 'felt';
    const deckCatalog = [
      {kind: 'deck', id: 'four-color', premium: false},
      {kind: 'deck', id: 'two-color', premium: false},
      {kind: 'deck', id: 'colorblind', premium: false},
      {kind: 'deck', id: 'high-constrast', premium: false},
      {kind: 'deck', id: 'casino', premium: true, price_cents: 500, price_fichas: 200000},
      {kind: 'deck', id: 'bicycle', premium: true, price_cents: 500, price_fichas: 200000},
      {kind: 'deck', id: 'vintage', premium: true, price_cents: 500, price_fichas: 200000},
      {kind: 'deck', id: 'golden', premium: true, price_cents: 1000, price_fichas: 500000},
      {kind: 'deck', id: 'pink', premium: true, price_cents: 1000, price_fichas: 500000},
      {kind: 'deck', id: 'alt', premium: true, price_cents: 1000, price_fichas: 500000},
    ];
    const feltCatalog = [
      {kind: 'felt', id: 'classic', premium: false},
      {kind: 'felt', id: 'midnight', premium: true, price_cents: 500, price_fichas: 1000000},
      {kind: 'felt', id: 'burgundy', premium: true, price_cents: 500, price_fichas: 1000000},
      {kind: 'felt', id: 'ocean', premium: true, price_cents: 500, price_fichas: 1000000},
    ];
    // Nothing is bought in mock-land, so only the free items come back owned.
    const entries = (kind === 'deck' ? deckCatalog : feltCatalog).map(entry => ({...entry, owned: !entry.premium}));
    return ok(page(entries), config);
  }
  if (method === 'GET' && path.startsWith('/v1.0/wallet/cosmetic-purchase')) return ok(page([]), config);
  if (method === 'GET' && path.startsWith('/v1.0/wallet/cosmetic-catalog')) return ok(page([]), config);
  const highlightMatch = method === 'GET' ? path.match(/^\/v1\.0\/rooms\/([^/]+)\/highlights\/today$/) : null;
  if (highlightMatch) {
    return ok({
      table_id: highlightMatch[1], date: new Date().toISOString().slice(0, 10),
      hand_id: 'mock-highlight-hand', pot: 18500, board: ['Kh', 'Kd', '7c', '2s', '9h'],
      revealed: [{player_id: MOCK_PLAYER_ID, name: mockProfile.name, hole_cards: ['Ks', 'As']}],
      recorded_at: Date.now(),
    }, config);
  }
  return ok({}, config);
}

export type MockConnectionStatus = 'connecting' | 'connected' | 'reconnecting' | 'disconnected' | 'error';

const baseSeats = () => [
  {
    player_id: MOCK_PLAYER_ID,
    name: 'Ana',
    stack: 4850,
    state: 'active',
    dealt_in: true,
    contributed: 50,
    hole_cards: ['AH', 'KD'],
    equity: .64
  },
  {
    player_id: 'bia_sp',
    name: 'Bia',
    playstyle_badge: 'explorer',
    stack: 3925,
    state: 'active',
    dealt_in: true,
    contributed: 75,
    hole_cards: ['back', 'back']
  },
  {
    player_id: 'leo_rio',
    name: 'Léo',
    stack: 6100,
    state: 'folded',
    dealt_in: true,
    contributed: 25,
    hole_cards: ['back', 'back']
  },
  {
    player_id: 'nina_recife',
    name: 'Nina',
    stack: 2775,
    state: 'active',
    dealt_in: true,
    contributed: 75,
    hole_cards: ['back', 'back']
  },
  // Nameless on purpose — exercises the is-pending-name placeholder in dev.
  {player_id: 'gui_bh', stack: 5000, state: 'sitting_out', dealt_in: false, contributed: 0},
  {
    player_id: 'joao_floripa',
    name: 'João',
    stack: 4375,
    state: 'active',
    dealt_in: true,
    contributed: 75,
    hole_cards: ['back', 'back']
  },
  {
    player_id: 'mari_belém',
    name: 'Mari',
    stack: 8200,
    state: 'active',
    dealt_in: true,
    connection_state: 'disconnected' as const,
    contributed: 0
  },
  {
    player_id: 'caio_goiânia',
    name: 'Caio',
    stack: 3400,
    state: 'all_in',
    dealt_in: true,
    contributed: 625,
    hole_cards: ['back', 'back']
  },
  // 9th seat — the room's max_seats, so dev/QA always sees the table at
  // real worst-case capacity, not a comfortably under-full one.
  {
    player_id: 'rafa_curitiba',
    name: 'Rafa',
    stack: 5200,
    state: 'active',
    dealt_in: true,
    contributed: 75,
    hole_cards: ['back', 'back']
  },
];

function revealShowdownCards(seats: SeatView[]) {
  return seats.map(seat => {
    if (seat.player_id === 'bia_sp') return {...seat, hole_cards: ['9S', '9D']};
    if (seat.player_id === 'nina_recife') return {...seat, hole_cards: ['QC', 'JD']};
    if (seat.player_id === 'caio_goiânia') return {...seat, hole_cards: ['7C', '7D']};
    return seat;
  });
}

// Six-handed lineup used by the interactive full-hand simulation. Blinds are
// already posted (viewer is the big blind, bia_sp the small blind).
function fullHandSeats(): SeatView[] {
  return [
    {
      player_id: MOCK_PLAYER_ID,
      name: 'Ana',
      stack: 4850,
      state: 'active',
      dealt_in: true,
      contributed: 50,
      hole_cards: ['AH', 'KD'],
      equity: .64
    },
    {
      player_id: 'bia_sp',
      name: 'Bia',
      playstyle_badge: 'explorer',
      stack: 3925,
      state: 'active',
      dealt_in: true,
      contributed: 25,
      hole_cards: ['back', 'back']
    },
    {
      player_id: 'leo_rio',
      name: 'Léo',
      stack: 6100,
      state: 'active',
      dealt_in: true,
      contributed: 0,
      hole_cards: ['back', 'back']
    },
    {
      player_id: 'nina_recife',
      name: 'Nina',
      stack: 2775,
      state: 'active',
      dealt_in: true,
      contributed: 0,
      hole_cards: ['back', 'back']
    },
    {
      player_id: 'joao_floripa',
      name: 'João',
      stack: 4375,
      state: 'active',
      dealt_in: true,
      contributed: 0,
      hole_cards: ['back', 'back']
    },
    {
      player_id: 'caio_goiânia',
      name: 'Caio',
      stack: 3400,
      state: 'active',
      dealt_in: true,
      contributed: 0,
      hole_cards: ['back', 'back']
    },
  ];
}

type InteractiveScenario = 'full_hand' | 'full_hand_loss' | 'full_hand_tie' | 'all_in' | 'auto_fold';
type HandVariant = {
  board: [string, string, string, string, string];
  reveal: Record<string, [string, string]>;
  rank: Record<string, number>;
  category: Record<string, string>;
};

const INTERACTIVE_SCENARIOS = new Set<MockScenario>([
  'full_hand', 'full_hand_loss', 'full_hand_tie', 'all_in', 'auto_fold'
]);

// Fixed, internally-consistent showdowns make the visual outcomes repeatable
// while bot decisions remain seeded and profile-driven. A tie uses Broadway
// on the board, so every player still in the hand genuinely shares the same
// five-card straight rather than merely receiving an artificial equal rank.
const WIN_REVEAL: Record<string, [string, string]> = {
  [MOCK_PLAYER_ID]: ['AH', 'KD'], // pair of aces, king kicker — best hand
  'bia_sp': ['9S', '9D'],         // pair of nines
  'nina_recife': ['6C', '6D'],    // pair of sixes
  'caio_goiânia': ['3C', '3D'],   // pair of threes
  'leo_rio': ['JH', 'TH'],        // jack high
  'joao_floripa': ['5S', '4D'],   // five high
};
const WIN_RANK: Record<string, number> = {
  [MOCK_PLAYER_ID]: 6, 'bia_sp': 5, 'nina_recife': 4, 'caio_goiânia': 3, 'leo_rio': 2, 'joao_floripa': 1,
};
const WIN_CATEGORY: Record<string, string> = {
  [MOCK_PLAYER_ID]: 'pair', 'bia_sp': 'pair', 'nina_recife': 'pair',
  'caio_goiânia': 'pair', 'leo_rio': 'high_card', 'joao_floripa': 'high_card'
};
const LOSS_REVEAL: Record<string, [string, string]> = {
  [MOCK_PLAYER_ID]: ['AH', 'KD'],
  'bia_sp': ['AS', 'AD'],
  'nina_recife': ['8S', '8D'],
  'caio_goiânia': ['7C', '7D'],
  'leo_rio': ['QH', 'QC'],
  'joao_floripa': ['2C', '2S'],
};
const LOSS_RANK: Record<string, number> = {
  [MOCK_PLAYER_ID]: 1, 'bia_sp': 6, 'leo_rio': 5, 'nina_recife': 4, 'caio_goiânia': 3, 'joao_floripa': 2,
};
const LOSS_CATEGORY: Record<string, string> = {
  [MOCK_PLAYER_ID]: 'pair', 'bia_sp': 'three_of_a_kind', 'leo_rio': 'three_of_a_kind',
  'nina_recife': 'three_of_a_kind', 'caio_goiânia': 'three_of_a_kind', 'joao_floripa': 'three_of_a_kind'
};
const TIE_REVEAL: Record<string, [string, string]> = {
  [MOCK_PLAYER_ID]: ['2C', '3D'],
  'bia_sp': ['4C', '5D'],
  'nina_recife': ['6C', '7D'],
  'caio_goiânia': ['8C', '9D'],
  'leo_rio': ['2S', '4D'],
  'joao_floripa': ['3S', '6D'],
};
const TIE_RANK = Object.fromEntries(Object.keys(TIE_REVEAL).map(id => [id, 1]));
const TIE_CATEGORY = Object.fromEntries(Object.keys(TIE_REVEAL).map(id => [id, 'straight']));
const HAND_VARIANTS: Record<InteractiveScenario, HandVariant> = {
  full_hand: {
    board: ['7H', '8C', 'QS', '2D', 'AC'], reveal: WIN_REVEAL, rank: WIN_RANK, category: WIN_CATEGORY
  },
  all_in: {
    board: ['7H', '8C', 'QS', '2D', 'AC'], reveal: WIN_REVEAL, rank: WIN_RANK, category: WIN_CATEGORY
  },
  auto_fold: {
    board: ['7H', '8C', 'QS', '2D', 'AC'], reveal: WIN_REVEAL, rank: WIN_RANK, category: WIN_CATEGORY
  },
  full_hand_loss: {
    board: ['7H', '8C', 'QS', '2D', 'AC'], reveal: LOSS_REVEAL, rank: LOSS_RANK, category: LOSS_CATEGORY
  },
  full_hand_tie: {
    board: ['AH', 'KD', 'QS', 'JC', 'TH'], reveal: TIE_REVEAL, rank: TIE_RANK, category: TIE_CATEGORY
  },
};

export function snapshotForScenario(scenario: MockScenario): TableSnapshot {
  const seats = baseSeats();
  if (scenario === 'heads_up' || scenario === 'six_max' || scenario === 'nine_max') {
    const layoutSeats = scenario === 'heads_up' ? fullHandSeats().slice(0, 2) :
      scenario === 'six_max' ? fullHandSeats() : seats;
    return {
      stage: 'flop',
      board: ['7H', '8C', 'QS'],
      seats: layoutSeats,
      current_player_id: MOCK_PLAYER_ID,
      legal_actions: {
        actions: ['fold', 'check', 'raise'], min_raise_to: 100, max_raise_to: 4900, step: 25,
        current_bet: 50, current_contribution: 50, half_pot_raise_to: 175, pot_raise_to: 300
      },
      dealer_player_id: layoutSeats.at(-1)?.player_id,
      small_blind_player_id: layoutSeats[1]?.player_id,
      big_blind_player_id: MOCK_PLAYER_ID,
      rake: 5
    };
  }
  if (scenario === 'rebuy') return {
    stage: 'waiting_for_players',
    board: [],
    seats: seats.slice(0, 3).map((seat, index) => index === 0 ? {
      ...seat, stack: 0, contributed: 0, state: 'sitting_out', dealt_in: false, auto_rebuy: false
    } : {...seat, contributed: 0, dealt_in: false}),
    rake: 0
  };
  if (scenario === 'waiting') return {
    stage: 'waiting_for_players',
    board: [],
    seats: seats.slice(0, 3).map(seat => ({...seat, contributed: 0})),
    rake: 0
  };
  if (INTERACTIVE_SCENARIOS.has(scenario)) {
    const handSeats = fullHandSeats();
    if (scenario === 'auto_fold') {
      const raiser = handSeats.find(seat => seat.player_id === 'bia_sp')!;
      raiser.stack -= 125;
      raiser.contributed = 150;
    }
    return {
      stage: 'pre_flop',
      board: [],
      seats: handSeats,
      current_player_id: scenario === 'auto_fold' ? MOCK_PLAYER_ID : 'leo_rio',
      legal_actions: {actions: []},
      rake: 5,
      // Dealer sits immediately before the small blind in turn order.
      dealer_player_id: 'caio_goiânia',
      small_blind_player_id: 'bia_sp',
      big_blind_player_id: MOCK_PLAYER_ID
    };
  }
  if (scenario === 'pre_flop' || scenario === 'action_error' || scenario === 'timeout' ||
    scenario === 'reconnecting' || scenario === 'reality_check') {
    return {
      stage: 'pre_flop',
      board: [],
      seats,
      current_player_id: scenario === 'reality_check' ? 'nina_recife' : MOCK_PLAYER_ID,
      legal_actions: scenario === 'reality_check' ? {actions: []} : {
        actions: ['fold', 'call', 'raise'],
        call_amount: 25,
        min_raise_to: 150,
        max_raise_to: 4900,
        step: 25
      },
      rake: 5,
      // leo_rio's 25 contributed-then-folded is the small blind; Ana's 50 is
      // the big blind she's now facing a raise on top of (baseSeats above).
      dealer_player_id: 'rafa_curitiba',
      small_blind_player_id: 'leo_rio',
      big_blind_player_id: MOCK_PLAYER_ID,
      ...(scenario === 'timeout' ? {action_deadline_unix_ms: Date.now() + 15000} : {})
    };
  }
  if (scenario === 'complete' || scenario === 'complete_loss' || scenario === 'complete_tie' || scenario === 'fold_win' || scenario === 'run_it_twice') {
    const variant = scenario === 'complete_loss' ? HAND_VARIANTS.full_hand_loss :
      scenario === 'complete_tie' ? HAND_VARIANTS.full_hand_tie : HAND_VARIANTS.full_hand;
    const resolvedSeats = fullHandSeats().map(seat => {
      const holeCards = variant.reveal[seat.player_id];
      const isFolded = scenario === 'fold_win' && seat.player_id !== MOCK_PLAYER_ID;
      return {
        ...seat,
        state: isFolded ? 'folded' : seat.state,
        hole_cards: isFolded ? ['back', 'back'] : holeCards,
        hole_cards_revealed: isFolded || scenario === 'fold_win' ? [false, false] : [true, true],
        hand_category: isFolded || !holeCards ? undefined : variant.category[seat.player_id]
      };
    });
    const winners = scenario === 'complete_loss' ? ['bia_sp'] :
      scenario === 'complete_tie' ? [MOCK_PLAYER_ID, 'bia_sp'] : [MOCK_PLAYER_ID];
    const payouts: Record<string, number> = scenario === 'complete_loss' ? {bia_sp: 1275} :
      scenario === 'complete_tie' ? {[MOCK_PLAYER_ID]: 638, bia_sp: 637} : {[MOCK_PLAYER_ID]: 1275};
    return {
      stage: 'complete',
      board: scenario === 'fold_win' ? [] : variant.board,
      ...(scenario === 'run_it_twice' ? {board_two: ['4C', '5D'], board_split_at: 3} : {}),
      seats: resolvedSeats,
      payouts,
      winners,
      pot_results: [{
        amount: 1275,
        payout_amount: 1275,
        eligible_player_ids: resolvedSeats.filter(seat => seat.state !== 'folded').map(seat => seat.player_id),
        winner_player_ids: winners,
        payouts
      }],
      rake: 5,
      next_hand_unix_ms: Date.now() + NEXT_HAND_DELAY_MS,
      won_without_showdown: scenario === 'fold_win',
      dealer_player_id: 'rafa_curitiba',
      small_blind_player_id: 'leo_rio',
      big_blind_player_id: MOCK_PLAYER_ID
    };
  }
  if (scenario === 'winner_cards' || scenario === 'rabbit_hunt') {
    const winnerID = scenario === 'winner_cards' ? 'bia_sp' : MOCK_PLAYER_ID;
    const resolvedSeats = fullHandSeats().map(seat => ({
      ...seat,
      state: seat.player_id === winnerID ? 'active' : 'folded',
      contributed: 0,
      hole_cards: seat.player_id === MOCK_PLAYER_ID ? ['AH', 'KD'] : ['back', 'back'],
      hole_cards_revealed: [false, false] as [boolean, boolean]
    }));
    return {
      stage: 'complete',
      board: ['7H', '8C', 'QS'],
      seats: resolvedSeats,
      payouts: {[winnerID]: 425},
      winners: [winnerID],
      pot_results: [{
        amount: 425,
        payout_amount: 425,
        eligible_player_ids: [winnerID],
        winner_player_ids: [winnerID],
        payouts: {[winnerID]: 425}
      }],
      rake: 5,
      won_without_showdown: true,
      ...(scenario === 'rabbit_hunt' ? {
        shuffle_server_seed_hex: mockHands[0].server_seed,
        shuffle_commit_hash: mockHands[0].commit_hash
      } : {}),
      dealer_player_id: 'caio_goiânia',
      small_blind_player_id: 'bia_sp',
      big_blind_player_id: MOCK_PLAYER_ID
    };
  }
  // A short-stacked all-in (Ana, 300) can only ever win the layer every
  // contributor matched — the main pot (300 * 3 = 900). The extra 400 that
  // Bia and Léo each put in above that forms a side pot (400 * 2 = 800) Ana
  // is not eligible for, even though her hand is the best of the three: this
  // is the one case where two different seats each show a win pill (and a
  // chip stack) from the same hand at once.
  if (scenario === 'side_pot') return {
    stage: 'complete',
    board: ['7H', '8C', 'QS', '2D', 'AC'],
    seats: [
      {
        player_id: MOCK_PLAYER_ID, name: 'Ana', stack: 0, state: 'all_in', dealt_in: true, contributed: 300,
        hole_cards: ['AH', '8D'], hand_category: 'two_pair'
      },
      {
        player_id: 'bia_sp', name: 'Bia', stack: 0, state: 'all_in', dealt_in: true, contributed: 700,
        hole_cards: ['JC', 'JD'], hand_category: 'pair'
      },
      {
        player_id: 'leo_rio', name: 'Léo', stack: 5400, state: 'active', dealt_in: true, contributed: 700,
        hole_cards: ['9S', '4D'], hand_category: 'high_card'
      },
      {
        player_id: 'nina_recife',
        name: 'Nina',
        stack: 2700,
        state: 'folded',
        dealt_in: true,
        contributed: 75,
        hole_cards: ['back', 'back']
      },
      {
        player_id: 'joao_floripa',
        name: 'João',
        stack: 4375,
        state: 'folded',
        dealt_in: true,
        contributed: 0,
        hole_cards: ['back', 'back']
      },
      {
        player_id: 'caio_goiânia',
        name: 'Caio',
        stack: 3350,
        state: 'folded',
        dealt_in: true,
        contributed: 50,
        hole_cards: ['back', 'back']
      },
    ],
    payouts: {[MOCK_PLAYER_ID]: 900, 'bia_sp': 800},
    winners: [MOCK_PLAYER_ID, 'bia_sp'],
    pot_results: [
      {
        amount: 900, payout_amount: 900,
        eligible_player_ids: [MOCK_PLAYER_ID, 'bia_sp', 'leo_rio'],
        winner_player_ids: [MOCK_PLAYER_ID],
        payouts: {[MOCK_PLAYER_ID]: 900}
      },
      {
        amount: 800, payout_amount: 800,
        eligible_player_ids: ['bia_sp', 'leo_rio'],
        winner_player_ids: ['bia_sp'],
        payouts: {bia_sp: 800}
      }
    ],
    rake: 25,
    next_hand_unix_ms: Date.now() + NEXT_HAND_DELAY_MS,
    dealer_player_id: 'joao_floripa',
    small_blind_player_id: 'nina_recife',
    big_blind_player_id: 'caio_goiânia'
  };
  if (scenario === 'flop') return {
    stage: 'flop',
    board: ['7H', '8C', 'QS'],
    seats,
    current_player_id: MOCK_PLAYER_ID,
    legal_actions: {
      actions: ['fold', 'check', 'raise'],
      call_amount: 0,
      min_raise_to: 100,
      max_raise_to: 4900,
      step: 25
    },
    rake: 8,
    dealer_player_id: 'rafa_curitiba',
    small_blind_player_id: 'leo_rio',
    big_blind_player_id: MOCK_PLAYER_ID
  };
  if (scenario === 'turn') return {
    stage: 'turn',
    board: ['7H', '8C', 'QS', '2D'],
    seats,
    current_player_id: MOCK_PLAYER_ID,
    legal_actions: {
      actions: ['fold', 'check', 'raise'],
      call_amount: 0,
      min_raise_to: 175,
      max_raise_to: 4900,
      step: 25
    },
    rake: 11,
    dealer_player_id: 'rafa_curitiba',
    small_blind_player_id: 'leo_rio',
    big_blind_player_id: MOCK_PLAYER_ID
  };
  if (scenario === 'river') return {
    stage: 'river',
    board: ['7H', '8C', 'QS', '2D', 'AC'],
    seats,
    current_player_id: 'nina_recife',
    legal_actions: {actions: [], call_amount: 0},
    rake: 14,
    dealer_player_id: 'rafa_curitiba',
    small_blind_player_id: 'leo_rio',
    big_blind_player_id: MOCK_PLAYER_ID
  };
  seats[0] = {...seats[0], stack: 6125, contributed: 0};
  return {
    stage: 'showdown',
    board: ['7H', '8C', 'QS', '2D', 'AC'],
    seats: revealShowdownCards(seats),
    payouts: {[MOCK_PLAYER_ID]: 1275},
    winners: [MOCK_PLAYER_ID],
    rake: 20,
    dealer_player_id: 'rafa_curitiba',
    small_blind_player_id: 'leo_rio',
    big_blind_player_id: MOCK_PLAYER_ID
  };
}

type MockHandlers = {
  onMessage: (message: ServerMessage) => void;
  onStatus: (status: MockConnectionStatus, attempt: number) => void
};

type BotProfile = {
  style: 'tight' | 'loose' | 'aggressive' | 'calling_station' | 'volatile';
  aggression: number;
  call: number;
  shove: number;
};

const BOT_PROFILES: Record<string, BotProfile> = {
  'bia_sp': {style: 'tight', aggression: .22, call: .58, shove: .02},
  'leo_rio': {style: 'aggressive', aggression: .7, call: .66, shove: .08},
  'nina_recife': {style: 'tight', aggression: .16, call: .46, shove: .01},
  'joao_floripa': {style: 'calling_station', aggression: .12, call: .9, shove: .02},
  'caio_goiânia': {style: 'volatile', aggression: .58, call: .6, shove: .2},
};

const AUTO_FOLD_TIMEOUT_MS = 6_000;
// Clockwise poker order, independent of the array's visual seat order.
const FULL_HAND_ORDER = ['bia_sp', MOCK_PLAYER_ID, 'leo_rio', 'nina_recife', 'joao_floripa', 'caio_goiânia'];

/** Stateful WebSocket-shaped client used by useTableRealtime in mock mode. */
export class MockTableService {
  private snapshot: TableSnapshot;
  private timers = new Set<ReturnType<typeof setTimeout>>();
  private attempt = 0;
  private status: MockConnectionStatus = 'connecting';
  private streetCommitted: Record<string, number> = {};
  private turnOrder: string[] = [];
  private turnTimer?: ReturnType<typeof setTimeout>;
  private decisionNumber = 0;
  private snapshotVersion = 0;

  constructor(private scenario: MockScenario, private delay: number, private handlers: MockHandlers) {
    this.snapshot = snapshotForScenario(scenario);
    applyActiveMockRebuy = (amount, autoRebuy) => this.applyRebuy(amount, autoRebuy);
    if (INTERACTIVE_SCENARIOS.has(scenario)) this.beginStreet(false);
  }

  connect() {
    this.setStatus('connecting');
    if (this.scenario === 'reconnecting') {
      this.later(() => {
        this.setStatus('connected');
        this.emitState();
        this.later(() => {
          this.setStatus('disconnected');
          this.attempt = 1;
          this.later(() => this.setStatus('reconnecting'), 1);
          this.later(() => {
            this.setStatus('connected');
            this.emitState();
          }, 4);
        }, 3);
      });
      return;
    }
    this.later(() => {
      this.setStatus('connected');
      if (INTERACTIVE_SCENARIOS.has(this.scenario)) this.startInteractiveHand();
      else this.emitState();
    });
  }

  reconnect() {
    this.attempt += 1;
    this.setStatus('reconnecting');
    this.later(() => {
      this.setStatus('connected');
      this.emitState();
    });
  }

  send(value: Record<string, unknown>) {
    // The timeout scenario models a server that accepts the connection but
    // never replies to anything — so every action (and the watchdog ping)
    // hangs until the client-side timeout fires.
    if (this.scenario === 'timeout') return true;
    if (this.status !== 'connected') return false;
    if (value.type === 'ping') {
      this.later(() => this.emitState());
      return true;
    }
    if (value.type === 'sync_state') {
      this.later(() => this.emitState(String(value.action_id || '')));
      return true;
    }
    if (value.type === 'ready' || value.type === 'post_big_blind' || value.type === 'show_cards') {
      if (value.type === 'ready') {
        const ready = Boolean(value.ready);
        this.snapshot = {
          ...this.snapshot,
          seats: this.snapshot.seats.map(seat => seat.player_id === MOCK_PLAYER_ID ? {
            ...seat,
            ready,
            state: !ready && !['complete', 'waiting_for_players'].includes(this.snapshot.stage)
              ? 'folded'
              : ready && seat.state === 'sitting_out' ? 'active' : seat.state
          } : seat)
        };
      }
      if (value.type === 'show_cards') {
        const index = typeof value.card_index === 'number' ? value.card_index : undefined;
        this.snapshot = {
          ...this.snapshot,
          seats: this.snapshot.seats.map(seat => {
            if (seat.player_id !== MOCK_PLAYER_ID) return seat;
            const revealed = [...(seat.hole_cards_revealed || [false, false])];
            if (index === undefined) revealed[0] = revealed[1] = true;
            else if (index === 0 || index === 1) revealed[index] = true;
            return {...seat, hole_cards_revealed: revealed};
          })
        };
      }
      this.later(() => this.handlers.onMessage({
        type: 'action_ack', action_id: String(value.action_id || '')
      }));
      if (value.type === 'ready' || value.type === 'show_cards') this.later(() => this.emitState(), 2);
      return true;
    }
    if (value.type === 'chat') {
      this.later(() => this.handlers.onMessage({
        type: 'chat',
        player_id: MOCK_PLAYER_ID,
        message: String(value.message || '')
      }));
      this.later(() => this.handlers.onMessage({type: 'chat', player_id: 'bia_sp', message: 'Boa! Vamos nessa 👋'}), 2);
      return true;
    }
    if (value.type === 'set_run_it_twice') {
      this.snapshot = {
        ...this.snapshot,
        seats: this.snapshot.seats.map(seat => seat.player_id === MOCK_PLAYER_ID ? {
          ...seat, run_it_twice: Boolean(value.run_it_twice)
        } : seat)
      };
      this.later(() => this.emitState());
      return true;
    }
    if (value.type === 'request_rabbit_hunt') {
      this.later(() => this.handlers.onMessage({
        type: 'action_ack', action_id: String(value.action_id || '')
      }));
      return true;
    }
    if (value.type === 'request_winner_cards') {
      const winnerID = this.snapshot.winners?.[0];
      const winnerCards = LOSS_REVEAL[winnerID || ''];
      if (this.scenario !== 'winner_cards' || !winnerID || !winnerCards) {
        this.later(() => this.handlers.onMessage({
          type: 'error', code: 'invalid_action', action_id: String(value.action_id || '')
        }));
        return true;
      }
      // Paying opens a request the winner has to answer; nothing is revealed
      // yet. The mock winner auto-accepts after a beat so the flow stays
      // playable without a second browser.
      this.snapshot = {
        ...this.snapshot,
        seats: this.snapshot.seats.map(seat => seat.player_id === MOCK_PLAYER_ID
          ? {...seat, stack: Math.max(0, seat.stack - 50)} : seat),
        pending_winner_cards: {
          requester_id: MOCK_PLAYER_ID, requester_name: 'Você',
          winner_id: winnerID, fee: 50, expires_at_unix_ms: Date.now() + 8000,
        },
      };
      this.later(() => this.handlers.onMessage({
        type: 'action_ack', action_id: String(value.action_id || '')
      }));
      this.later(() => this.emitState(), 2);
      this.later(() => {
        // Only if the request is still outstanding — a real winner answering
        // first must win over the mock's auto-accept.
        if (this.snapshot.pending_winner_cards?.winner_id !== winnerID) return;
        this.snapshot = {
          ...this.snapshot,
          pending_winner_cards: undefined,
          seats: this.snapshot.seats.map(seat => seat.player_id === winnerID ? {
            ...seat,
            stack: seat.stack + 25,
            hole_cards: winnerCards,
            hole_cards_revealed: [true, true],
            hand_category: 'pair'
          } : seat),
          rake: (this.snapshot.rake || 0) + 25
        };
        this.emitState();
      }, 8);
      return true;
    }
    if (value.type === 'accept_winner_cards' || value.type === 'decline_winner_cards') {
      const request = this.snapshot.pending_winner_cards;
      if (!request) {
        this.later(() => this.handlers.onMessage({
          type: 'error', code: 'invalid_action', action_id: String(value.action_id || '')
        }));
        return true;
      }
      const accepted = value.type === 'accept_winner_cards';
      const winnerCards = LOSS_REVEAL[request.winner_id] || ['back', 'back'];
      this.snapshot = {
        ...this.snapshot,
        pending_winner_cards: undefined,
        seats: this.snapshot.seats.map(seat => {
          if (accepted && seat.player_id === request.winner_id) {
            return {...seat, stack: seat.stack + request.fee / 2, hole_cards: winnerCards,
              hole_cards_revealed: [true, true], hand_category: 'pair'};
          }
          if (!accepted && seat.player_id === request.requester_id) {
            return {...seat, stack: seat.stack + request.fee};
          }
          return seat;
        }),
        rake: (this.snapshot.rake || 0) + (accepted ? request.fee - request.fee / 2 : 0),
      };
      this.later(() => this.handlers.onMessage({
        type: 'action_ack', action_id: String(value.action_id || '')
      }));
      this.later(() => this.emitState(), 2);
      return true;
    }
    if (value.type === 'reaction') {
      this.later(() => this.handlers.onMessage({
        type: 'reaction',
        player_id: MOCK_PLAYER_ID,
        reaction_id: String(value.reaction_id || ''),
        target_player_id: String(value.target_player_id || '')
      }));
      return true;
    }
    if (value.type === 'preselect_action') {
      const selection = String(value.action || '') as ActionPreselection | '';
      const amount = Number(value.amount || 0);
      const prospective = this.legalActionsFor(this.snapshot.seats, MOCK_PLAYER_ID).call_amount || 0;
      if (selection === 'call' && (amount <= 0 || amount !== prospective)) {
        this.later(() => this.handlers.onMessage({
          type: 'error', code: 'invalid_action', action_id: String(value.action_id || '')
        }));
        return true;
      }
      this.snapshot = {
        ...this.snapshot,
        action_preselection: selection || undefined,
        action_preselection_amount: selection === 'call' ? amount : 0
      };
      this.later(() => this.handlers.onMessage({type: 'action_ack', action_id: String(value.action_id || '')}));
      this.later(() => this.emitState(), 2);
      return true;
    }
    if (value.type !== 'act') return true;
    if (this.scenario === 'action_error') {
      this.later(() => this.handlers.onMessage({
        type: 'error',
        code: 'invalid_action',
        action_id: String(value.action_id || '')
      }));
      return true;
    }
    if (INTERACTIVE_SCENARIOS.has(this.scenario)) {
      if (this.snapshot.current_player_id !== MOCK_PLAYER_ID) {
        this.later(() => this.handlers.onMessage({
          type: 'error',
          code: 'invalid_action',
          action_id: String(value.action_id || '')
        }));
        return true;
      }
      const action = value.action as PokerAction;
      const legal = this.legalActionsFor(this.snapshot.seats, MOCK_PLAYER_ID);
      if (!legal.actions.includes(action)) {
        this.later(() => this.handlers.onMessage({
          type: 'error',
          code: 'invalid_action',
          action_id: String(value.action_id || '')
        }));
        return true;
      }
      this.later(() => this.handlers.onMessage({
        type: 'action_ack', action_id: String(value.action_id || '')
      }));
      this.resolveAction(MOCK_PLAYER_ID, action, Number(value.amount || 0));
      return true;
    }
    this.later(() => {
      this.handlers.onMessage({type: 'action_ack', action_id: String(value.action_id || '')});
      const seats = this.snapshot.seats.map(seat => ({...seat}));
      const viewer = seats.find(seat => seat.player_id === MOCK_PLAYER_ID);
      if (viewer && actionFolds(value.action as PokerAction)) viewer.state = 'folded';
      if (viewer && (value.action === 'call' || value.action === 'raise')) {
        const target = value.action === 'raise' ? Number(value.amount || viewer.contributed) : Math.max(...seats.map(seat => seat.contributed));
        const added = Math.max(0, target - viewer.contributed);
        viewer.stack -= added;
        viewer.contributed = target;
      }
      this.snapshot = {...this.snapshot, seats, current_player_id: 'bia_sp', legal_actions: {actions: []}};
      this.emitState();
      if (value.action === 'raise') this.later(() => this.handlers.onMessage({
        type: 'achievement_unlocked',
        key: 'primeiro_aumento',
        stars: 2
      }), 2);
    });
    return true;
  }

  close() {
    this.timers.forEach(clearTimeout);
    this.timers.clear();
    this.turnTimer = undefined;
    mockPlayerDealtIn = false;
    applyActiveMockRebuy = null;
  }

  private later(task: () => void, factor = 1) {
    this.laterMs(task, this.delay * factor);
  }

  private laterMs(task: () => void, milliseconds: number) {
    const timer = setTimeout(() => {
      this.timers.delete(timer);
      task();
    }, milliseconds);
    this.timers.add(timer);
    return timer;
  }

  // --- Interactive mini-table engine ---------------------------------------

  private setStatus(status: MockConnectionStatus) {
    this.status = status;
    this.handlers.onStatus(status, this.attempt);
  }

  private beginStreet(clear: boolean) {
    this.streetCommitted = {};
    for (const seat of this.snapshot.seats) {
      if (seat.state === 'active' || seat.state === 'all_in') {
        this.streetCommitted[seat.player_id] = clear ? 0 : (seat.contributed || 0);
      }
    }
  }

  private streetBet(seats: SeatView[]) {
    return Math.max(0, ...seats.map(s => this.streetCommitted[s.player_id] || 0));
  }

  private legalActionsFor(seats: SeatView[], playerId: string): LegalActionState {
    const seat = seats.find(s => s.player_id === playerId);
    if (!seat || seat.state !== 'active') return {actions: []};
    const committed = this.streetCommitted[playerId] || 0;
    const currentBet = this.streetBet(seats);
    const callAmount = Math.min(seat.stack, Math.max(0, currentBet - committed));
    const maxTo = seat.stack + committed;
    const minTo = currentBet + 25;
    const actions: PokerAction[] = callAmount > 0 ? ['fold', 'call'] : ['fold', 'check'];
    if (maxTo > currentBet && seat.stack > callAmount) actions.push('raise');
    return {
      actions,
      call_amount: callAmount,
      min_raise_to: Math.min(maxTo, minTo),
      max_raise_to: Math.max(0, maxTo),
      step: 25
    };
  }

  private startInteractiveHand() {
    if (this.scenario === 'auto_fold') {
      this.turnOrder = [MOCK_PLAYER_ID];
    } else {
      this.turnOrder = this.orderAfter(this.snapshot.big_blind_player_id || MOCK_PLAYER_ID)
        .filter(id => this.canAct(id));
    }
    this.activateNextTurn();
  }

  private orderAfter(playerId: string) {
    const seated = new Set(this.snapshot.seats.map(seat => seat.player_id));
    const ids = FULL_HAND_ORDER.filter(id => seated.has(id));
    const start = ids.indexOf(playerId);
    return [...ids.slice(start + 1), ...ids.slice(0, start + 1)];
  }

  private canAct(playerId: string) {
    return this.snapshot.seats.find(seat => seat.player_id === playerId)?.state === 'active';
  }

  private contenders(seats = this.snapshot.seats) {
    return seats.filter(seat => seat.state === 'active' || seat.state === 'all_in');
  }

  private cancelTurnTimer() {
    if (!this.turnTimer) return;
    clearTimeout(this.turnTimer);
    this.timers.delete(this.turnTimer);
    this.turnTimer = undefined;
  }

  private activateNextTurn() {
    this.cancelTurnTimer();
    this.turnOrder = this.turnOrder.filter(id => this.canAct(id));
    if (this.contenders().length <= 1) {
      this.finishWithoutShowdown();
      return;
    }
    if (this.turnOrder.length === 0) {
      this.advanceStreet();
      return;
    }
    const playerId = this.turnOrder[0];
    const timeout = this.scenario === 'auto_fold' ? AUTO_FOLD_TIMEOUT_MS : DEFAULT_TURN_TIMEOUT_MS;
    const baseDeadline = Date.now() + timeout;
    const actingSeat = this.snapshot.seats.find(seat => seat.player_id === playerId);
    const timeBank = actingSeat?.time_bank_ms ?? 10_000;
    this.snapshot = {
      ...this.snapshot,
      current_player_id: playerId,
      legal_actions: playerId === MOCK_PLAYER_ID ? this.legalActionsFor(this.snapshot.seats, playerId) : {actions: []},
      prospective_call_amount: this.legalActionsFor(this.snapshot.seats, MOCK_PLAYER_ID).call_amount || 0,
      action_base_deadline_unix_ms: baseDeadline,
      action_deadline_unix_ms: baseDeadline + timeBank
    };
    this.emitState();
    this.turnTimer = this.laterMs(() => {
      this.turnTimer = undefined;
      if (this.snapshot.current_player_id === playerId) this.resolveAction(playerId, 'fold', 0);
    }, timeout + timeBank);
    if (playerId !== MOCK_PLAYER_ID) {
      const thinkTime = Math.max(300, Math.min(1600, this.delay * (1.2 + this.seededRandom(playerId))));
      this.laterMs(() => {
        if (this.snapshot.current_player_id !== playerId) return;
        const decision = this.botDecision(playerId);
        this.resolveAction(playerId, decision.action, decision.amount);
      }, thinkTime);
    }
  }

  private commit(seat: SeatView, streetTotal: number) {
    const committed = this.streetCommitted[seat.player_id] || 0;
    const add = Math.min(seat.stack, Math.max(0, streetTotal - committed));
    seat.stack -= add;
    seat.contributed += add;
    this.streetCommitted[seat.player_id] = committed + add;
    if (seat.stack === 0) seat.state = 'all_in';
  }

  private resolveAction(playerId: string, action: PokerAction, amount: number) {
    if (this.snapshot.current_player_id !== playerId) return;
    this.cancelTurnTimer();
    const seats = this.snapshot.seats.map(seat => ({
      ...seat,
      hole_cards: seat.hole_cards ? [...seat.hole_cards] : undefined
    }));
    const seat = seats.find(item => item.player_id === playerId);
    if (!seat || seat.state !== 'active') return;
    const bankUsed = this.snapshot.action_base_deadline_unix_ms ?
      Math.max(0, Date.now() - this.snapshot.action_base_deadline_unix_ms) : 0;
    seat.time_bank_ms = Math.max(0, (seat.time_bank_ms ?? 10_000) - bankUsed);
    const currentBet = this.streetBet(seats);
    const committed = this.streetCommitted[playerId] || 0;
    let raised = false;
    if (action === 'fold') {
      seat.state = 'folded';
    } else if (action === 'raise') {
      const maximum = committed + seat.stack;
      const target = Math.min(maximum, Math.max(currentBet + 25, amount || currentBet + 25));
      this.commit(seat, target);
      raised = this.streetCommitted[playerId] > currentBet;
    } else {
      this.commit(seat, committed + Math.max(0, currentBet - committed));
    }

    this.snapshot = {
      ...this.snapshot,
      seats,
      current_player_id: undefined,
      legal_actions: {actions: []},
      action_deadline_unix_ms: undefined,
      action_base_deadline_unix_ms: undefined
    };
    this.emitState();
    if (action === 'raise' && playerId === MOCK_PLAYER_ID) this.later(() => this.handlers.onMessage({
      type: 'achievement_unlocked',
      key: 'primeiro_aumento',
      stars: 2
    }), 2);

    if (this.contenders(seats).length <= 1) {
      this.later(() => this.finishWithoutShowdown(), 1);
      return;
    }
    this.turnOrder.shift();
    if (raised) {
      this.turnOrder = this.orderAfter(playerId)
        .filter(id => id !== playerId && this.canActIn(seats, id));
    }
    this.later(() => this.activateNextTurn(), 1);
  }

  private canActIn(seats: SeatView[], playerId: string) {
    return seats.find(seat => seat.player_id === playerId)?.state === 'active';
  }

  private advanceStreet() {
    const stage = this.snapshot.stage;
    const next = stage === 'pre_flop' ? 'flop' : stage === 'flop' ? 'turn' : stage === 'turn' ? 'river' : 'showdown';
    if (next === 'showdown') {
      this.reachShowdown();
      return;
    }
    const variant = HAND_VARIANTS[this.scenario as InteractiveScenario];
    const board = next === 'flop' ? variant.board.slice(0, 3) :
      next === 'turn' ? variant.board.slice(0, 4) : variant.board;
    this.beginStreet(true);
    const rake = next === 'flop' ? 8 : next === 'turn' ? 11 : 14;
    this.snapshot = {
      ...this.snapshot,
      stage: next,
      board,
      current_player_id: undefined,
      legal_actions: {actions: []},
      action_deadline_unix_ms: undefined,
      action_base_deadline_unix_ms: undefined,
      rake
    };
    this.emitState();
    const active = this.snapshot.seats.filter(seat => seat.state === 'active');
    if (active.length < 2) {
      this.later(() => this.advanceStreet(), 2);
      return;
    }
    this.turnOrder = this.orderAfter(this.snapshot.dealer_player_id || '')
      .filter(id => this.canAct(id));
    this.later(() => this.activateNextTurn(), 2);
  }

  private reachShowdown() {
    this.cancelTurnTimer();
    const variant = HAND_VARIANTS[this.scenario as InteractiveScenario];
    const revealed = this.snapshot.seats.map(seat => {
      const holeCards = variant.reveal[seat.player_id];
      if (!holeCards || (seat.state !== 'active' && seat.state !== 'all_in')) return seat;
      return {
        ...seat,
        hole_cards: holeCards,
        hand_category: variant.category[seat.player_id]
      };
    });
    const {payouts, winnerIds, potResults} = this.distributePots(revealed, variant.rank);
    this.snapshot = {
      ...this.snapshot,
      stage: 'showdown',
      board: variant.board,
      seats: revealed,
      current_player_id: undefined,
      legal_actions: {actions: []},
      action_deadline_unix_ms: undefined,
      action_base_deadline_unix_ms: undefined,
      payouts,
      winners: winnerIds,
      pot_results: potResults,
      rake: 20
    };
    this.emitState();
    this.later(() => {
      this.snapshot = {
        ...this.snapshot,
        stage: 'complete',
        next_hand_unix_ms: Date.now() + NEXT_HAND_DELAY_MS
      };
      this.emitState();
    }, 6);
  }

  /** Resolve main/side pots one contribution layer at a time. Folded chips
   * stay in every layer they funded, while only active/all-in seats are
   * eligible to win it. A lone unmatched top layer is a refund (payout but
   * not a winner), matching the production contract's winners/payouts split. */
  private distributePots(seats: SeatView[], ranks: Record<string, number>) {
    const levels = [...new Set(seats.map(seat => seat.contributed).filter(amount => amount > 0))]
      .sort((a, b) => a - b);
    const payouts: Record<string, number> = {};
    const winnerIds = new Set<string>();
    const potResults: NonNullable<TableSnapshot['pot_results']> = [];
    let previous = 0;
    for (const level of levels) {
      const contributors = seats.filter(seat => seat.contributed >= level);
      const layer = (level - previous) * contributors.length;
      previous = level;
      if (layer <= 0) continue;
      if (contributors.length === 1) {
        const [recipient] = contributors;
        recipient.stack += layer;
        payouts[recipient.player_id] = (payouts[recipient.player_id] || 0) + layer;
        potResults.push({
          amount: layer, payout_amount: layer,
          eligible_player_ids: [recipient.player_id], winner_player_ids: [],
          payouts: {[recipient.player_id]: layer}, refund: true
        });
        continue;
      }
      const eligible = contributors.filter(seat => seat.state === 'active' || seat.state === 'all_in');
      if (eligible.length === 0) continue;
      const bestRank = Math.max(...eligible.map(seat => ranks[seat.player_id] || 0));
      const layerWinners = eligible.filter(seat => (ranks[seat.player_id] || 0) === bestRank);
      const share = Math.floor(layer / layerWinners.length);
      let remainder = layer - share * layerWinners.length;
      const layerPayouts: Record<string, number> = {};
      for (const winner of layerWinners) {
        const amount = share + (remainder-- > 0 ? 1 : 0);
        winner.stack += amount;
        payouts[winner.player_id] = (payouts[winner.player_id] || 0) + amount;
        layerPayouts[winner.player_id] = amount;
        if (contributors.length > 1) winnerIds.add(winner.player_id);
      }
      potResults.push({
        amount: layer,
        payout_amount: layer,
        eligible_player_ids: eligible.map(seat => seat.player_id),
        winner_player_ids: layerWinners.map(seat => seat.player_id),
        payouts: layerPayouts
      });
    }
    return {payouts, winnerIds: [...winnerIds], potResults};
  }

  private finishWithoutShowdown() {
    this.cancelTurnTimer();
    const seats = this.snapshot.seats.map(seat => ({...seat}));
    const winner = this.contenders(seats)[0];
    if (!winner) return;
    const pot = seats.reduce((total, seat) => total + seat.contributed, 0);
    const payouts: Record<string, number> = {};
    winner.stack += pot;
    payouts[winner.player_id] = pot;
    this.snapshot = {
      ...this.snapshot,
      seats,
      current_player_id: undefined,
      legal_actions: {actions: []},
      payouts,
      winners: [winner.player_id],
      pot_results: [{
        amount: pot, payout_amount: pot,
        eligible_player_ids: [winner.player_id], winner_player_ids: [winner.player_id],
        payouts: {[winner.player_id]: pot}
      }],
      stage: 'complete',
      action_deadline_unix_ms: undefined,
      next_hand_unix_ms: Date.now() + NEXT_HAND_DELAY_MS,
      won_without_showdown: true
    };
    this.emitState();
  }

  private botDecision(playerId: string): { action: PokerAction; amount: number } {
    const legal = this.legalActionsFor(this.snapshot.seats, playerId);
    const seat = this.snapshot.seats.find(item => item.player_id === playerId)!;
    const profile = BOT_PROFILES[playerId] || BOT_PROFILES.bia_sp;
    const callAmount = legal.call_amount || 0;
    const currentBet = this.streetBet(this.snapshot.seats);
    const committed = this.streetCommitted[playerId] || 0;
    const decisionIndex = this.decisionNumber++;
    const random = this.seededRandom(`${playerId}:${profile.style}`);

    // The first orbit deliberately covers poker's basic vocabulary; later
    // decisions come from the profiles below. This keeps every QA run useful
    // without turning the whole simulation into a rigid action script.
    if (this.snapshot.stage === 'pre_flop' && decisionIndex < 5) {
      const opening: Record<string, PokerAction> = {
        'leo_rio': 'fold',
        'nina_recife': 'call',
        'joao_floripa': 'raise',
        'caio_goiânia': this.scenario === 'all_in' ? 'raise' : 'call',
        'bia_sp': 'call'
      };
      const preferred = opening[playerId];
      if (preferred && legal.actions.includes(preferred)) {
        if (preferred !== 'raise') return {action: preferred, amount: 0};
        const shove = this.scenario === 'all_in' && playerId === 'caio_goiânia';
        return {action: 'raise', amount: shove ? legal.max_raise_to || 0 : Math.min(150, legal.max_raise_to || 150)};
      }
    }

    // The loss/tie scenarios guarantee that one rival reaches showdown with
    // Ana; otherwise a perfectly valid random fold could turn the named
    // scenario into an uncontested win and make the QA outcome non-repeatable.
    if (playerId === 'bia_sp' && (this.scenario === 'full_hand_loss' || this.scenario === 'full_hand_tie')) {
      return callAmount > 0 ? {action: 'call', amount: 0} : {action: 'check', amount: 0};
    }

    if (legal.actions.includes('raise') && (random < profile.shove || random < profile.aggression * .55)) {
      const maximum = legal.max_raise_to || committed + seat.stack;
      const minimum = legal.min_raise_to || currentBet + 25;
      const shove = random < profile.shove;
      const target = shove ? maximum : Math.min(maximum, Math.max(minimum, currentBet + Math.max(25, Math.round((seat.stack * .08) / 25) * 25)));
      return {action: 'raise', amount: target};
    }
    if (callAmount > 0) {
      const pressure = callAmount / Math.max(1, seat.stack + callAmount);
      if (random > profile.call - pressure * .65) return {action: 'fold', amount: 0};
      return {action: 'call', amount: 0};
    }
    return {action: 'check', amount: 0};
  }

  private seededRandom(salt: string) {
    const source = `${this.scenario}:${this.snapshot.stage}:${this.decisionNumber}:${salt}`;
    let hash = 2166136261;
    for (let i = 0; i < source.length; i++) {
      hash ^= source.charCodeAt(i);
      hash = Math.imul(hash, 16777619);
    }
    return (hash >>> 0) / 4294967296;
  }

  private emitState(actionId?: string) {
    const stage = this.snapshot.stage;
    const handInProgress = stage !== 'waiting_for_players' && stage !== 'complete';
    const viewer = this.snapshot.seats.find(s => s.player_id === MOCK_PLAYER_ID);
    mockPlayerDealtIn = handInProgress && (viewer?.state === 'active' || viewer?.state === 'all_in');
    const prospectiveCallAmount = handInProgress ?
      this.legalActionsFor(this.snapshot.seats, MOCK_PLAYER_ID).call_amount || 0 : 0;
    const fixedCallInvalid = this.snapshot.action_preselection === 'call' &&
      this.snapshot.action_preselection_amount !== prospectiveCallAmount;
    this.snapshotVersion += 1;
    this.snapshot = {
      ...this.snapshot,
      snapshot_version: this.snapshotVersion,
      protocol_version: 8,
      prospective_call_amount: prospectiveCallAmount,
      ...(fixedCallInvalid ? {action_preselection: undefined, action_preselection_amount: 0} : {}),
      seats: this.snapshot.seats.map(seat => ({
        ...seat,
        time_bank_ms: seat.time_bank_ms ?? 10_000
      })),
      hand_id: this.snapshot.hand_id || (stage === 'waiting_for_players' ? undefined : `mock-${this.scenario}-hand`)
    };
    this.handlers.onMessage({type: 'state', snapshot: this.snapshot, action_id: actionId});
  }

  applyRebuy(amount: number, autoRebuy: boolean) {
    if (this.scenario !== 'rebuy' || !Number.isFinite(amount) || amount <= 0) return;
    this.snapshot = {
      ...this.snapshot,
      seats: this.snapshot.seats.map(seat => seat.player_id === MOCK_PLAYER_ID ? {
        ...seat, stack: amount, state: 'active', ready: true, auto_rebuy: autoRebuy
      } : seat)
    };
    this.emitState();
  }
}

function actionFolds(action: PokerAction) {
  return action === 'fold';
}
