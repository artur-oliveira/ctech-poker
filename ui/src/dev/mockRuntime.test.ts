import {afterEach, describe, expect, test, vi} from 'vitest';
import {MOCK_PLAYER_ID, mockAdapter, type MockScenario, MockTableService, snapshotForScenario} from './mockRuntime';
import type {InternalAxiosRequestConfig} from 'axios';

const scenarios: MockScenario[] = [
  'full_hand', 'full_hand_loss', 'full_hand_tie', 'all_in', 'auto_fold',
  'waiting', 'pre_flop', 'flop', 'turn', 'river', 'showdown', 'side_pot',
  'run_it_twice', 'winner_cards', 'rabbit_hunt', 'rebuy', 'reality_check',
  'reconnecting', 'action_error', 'timeout', 'complete_loss',
  'complete_tie', 'fold_win', 'complete',
];

describe('mock store REST contract', () => {
  const request = (method: string, url: string, data?: unknown) => mockAdapter({
    method, url, data: data ? JSON.stringify(data) : undefined, headers: {},
  } as InternalAxiosRequestConfig);

  test('creates a pending Pix purchase with a QR image and exposes it in history', async () => {
    localStorage.setItem('ctech_poker_mock_delay', '0');
    const catalog = await request('GET', '/v1.0/wallet/sandbox-purchase/skus');
    expect(catalog.data).toHaveLength(4);

    const created = await request('POST', '/v1.0/wallet/sandbox-purchase/', {
      sku: 'pack_1000', idem_key: 'store-test',
    });
    expect(created.data).toMatchObject({
      sku: 'pack_1000', status: 'pending', total_credits: 1000,
    });
    expect(created.data.pix_copia_e_cola).toContain('MOCK.CTECH.POKER');
    expect(created.data.qr_code_base64).toMatch(/^PHN2Zy/);

    const history = await request('GET', '/v1.0/wallet/sandbox-purchase/');
    expect(history.data).toEqual(expect.arrayContaining([
      expect.objectContaining({purchase_id: created.data.purchase_id, status: 'pending'}),
    ]));
  });

  test('accepts the daily-reward trailing slash used by the store client', async () => {
    sessionStorage.removeItem('mock_next_credit_at');
    const cooldown = await request('GET', '/v1.0/sandbox-credits/');
    expect(cooldown.data).toEqual({remaining_time_seconds: 0});

    const reward = await request('POST', '/v1.0/sandbox-credits/');
    expect(reward.data).toEqual({amount: 250, remaining_time_seconds: 90});
    expect(sessionStorage.getItem('mock_next_credit_at')).not.toBeNull();
  });

  test('creates hand shares through the production singular hand route', async () => {
    localStorage.setItem('ctech_poker_mock_delay', '0');
    const history = await request('GET', '/v1.0/players/me/hands');
    const handId = history.data.data[0].hand_id;
    const share = await request('POST', `/v1.0/players/me/hand/${handId}/share`, {
      kind: 'brag', include_hero_cards: true, expiry_days: 7, mode: 'sandbox',
    });

    expect(share.data).toMatchObject({
      token: 'mock-share-demo', kind: 'brag', hero_cards: expect.any(Array),
    });
  });

  test('named table scenes deep-link into a seated table', async () => {
    window.history.replaceState({}, '', '/table?scenario=winner_cards');
    try {
      const seated = await request('GET', '/v1.0/rooms/01ARZ3NDEKTSV4RRFFQ69G5FAV/seated');
      expect(seated.data).toEqual({seated: true, stack: 4850});
    } finally {
      window.history.replaceState({}, '', '/');
    }
  });
});

describe('mock table state contract', () => {
  test.each(scenarios)('%s always returns a renderable backend snapshot', scenario => {
    const snapshot = snapshotForScenario(scenario);
    
    expect(snapshot.stage).toBeTruthy();
    expect(snapshot.seats.length).toBeGreaterThan(0);
    expect(snapshot.board.length).toBeLessThanOrEqual(5);
    expect(new Set(snapshot.seats.map(seat => seat.player_id)).size).toBe(snapshot.seats.length);
    expect(snapshot.seats.every(seat => seat.stack >= 0 && seat.contributed >= 0)).toBe(true);
    if (snapshot.current_player_id) {
      expect(snapshot.seats.some(seat => seat.player_id === snapshot.current_player_id)).toBe(true);
    }
  });
  
  test('waiting has no board, pot contribution or active decision', () => {
    const snapshot = snapshotForScenario('waiting');
    expect(snapshot).toMatchObject({stage: 'waiting_for_players', board: []});
    expect(snapshot.seats.every(seat => seat.contributed === 0)).toBe(true);
    expect(snapshot.current_player_id).toBeUndefined();
  });
  
  test('every street exposes the expected number of community cards', () => {
    expect(snapshotForScenario('pre_flop').board).toHaveLength(0);
    expect(snapshotForScenario('flop').board).toHaveLength(3);
    expect(snapshotForScenario('turn').board).toHaveLength(4);
    expect(snapshotForScenario('river').board).toHaveLength(5);
    expect(snapshotForScenario('showdown').board).toHaveLength(5);
  });
  
  test('normal decision exposes coherent call and raise limits', () => {
    const snapshot = snapshotForScenario('pre_flop');
    expect(snapshot.current_player_id).toBe(MOCK_PLAYER_ID);
    expect(snapshot.legal_actions?.actions).toEqual(['fold', 'call', 'raise']);
    expect(snapshot.legal_actions!.min_raise_to).toBeLessThan(snapshot.legal_actions!.max_raise_to!);
  });
  
  test.each([
    ['complete', [MOCK_PLAYER_ID], 1275],
    ['complete_loss', ['bia_sp'], 1275],
    ['complete_tie', [MOCK_PLAYER_ID, 'bia_sp'], 1275],
  ] as const)('%s payouts reconcile with the pot', (scenario, winners, total) => {
    const snapshot = snapshotForScenario(scenario);
    expect(snapshot.winners).toEqual(winners);
    expect(Object.values(snapshot.payouts || {}).reduce((sum, value) => sum + value, 0)).toBe(total);
    expect(snapshot.stage).toBe('complete');
  });
  
  test('fold win does not leak any private cards', () => {
    const snapshot = snapshotForScenario('fold_win');
    expect(snapshot.board).toEqual([]);
    expect(snapshot.seats.filter(seat => seat.player_id !== MOCK_PLAYER_ID)
      .flatMap(seat => seat.hole_cards || []).every(card => card === 'back')).toBe(true);
    expect(snapshot.seats.flatMap(seat => seat.hole_cards_revealed || []).every(Boolean)).toBe(false);
  });

  test('paid post-hand scenes expose only the intended purchase', () => {
    const winnerCards = snapshotForScenario('winner_cards');
    expect(winnerCards).toMatchObject({stage: 'complete', won_without_showdown: true, winners: ['bia_sp']});
    expect(winnerCards.seats.find(seat => seat.player_id === 'bia_sp')?.hole_cards).toEqual(['back', 'back']);
    expect(winnerCards.shuffle_server_seed_hex).toBeUndefined();

    const rabbit = snapshotForScenario('rabbit_hunt');
    expect(rabbit).toMatchObject({stage: 'complete', won_without_showdown: true, winners: [MOCK_PLAYER_ID]});
    expect(rabbit.shuffle_server_seed_hex).toBeTruthy();
    expect(rabbit.shuffle_commit_hash).toBeTruthy();
  });

  test('rebuy starts the viewer busted and sitting out', () => {
    const snapshot = snapshotForScenario('rebuy');
    expect(snapshot.seats.find(seat => seat.player_id === MOCK_PLAYER_ID)).toMatchObject({
      stack: 0, state: 'sitting_out', dealt_in: false
    });
  });

  test('long-session reminder waits until another player has the turn', () => {
    const snapshot = snapshotForScenario('reality_check');
    expect(snapshot.current_player_id).not.toBe(MOCK_PLAYER_ID);
    expect(snapshot.legal_actions?.actions).toEqual([]);
  });
  
  test('side pots list eligible players and reconcile their amounts', () => {
    const snapshot = snapshotForScenario('side_pot');
    expect(snapshot.pot_results?.length).toBeGreaterThan(1);
    expect(snapshot.pot_results?.every(pot => pot.amount > 0 && pot.eligible_player_ids.length > 0)).toBe(true);
  });
});

describe('mock realtime service contract', () => {
  afterEach(() => vi.useRealTimers());
  
  function serviceFor(scenario: MockScenario, delay = 10) {
    const messages: Array<Record<string, unknown>> = [];
    const statuses: Array<[string, number]> = [];
    const service = new MockTableService(scenario, delay, {
      onMessage: message => messages.push(message as unknown as Record<string, unknown>),
      onStatus: (status, attempt) => statuses.push([status, attempt]),
    });
    return {service, messages, statuses};
  }
  
  test('connects, responds to ping/sync and reconnects with an incremented attempt', () => {
    vi.useFakeTimers();
    const {service, messages, statuses} = serviceFor('flop');
    expect(service.send({type: 'ping'})).toBe(false);
    
    service.connect();
    vi.runOnlyPendingTimers();
    expect(statuses).toContainEqual(['connected', 0]);
    expect(messages.some(message => message.type === 'state')).toBe(true);
    
    messages.length = 0;
    expect(service.send({type: 'ping'})).toBe(true);
    expect(service.send({type: 'sync_state', action_id: 'sync-1'})).toBe(true);
    vi.runOnlyPendingTimers();
    expect(messages).toEqual(expect.arrayContaining([
      expect.objectContaining({type: 'state'}),
      expect.objectContaining({type: 'state', action_id: 'sync-1'}),
    ]));
    
    service.reconnect();
    vi.runOnlyPendingTimers();
    expect(statuses).toContainEqual(['reconnecting', 1]);
    expect(statuses).toContainEqual(['connected', 1]);
    service.close();
  });
  
  test('acknowledges ready and selective reveals and publishes their updated state', () => {
    vi.useFakeTimers();
    const {service, messages} = serviceFor('fold_win');
    service.connect();
    vi.runAllTimers();
    messages.length = 0;
    
    service.send({type: 'ready', ready: false, action_id: 'ready-1'});
    service.send({type: 'show_cards', card_index: 1, action_id: 'show-1'});
    vi.runOnlyPendingTimers();
    
    expect(messages).toEqual(expect.arrayContaining([
      expect.objectContaining({type: 'action_ack', action_id: 'ready-1'}),
      expect.objectContaining({type: 'action_ack', action_id: 'show-1'}),
    ]));
    const states = messages.filter(message => message.type === 'state');
    const latest = states.at(-1)?.snapshot as ReturnType<typeof snapshotForScenario>;
    const hero = latest.seats.find(seat => seat.player_id === MOCK_PLAYER_ID);
    expect(hero?.ready).toBe(false);
    expect(hero?.hole_cards_revealed).toEqual([false, true]);
    service.close();
  });

  test('acknowledges rabbit hunt and reveals paid winner cards in the next snapshot', () => {
    vi.useFakeTimers();
    const rabbit = serviceFor('rabbit_hunt');
    rabbit.service.connect();
    vi.runAllTimers();
    rabbit.messages.length = 0;
    rabbit.service.send({type: 'request_rabbit_hunt', action_id: 'rabbit-1'});
    vi.runAllTimers();
    expect(rabbit.messages).toContainEqual(expect.objectContaining({type: 'action_ack', action_id: 'rabbit-1'}));

    const paid = serviceFor('winner_cards');
    paid.service.connect();
    vi.runAllTimers();
    paid.messages.length = 0;
    paid.service.send({type: 'request_winner_cards', action_id: 'winner-1'});
    vi.runAllTimers();
    expect(paid.messages).toContainEqual(expect.objectContaining({type: 'action_ack', action_id: 'winner-1'}));
    const latest = paid.messages.filter(message => message.type === 'state').at(-1)?.snapshot as ReturnType<typeof snapshotForScenario>;
    expect(latest.seats.find(seat => seat.player_id === 'bia_sp')).toMatchObject({
      hole_cards: ['AS', 'AD'], hole_cards_revealed: [true, true]
    });
    rabbit.service.close();
    paid.service.close();
  });

  test('applies a rebuy to the busted viewer and publishes the new stack', () => {
    vi.useFakeTimers();
    const {service, messages} = serviceFor('rebuy');
    service.connect();
    vi.runAllTimers();
    messages.length = 0;
    service.applyRebuy(5500, true);
    const latest = messages.at(-1)?.snapshot as ReturnType<typeof snapshotForScenario>;
    expect(latest.seats.find(seat => seat.player_id === MOCK_PLAYER_ID)).toMatchObject({
      stack: 5500, state: 'active', ready: true, auto_rebuy: true
    });
    service.close();
  });
  
  test('echoes chat/reactions and validates exact call preselection amount', () => {
    vi.useFakeTimers();
    const {service, messages} = serviceFor('flop');
    service.connect();
    vi.runOnlyPendingTimers();
    messages.length = 0;
    
    service.send({type: 'chat', message: 'olá'});
    service.send({type: 'reaction', reaction_id: 'angry', target_player_id: 'bia_sp'});
    service.send({type: 'preselect_action', action: 'call', amount: 999, action_id: 'bad-call'});
    vi.runOnlyPendingTimers();
    
    expect(messages).toEqual(expect.arrayContaining([
      expect.objectContaining({type: 'chat', player_id: MOCK_PLAYER_ID, message: 'olá'}),
      expect.objectContaining({type: 'chat', player_id: 'bia_sp'}),
      expect.objectContaining({type: 'reaction', reaction_id: 'angry', target_player_id: 'bia_sp'}),
      expect.objectContaining({type: 'error', code: 'invalid_action', action_id: 'bad-call'}),
    ]));
    service.close();
  });
  
  test('applies a normal action and emits the resulting snapshot and achievement', () => {
    vi.useFakeTimers();
    const {service, messages} = serviceFor('flop');
    service.connect();
    vi.runOnlyPendingTimers();
    messages.length = 0;
    
    expect(service.send({type: 'act', action: 'raise', amount: 300, action_id: 'raise-1'})).toBe(true);
    vi.runAllTimers();
    
    expect(messages).toEqual(expect.arrayContaining([
      expect.objectContaining({type: 'action_ack', action_id: 'raise-1'}),
      expect.objectContaining({type: 'achievement_unlocked', key: 'primeiro_aumento'}),
    ]));
    const state = messages.find(message => message.type === 'state');
    const hero = (state?.snapshot as ReturnType<typeof snapshotForScenario>).seats
      .find(seat => seat.player_id === MOCK_PLAYER_ID);
    expect(hero?.contributed).toBe(300);
    expect(hero?.stack).toBe(4600);
    service.close();
  });
  
  test('models explicit server rejection and a server that never responds', () => {
    vi.useFakeTimers();
    const rejected = serviceFor('action_error');
    rejected.service.connect();
    vi.runOnlyPendingTimers();
    rejected.messages.length = 0;
    rejected.service.send({type: 'act', action: 'fold', action_id: 'bad-1'});
    vi.runOnlyPendingTimers();
    expect(rejected.messages).toContainEqual(expect.objectContaining({
      type: 'error', code: 'invalid_action', action_id: 'bad-1',
    }));
    
    const timeout = serviceFor('timeout');
    timeout.service.connect();
    vi.runOnlyPendingTimers();
    timeout.messages.length = 0;
    expect(timeout.service.send({type: 'act', action: 'fold', action_id: 'hang'})).toBe(true);
    vi.runOnlyPendingTimers();
    expect(timeout.messages).toEqual([]);
    rejected.service.close();
    timeout.service.close();
  });
  
  test('runs the reconnecting scenario through disconnect and recovery', () => {
    vi.useFakeTimers();
    const {service, statuses, messages} = serviceFor('reconnecting');
    service.connect();
    vi.runAllTimers();
    expect(statuses.map(([status]) => status)).toEqual([
      'connecting', 'connected', 'disconnected', 'reconnecting', 'connected',
    ]);
    expect(messages.filter(message => message.type === 'state')).toHaveLength(2);
    service.close();
  });
});
