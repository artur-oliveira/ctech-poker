'use client';
import {useCallback, useEffect, useRef} from 'react';
import {useQueryClient} from '@tanstack/react-query';
import {getAccessToken, subscribeAccessToken} from '@/lib/api/client';
import {useWebSocket} from '@aoctech/ws-client';
import {USE_MOCK} from '@/lib/mock';
import type {Room} from '@/lib/api/rooms';
import {pushNotification} from '@/lib/notify';

interface LobbyMessage {
  type: string;
  room?: Room;
  room_id?: string;
  seats_taken?: number;
  amount?: number;
  text?: string;
}

export function useLobbyRealtime() {
  const queryClient = useQueryClient();
  const sendRef = useRef<(value: object) => boolean>(() => false);

  const receive = useCallback((message: LobbyMessage) => {
    if (message.type === 'room_created' && message.room) {
      const newRoom = message.room;
      queryClient.setQueryData<Room[]>(['rooms'], (oldRooms) => {
        if (!oldRooms) return [newRoom];
        const id = newRoom.room_id || newRoom.id;
        if (oldRooms.some(r => (r.room_id || r.id) === id)) {
          return oldRooms;
        }
        return [newRoom, ...oldRooms];
      });
    } else if (message.type === 'room_updated' && message.room_id !== undefined && message.seats_taken !== undefined) {
      const {room_id, seats_taken} = message;
      queryClient.setQueryData<Room[]>(['rooms'], (oldRooms) => {
        if (!oldRooms) return [];
        return oldRooms.map(r => {
          const id = r.room_id || r.id;
          if (id === room_id) {
            return { ...r, seats_taken };
          }
          return r;
        });
      });
    } else if (message.type === 'payment_received') {
      const amount = message.amount || 0;
      pushNotification(`Pagamento recebido: R$ ${(amount / 100).toLocaleString('pt-BR', { minimumFractionDigits: 2 })}`, 'info');
    } else if (message.type === 'system_broadcast') {
      pushNotification(message.text || '', 'info');
    }
  }, [queryClient]);

  const origin = (process.env.NEXT_PUBLIC_API_URL || (typeof window !== 'undefined' ? window.location.origin : '')).replace(/^http/, 'ws');
  const wsUrl = `${origin}/v1.0/ws`;

  const subscribeToken = useCallback((callback: (token: string) => void) => subscribeAccessToken(token => {
    if (token) callback(token);
  }), []);

  const handleOpen = useCallback(() => {
    sendRef.current({type: 'ping'});
  }, []);

  const {status, send, reconnect} = useWebSocket({
    url: wsUrl,
    onMessage: data => receive(data as LobbyMessage),
    enabled: !USE_MOCK,
    authToken: getAccessToken() || undefined,
    subscribeToken,
    onOpen: handleOpen
  });

  useEffect(() => {
    sendRef.current = send;
  }, [send]);

  return { status, reconnect };
}
