import {act, renderHook} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import {
  FIRST_OPEN_GRACE_MS,
  lobbyReconcileCount,
  RECONNECT_RECONCILE_DEBOUNCE_MS,
  resetLobbyReconcileCount,
  useLobbyRealtime,
} from './useLobbyRealtime';
import {ROOM_BUCKETS_QUERY_KEY} from '@/lib/lobbyBuckets';

const state = vi.hoisted(() => ({
  options: null as null | {
    onMessage: (message: Record<string, unknown>) => void;
    onOpen: () => void;
  },
  send: vi.fn(() => true),
  reconnect: vi.fn(),
  setQueryData: vi.fn(),
  invalidateQueries: vi.fn(),
  notify: vi.fn(),
  recover: vi.fn(),
  push: vi.fn(),
  acceptInvite: vi.fn(() => Promise.resolve(undefined)),
  declineInvite: vi.fn(() => Promise.resolve(undefined)),
}));

vi.mock('@aoctech/ws-client', () => ({
  useWebSocket: vi.fn((options: typeof state.options) => {
    state.options = options;
    return {status: 'connected', send: state.send, reconnect: state.reconnect};
  }),
}));
vi.mock('@tanstack/react-query', () => ({
  useQueryClient: () => ({setQueryData: state.setQueryData, invalidateQueries: state.invalidateQueries}),
}));
vi.mock('@/lib/mockConfig', () => ({USE_MOCK: false}));
vi.mock('@/lib/api/client', () => ({
  getAccessToken: () => 'token',
  subscribeAccessToken: () => vi.fn(),
}));
vi.mock('@/lib/notify', () => ({pushNotification: state.notify}));
vi.mock('@/lib/auth/session', () => ({recoverSession: state.recover}));
vi.mock('@/lib/network/NetworkProvider', () => ({
  useApiLiveness: () => ({status: 'available', reason: null, checkedAt: 1}),
}));
vi.mock('@/lib/network/liveness', () => ({checkApiLiveness: vi.fn()}));
vi.mock('next/navigation', () => ({useRouter: () => ({push: state.push})}));
vi.mock('@/lib/api/social', () => ({
  acceptTableInvite: state.acceptInvite,
  declineTableInvite: state.declineInvite,
}));

describe('useLobbyRealtime', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    state.options = null;
  });
  
  test('opens an authenticated lobby socket and exposes reconnect', () => {
    const {result} = renderHook(() => useLobbyRealtime());
    expect(state.options).toMatchObject({
      url: expect.stringMatching(/\/v1\.0\/ws$/),
      enabled: true,
      authToken: 'token',
      binaryType: 'arraybuffer',
    });
    act(() => state.options?.onOpen());
    expect(state.send).toHaveBeenCalledWith({type: 'ping'});
    act(() => result.current.reconnect());
    expect(state.reconnect).toHaveBeenCalled();
  });

  describe('reconnect reconciliation', () => {
    beforeEach(() => {
      vi.useFakeTimers();
      resetLobbyReconcileCount();
    });
    afterEach(() => vi.useRealTimers());

    test('the page-load open reconciles nothing — the mount that opened the socket just read it', () => {
      renderHook(() => useLobbyRealtime());
      act(() => state.options?.onOpen());
      act(() => vi.advanceTimersByTime(RECONNECT_RECONCILE_DEBOUNCE_MS * 4));
      expect(state.invalidateQueries).not.toHaveBeenCalled();
      expect(lobbyReconcileCount()).toBe(0);
    });

    test('a genuine re-open reconciles the three durable roots, observed queries only', () => {
      renderHook(() => useLobbyRealtime());
      act(() => vi.advanceTimersByTime(FIRST_OPEN_GRACE_MS + 1));
      act(() => state.options?.onOpen());
      expect(state.invalidateQueries).not.toHaveBeenCalled();
      act(() => vi.advanceTimersByTime(RECONNECT_RECONCILE_DEBOUNCE_MS));
      expect(state.invalidateQueries).toHaveBeenCalledTimes(3);
      expect(state.invalidateQueries).toHaveBeenCalledWith({queryKey: ROOM_BUCKETS_QUERY_KEY, refetchType: 'active'});
      expect(state.invalidateQueries).toHaveBeenCalledWith({queryKey: ['player', 'me'], refetchType: 'active'});
      expect(state.invalidateQueries).toHaveBeenCalledWith({queryKey: ['social'], refetchType: 'active'});
      // No dead `['wallet','balance']` key — balance rides on `['player','me']` (issue #101).
      expect(state.invalidateQueries).not.toHaveBeenCalledWith({queryKey: ['wallet', 'balance']});
      expect(lobbyReconcileCount()).toBe(1);
    });

    test('a reconnect storm costs one reconciliation, not one per open', () => {
      renderHook(() => useLobbyRealtime());
      act(() => vi.advanceTimersByTime(FIRST_OPEN_GRACE_MS + 1));
      for (let attempt = 0; attempt < 6; attempt += 1) {
        act(() => state.options?.onOpen());
        act(() => vi.advanceTimersByTime(RECONNECT_RECONCILE_DEBOUNCE_MS - 1));
      }
      act(() => vi.advanceTimersByTime(RECONNECT_RECONCILE_DEBOUNCE_MS));
      expect(lobbyReconcileCount()).toBe(1);
      expect(state.invalidateQueries).toHaveBeenCalledTimes(3);
    });

    test('an unmount before the window closes spends no reads at all', () => {
      const {unmount} = renderHook(() => useLobbyRealtime());
      act(() => vi.advanceTimersByTime(FIRST_OPEN_GRACE_MS + 1));
      act(() => state.options?.onOpen());
      unmount();
      act(() => vi.advanceTimersByTime(RECONNECT_RECONCILE_DEBOUNCE_MS * 4));
      expect(lobbyReconcileCount()).toBe(0);
      expect(state.invalidateQueries).not.toHaveBeenCalled();
    });
  });

  test('refreshes the lobby aggregate on a new room and mirrors occupancy onto the room cache', () => {
    renderHook(() => useLobbyRealtime());
    act(() => state.options?.onMessage({
      type: 'room_created',
      room: {room_id: 'new-room', seats_taken: 1},
    }));
    // The lobby renders the server's bucket aggregate now, so a new public
    // table is a refetch of it rather than a local room-list splice (#205).
    expect(state.invalidateQueries).toHaveBeenCalledWith({queryKey: ROOM_BUCKETS_QUERY_KEY});

    act(() => state.options?.onMessage({
      type: 'room_updated',
      room_id: 'room-1',
      seats_taken: 5,
    }));
    // Seat churn only updates the open table's own cache entry — it must not
    // invalidate the aggregate, which fires on every seat change fleet-wide.
    const update = state.setQueryData.mock.calls.at(-1)?.[1] as (room?: Record<string, unknown>) => unknown;
    expect(update()).toBeUndefined();
    expect(update({room_id: 'room-1', seats_taken: 2})).toEqual({room_id: 'room-1', seats_taken: 5});
    expect(state.invalidateQueries).toHaveBeenCalledTimes(1);
  });

  test('turns payment and system messages into localized notifications', () => {
    renderHook(() => useLobbyRealtime());
    act(() => state.options?.onMessage({type: 'payment_received', amount: 12345}));
    expect(state.notify).toHaveBeenCalledWith('Pagamento recebido: R$ 123,45', 'info');
    
    act(() => state.options?.onMessage({type: 'system_broadcast', text: 'Manutenção em breve'}));
    expect(state.notify).toHaveBeenCalledWith('Manutenção em breve', 'info');
  });

  test('invalidates wallet queries and notifies on sandbox_purchase_update', () => {
    renderHook(() => useLobbyRealtime());
    act(() => state.options?.onMessage({type: 'sandbox_purchase_update', purchase_id: 'sbxp-1', code: 'confirmed', amount: 110000}));
    // One root invalidation covers balance, catalogs (which carry ownership) and history.
    expect(state.invalidateQueries).toHaveBeenCalledWith({queryKey: ['wallet']});
    expect(state.notify).toHaveBeenCalledWith(expect.stringContaining('confirmada'), 'info');
  });

  test('refreshes reaction ownership and notifies on reaction_purchase_update', () => {
    renderHook(() => useLobbyRealtime());
    act(() => state.options?.onMessage({type: 'reaction_purchase_update', purchase_id: 'prdp-1', code: 'confirmed'}));
    expect(state.invalidateQueries).toHaveBeenCalledWith({queryKey: ['wallet']});
    expect(state.notify).toHaveBeenCalledWith('Reação premium liberada!', 'info');
  });

  // #144: the cosmetic dialog reads its status from the ['cosmetic-purchase']
  // root, so this frame has to invalidate it — that is what resolves an open
  // purchase immediately instead of on the dialog's 4s fallback poll.
  test('resolves an open cosmetic purchase on cosmetic_purchase_update', () => {
    renderHook(() => useLobbyRealtime());
    act(() => state.options?.onMessage({type: 'cosmetic_purchase_update', purchase_id: 'prdp-cos-1', code: 'confirmed'}));
    expect(state.invalidateQueries).toHaveBeenCalledWith({queryKey: ['cosmetic-purchase']});
    expect(state.invalidateQueries).toHaveBeenCalledWith({queryKey: ['wallet']});
    expect(state.notify).toHaveBeenCalledWith('Item cosmético liberado!', 'info');
  });

  test('renews the session instead of looping on an unauthorized frame', () => {
    renderHook(() => useLobbyRealtime());
    act(() => state.options?.onMessage({type: 'error', code: 'unauthorized'}));
    expect(state.recover).toHaveBeenCalledOnce();

    act(() => state.options?.onMessage({type: 'error', code: 'rate_limited'}));
    expect(state.recover).toHaveBeenCalledOnce();
  });

  test('ignores an incomplete room update', () => {
    renderHook(() => useLobbyRealtime());
    act(() => state.options?.onMessage({type: 'room_updated', room_id: 'room-1'}));
    act(() => state.options?.onMessage({type: 'room_created'}));
    expect(state.setQueryData).not.toHaveBeenCalled();
  });

  test.each([
    ['sandbox_purchase_update', 'refunded', 'Compra estornada.'],
    ['sandbox_purchase_update', 'expired', 'Compra expirou sem pagamento.'],
    ['sandbox_purchase_update', 'failed', 'Falha na compra.'],
    ['sandbox_purchase_update', undefined, 'Atualização na sua compra de créditos.'],
    ['reaction_purchase_update', 'refunded', 'Compra da reação estornada.'],
    ['reaction_purchase_update', 'expired', 'Compra da reação expirou sem pagamento.'],
    ['reaction_purchase_update', 'failed', 'Falha na compra da reação.'],
    ['reaction_purchase_update', 'brand_new', 'Atualização na compra da sua reação.'],
    ['cosmetic_purchase_update', 'refunded', 'Compra do item cosmético estornada.'],
    ['cosmetic_purchase_update', 'expired', 'Compra do item cosmético expirou sem pagamento.'],
    ['cosmetic_purchase_update', 'failed', 'Falha na compra do item cosmético.'],
    ['cosmetic_purchase_update', undefined, 'Atualização na compra do seu item cosmético.'],
  ])('translates a %s with code %s', (type, code, message) => {
    renderHook(() => useLobbyRealtime());
    act(() => state.options?.onMessage({type, code}));
    expect(state.notify).toHaveBeenCalledWith(message, 'info');
  });

  test.each([
    ['friend_request', 'Você recebeu uma solicitação de amizade.'],
    ['friend_accepted', 'Sua solicitação de amizade foi aceita.'],
    ['table_invite', 'Você recebeu um convite para uma mesa.'],
    ['something_new', 'Nova atividade em Pessoas.'],
    [undefined, 'Nova atividade em Pessoas.'],
  ])('invalidates the social surface and announces a %s push', (eventType, message) => {
    renderHook(() => useLobbyRealtime());
    act(() => state.options?.onMessage({
      type: 'social_event', social_event: eventType ? {type: eventType} : undefined
    }));
    expect(state.invalidateQueries).toHaveBeenCalledWith({queryKey: ['social']});
    expect(state.notify).toHaveBeenCalledWith(message, 'info', expect.any(Array));
  });

  test('offers accept and decline on a table invite push', async () => {
    renderHook(() => useLobbyRealtime());
    act(() => state.options?.onMessage({
      type: 'social_event',
      social_event: {event_id: 'ev-1', type: 'table_invite', actor_id: 'p2', room_id: 'room-9'},
    }));
    const actions = state.notify.mock.calls.at(-1)![2] as {label: string; run: () => Promise<void>}[];
    expect(actions.map(action => action.label)).toEqual(['Entrar', 'Recusar']);

    await act(() => actions[0].run());
    expect(state.acceptInvite).toHaveBeenCalledWith('ev-1');
    expect(state.push).toHaveBeenCalledWith('/table?id=room-9');

    await act(() => actions[1].run());
    expect(state.declineInvite).toHaveBeenCalledWith('ev-1');
  });

  test('sends any other social push to the activity tab', async () => {
    renderHook(() => useLobbyRealtime());
    act(() => state.options?.onMessage({
      type: 'social_event', social_event: {event_id: 'ev-2', type: 'friend_request', actor_id: 'p2'},
    }));
    const actions = state.notify.mock.calls.at(-1)![2] as {label: string; run: () => Promise<void>}[];
    expect(actions.map(action => action.label)).toEqual(['Ver atividades']);
    await act(() => actions[0].run());
    expect(state.push).toHaveBeenCalledWith('/people?tab=activity');
  });

  test('refreshes only presence-bearing lists on a presence change', () => {
    renderHook(() => useLobbyRealtime());
    act(() => state.options?.onMessage({type: 'social_presence_changed'}));
    expect(state.invalidateQueries).toHaveBeenCalledWith({queryKey: ['social', 'friends']});
    expect(state.invalidateQueries).toHaveBeenCalledWith({queryKey: ['social', 'recent']});
    expect(state.notify).not.toHaveBeenCalled();
  });

  test('writes the unread badge straight from the counter push', () => {
    renderHook(() => useLobbyRealtime());
    act(() => state.options?.onMessage({type: 'social_inbox_count', unread_count: 3}));
    expect(state.setQueryData).toHaveBeenCalledWith(['social', 'summary'], {unread_count: 3});

    state.setQueryData.mockClear();
    act(() => state.options?.onMessage({type: 'social_inbox_count'}));
    expect(state.setQueryData).not.toHaveBeenCalled();
  });

  test('falls back to zero for a payment with no amount and an empty broadcast', () => {
    renderHook(() => useLobbyRealtime());
    act(() => state.options?.onMessage({type: 'payment_received'}));
    expect(state.notify).toHaveBeenCalledWith('Pagamento recebido: R$ 0,00', 'info');

    act(() => state.options?.onMessage({type: 'system_broadcast'}));
    expect(state.notify).toHaveBeenCalledWith('', 'info');
  });
});
