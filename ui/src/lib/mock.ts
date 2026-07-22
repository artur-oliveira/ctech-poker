// Development-only in-process API and realtime simulation. The environment
// flag selects the adapter; no mock HTTP or WebSocket server is started.
import {AxiosError, type AxiosResponse, type InternalAxiosRequestConfig} from 'axios';
import type {LegalActionState, PokerAction, SeatView, ServerMessage, TableSnapshot} from '@/lib/api/table';

export const USE_MOCK = process.env.NEXT_PUBLIC_MOCK_API === 'true';
export const MOCK_PLAYER_ID = 'mock_player_ana';

const ROOM_ID = '11111111111111111111111111111111';
const rooms = [
  {
    room_id: ROOM_ID,
    visibility: 'public',
    currency_mode: 'sandbox',
    small_blind: 25,
    big_blind: 50,
    max_seats: 8,
    buy_in_min: 1000,
    buy_in_max: 10000,
    status: 'playing'
  },
  {
    room_id: '22222222222222222222222222222222',
    visibility: 'public',
    currency_mode: 'real',
    small_blind: 50,
    big_blind: 100,
    max_seats: 6,
    buy_in_min: 2000,
    buy_in_max: 20000,
    status: 'waiting'
  },
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
    throw new AxiosError('Mock request failed', String(rule.status), config, undefined, {
      data: rule.body || {detail: 'Erro simulado'}, status: rule.status, statusText: 'Mock Error', headers: {}, config
    });
  }
  const body = typeof config.data === 'string' ? JSON.parse(config.data || '{}') : (config.data || {});
  if (method === 'GET' && path === '/v1.0/players/me') return ok({
    user_id: MOCK_PLAYER_ID,
    poker_terms_accepted: true
  }, config);
  if (method === 'POST' && path === '/v1.0/players/me/terms/accept') return ok({
    user_id: MOCK_PLAYER_ID,
    poker_terms_accepted: true
  }, config);
  if (method === 'GET' && path === '/v1.0/rooms') return ok(rooms, config);
  const roomMatch = method === 'GET' ? path.match(/^\/v1\.0\/rooms\/([^/]+)$/) : null;
  if (roomMatch) return ok(rooms.find(r => r.room_id === roomMatch[1]) || rooms[0], config);
  if (method === 'GET' && path === '/v1.0/rooms/stakes') return ok({
    stakes: [{
      small_blind: 10,
      big_blind: 20
    }, {small_blind: 25, big_blind: 50}, {small_blind: 50, big_blind: 100}]
  }, config);
  if (method === 'POST' && path === '/v1.0/rooms') {
    const room = {
      ...body,
      room_id: crypto.randomUUID().replaceAll('-', ''),
      currency_mode: 'sandbox',
      status: 'waiting'
    };
    rooms.unshift(room);
    return ok(room, config);
  }
  if (method === 'POST' && /^\/v1\.0\/rooms\/[^/]+\/join$/.test(path)) return ok({}, config);
  if (method === 'GET' && path === '/v1.0/leaderboard') return ok([
    {player_id: 'bia_sp', hands_played: 248, hands_won: 71, win_rate: .286},
    {player_id: MOCK_PLAYER_ID, hands_played: 184, hands_won: 49, win_rate: .266},
    {player_id: 'leo_rio', hands_played: 213, hands_won: 52, win_rate: .244},
  ], config);
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
  | 'reconnecting'
  | 'action_error'
  | 'timeout';
export type MockConnectionStatus = 'connecting' | 'connected' | 'reconnecting' | 'disconnected' | 'error';

const baseSeats = () => [
  {player_id: MOCK_PLAYER_ID, stack: 4850, state: 'active', contributed: 50, hole_cards: ['AH', 'KD'], equity: .64},
  {player_id: 'bia_sp', stack: 3925, state: 'active', contributed: 75, hole_cards: ['back', 'back']},
  {player_id: 'leo_rio', stack: 6100, state: 'folded', contributed: 25, hole_cards: ['back', 'back']},
  {player_id: 'nina_recife', stack: 2775, state: 'active', contributed: 75, hole_cards: ['back', 'back']},
  {player_id: 'gui_bh', stack: 5000, state: 'sitting_out', contributed: 0},
  {player_id: 'joao_floripa', stack: 4375, state: 'active', contributed: 75, hole_cards: ['back', 'back']},
  {player_id: 'mari_belém', stack: 8200, state: 'disconnected', contributed: 0},
  {player_id: 'caio_goiânia', stack: 3400, state: 'all_in', contributed: 625, hole_cards: ['back', 'back']},
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
    {player_id: MOCK_PLAYER_ID, stack: 4850, state: 'active', contributed: 50, hole_cards: ['AH', 'KD'], equity: .64},
    {player_id: 'bia_sp', stack: 3925, state: 'active', contributed: 25, hole_cards: ['back', 'back']},
    {player_id: 'leo_rio', stack: 6100, state: 'active', contributed: 0, hole_cards: ['back', 'back']},
    {player_id: 'nina_recife', stack: 2775, state: 'active', contributed: 0, hole_cards: ['back', 'back']},
    {player_id: 'joao_floripa', stack: 4375, state: 'active', contributed: 0, hole_cards: ['back', 'back']},
    {player_id: 'caio_goiânia', stack: 3400, state: 'active', contributed: 0, hole_cards: ['back', 'back']},
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
      rake: 5
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
      rake: 5
    };
  }
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
    rake: 8
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
    rake: 11
  };
  if (scenario === 'river') return {
    stage: 'river',
    board: ['7H', '8C', 'QS', '2D', 'AC'],
    seats,
    current_player_id: 'nina_recife',
    legal_actions: {actions: [], call_amount: 0},
    rake: 14
  };
  seats[0] = {...seats[0], stack: 6125, contributed: 0};
  return {
    stage: 'showdown',
    board: ['7H', '8C', 'QS', '2D', 'AC'],
    seats: revealShowdownCards(seats),
    payouts: {[MOCK_PLAYER_ID]: 1275},
    rake: 20
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
  
  private later(task: () => void, factor = 1) {
    const timer = setTimeout(() => {
      this.timers.delete(timer);
      task();
    }, this.delay * factor);
    this.timers.add(timer);
  }
  
  private setStatus(status: MockConnectionStatus) {
    this.status = status;
    this.handlers.onStatus(status, this.attempt);
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
  
  // --- Full-hand engine -----------------------------------------------------
  
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
      current_player_id: undefined, legal_actions: {actions: []}, payouts, rake: 20
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
      stage: 'complete'
    };
    this.emitState();
  }
  
  private emitState() {
    this.handlers.onMessage({type: 'state', snapshot: this.snapshot});
  }
  
  close() {
    this.timers.forEach(clearTimeout);
    this.timers.clear();
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
