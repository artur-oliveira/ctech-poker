import {renderHook, act} from '@testing-library/react';
import {beforeEach, describe, expect, test, vi} from 'vitest';

const state = vi.hoisted(() => ({
  options: null as null | {
    onMessage: (message: Record<string, unknown>) => void;
    onOpen: () => void;
  },
  send: vi.fn(() => true),
  reconnect: vi.fn(),
  setQueryData: vi.fn(),
  notify: vi.fn(),
}));

vi.mock('@aoctech/ws-client', () => ({
  useWebSocket: vi.fn((options: typeof state.options) => {
    state.options = options;
    return {status: 'connected', send: state.send, reconnect: state.reconnect};
  }),
}));
vi.mock('@tanstack/react-query', () => ({
  useQueryClient: () => ({setQueryData: state.setQueryData}),
}));
vi.mock('@/lib/mockConfig', () => ({USE_MOCK: false}));
vi.mock('@/lib/api/client', () => ({
  getAccessToken: () => 'token',
  subscribeAccessToken: () => vi.fn(),
}));
vi.mock('@/lib/notify', () => ({pushNotification: state.notify}));

import {useLobbyRealtime} from './useLobbyRealtime';

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
});
