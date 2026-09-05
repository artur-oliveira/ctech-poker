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
import {decodeLobbyServerMessage, encodeLobbyClientMessage} from "@/lib/ws/lobbyCodec";
import {useRouter} from 'next/navigation';
import {acceptTableInvite, declineTableInvite, type SocialEventType} from '@/lib/api/social';
import {SOCIAL_KEYS} from '@/lib/social';
import {WALLET_QUERY_ROOT} from '@/lib/api/wallet';
import {COSMETIC_PURCHASE_QUERY_ROOT} from '@/lib/api/cosmeticPurchases';
import {ROOM_BUCKETS_QUERY_KEY} from '@/lib/lobbyBuckets';
import {checkApiLiveness} from '@/lib/network/liveness';
import {useApiLiveness} from '@/lib/network/NetworkProvider';

interface LobbyMessage {
  type: string;
  code?: string;
  // The lobby codec decodes field 9 as an empty marker message: `room_created`
  // only needs to know a room rode along, and the bucket aggregate is re-read
  // over HTTP (#205/#228).
  room?: object;
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

/** A settling window for the reconnect reconciliation.
 *
 * `ws-client` re-opens on its own backoff, so waking a laptop, switching
 * networks or rolling the gateway produces a burst of opens within a couple of
 * seconds — and each one used to re-read the bucket aggregate, the player row
 * and the whole social root. The reconciliation is therefore *trailing*: every
 * open re-arms it, and only the open the storm ends on actually spends the
 * reads. Nothing can be missed by waiting, because the last open always fires
 * it. */
export const RECONNECT_RECONCILE_DEBOUNCE_MS = 400;

/** The page-load open is not a reconnect.
 *
 * The socket connects because the app mounted, and the same mount already
 * fetched these queries — there is no offline gap to reconcile, only a
 * duplicate read of answers that are seconds old. Anything later than this is
 * a genuine re-open (the token arriving late, liveness recovering, a dropped
 * connection) and does reconcile. Measured from the hook's mount, which lives
 * for the whole session, so a long outage still reconciles on its first open. */
export const FIRST_OPEN_GRACE_MS = 5000;

// Not a beacon: the app has no metrics sink (`lib/telemetry.ts` is the
// client-*error* sink), so "refetches per reconnect" is an assertable counter,
// the same shape as `settleRefetchReads()`/`sessionRefreshCount()`.
let reconciles = 0;

/** How many reconnect reconciliations this session has spent. Each one costs
 *  three invalidations, and only observed queries actually refetch. */
export function lobbyReconcileCount() {
  return reconciles;
}

export function resetLobbyReconcileCount() {
  reconciles = 0;
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
      // The lobby renders the server's bucket aggregate, not a room list, so
      // a new public table is a refetch of that aggregate rather than a local
      // splice (#205). Rare enough to invalidate on every one.
      void queryClient.invalidateQueries({queryKey: ROOM_BUCKETS_QUERY_KEY});
    } else if (message.type === 'room_updated' && message.room_id !== undefined && message.seats_taken !== undefined) {
      const {room_id, seats_taken} = message;
      // Deliberately NOT invalidating the bucket aggregate here: this fires on
      // every seat change at every public table, and the aggregate is only an
      // availability hint — the seat itself is resolved server-side by
      // join-or-create. It refreshes on room_created and on socket open.
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
    } else if (message.type === 'cosmetic_purchase_update') {
      // #144: the deck/felt dialog reads its status from
      // COSMETIC_PURCHASE_QUERY_ROOT, so invalidating it here resolves an open
      // purchase on the frame instead of on the dialog's 4s fallback poll.
      void queryClient.invalidateQueries({queryKey: COSMETIC_PURCHASE_QUERY_ROOT});
      void queryClient.invalidateQueries({queryKey: WALLET_QUERY_ROOT});
      const statusLabel: Record<string, string> = {
        confirmed: 'Item cosmético liberado!',
        refunded: 'Compra do item cosmético estornada.',
        expired: 'Compra do item cosmético expirou sem pagamento.',
        failed: 'Falha na compra do item cosmético.',
      };
      pushNotification(statusLabel[message.code || ''] || 'Atualização na compra do seu item cosmético.', 'info');
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
  
  const reconcile = useCallback(() => {
    reconciles += 1;
    // Deltas sent while offline are not replayed, so the durable queries have
    // to be reconciled after a gap. `refetchType: 'active'` is React Query's
    // default and is spelled out here because it is the point: an unobserved
    // query is only marked stale, so a reconnect costs reads for the surfaces
    // actually on screen, not for everything the session ever loaded.
    for (const queryKey of [
      ROOM_BUCKETS_QUERY_KEY,
      // `['player','me']` carries the wallet balances too (see BALANCE_QUERY_KEY).
      ['player', 'me'],
      // Social deltas are never replayed either, and the unread badge is the
      // most visible thing that goes stale.
      SOCIAL_KEYS.root,
    ]) {
      void queryClient.invalidateQueries({queryKey, refetchType: 'active'});
    }
  }, [queryClient]);

  // Stamped in an effect, not during render (`Date.now()` is impure there). A
  // still-zero stamp means the socket opened before the mount effect ran — the
  // page-load open by definition, so it takes the same skip.
  const mountedAtRef = useRef(0);
  const reconcileTimerRef = useRef<ReturnType<typeof setTimeout>>(undefined);
  useEffect(() => {
    mountedAtRef.current = Date.now();
    return () => clearTimeout(reconcileTimerRef.current);
  }, []);

  const handleOpen = useCallback(() => {
    sendRef.current({type: 'ping'});
    if (!mountedAtRef.current || Date.now() - mountedAtRef.current < FIRST_OPEN_GRACE_MS) return;
    clearTimeout(reconcileTimerRef.current);
    reconcileTimerRef.current = setTimeout(reconcile, RECONNECT_RECONCILE_DEBOUNCE_MS);
  }, [reconcile]);
  
  const {status, send, reconnect} = useWebSocket({
    url: wsUrl,
    binaryType: 'arraybuffer',
    encode: encodeLobbyClientMessage,
    decode: decodeLobbyServerMessage,
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
