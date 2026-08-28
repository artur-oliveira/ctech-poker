import {act, renderHook} from '@testing-library/react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {useLobbyRealtime} from './useLobbyRealtime';

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
    expect(state.invalidateQueries).toHaveBeenCalledTimes(4);
    expect(state.invalidateQueries).toHaveBeenCalledWith({queryKey: ['social']});
    act(() => result.current.reconnect());
    expect(state.reconnect).toHaveBeenCalled();
  });
  
  test('adds a new room once and updates occupancy without losing other rooms', () => {
    renderHook(() => useLobbyRealtime());
    act(() => state.options?.onMessage({
      type: 'room_created',
      room: {room_id: 'new-room', seats_taken: 1},
    }));
    const add = state.setQueryData.mock.calls[0][1] as (rooms?: Array<Record<string, unknown>>) => Array<Record<string, unknown>>;
    expect(add()).toEqual([{room_id: 'new-room', seats_taken: 1}]);
    expect(add([{room_id: 'new-room'}])).toEqual([{room_id: 'new-room'}]);
    
    act(() => state.options?.onMessage({
      type: 'room_updated',
      room_id: 'room-1',
      seats_taken: 5,
    }));
    const update = state.setQueryData.mock.calls[1][1] as (rooms?: Array<Record<string, unknown>>) => Array<Record<string, unknown>>;
    expect(update()).toEqual([]);
    expect(update([{id: 'room-1', seats_taken: 2}, {id: 'room-2', seats_taken: 3}]))
      .toEqual([{id: 'room-1', seats_taken: 5}, {id: 'room-2', seats_taken: 3}]);
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

  test('renews the session instead of looping on an unauthorized frame', () => {
    renderHook(() => useLobbyRealtime());
    act(() => state.options?.onMessage({type: 'error', code: 'unauthorized'}));
    expect(state.recover).toHaveBeenCalledOnce();

    act(() => state.options?.onMessage({type: 'error', code: 'rate_limited'}));
    expect(state.recover).toHaveBeenCalledOnce();
  });

  test('mirrors an occupancy change onto the single-room cache entry', () => {
    renderHook(() => useLobbyRealtime());
    act(() => state.options?.onMessage({type: 'room_updated', room_id: 'room-1', seats_taken: 5}));
    const [key, updater] = state.setQueryData.mock.calls[1];
    expect(key).toEqual(['room', 'room-1']);
    const patch = updater as (room?: Record<string, unknown>) => Record<string, unknown> | undefined;
    expect(patch({room_id: 'room-1', seats_taken: 2})).toEqual({room_id: 'room-1', seats_taken: 5});
    expect(patch(undefined)).toBeUndefined();
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
