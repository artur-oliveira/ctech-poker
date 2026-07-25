// Development-only in-process API and realtime simulation. The environment
// flag selects the adapter; no mock HTTP or WebSocket server is started.
import {AxiosError, type AxiosResponse, type InternalAxiosRequestConfig} from 'axios';
import type {Achievement, PlayerAchievementProgress, Tier} from '@/lib/api/achievements';
import type {HandItem} from '@/lib/api/player';
import type {Room} from '@/lib/api/rooms';
import type {Page} from '@/lib/api/client';
import type {
  HandHistoryAction,
  LegalActionState,
  PokerAction,
  SeatView,
  ServerMessage,
  TableSnapshot
} from '@/lib/api/table';

export const USE_MOCK = process.env.NEXT_PUBLIC_MOCK_API === 'true';
export const MOCK_PLAYER_ID = 'mock_player_ana';

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
    seats_taken: 6
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
    ended_at: Math.floor(Date.now() / 1000) - 60 * 12,
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
    ended_at: Math.floor(Date.now() / 1000) - 60 * 40,
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
    ended_at: Math.floor(Date.now() / 1000) - 60 * 90,
    board: ['9c', '2d', '9h', '8h', '9s'],
    hole_cards: ['Kc', '7d'],
    opponents: [{player_id: 'leo_rio', name: 'Leo', hole_cards: ['6s', '8c'], won: true}],
    server_seed: 'dc3e9e4d25dd4c612648a0eada3539b4912da6fbd0a79341df221e8f51657b96',
    commit_hash: '063062170463cd2c086f4603c9635f6fb67c7baa1ba0b8597d01966650c2c685'
  }
];

// Each action lands a few seconds after the previous one, ending at the
// hand's own ended_at — mirrors how CommitAction stamps Timestamp in prod.
function timedActions(endedAtSeconds: number, actions: Omit<HandHistoryAction, 'timestamp'>[]): HandHistoryAction[] {
  const startMs = endedAtSeconds * 1000 - actions.length * 4000;
  return actions.map((a, i) => ({...a, timestamp: startMs + i * 4000}));
}

const mockHandActions: Record<string, HandHistoryAction[]> = {
  hand_0003: timedActions(mockHands[0].ended_at, [
    {seq: 1, player_id: 'bia_sp', action: 'raise', amount: 150},
    {seq: 2, player_id: MOCK_PLAYER_ID, action: 'call', amount: 150},
    {seq: 3, player_id: 'bia_sp', action: 'check', amount: 0},
    {seq: 4, player_id: MOCK_PLAYER_ID, action: 'raise', amount: 400},
    {seq: 5, player_id: 'bia_sp', action: 'call', amount: 400},
    {seq: 6, player_id: 'bia_sp', action: 'check', amount: 0},
    {seq: 7, player_id: MOCK_PLAYER_ID, action: 'raise', amount: 1200},
    {seq: 8, player_id: 'bia_sp', action: 'fold', amount: 0}
  ]),
  hand_0002: timedActions(mockHands[1].ended_at, [
    {seq: 1, player_id: MOCK_PLAYER_ID, action: 'raise', amount: 100},
    {seq: 2, player_id: 'leo_rio', action: 'call', amount: 100},
    {seq: 3, player_id: MOCK_PLAYER_ID, action: 'check', amount: 0},
    {seq: 4, player_id: 'leo_rio', action: 'raise', amount: 300},
    {seq: 5, player_id: MOCK_PLAYER_ID, action: 'call', amount: 300},
    {seq: 6, player_id: MOCK_PLAYER_ID, action: 'check', amount: 0},
    {seq: 7, player_id: 'leo_rio', action: 'raise', amount: 900},
    {seq: 8, player_id: MOCK_PLAYER_ID, action: 'call', amount: 900}
  ]),
  hand_0001: timedActions(mockHands[2].ended_at, [
    {seq: 1, player_id: 'leo_rio', action: 'check', amount: 0},
    {seq: 2, player_id: MOCK_PLAYER_ID, action: 'check', amount: 0},
    {seq: 3, player_id: 'leo_rio', action: 'raise', amount: 200},
    {seq: 4, player_id: MOCK_PLAYER_ID, action: 'call', amount: 200},
    {seq: 5, player_id: 'leo_rio', action: 'check', amount: 0},
    {seq: 6, player_id: MOCK_PLAYER_ID, action: 'check', amount: 0}
  ])
};

const mockProfile = {
  user_id: MOCK_PLAYER_ID,
  name: 'Ana',
  wallet_mode: 'sandbox' as 'sandbox' | 'real',
  poker_terms_accepted: true,
  game_balance: 12500,
  sandbox_balance: 4850
};

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
const creditCooldown = () => Math.max(0, Math.ceil((nextCreditAt - Date.now()) / 1000));

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

// Set by MockTableService as the active hand progresses — lets the REST
// `leave` mock mirror the real engine's rule (hand.go: RemovePlayerForActor)
// that a player still dealt in (active/all-in, hand in progress) can't cash out.
let mockPlayerDealtIn = false;

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
  if (method === 'GET' && path === '/v1.0/players/me/sessions') return ok(page([]), config);
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
    return ok({...mockProfile}, config);
  }
  if (method === 'GET' && path === '/v1.0/rooms') return ok(page(rooms), config);
  // Checked before the generic single-segment room-id match below, since
  // "stakes" would otherwise itself match `/rooms/:id` and never reach here.
  if (method === 'GET' && path === '/v1.0/rooms/stakes') return ok({
    stakes: [{
      small_blind: 10,
      big_blind: 20
    }, {small_blind: 25, big_blind: 50}, {small_blind: 50, big_blind: 100}]
  }, config);
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
    return ok({}, config);
  }
  if (method === 'GET' && /^\/v1\.0\/rooms\/[^/]+\/seated$/.test(path)) {
    return ok({seated: false, stack: 0}, config);
  }
  const leaveMatch = method === 'POST' ? path.match(/^\/v1\.0\/rooms\/([^/]+)\/leave$/) : null;
  if (leaveMatch) {
    if (!rooms.find(r => r.room_id === leaveMatch[1])) fail(404, 'room not found', config);
    if (mockPlayerDealtIn) fail(409, 'cannot remove player mid-hand while still dealt in', config);
    return ok({amount: 4850}, config);
  }
  if (method === 'GET' && path === '/v1.0/players/me/hands') return ok(page(mockHands), config);
  const handMatch = method === 'GET' ? path.match(/^\/v1\.0\/players\/me\/hands\/([^/]+)$/) : null;
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
  if (method === 'GET' && path === '/v1.0/leaderboard') return ok(page([
    {player_id: 'bia_sp', player_name: 'Bia', hands_played: 248, hands_won: 71, win_rate: .286},
    {player_id: MOCK_PLAYER_ID, player_name: mockProfile.name, hands_played: 184, hands_won: 49, win_rate: .266},
    {player_id: 'leo_rio', player_name: 'Leo', hands_played: 213, hands_won: 52, win_rate: .244},
  ]), config);
  if (method === 'GET' && path === '/v1.0/achievements') return ok(achievementCatalog, config);
  if (method === 'GET' && path === '/v1.0/players/me/achievements') return ok(page(mockAchievementProgress), config);
  if (method === 'GET' && path === '/v1.0/sandbox-credits') return ok({remaining_time_seconds: creditCooldown()}, config);
  if (method === 'POST' && path === '/v1.0/sandbox-credits') {
    if (creditCooldown() > 0) return ok({amount: 0, remaining_time_seconds: creditCooldown()}, config);
    nextCreditAt = Date.now() + MOCK_CREDIT_COOLDOWN_S * 1000;
    sessionStorage.setItem(CREDIT_KEY, String(nextCreditAt));
    return ok({amount: 250, remaining_time_seconds: MOCK_CREDIT_COOLDOWN_S}, config);
  }
  return ok({}, config);
}

export type MockScenario =
  'full_hand'
  | 'waiting'
  | 'pre_flop'
  | 'flop'
  | 'turn'
  | 'river'
  | 'showdown'
  | 'side_pot'
  | 'reconnecting'
  | 'action_error'
  | 'timeout'
  | 'complete';
export type MockConnectionStatus = 'connecting' | 'connected' | 'reconnecting' | 'disconnected' | 'error';

const baseSeats = () => [
  {
    player_id: MOCK_PLAYER_ID,
    name: 'Ana',
    stack: 4850,
    state: 'active',
    contributed: 50,
    hole_cards: ['AH', 'KD'],
    equity: .64
  },
  {player_id: 'bia_sp', name: 'Bia', stack: 3925, state: 'active', contributed: 75, hole_cards: ['back', 'back']},
  {player_id: 'leo_rio', name: 'Léo', stack: 6100, state: 'folded', contributed: 25, hole_cards: ['back', 'back']},
  {player_id: 'nina_recife', name: 'Nina', stack: 2775, state: 'active', contributed: 75, hole_cards: ['back', 'back']},
  // Nameless on purpose — exercises the is-pending-name placeholder in dev.
  {player_id: 'gui_bh', stack: 5000, state: 'sitting_out', contributed: 0},
  {
    player_id: 'joao_floripa',
    name: 'João',
    stack: 4375,
    state: 'active',
    contributed: 75,
    hole_cards: ['back', 'back']
  },
  {player_id: 'mari_belém', name: 'Mari', stack: 8200, state: 'disconnected', contributed: 0},
  {
    player_id: 'caio_goiânia',
    name: 'Caio',
    stack: 3400,
    state: 'all_in',
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
      contributed: 50,
      hole_cards: ['AH', 'KD'],
      equity: .64
    },
    {player_id: 'bia_sp', name: 'Bia', stack: 3925, state: 'active', contributed: 25, hole_cards: ['back', 'back']},
    {player_id: 'leo_rio', name: 'Léo', stack: 6100, state: 'active', contributed: 0, hole_cards: ['back', 'back']},
    {
      player_id: 'nina_recife',
      name: 'Nina',
      stack: 2775,
      state: 'active',
      contributed: 0,
      hole_cards: ['back', 'back']
    },
    {
      player_id: 'joao_floripa',
      name: 'João',
      stack: 4375,
      state: 'active',
      contributed: 0,
      hole_cards: ['back', 'back']
    },
    {
      player_id: 'caio_goiânia',
      name: 'Caio',
      stack: 3400,
      state: 'active',
      contributed: 0,
      hole_cards: ['back', 'back']
    },
  ];
}

// Final hole cards revealed at showdown, paired with a hand strength rank so
// the mock can name a winner without a full evaluator. Ranks are consistent
// with the fixed board (7H 8C QS 2D AC): the viewer wins with top pair (aces),
// nobody makes a set from the board, so the result is honest and inspectable.
const FULL_HAND_REVEAL: Record<string, [string, string]> = {
  [MOCK_PLAYER_ID]: ['AH', 'KD'], // pair of aces, king kicker — best hand
  'bia_sp': ['9S', '9D'],         // pair of nines
  'nina_recife': ['6C', '6D'],    // pair of sixes
  'caio_goiânia': ['3C', '3D'],   // pair of threes
  'leo_rio': ['JH', 'TH'],        // jack high
  'joao_floripa': ['5S', '4D'],   // five high
};
const FULL_HAND_RANK: Record<string, number> = {
  [MOCK_PLAYER_ID]: 6, 'bia_sp': 5, 'nina_recife': 4, 'caio_goiânia': 3, 'leo_rio': 2, 'joao_floripa': 1,
};

export function snapshotForScenario(scenario: MockScenario): TableSnapshot {
  const seats = baseSeats();
  if (scenario === 'waiting') return {
    stage: 'waiting_for_players',
    board: [],
    seats: seats.slice(0, 3).map(seat => ({...seat, contributed: 0})),
    rake: 0
  };
  if (scenario === 'full_hand') {
    return {
      stage: 'pre_flop',
      board: [],
      seats: fullHandSeats(),
      current_player_id: MOCK_PLAYER_ID,
      legal_actions: {
        actions: ['fold', 'check', 'raise'],
        call_amount: 0,
        min_raise_to: 75,
        max_raise_to: 4900,
        step: 25
      },
      rake: 5,
      // Dealer sits immediately before the small blind in turn order.
      dealer_player_id: 'caio_goiânia',
      small_blind_player_id: 'bia_sp',
      big_blind_player_id: MOCK_PLAYER_ID
    };
  }
  if (scenario === 'pre_flop' || scenario === 'action_error' || scenario === 'timeout' || scenario === 'reconnecting') {
    return {
      stage: 'pre_flop',
      board: [],
      seats,
      current_player_id: MOCK_PLAYER_ID,
      legal_actions: {
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
  if (scenario === 'complete') return {
    stage: 'complete',
    board: ['7H', '8C', 'QS', '2D', 'AC'],
    seats: revealShowdownCards(seats),
    payouts: {[MOCK_PLAYER_ID]: 250},
    winners: [MOCK_PLAYER_ID],
    rake: 5,
    next_hand_unix_ms: Date.now() + 5000,
    dealer_player_id: 'rafa_curitiba',
    small_blind_player_id: 'leo_rio',
    big_blind_player_id: MOCK_PLAYER_ID
  };
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
        player_id: MOCK_PLAYER_ID, name: 'Ana', stack: 0, state: 'all_in', contributed: 300,
        hole_cards: ['AH', '8D'], hand_category: 'two_pair'
      },
      {
        player_id: 'bia_sp', name: 'Bia', stack: 0, state: 'all_in', contributed: 700,
        hole_cards: ['JC', 'JD'], hand_category: 'pair'
      },
      {
        player_id: 'leo_rio', name: 'Léo', stack: 5400, state: 'active', contributed: 700,
        hole_cards: ['9S', '4D'], hand_category: 'high_card'
      },
      {
        player_id: 'nina_recife',
        name: 'Nina',
        stack: 2700,
        state: 'folded',
        contributed: 75,
        hole_cards: ['back', 'back']
      },
      {
        player_id: 'joao_floripa',
        name: 'João',
        stack: 4375,
        state: 'folded',
        contributed: 0,
        hole_cards: ['back', 'back']
      },
      {
        player_id: 'caio_goiânia',
        name: 'Caio',
        stack: 3350,
        state: 'folded',
        contributed: 50,
        hole_cards: ['back', 'back']
      },
    ],
    payouts: {[MOCK_PLAYER_ID]: 900, 'bia_sp': 800},
    winners: [MOCK_PLAYER_ID, 'bia_sp'],
    rake: 25,
    next_hand_unix_ms: Date.now() + 5000,
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

/** Stateful WebSocket-shaped client used by useTableRealtime in mock mode. */
export class MockTableService {
  private snapshot: TableSnapshot;
  private timers = new Set<ReturnType<typeof setTimeout>>();
  private attempt = 0;
  private status: MockConnectionStatus = 'connecting';
  private streetCommitted: Record<string, number> = {};

  constructor(private scenario: MockScenario, private delay: number, private handlers: MockHandlers) {
    this.snapshot = snapshotForScenario(scenario);
    if (scenario === 'full_hand') this.beginStreet(false);
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
      this.emitState();
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
    if (value.type === 'ping' || value.type === 'ready') {
      this.later(() => this.emitState());
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
    if (value.type !== 'act') return true;
    if (this.scenario === 'action_error') {
      this.later(() => this.handlers.onMessage({
        type: 'error',
        code: 'invalid_action',
        action_id: String(value.action_id || '')
      }));
      return true;
    }
    if (this.scenario === 'full_hand') {
      this.resolveFullHand(value.action as PokerAction, Number(value.amount || 0));
      return true;
    }
    this.later(() => {
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
    mockPlayerDealtIn = false;
  }

  private later(task: () => void, factor = 1) {
    const timer = setTimeout(() => {
      this.timers.delete(timer);
      task();
    }, this.delay * factor);
    this.timers.add(timer);
  }

  // --- Full-hand engine -----------------------------------------------------

  private setStatus(status: MockConnectionStatus) {
    this.status = status;
    this.handlers.onStatus(status, this.attempt);
  }

  /** Seed this street's per-street commitment. When `clear` is false the
   * current contributions (blinds) are carried in; otherwise the street starts
   * fresh at zero. */
  private beginStreet(clear: boolean) {
    this.streetCommitted = {};
    for (const seat of this.snapshot.seats) {
      if (seat.state === 'active') this.streetCommitted[seat.player_id] = clear ? 0 : (seat.contributed || 0);
    }
  }

  private streetBet(seats: SeatView[]) {
    // Includes folded seats: a player's bet stays on the table as the amount
    // the rest of the table must still match, even after they fold.
    return Math.max(0, ...seats.map(s => this.streetCommitted[s.player_id] || 0));
  }

  private legalActionsFor(seats: SeatView[], playerId: string): LegalActionState {
    const seat = seats.find(s => s.player_id === playerId);
    if (!seat || seat.state !== 'active') return {actions: []};
    const committed = this.streetCommitted[playerId] || 0;
    const currentBet = this.streetBet(seats);
    const callAmount = Math.max(0, currentBet - committed);
    const maxTo = seat.stack + committed;
    const minTo = currentBet + 25;
    const actions: PokerAction[] = callAmount > 0 ? ['fold', 'call', 'raise'] : ['fold', 'check', 'raise'];
    return {
      actions,
      call_amount: callAmount,
      min_raise_to: Math.min(maxTo, minTo),
      max_raise_to: Math.max(0, maxTo),
      step: 25
    };
  }

  private resolveFullHand(action: PokerAction, amount: number) {
    const seats = this.snapshot.seats.map(s => ({...s, hole_cards: s.hole_cards ? [...s.hole_cards] : undefined}));
    const viewer = seats.find(s => s.player_id === MOCK_PLAYER_ID);
    if (!viewer) return;
    const commit = (seat: SeatView, total: number) => {
      const add = Math.max(0, total - seat.contributed);
      seat.contributed += add;
      this.streetCommitted[seat.player_id] = (this.streetCommitted[seat.player_id] || 0) + add;
      seat.stack -= add;
      if (seat.stack === 0) seat.state = 'all_in';
    };
    if (actionFolds(action)) {
      viewer.state = 'folded';
    } else if (viewer.state === 'active') {
      const committed = this.streetCommitted[viewer.player_id] || 0;
      const currentBet = this.streetBet(seats);
      if (action === 'raise') {
        const maxTo = viewer.stack + committed;
        const target = Math.min(maxTo, Math.max(currentBet + 25, amount || currentBet + 25));
        commit(viewer, target);
      } else {
        const need = currentBet - committed;
        commit(viewer, committed + Math.min(viewer.stack, Math.max(0, need)));
      }
    }
    // Auto-play the remaining active players (call/check only, no raises) so
    // the betting round resolves and the hand can advance on its own.
    for (const seat of seats) {
      if (seat.player_id === MOCK_PLAYER_ID || seat.state !== 'active') continue;
      const committed = this.streetCommitted[seat.player_id] || 0;
      const need = this.streetBet(seats) - committed;
      if (need > 0 && seat.stack > 0) commit(seat, committed + Math.min(seat.stack, need));
    }
    const activeSeats = seats.filter(s => s.state === 'active');
    const pot = seats.reduce((total, seat) => total + seat.contributed, 0);
    if (viewer.state !== 'active') {
      this.finishHand(seats, activeSeats, pot);
      return;
    }
    const currentBet = this.streetBet(seats);
    const matched = activeSeats.every(s => (this.streetCommitted[s.player_id] || 0) === currentBet || s.state === 'all_in');
    this.snapshot = {
      ...this.snapshot,
      seats,
      current_player_id: MOCK_PLAYER_ID,
      legal_actions: this.legalActionsFor(seats, MOCK_PLAYER_ID)
    };
    this.emitState();
    if (action === 'raise') this.later(() => this.handlers.onMessage({
      type: 'achievement_unlocked',
      key: 'primeiro_aumento',
      stars: 2
    }), 2);
    if (matched) {
      if (this.snapshot.stage === 'river') this.later(() => this.reachShowdown(seats, pot), 1);
      else this.later(() => this.advanceStreet(seats), 1);
    }
  }

  private advanceStreet(seats: SeatView[]) {
    const stage = this.snapshot.stage;
    const next = stage === 'pre_flop' ? 'flop' : stage === 'flop' ? 'turn' : stage === 'turn' ? 'river' : 'showdown';
    const board = next === 'flop' ? ['7H', '8C', 'QS'] : next === 'turn' ? ['7H', '8C', 'QS', '2D'] : ['7H', '8C', 'QS', '2D', 'AC'];
    if (next === 'showdown') {
      this.reachShowdown(seats, seats.reduce((total, seat) => total + seat.contributed, 0));
      return;
    }
    this.beginStreet(true);
    const rake = next === 'flop' ? 8 : next === 'turn' ? 11 : 14;
    this.snapshot = {
      ...this.snapshot, stage: next, board, seats,
      current_player_id: MOCK_PLAYER_ID, legal_actions: this.legalActionsFor(seats, MOCK_PLAYER_ID), rake
    };
    this.emitState();
  }

  private reachShowdown(seats: SeatView[], pot: number) {
    const revealed = seats.map(s => FULL_HAND_REVEAL[s.player_id] ? {
      ...s,
      hole_cards: FULL_HAND_REVEAL[s.player_id]
    } : s);
    const contenders = revealed.filter(s => s.state === 'active');
    const winner = bestHand(contenders);
    const payouts: Record<string, number> = {};
    if (winner) {
      winner.stack += pot;
      payouts[winner.player_id] = pot;
    }
    this.snapshot = {
      ...this.snapshot, stage: 'showdown', board: ['7H', '8C', 'QS', '2D', 'AC'], seats: revealed,
      current_player_id: undefined, legal_actions: {actions: []}, payouts,
      winners: winner ? [winner.player_id] : undefined, rake: 20
    };
    this.emitState();
    this.later(() => {
      this.snapshot = {...this.snapshot, stage: 'complete'};
      this.emitState();
    }, 6);
  }

  private finishHand(seats: SeatView[], activeSeats: SeatView[], pot: number) {
    const winner = bestHand(activeSeats);
    const payouts: Record<string, number> = {};
    if (winner) {
      winner.stack += pot;
      payouts[winner.player_id] = pot;
    }
    this.snapshot = {
      ...this.snapshot,
      seats,
      current_player_id: undefined,
      legal_actions: {actions: []},
      payouts,
      winners: winner ? [winner.player_id] : undefined,
      stage: 'complete'
    };
    this.emitState();
  }

  private emitState() {
    const stage = this.snapshot.stage;
    const handInProgress = stage !== 'waiting_for_players' && stage !== 'complete';
    const viewer = this.snapshot.seats.find(s => s.player_id === MOCK_PLAYER_ID);
    mockPlayerDealtIn = handInProgress && (viewer?.state === 'active' || viewer?.state === 'all_in');
    this.handlers.onMessage({type: 'state', snapshot: this.snapshot});
  }
}

function actionFolds(action: PokerAction) {
  return action === 'fold';
}

/** Pick the strongest shown hand among contenders using the pre-ranked mock hands. */
function bestHand(contenders: SeatView[]): SeatView | undefined {
  let best: SeatView | undefined;
  for (const seat of contenders) {
    if (!best || (FULL_HAND_RANK[seat.player_id] || 0) > (FULL_HAND_RANK[best.player_id] || 0)) best = seat;
  }
  return best;
}
