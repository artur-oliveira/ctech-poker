import {act, renderHook, waitFor} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import type {ServerMessage, TableSnapshot} from '@/lib/api/table';
import {playSound} from '@/lib/sound';
import {useTableRealtime} from './useTableRealtime';

const ws = vi.hoisted(() => ({
  options: null as null | {
    onMessage: (message: ServerMessage) => void;
    onOpen: () => void;
  },
  send: vi.fn((frame: object): boolean => Boolean(frame)),
  reconnect: vi.fn(),
  status: 'connected' as 'connected' | 'connecting' | 'reconnecting' | 'disconnected' | 'error',
  attempt: 0,
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
  MAX_RECONNECT_ATTEMPTS: 10,
  useWebSocket: vi.fn((options: typeof ws.options) => {
    ws.options = options;
    return {status: ws.status, attempt: ws.attempt ?? 0, send: ws.send, reconnect: ws.reconnect};
  }),
}));

vi.mock('@/lib/mockConfig', () => ({USE_MOCK: false}));

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
vi.mock('@/lib/network/NetworkProvider', () => ({
  useApiLiveness: () => ({status: 'available', reason: null, checkedAt: 1}),
}));
vi.mock('@/lib/network/liveness', () => ({checkApiLiveness: vi.fn()}));

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
  
  test('keeps realtime disabled when no table was selected', () => {
    const {unmount} = renderHook(() => useTableRealtime('', VIEWER));
    
    expect(ws.options).toMatchObject({url: null, enabled: false});
    unmount();
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
    // Still within the retry budget — auto-retry is in flight, so no alert
    // yet (see the auto-retry-cap test below for the exhausted case).
    expect(result.current.actionError).toBeNull();
    act(() => vi.advanceTimersByTime(50));
    expect(ws.send).toHaveBeenLastCalledWith({type: 'sync_state', action_id: 'action-1'});

    receive({type: 'bot_challenge'});
    expect(result.current.botChallengeRequired).toBe(true);
    receive({type: 'bot_challenge_passed'});
    expect(result.current.botChallengeRequired).toBe(false);
    expect(result.current.actionError).toBeNull();
  });
  
  test('still auto-retries a stale_state that arrives after an unrelated broadcast raced ahead of it', () => {
    vi.useFakeTimers();
    vi.spyOn(Math, 'random').mockReturnValue(0);
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    receive({type: 'state', snapshot: snapshot()});
    act(() => result.current.act('check'));

    // The server's pubsub broadcast for a concurrent event can reach this
    // socket before the direct reply to our own request.
    receive({type: 'state', snapshot: snapshot({snapshot_version: 2})});
    expect(result.current.pendingAction).toBe('check');

    receive({type: 'error', code: 'stale_state', action_id: 'action-1'});
    act(() => vi.advanceTimersByTime(50));
    expect(ws.send).toHaveBeenLastCalledWith({type: 'sync_state', action_id: 'action-1'});

    receive({type: 'state', snapshot: snapshot({snapshot_version: 3}), action_id: 'action-1'});
    expect(result.current.pendingAction).toBe('check');
    expect(ws.send).toHaveBeenLastCalledWith(expect.objectContaining({
      type: 'act', action: 'check', expected_snapshot_version: 3
    }));
  });

  test('auto-retries a stale_state action against each fresh resync up to the retry cap', () => {
    vi.useFakeTimers();
    vi.spyOn(Math, 'random').mockReturnValue(0);
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    receive({type: 'state', snapshot: snapshot()});
    act(() => result.current.act('call'));
    expect(ws.send).toHaveBeenLastCalledWith(expect.objectContaining({type: 'act', expected_snapshot_version: 1}));

    // Backoff doubles per retry already spent (50ms, 100ms, 200ms) since
    // Math.random is pinned to 0 here.
    for (const [delayMs, nextVersion] of [[50, 2], [100, 3], [200, 4]] as const) {
      receive({type: 'error', code: 'stale_state', action_id: 'action-1'});
      expect(result.current.pendingAction).toBe('call');
      act(() => vi.advanceTimersByTime(delayMs));
      expect(ws.send).toHaveBeenLastCalledWith({type: 'sync_state', action_id: 'action-1'});

      receive({type: 'state', snapshot: snapshot({snapshot_version: nextVersion}), action_id: 'action-1'});
      expect(result.current.pendingAction).toBe('call');
      expect(ws.send).toHaveBeenLastCalledWith(expect.objectContaining({
        type: 'act', expected_snapshot_version: nextVersion
      }));
    }

    // The 4th stale_state has exhausted MAX_ACTION_RETRIES: give up for real.
    receive({type: 'error', code: 'stale_state', action_id: 'action-1'});
    expect(result.current.pendingAction).toBeNull();
    expect(result.current.actionError).toMatchObject({code: 'stale_state'});
  });

  test('two commands resyncing close together do not starve each others sync_state', () => {
    vi.useFakeTimers();
    vi.spyOn(Math, 'random').mockReturnValue(0);
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    receive({type: 'state', snapshot: snapshot()});
    act(() => result.current.act('raise', 100));
    act(() => result.current.requestRabbitHunt());

    receive({type: 'error', code: 'invalid_action', action_id: 'action-1'});
    receive({type: 'error', code: 'invalid_action', action_id: 'action-2'});
    act(() => vi.advanceTimersByTime(50));

    expect(ws.send).toHaveBeenCalledWith({type: 'sync_state', action_id: 'action-1'});
    expect(ws.send).toHaveBeenCalledWith({type: 'sync_state', action_id: 'action-2'});
  });

  test('an unrelated broadcast does not disarm the resync watchdog, so a lost resync reply still escalates', () => {
    vi.useFakeTimers();
    vi.spyOn(Math, 'random').mockReturnValue(0);
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    receive({type: 'state', snapshot: snapshot()});
    act(() => result.current.act('raise', 100));

    receive({type: 'error', code: 'invalid_action', action_id: 'action-1'});
    act(() => vi.advanceTimersByTime(50));
    expect(ws.send).toHaveBeenLastCalledWith({type: 'sync_state', action_id: 'action-1'});

    // Someone else's turn broadcasts a newer, uncorrelated snapshot before
    // our own resync's reply comes back.
    receive({type: 'state', snapshot: snapshot({snapshot_version: 2})});
    expect(ws.reconnect).not.toHaveBeenCalled();

    // The resync's own reply never arrives — the watchdog must still fire.
    act(() => vi.advanceTimersByTime(2500));
    expect(ws.reconnect).toHaveBeenCalled();
  });

  test('resyncs after invalid_action and reconnects when the snapshot never arrives', () => {
    vi.useFakeTimers();
    vi.spyOn(Math, 'random').mockReturnValue(0);
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    receive({type: 'state', snapshot: snapshot()});
    act(() => result.current.act('raise', 100));
    
    receive({type: 'error', code: 'invalid_action', action_id: 'action-1'});
    expect(result.current.actionError).toMatchObject({code: 'invalid_action'});
    
    act(() => vi.advanceTimersByTime(50));
    expect(ws.send).toHaveBeenLastCalledWith({type: 'sync_state', action_id: 'action-1'});
    
    // A server that answers frames but cannot load the table never sends the
    // snapshot back — the socket must be replaced without a page reload.
    expect(ws.reconnect).not.toHaveBeenCalled();
    act(() => vi.advanceTimersByTime(2500));
    expect(ws.reconnect).toHaveBeenCalled();
  });
  
  test('a snapshot answer to a resync cancels the reconnect escalation', () => {
    vi.useFakeTimers();
    vi.spyOn(Math, 'random').mockReturnValue(0);
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    receive({type: 'state', snapshot: snapshot()});
    act(() => result.current.act('raise', 100));
    
    receive({type: 'error', code: 'invalid_action', action_id: 'action-1'});
    act(() => vi.advanceTimersByTime(50));
    // The resync's own reply echoes the action_id it was sent with — an
    // unrelated broadcast (no action_id) must NOT cancel this watchdog, only
    // the correlated reply may.
    receive({type: 'state', snapshot: snapshot({snapshot_version: 2}), action_id: 'action-1'});
    act(() => vi.advanceTimersByTime(2500));
    expect(ws.reconnect).not.toHaveBeenCalled();
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
    // The history a snapshot hydrates on connect must never pop a seat bubble.
    expect(result.current.chatBubbles).toEqual({});

    receive({type: 'chat', action_id: 'chat-1', player_id: 'player-2', message: 'oi'});
    expect(result.current.chat).toHaveLength(1);
    // A replay of an already-known id (the legacy compat event for a message
    // the snapshot already carried) must not surface a stale bubble either.
    expect(result.current.chatBubbles).toEqual({});
    receive({type: 'chat', action_id: 'chat-2', player_id: VIEWER, message: 'olá'});
    expect(result.current.chat).toHaveLength(2);
    expect(result.current.chatBubbles).toEqual({[VIEWER]: {id: 'chat-2', message: 'olá'}});

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

  // A rabbit hunt (like every auxiliary command) carries no
  // expected_snapshot_version, so a server that judged it against state this
  // client does not have can only answer with a flat rejection. Those are
  // resubmitted under the same action_id — the server rejects them before
  // commit, so no idempotency guard was written — instead of failing the
  // player's click outright on the first try.
  test('retries a rabbit hunt rejected against stale state, then surfaces it once exhausted', () => {
    vi.useFakeTimers();
    vi.spyOn(Math, 'random').mockReturnValue(0);
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));

    act(() => {
      expect(result.current.requestRabbitHunt()).toBe(true);
      expect(result.current.requestRabbitHunt()).toBe(false);
    });
    expect(result.current.requestRabbitHuntPending).toBe(true);
    expect(ws.send).toHaveBeenLastCalledWith({type: 'request_rabbit_hunt', action_id: 'action-1'});

    for (let attempt = 1; attempt <= 3; attempt++) {
      ws.send.mockClear();
      receive({type: 'error', code: 'invalid_action', action_id: 'action-1'});
      expect(result.current.requestRabbitHuntPending).toBe(true);
      expect(result.current.actionError).toBeNull();
      act(() => vi.advanceTimersByTime(700 * 2 ** (attempt - 1)));
      expect(ws.send).toHaveBeenCalledWith({type: 'request_rabbit_hunt', action_id: 'action-1'});
    }

    receive({type: 'error', code: 'invalid_action', action_id: 'action-1'});
    expect(result.current.requestRabbitHuntPending).toBe(false);
    expect(result.current.actionError).toMatchObject({code: 'invalid_action'});
  });

  test('acknowledges a successful rabbit hunt request and unlocks it', () => {
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    act(() => {
      expect(result.current.requestRabbitHunt()).toBe(true);
    });
    receive({type: 'action_ack', action_id: 'action-1'});
    expect(result.current.requestRabbitHuntPending).toBe(false);
  });

  test('locks the exit request until acknowledgement', () => {
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    act(() => {
      expect(result.current.requestExit()).toBe(true);
      expect(result.current.requestExit()).toBe(false);
    });
    expect(result.current.requestExitPending).toBe(true);
    expect(ws.send).toHaveBeenLastCalledWith({type: 'request_exit', action_id: 'action-1'});

    receive({type: 'action_ack', action_id: 'action-1'});
    expect(result.current.requestExitPending).toBe(false);
  });

  test('sends cancel_exit', () => {
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    act(() => result.current.cancelExit());
    expect(ws.send).toHaveBeenLastCalledWith({type: 'cancel_exit', action_id: 'action-1'});
  });

  test('a removed frame carries the settled stack', () => {
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    receive({type: 'removed', code: 'exit_requested', amount: 480});
    expect(result.current.removed).toEqual({code: 'exit_requested', amount: 480});
  });

  test('locks the winner-cards request until acknowledgement and surfaces a rejection', () => {
    vi.useFakeTimers();
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    act(() => {
      expect(result.current.requestWinnerCards()).toBe(true);
      expect(result.current.requestWinnerCards()).toBe(false);
    });
    expect(result.current.requestWinnerCardsPending).toBe(true);
    expect(ws.send).toHaveBeenLastCalledWith({type: 'request_winner_cards', action_id: 'action-1'});
    // Resync-class rejections are resubmitted first (see the rabbit hunt test);
    // only an exhausted retry budget surfaces the failure to the player.
    for (let attempt = 0; attempt <= 3; attempt++) {
      receive({type: 'error', code: 'invalid_action', action_id: 'action-1'});
      act(() => vi.advanceTimersByTime(3000));
    }
    expect(result.current.requestWinnerCardsPending).toBe(false);
    expect(result.current.actionError).toMatchObject({code: 'invalid_action'});
  });

  test.each([
    {accept: true, frame: 'accept_winner_cards'},
    {accept: false, frame: 'decline_winner_cards'},
  ])('sends the winner\'s $frame answer and unlocks it on acknowledgement', ({accept, frame}) => {
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    act(() => {
      expect(result.current.answerWinnerCards(accept)).toBe(true);
      // Requester and winner share one in-flight slot; a second answer while
      // the first is unacknowledged must not go out.
      expect(result.current.answerWinnerCards(accept)).toBe(false);
    });
    expect(result.current.requestWinnerCardsPending).toBe(true);
    expect(ws.send).toHaveBeenLastCalledWith({type: frame, action_id: 'action-1'});
    receive({type: 'action_ack', action_id: 'action-1'});
    expect(result.current.requestWinnerCardsPending).toBe(false);
  });

  test('reports a rabbit hunt verification failure as a fire-and-forget frame', () => {
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    act(() => {
      expect(result.current.reportRabbitHuntVerifyFailed()).toBe(true);
    });
    expect(ws.send).toHaveBeenLastCalledWith({type: 'rabbit_hunt_verify_failed', action_id: 'action-1'});
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
  
  test('clears the stale post_big_blind action id once the seat leaves pending_entry, so a fresh pending_entry retries immediately', () => {
    vi.useFakeTimers();
    renderHook(() => useTableRealtime('table-1', VIEWER));
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
    expect(ws.send).toHaveBeenCalledWith({type: 'post_big_blind', action_id: 'action-1'});

    // The seat leaves pending_entry before that post ever acks (e.g. it was
    // actually accepted, or the hand moved on) ...
    receive({
      type: 'state',
      snapshot: snapshot({
        snapshot_version: 2, hand_id: 'hand-2',
        seats: [
          {player_id: VIEWER, stack: 980, state: 'active', contributed: 20},
          {player_id: 'player-2', stack: 1000, state: 'active', contributed: 0},
        ],
      }),
    });
    // ... then re-enters pending_entry on a later hand. Without clearing the
    // stale action id here, this is silently dropped until the internal
    // 2s x3 retry loop times it out.
    receive({
      type: 'state',
      snapshot: snapshot({
        snapshot_version: 3, hand_id: 'hand-3', stage: 'waiting_for_players',
        seats: [
          {player_id: VIEWER, stack: 980, state: 'pending_entry', contributed: 0},
          {player_id: 'player-2', stack: 1000, state: 'active', contributed: 0},
        ],
      }),
    });
    expect(ws.send).toHaveBeenCalledWith({type: 'post_big_blind', action_id: 'action-2'});
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
  
  test('emits all lightweight table commands with their protocol preconditions', () => {
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    receive({type: 'state', snapshot: snapshot({snapshot_version: 9, hand_id: 'hand-9'})});
    
    act(() => {
      expect(result.current.keepSeat()).toBe(true);
      expect(result.current.setRunItTwice(true)).toBe(true);
      expect(result.current.sendChat('boa mão')).toBe(true);
      expect(result.current.sendReaction('angry', 'player-2')).toBe(true);
      expect(result.current.preselectAction('call', 40)).toBe(true);
      expect(result.current.submitBotChallenge('turnstile-token')).toBe(true);
    });
    
    expect(ws.send).toHaveBeenCalledWith({type: 'keep_seat', action_id: 'action-1'});
    expect(ws.send).toHaveBeenCalledWith({type: 'set_run_it_twice', run_it_twice: true});
    expect(ws.send).toHaveBeenCalledWith({
      type: 'chat', message: 'boa mão', action_id: 'action-2',
    });
    expect(ws.send).toHaveBeenCalledWith({
      type: 'reaction', reaction_id: 'angry', target_player_id: 'player-2', action_id: 'action-3',
    });
    expect(ws.send).toHaveBeenCalledWith(expect.objectContaining({
      type: 'preselect_action',
      action: 'call',
      amount: 40,
      expected_snapshot_version: 9,
      expected_hand_id: 'hand-9',
    }));
    expect(ws.send).toHaveBeenCalledWith(expect.objectContaining({
      type: 'bot_challenge', turnstile_token: 'turnstile-token',
    }));
  });
  
  test('keeps a pending action alive through an unrelated broadcast, but releases it on a correlated or reconnect snapshot', () => {
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    receive({type: 'state', snapshot: snapshot()});

    // An unrelated newer broadcast (someone else's action) must not clear our
    // own pending action: the direct reply to it (ack, or a stale_state that
    // arms a retry) can still arrive after this on the same socket.
    act(() => result.current.act('call'));
    receive({type: 'state', snapshot: snapshot({snapshot_version: 2})});
    expect(result.current.pendingAction).toBe('call');
    receive({type: 'action_ack', action_id: 'action-1'});
    expect(result.current.pendingAction).toBeNull();

    act(() => result.current.act('fold'));
    receive({type: 'state', action_id: 'action-2', snapshot: snapshot({snapshot_version: 2})});
    expect(result.current.pendingAction).toBeNull();
    
    act(() => result.current.act('raise', 100));
    receive({type: 'connected'});
    receive({type: 'state', snapshot: snapshot({snapshot_version: 2})});
    expect(result.current.pendingAction).toBeNull();
  });
  
  test('uses an unversioned snapshot as acknowledgement for legacy auxiliary commands', () => {
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    act(() => result.current.ready());
    receive({
      type: 'state',
      snapshot: snapshot({snapshot_version: 0, protocol_version: 0, hand_id: undefined}),
    });
    expect(result.current.readyPending).toBe(false);
  });
  
  test('reports auxiliary command failures and timeouts, then permits retry', () => {
    vi.useFakeTimers();
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    
    act(() => result.current.ready(false));
    receive({type: 'error', code: 'invalid_action', action_id: 'action-1'});
    expect(result.current.readyPending).toBe(false);
    expect(result.current.actionError).toMatchObject({code: 'invalid_action'});
    
    act(() => result.current.showCards(1));
    act(() => vi.advanceTimersByTime(8000));
    expect(result.current.showCardsPending).toBe(false);
    expect(result.current.actionError).toMatchObject({code: 'action_timeout'});
    
    ws.send.mockReturnValue(false);
    act(() => expect(result.current.ready()).toBe(false));
    expect(result.current.readyPending).toBe(false);
    expect(result.current.actionError).toMatchObject({code: 'not_connected'});
    act(() => result.current.clearActionError());
    expect(result.current.actionError).toBeNull();
  });
  
  // The token keep-alive itself lives in lib/auth/session (mounted app-wide by
  // QueryProvider), not here: it has to keep running in the lobby too.
  test('retries a failed action connection without closing the socket again', () => {
    vi.useFakeTimers();
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    receive({type: 'state', snapshot: snapshot()});
    act(() => result.current.act('call'));
    
    ws.send.mockReturnValue(false);
    act(() => vi.advanceTimersByTime(8000));
    expect(result.current.actionError).toMatchObject({code: 'connection_lost'});
    expect(ws.reconnect).toHaveBeenCalledTimes(1);
    
    ws.send.mockReturnValue(true);
    act(() => vi.advanceTimersByTime(4 * 60 * 1000));
    expect(ws.reconnect).toHaveBeenCalledTimes(1);
  });
  
  test('reconnects a disconnected socket when the page becomes visible', () => {
    ws.status = 'disconnected';
    const visibility = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible');
    renderHook(() => useTableRealtime('table-1', VIEWER));
    
    act(() => document.dispatchEvent(new Event('visibilitychange')));
    expect(ws.reconnect).toHaveBeenCalledTimes(1);
    visibility.mockRestore();
    ws.status = 'connected';
    ws.attempt = 0;
  });

  test('reports a drop as reconnecting, not disconnected/error, while retries remain', () => {
    ws.status = 'error';
    ws.attempt = 1;
    const {result, rerender} = renderHook(() => useTableRealtime('table-1', VIEWER));
    expect(result.current.status).toBe('reconnecting');

    ws.status = 'disconnected';
    rerender();
    expect(result.current.status).toBe('reconnecting');

    // Once @aoctech/ws-client has actually given up (attempt > MAX_RECONNECT_ATTEMPTS),
    // the raw status must surface as-is so the UI's "give up" copy can show.
    ws.attempt = 11;
    rerender();
    expect(result.current.status).toBe('disconnected');

    ws.status = 'connected';
    ws.attempt = 0;
  });

  test('resets table-scoped state when navigating between tables', () => {
    const {result, rerender} = renderHook(
      ({tableId}) => useTableRealtime(tableId, VIEWER),
      {initialProps: {tableId: 'table-1'}},
    );
    receive({type: 'state', snapshot: snapshot({snapshot_version: 100})});
    expect(result.current.snapshot?.snapshot_version).toBe(100);
    
    rerender({tableId: 'table-2'});
    act(() => ws.options?.onOpen());
    expect(result.current.snapshot).toBeNull();
    receive({type: 'state', snapshot: snapshot({snapshot_version: 1, hand_id: 'other-hand'})});
    expect(result.current.snapshot?.snapshot_version).toBe(1);
  });
  
  test('announces payouts and selects sounds for bets, all-ins and showdown', () => {
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    receive({type: 'state', snapshot: snapshot({current_player_id: 'player-2'})});
    receive({
      type: 'state',
      snapshot: snapshot({
        snapshot_version: 2,
        current_player_id: 'player-2',
        seats: snapshot().seats.map(seat => seat.player_id === 'player-2'
          ? {...seat, contributed: 80}
          : seat),
      }),
    });
    expect(playSound).toHaveBeenCalledWith('half_pot');
    
    receive({
      type: 'state',
      snapshot: snapshot({
        snapshot_version: 3,
        current_player_id: 'player-2',
        seats: snapshot().seats.map(seat => seat.player_id === 'player-2'
          ? {...seat, contributed: 80, state: 'all_in'}
          : seat),
      }),
    });
    expect(playSound).toHaveBeenCalledWith('all_in');
    
    receive({
      type: 'state',
      snapshot: snapshot({
        snapshot_version: 4,
        stage: 'complete',
        current_player_id: undefined,
        payouts: {[VIEWER]: 120},
        winners: [VIEWER],
        pot_results: [{
          amount: 120,
          eligible_player_ids: [VIEWER],
          winner_player_ids: [VIEWER],
          payout_amount: 120,
          payouts: {[VIEWER]: 120},
        }],
      }),
    });
    expect(result.current.announcement).toContain('Você ganhou 120 fichas');
    expect(playSound).toHaveBeenCalledWith('reveal');
  });

  test('announces a refunded pot and a split pot in the same resolution', () => {
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    receive({type: 'state', snapshot: snapshot()});
    receive({
      type: 'state',
      snapshot: snapshot({
        snapshot_version: 2,
        stage: 'complete',
        current_player_id: undefined,
        payouts: {[VIEWER]: 60, 'player-2': 100},
        winners: [VIEWER, 'player-2'],
        pot_results: [
          {
            amount: 40, payout_amount: 40, eligible_player_ids: [VIEWER],
            winner_player_ids: [], refund: true, payouts: {[VIEWER]: 40},
          },
          {
            amount: 200, payout_amount: 200, eligible_player_ids: [VIEWER, 'player-2'],
            winner_player_ids: [VIEWER, 'player-2'], payouts: {[VIEWER]: 100, 'player-2': 100},
          },
        ],
      }),
    });
    expect(result.current.announcement).toContain('Você recebeu 40 fichas devolvidas');
    expect(result.current.announcement).toContain('dividiram um pote de 200 fichas');
  });

  test('falls back to aggregate payouts on a protocol without pot results', () => {
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    receive({type: 'state', snapshot: snapshot({protocol_version: 1})});
    receive({
      type: 'state',
      snapshot: snapshot({
        snapshot_version: 2, protocol_version: 1, stage: 'complete', current_player_id: undefined,
        payouts: {[VIEWER]: 120, 'player-2': 0}, winners: [VIEWER],
      }),
    });
    expect(result.current.announcement).toContain('Você recebeu 120 fichas');
    expect(result.current.announcement).not.toContain('Bia');
  });

  test('plays one reveal per flop card, staggered to match the deal animation', () => {
    vi.useFakeTimers();
    renderHook(() => useTableRealtime('table-1', VIEWER));
    receive({type: 'state', snapshot: snapshot()});
    vi.mocked(playSound).mockClear();
    receive({type: 'state', snapshot: snapshot({snapshot_version: 2, stage: 'flop', board: ['2H', '5D', '9C']})});
    expect(playSound).toHaveBeenCalledTimes(1);

    act(() => void vi.advanceTimersByTime(800));
    expect(playSound).toHaveBeenCalledTimes(3);
    expect(vi.mocked(playSound).mock.calls.every(([name]) => name === 'reveal')).toBe(true);
  });

  test('keeps retrying the automatic big blind post until it is acknowledged', () => {
    vi.useFakeTimers();
    renderHook(() => useTableRealtime('table-1', VIEWER));
    receive({
      type: 'state',
      snapshot: snapshot({
        protocol_version: 7,
        seats: snapshot().seats.map(seat => seat.player_id === VIEWER
          ? {...seat, state: 'pending_entry'} : seat),
      }),
    });
    const posts = () => ws.send.mock.calls
      .filter(call => (call[0] as {type: string} | undefined)?.type === 'post_big_blind').length;
    expect(posts()).toBe(1);

    act(() => void vi.advanceTimersByTime(2000));
    expect(posts()).toBe(2);
    act(() => void vi.advanceTimersByTime(4000));
    expect(posts()).toBe(3);
  });

  test('stops posting the big blind when the socket refuses the frame', () => {
    ws.send.mockReturnValue(false);
    renderHook(() => useTableRealtime('table-1', VIEWER));
    receive({
      type: 'state',
      snapshot: snapshot({
        protocol_version: 7,
        seats: snapshot().seats.map(seat => seat.player_id === VIEWER
          ? {...seat, state: 'pending_entry'} : seat),
      }),
    });
    expect(ws.send.mock.calls
      .filter(call => (call[0] as {type: string} | undefined)?.type === 'post_big_blind')).toHaveLength(1);
  });

  test('backs off longer before resyncing after a rate limit', () => {
    vi.useFakeTimers();
    vi.spyOn(Math, 'random').mockReturnValue(0);
    renderHook(() => useTableRealtime('table-1', VIEWER));
    receive({type: 'state', snapshot: snapshot()});
    ws.send.mockClear();
    receive({type: 'error', code: 'rate_limited'});

    act(() => void vi.advanceTimersByTime(700));
    expect(ws.send).not.toHaveBeenCalled();
    act(() => void vi.advanceTimersByTime(200));
    expect(ws.send).toHaveBeenCalledWith(expect.objectContaining({type: 'sync_state'}));
  });

  test('accepts a broadcast chat message and reaction without a correlation id', () => {
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    receive({type: 'state', snapshot: snapshot()});
    receive({type: 'chat', player_id: 'player-2', message: 'boa mão'});
    receive({type: 'reaction', player_id: 'player-2', reaction_id: 'clap'});

    expect(result.current.chat.at(-1)).toEqual(expect.objectContaining({player: 'player-2', message: 'boa mão'}));
    expect(result.current.reactions.at(-1)).toEqual(expect.objectContaining({
      playerId: 'player-2', reactionId: 'clap',
    }));
  });

  test('suppresses a muted player without touching their seat or actions', () => {
    const suppressed = new Set(['player-2']);
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER, undefined, undefined, suppressed));
    receive({
      type: 'state',
      snapshot: snapshot({
        chat_messages: [
          {id: 'chat-1', player_id: 'player-2', message: 'oi', timestamp: 9_000},
          {id: 'chat-2', player_id: VIEWER, message: 'olá', timestamp: 9_100},
        ],
        reactions: [{
          id: 'reaction-1', player_id: 'player-2', reaction_id: 'angry', timestamp: 9_000,
          expires_at: Date.now() + 10_000,
        }],
      }),
    });
    receive({type: 'chat', action_id: 'chat-3', player_id: 'player-2', message: 'de novo'});
    receive({type: 'reaction', action_id: 'reaction-2', player_id: 'player-2', reaction_id: 'clap'});

    expect(result.current.chat).toEqual([expect.objectContaining({player: VIEWER})]);
    expect(result.current.chatBubbles['player-2']).toBeUndefined();
    expect(result.current.reactions).toHaveLength(0);
    // The poker surface is untouched: the seat, its stack and its bets remain.
    expect(result.current.snapshot?.seats.some(seat => seat.player_id === 'player-2')).toBe(true);
  });

  test('drops content already on screen the moment a mute is confirmed', () => {
    const {result, rerender} = renderHook(
      ({suppressed}: {suppressed?: ReadonlySet<string>}) =>
        useTableRealtime('table-1', VIEWER, undefined, undefined, suppressed),
      {initialProps: {suppressed: undefined as ReadonlySet<string> | undefined}});
    receive({type: 'state', snapshot: snapshot()});
    receive({type: 'chat', action_id: 'chat-1', player_id: 'player-2', message: 'provocação'});
    receive({type: 'reaction', action_id: 'reaction-1', player_id: 'player-2', reaction_id: 'clap'});
    expect(result.current.chat).toHaveLength(1);
    expect(result.current.chatBubbles['player-2']).toBeDefined();
    expect(result.current.reactions).toHaveLength(1);

    rerender({suppressed: new Set(['player-2'])});
    expect(result.current.chat).toHaveLength(0);
    expect(result.current.chatBubbles['player-2']).toBeUndefined();
    expect(result.current.reactions).toHaveLength(0);
  });

  test('ignores a reaction the client does not know how to draw', () => {
    const {result} = renderHook(() => useTableRealtime('table-1', VIEWER));
    receive({type: 'state', snapshot: snapshot()});
    receive({type: 'reaction', player_id: 'player-2', reaction_id: 'not-a-reaction'});
    expect(result.current.reactions).toHaveLength(0);
  });
});
