'use client';
import {useCallback, useEffect, useRef, useState} from 'react';
import {useQueryClient} from '@tanstack/react-query';
import {getAccessToken, subscribeAccessToken} from '@/lib/api/client';
import {wsOrigin} from '@/lib/ws/origin';
import {recoverSession} from '@/lib/auth/session';
import {useWebSocket} from '@aoctech/ws-client';
import {USE_MOCK} from '@/lib/mockConfig';
import type {Room} from '@/lib/api/rooms';
import {pushNotification} from '@/lib/notify';
import {decodeServerMessage, encodeClientMessage} from "@/lib/ws/utils";
import {useRouter} from 'next/navigation';
import {acceptTableInvite, declineTableInvite, type SocialEventType} from '@/lib/api/social';
import {SOCIAL_KEYS} from '@/lib/social';
import {WALLET_QUERY_ROOT} from '@/lib/api/wallet';
import {checkApiLiveness} from '@/lib/network/liveness';
import {useApiLiveness} from '@/lib/network/NetworkProvider';

interface LobbyMessage {
  type: string;
  code?: string;
  room?: Room;
  room_id?: string;
  seats_taken?: number;
  amount?: number;
  text?: string;
  purchase_id?: string;
  // Social pushes are invalidation-only: the durable state always comes back
  // over HTTP, never from the socket payload.
  social_event?: {type?: SocialEventType | string; event_id?: string; room_id?: string};
  unread_count?: number;
}

const SOCIAL_EVENT_COPY: Record<SocialEventType, string> = {
  friend_request: 'Você recebeu uma solicitação de amizade.',
  friend_accepted: 'Sua solicitação de amizade foi aceita.',
  table_invite: 'Você recebeu um convite para uma mesa.'
};

export function useLobbyRealtime() {
  const queryClient = useQueryClient();
  const sendRef = useRef<(value: object) => boolean>(() => false);
  const [socketAuthToken, setSocketAuthToken] = useState(() => getAccessToken());
  const apiLiveness = useApiLiveness();
  useEffect(() => subscribeAccessToken(setSocketAuthToken), []);
  const router = useRouter();
  
  const receive = useCallback((message: LobbyMessage) => {
    if (message.type === 'error' && message.code === 'unauthorized') {
      // The server accepts the upgrade and only then rejects the auth frame,
      // which resets ws-client's backoff, so an expired token here means an
      // endless reconnect loop (prod: 325 attempts in ten minutes) unless the
      // token is actually renewed.
      recoverSession();
    } else if (message.type === 'room_created' && message.room) {
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
            return {...r, seats_taken};
          }
          return r;
        });
      });
      queryClient.setQueryData<Room | undefined>(['room', room_id], oldRoom =>
        oldRoom ? {...oldRoom, seats_taken} : oldRoom);
    } else if (message.type === 'sandbox_purchase_update') {
      // One root invalidation: a purchase moves balance, catalog ownership and
      // history together, and naming a subset is what left the store showing
      // ownership that no longer existed.
      void queryClient.invalidateQueries({queryKey: WALLET_QUERY_ROOT});
      void queryClient.invalidateQueries({queryKey: ['player', 'me']});
      const statusLabel: Record<string, string> = {
        confirmed: 'Compra confirmada — créditos adicionados!',
        refunded: 'Compra estornada.',
        expired: 'Compra expirou sem pagamento.',
        failed: 'Falha na compra.',
      };
      pushNotification(statusLabel[message.code || ''] || 'Atualização na sua compra de créditos.', 'info');
    } else if (message.type === 'reaction_purchase_update') {
      void queryClient.invalidateQueries({queryKey: WALLET_QUERY_ROOT});
      const statusLabel: Record<string, string> = {
        confirmed: 'Reação premium liberada!',
        refunded: 'Compra da reação estornada.',
        expired: 'Compra da reação expirou sem pagamento.',
        failed: 'Falha na compra da reação.',
      };
      pushNotification(statusLabel[message.code || ''] || 'Atualização na compra da sua reação.', 'info');
    } else if (message.type === 'social_event') {
      void queryClient.invalidateQueries({queryKey: SOCIAL_KEYS.root});
      const event = message.social_event;
      const eventType = event?.type as SocialEventType | undefined;
      const copy = eventType ? SOCIAL_EVENT_COPY[eventType] || 'Nova atividade em Pessoas.'
        : 'Nova atividade em Pessoas.';
      // Answering from the toast also marks the inbox event read (both server
      // handlers call notifyUnread), so the badge clears without opening the
      // list. Accepting authorises nothing on its own: expiry, friendship,
      // room status and capacity are revalidated by the server and again by
      // the normal join flow.
      if (eventType === 'table_invite' && event?.event_id && event.room_id) {
        const eventId = event.event_id;
        const roomId = event.room_id;
        pushNotification(copy, 'info', [
          {
            label: 'Entrar', run: async () => {
              await acceptTableInvite(eventId);
              await queryClient.invalidateQueries({queryKey: SOCIAL_KEYS.root});
              router.push(`/table?id=${roomId}`);
            }
          },
          {
            label: 'Recusar', run: async () => {
              await declineTableInvite(eventId);
              await queryClient.invalidateQueries({queryKey: SOCIAL_KEYS.root});
            }
          },
        ]);
      } else {
        pushNotification(copy, 'info', [
          {label: 'Ver atividades', run: () => router.push('/people?tab=activity')},
        ]);
      }
    } else if (message.type === 'social_presence_changed') {
      // Presence rides along with the friend and recent lists; nothing else
      // on screen depends on it.
      void queryClient.invalidateQueries({queryKey: SOCIAL_KEYS.friends});
      void queryClient.invalidateQueries({queryKey: SOCIAL_KEYS.recent});
    } else if (message.type === 'social_inbox_count' && message.unread_count !== undefined) {
      queryClient.setQueryData(SOCIAL_KEYS.summary, {unread_count: message.unread_count});
    } else if (message.type === 'payment_received') {
      const amount = message.amount || 0;
      pushNotification(`Pagamento recebido: R$ ${(amount / 100).toLocaleString('pt-BR', {minimumFractionDigits: 2})}`, 'info');
    } else if (message.type === 'system_broadcast') {
      pushNotification(message.text || '', 'info');
    }
  }, [queryClient, router]);
  
  const origin = wsOrigin();
  const wsUrl = `${origin}/v1.0/ws`;
  
  const handleOpen = useCallback(() => {
    sendRef.current({type: 'ping'});
    // Deltas sent while offline are not replayed. Reconcile the durable
    // queries on every open; this is cheap and prevents an indefinitely stale
    // lobby after sleep/network changes.
    void queryClient.invalidateQueries({queryKey: ['rooms']});
    // `['player','me']` carries the wallet balances too (see BALANCE_QUERY_KEY).
    void queryClient.invalidateQueries({queryKey: ['player', 'me']});
    // Social deltas sent while the socket was down are never replayed either,
    // and the unread badge is the most visible thing that goes stale.
    void queryClient.invalidateQueries({queryKey: SOCIAL_KEYS.root});
  }, [queryClient]);
  
  const {status, send, reconnect} = useWebSocket({
    url: wsUrl,
    binaryType: 'arraybuffer',
    encode: encodeClientMessage,
    decode: decodeServerMessage,
    onMessage: data => receive(data as LobbyMessage),
    // No token means no session to authenticate with: connecting anyway only
    // produces the same unauthorized/close loop against the server.
    enabled: !USE_MOCK && Boolean(socketAuthToken) && apiLiveness.status === 'available',
    authToken: socketAuthToken || undefined,
    onOpen: handleOpen
  });
  
  useEffect(() => {
    sendRef.current = send;
  }, [send]);

  useEffect(() => {
    if (!USE_MOCK && socketAuthToken && (status === 'error' || status === 'disconnected')) {
      void checkApiLiveness();
    }
  }, [socketAuthToken, status]);
  
  return {status, reconnect};
}
