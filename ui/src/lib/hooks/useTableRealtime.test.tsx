import {act, renderHook, waitFor} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import type {ServerMessage, TableSnapshot} from '@/lib/api/table';

const ws = vi.hoisted(() => ({
  options: null as null | {
    onMessage: (message: ServerMessage) => void;
    onOpen: () => void;
  },
  send: vi.fn(() => true),
  reconnect: vi.fn(),
  status: 'connected' as const,
}));

const auth = vi.hoisted(() => ({
  token: 'access-token' as string | null,
  subscriber: null as null | ((token: string | null) => void),
  refresh: vi.fn(),
  setAccessToken: vi.fn(),
  setUsername: vi.fn(),
  setPlayerId: vi.fn(),
}));

vi.mock('@aoctech/ws-client', () => ({
  useWebSocket: vi.fn((options: typeof ws.options) => {
    ws.options = options;
    return {status: ws.status, attempt: 0, send: ws.send, reconnect: ws.reconnect};
  }),
}));

vi.mock('@/lib/mock', async importOriginal => {
  const original = await importOriginal<typeof import('@/lib/mock')>();
  return {...original, USE_MOCK: false};
});

vi.mock('@/lib/api/client', () => ({
  getAccessToken: () => auth.token,
  subscribeAccessToken: (subscriber: (token: string | null) => void) => {
    auth.subscriber = subscriber;
    return () => {
      auth.subscriber = null;
    };
  },
  setAccessToken: auth.setAccessToken,
  setUsername: auth.setUsername,
  setPlayerId: auth.setPlayerId,
}));

vi.mock('@/lib/auth/oauth', () => ({doRefresh: auth.refresh}));
vi.mock('@/lib/sound', () => ({playSound: vi.fn()}));

import {playSound} from '@/lib/sound';
import {useTableRealtime} from './useTableRealtime';

const VIEWER = 'player-1';

function snapshot(overrides: Partial<TableSnapshot> = {}): TableSnapshot {
  return {
    stage: 'pre_flop',
    board: [],
    seats: [
      {
        player_id: VIEWER,
        name: 'Artur',
        stack: 980,
        state: 'active',
        dealt_in: true,
        contributed: 20,
      },
      {
        player_id: 'player-2',
        name: 'Bia',
        stack: 960,
        state: 'active',
        dealt_in: true,
        contributed: 40,
      },
    ],
    current_player_id: VIEWER,
    legal_actions: {actions: ['fold', 'call', 'raise'], call_amount: 20, min_raise_to: 80, max_raise_to: 980},
    snapshot_version: 1,
    protocol_version: 6,
    hand_id: 'hand-1',
    ...overrides,
  };
}

function receive(message: ServerMessage) {
  act(() => ws.options?.onMessage(message));
}

describe('useTableRealtime', () => {
  beforeEach(() => {
    vi.useRealTimers();
    vi.clearAllMocks();
    ws.options = null;
    ws.send.mockReturnValue(true);
    auth.token = 'access-token';
    auth.refresh.mockResolvedValue(null);
    Object.defineProperty(globalThis.crypto, 'randomUUID', {
      configurable: true,
      value: vi.fn()
        .mockReturnValueOnce('action-1')
        .mockReturnValueOnce('action-2')
        .mockReturnValueOnce('action-3'),
    });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  test('configures the authenticated table socket and pings when connected', () => {
    const {result} = renderHook(() => useTableRealtime('table / 1', VIEWER, 'invite'));

    expect(ws.options).toMatchObject({
      url: expect.stringMatching(/\/v1\.0\/tables\/table%20%2F%201\/ws$/),
      enabled: true,
      authToken: 'access-token',
      shareCode: 'invite',
      binaryType: 'arraybuffer',
    });

    act(() => ws.options?.onOpen());
    expect(ws.send).toHaveBeenCalledWith({type: 'ping'});
    expect(result.current.status).toBe('connected');
  });

  test('publishes a snapshot, announcement and turn sound, while rejecting stale state', () => {
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    receive({type: 'state', snapshot: snapshot({current_player_id: 'player-2'})});
    expect(result.current.snapshot?.snapshot_version).toBe(1);
    expect(result.current.announcement).toContain('pré-flop');

    receive({
      type: 'state',
      snapshot: snapshot({
        snapshot_version: 2,
        stage: 'flop',
        board: ['As', 'Kh', '2d'],
        current_player_id: VIEWER,
      }),
    });
    expect(result.current.snapshot?.board).toEqual(['As', 'Kh', '2d']);
    expect(result.current.announcement).toContain('Flop');
    expect(playSound).toHaveBeenCalledWith('your_turn');
    expect(playSound).toHaveBeenCalledWith('reveal');

    receive({type: 'state', snapshot: snapshot({snapshot_version: 1, stage: 'river'})});
    expect(result.current.snapshot?.stage).toBe('flop');
  });

  test('submits a versioned action once and clears it only after its ACK', () => {
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    receive({type: 'state', snapshot: snapshot()});

    act(() => {
      expect(result.current.act('call', 20)).toBe(true);
      expect(result.current.act('fold')).toBe(false);
    });
    expect(result.current.pendingAction).toBe('call');
    expect(ws.send).toHaveBeenCalledWith({
      type: 'act',
      action: 'call',
      amount: 20,
      action_id: 'action-1',
      expected_snapshot_version: 1,
      expected_hand_id: 'hand-1',
    });

    receive({type: 'action_ack', action_id: 'some-other-action'});
    expect(result.current.pendingAction).toBe('call');
    receive({type: 'action_ack', action_id: 'action-1'});
    expect(result.current.pendingAction).toBeNull();
  });

  test('prevents actions before an authoritative hand snapshot or while disconnected', () => {
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    act(() => expect(result.current.act('fold')).toBe(false));
    expect(result.current.actionError).toMatchObject({code: 'missing_precondition'});

    receive({type: 'state', snapshot: snapshot()});
    ws.send.mockReturnValue(false);
    act(() => expect(result.current.act('fold')).toBe(false));
    expect(result.current.actionError).toMatchObject({code: 'not_connected'});
    expect(result.current.pendingAction).toBeNull();
  });

  test('requests an authoritative sync after an action timeout and reconnects if sync cannot be sent', () => {
    vi.useFakeTimers();
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    receive({type: 'state', snapshot: snapshot()});
    act(() => expect(result.current.act('raise', 100)).toBe(true));

    act(() => vi.advanceTimersByTime(8000));
    expect(result.current.actionError).toMatchObject({code: 'action_timeout'});
    expect(ws.send).toHaveBeenLastCalledWith({type: 'sync_state', action_id: 'action-1'});

    ws.send.mockReturnValue(false);
    act(() => vi.advanceTimersByTime(8000));
    expect(result.current.pendingAction).toBe('raise');
  });

  test('handles action errors, stale-state recovery and bot challenge state', () => {
    vi.useFakeTimers();
    vi.spyOn(Math, 'random').mockReturnValue(0);
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    receive({type: 'state', snapshot: snapshot()});
    act(() => result.current.act('call'));

    receive({type: 'error', code: 'stale_state', action_id: 'action-1'});
    expect(result.current.actionError).toMatchObject({code: 'stale_state'});
    act(() => vi.advanceTimersByTime(50));
    expect(ws.send).toHaveBeenLastCalledWith({type: 'sync_state', action_id: 'action-1'});

    receive({type: 'bot_challenge'});
    expect(result.current.botChallengeRequired).toBe(true);
    receive({type: 'bot_challenge_passed'});
    expect(result.current.botChallengeRequired).toBe(false);
    expect(result.current.actionError).toBeNull();
  });

  test('deduplicates chat, expires reactions and restores persisted realtime content', () => {
    vi.useFakeTimers();
    vi.setSystemTime(10_000);
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    receive({
      type: 'state',
      snapshot: snapshot({
        chat_messages: [{id: 'chat-1', player_id: 'player-2', message: 'oi', timestamp: 9_000}],
        reactions: [{
          id: 'reaction-1',
          player_id: 'player-2',
          reaction_id: 'angry',
          timestamp: 9_000,
          expires_at: 12_000,
        }],
      }),
    });
    expect(result.current.chat).toHaveLength(1);
    expect(result.current.reactions).toMatchObject([{id: 'reaction-1', reactionId: 'angry'}]);

    receive({type: 'chat', action_id: 'chat-1', player_id: 'player-2', message: 'oi'});
    expect(result.current.chat).toHaveLength(1);
    receive({type: 'chat', action_id: 'chat-2', player_id: VIEWER, message: 'olá'});
    expect(result.current.chat).toHaveLength(2);

    act(() => vi.advanceTimersByTime(2_000));
    expect(result.current.reactions).toEqual([]);
  });

  test('locks ready and card reveal commands until acknowledgement', () => {
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));

    act(() => {
      expect(result.current.ready()).toBe(true);
      expect(result.current.ready()).toBe(false);
    });
    expect(result.current.readyPending).toBe(true);
    receive({type: 'action_ack', action_id: 'action-1'});
    expect(result.current.readyPending).toBe(false);

    act(() => {
      expect(result.current.showCards(0)).toBe(true);
      expect(result.current.showCards(1)).toBe(false);
    });
    expect(ws.send).toHaveBeenLastCalledWith({
      type: 'show_cards',
      action_id: 'action-2',
      card_index: 0,
    });
    receive({type: 'action_ack', action_id: 'action-2'});
    expect(result.current.showCardsPending).toBe(false);
    expect(playSound).toHaveBeenCalledWith('showing_card');
  });

  test('automatically posts the big blind once for a pending entry', () => {
    vi.useFakeTimers();
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    receive({
      type: 'state',
      snapshot: snapshot({
        stage: 'waiting_for_players',
        hand_id: '',
        seats: [
          {player_id: VIEWER, stack: 1000, state: 'pending_entry', contributed: 0},
          {player_id: 'player-2', stack: 1000, state: 'active', contributed: 0},
        ],
      }),
    });
    expect(ws.send).toHaveBeenCalledWith({
      type: 'post_big_blind',
      action_id: 'action-1',
    });

    receive({type: 'state', snapshot: snapshot({snapshot_version: 2, seats: result.current.snapshot!.seats})});
    const sentFrames = ws.send.mock.calls as unknown as Array<[Record<string, unknown>]>;
    expect(sentFrames.filter(([frame]) => frame.type === 'post_big_blind')).toHaveLength(1);
    receive({type: 'action_ack', action_id: 'action-1'});
  });

  test('refreshes an unauthorized session and exposes server-side lifecycle events', async () => {
    auth.refresh.mockResolvedValue({
      accessToken: 'fresh-token',
      username: 'artur',
    });
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));

    receive({type: 'error', code: 'unauthorized'});
    await waitFor(() => expect(auth.setAccessToken).toHaveBeenCalledWith('fresh-token'));
    expect(auth.setUsername).toHaveBeenCalledWith('artur');

    receive({type: 'achievement_unlocked', key: 'first_win', stars: 3});
    receive({type: 'removed', code: 'idle_timeout'});
    expect(result.current.unlock).toEqual({key: 'first_win', stars: 3});
    expect(result.current.removed).toEqual({code: 'idle_timeout'});
  });

  test('updates only equity that belongs to the latest snapshot version', () => {
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    receive({type: 'state', snapshot: snapshot({snapshot_version: 7})});
    receive({type: 'equity', player_id: VIEWER, equity: 0.75, snapshot_version: 6});
    expect(result.current.snapshot?.seats[0].equity).toBeUndefined();
    receive({type: 'equity', player_id: VIEWER, equity: 0.75, snapshot_version: 7});
    expect(result.current.snapshot?.seats[0].equity).toBe(0.75);
  });
});
