'use client';
import {useEffect, useRef, useState} from 'react';
import {useRouter} from 'next/navigation';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {getRoom, getSeated} from '@/lib/api/rooms';
import {getHands, getMe, getSessions, type PlayerSession} from '@/lib/api/player';
import {getPlayerNotes} from '@/lib/api/playerNotes';
import {
  listReactionCatalog, listReactionPurchases, REACTION_PURCHASE_FIRST_PAGE_KEY
} from '@/lib/api/reactionPurchases';
import {pushNotification} from '@/lib/notify';

const REMOVED_REASON_COPY: Record<string, string> = {
  idle: 'Você foi removido da mesa por inatividade.',
  disconnected: 'Você foi removido da mesa após ficar desconectado por muito tempo.',
  exit_requested: 'Você saiu da mesa.'
};

export type TableRemoval = { code?: string; amount?: number } | null;
export type SessionRecap = { joinedAt: number; buyIn: number; finalStack: number };

/** Every server read the table surface needs, in one place.
 *
 * Buy-in is an explicit ceremony: nothing is debited until the player confirms
 * an amount. The server (not local browser storage) is the source of truth for
 * "is this player already seated", which is what lets a player return via a
 * new tab, a different browser, or a different device without repeating the
 * ceremony for a seat they already have. */
export function useTableSession(id: string, valid: boolean) {
  const {data: room} = useQuery({
    queryKey: ['room', id], queryFn: () => getRoom(id), enabled: valid
  });
  const {data: seatedStatus, isLoading: seatedLoading} = useQuery({
    queryKey: ['seated', id], queryFn: () => getSeated(id), enabled: valid
  });
  const seated = seatedStatus?.seated ?? false;
  // Last-winners strip: sourced from the player's own hand-history endpoint
  // (not live socket state) so it's populated from table load, not only after
  // the viewer sits through a fresh resolution.
  const {data: tableHands = []} = useQuery({
    queryKey: ['hands', id], queryFn: () => getHands({tableId: id}), enabled: valid,
    select: page => page.data
  });
  const {data: sessions = [], isLoading: sessionsLoading} = useQuery({
    queryKey: ['sessions', 'me'], queryFn: () => getSessions(), enabled: valid && seated
  });
  const {data: playerNotes = []} = useQuery({
    queryKey: ['player-notes'], queryFn: getPlayerNotes, enabled: valid && seated
  });
  const {data: reactionCatalog = [], isLoading: reactionCatalogLoading} = useQuery({
    queryKey: ['wallet', 'reaction-catalog'], queryFn: listReactionCatalog, enabled: valid && seated
  });
  const {data: reactionPurchases = [], isLoading: reactionPurchasesLoading} = useQuery({
    // First page only: this list drives the in-table "refunding" badge, not
    // ownership (which comes from the catalog's `owned` flag).
    queryKey: REACTION_PURCHASE_FIRST_PAGE_KEY,
    queryFn: () => listReactionPurchases().then(page => page.data),
    enabled: valid && seated
  });
  const {data: profile} = useQuery({
    queryKey: ['player', 'me'], queryFn: getMe, enabled: valid && seated
  });
  return {
    room, seated, seatedLoading, tableHands, sessions, sessionsLoading, playerNotes,
    reactionCatalog, reactionCatalogLoading, reactionPurchases, reactionPurchasesLoading, profile,
    openSession: sessions.find(session => session.table_id === id && session.ended_at === 0)
  };
}

/** Reacts to the server dropping the viewer from the table.
 *
 * The server never closes a removed player's socket (it just stops targeting
 * it in future broadcasts). Without reacting to this message the client would
 * otherwise sit frozen on the last snapshot it received, or silently reconnect
 * into a seat it no longer holds. */
export function useTableRemoval({id, removed, terminalError, sessions, sessionsLoading}: {
  id: string;
  removed: TableRemoval;
  terminalError: string | null;
  sessions: PlayerSession[];
  sessionsLoading: boolean;
}) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const [sessionRecap, setSessionRecap] = useState<SessionRecap | null>(null);
  const handledRemovalRef = useRef<TableRemoval>(null);
  useEffect(() => {
    if (!removed || removed === handledRemovalRef.current) return;
    // An exit the player requested (rather than an idle/disconnect kick)
    // reuses the same recap treatment LeaveDialog's own immediate-leave path
    // shows — the player is already gone from the snapshot by the time this
    // fires, so there is no viewer seat left to fall back to.
    if (removed.code === 'exit_requested' && removed.amount !== undefined) {
      // The recap's real join time and buy-in live in the sessions query, a
      // separate request that can still be in flight when the server answers
      // a "not dealt in, instant leave" right after sitting down. Waiting for
      // it is what keeps the recap from reporting a 0-chip buy-in and a join
      // time of "now"; the effect re-runs once the query settles. Nothing is
      // marked handled before that, so the removal is still handled exactly
      // once — just later.
      if (sessionsLoading) return;
      handledRemovalRef.current = removed;
      const amount = removed.amount;
      const openSessionAtRemoval = sessions.find(session => session.table_id === id && session.ended_at === 0);
      pushNotification(`Você saiu com ${amount.toLocaleString('pt-BR')} fichas.`, 'info');
      const recap = {
        joinedAt: openSessionAtRemoval?.joined_at || Date.now(),
        buyIn: openSessionAtRemoval?.buyin_amount || 0,
        finalStack: amount
      };
      queueMicrotask(() => setSessionRecap(recap));
      queryClient.setQueryData(['seated', id], {seated: false, stack: 0});
      return;
    }
    // queryClient/router aren't guaranteed referentially stable, which would
    // otherwise re-run this on every unrelated render once setSessionRecap
    // above has fired once — same removal object, so the ref comparison is
    // what actually stops it, not the dependency array.
    handledRemovalRef.current = removed;
    pushNotification(REMOVED_REASON_COPY[removed.code || ''] || 'Você foi removido da mesa.', 'info');
    queryClient.setQueryData(['seated', id], {seated: false, stack: 0});
    router.push('/lobby');
  }, [removed, id, queryClient, router, sessions, sessionsLoading]);
  useEffect(() => {
    if (!terminalError) return;
    pushNotification(terminalError === 'forbidden' ? 'Você não tem acesso a esta mesa.' :
      'Essa sala não está mais disponível.', 'info');
    queryClient.setQueryData(['seated', id], {seated: false, stack: 0});
    router.push('/lobby');
  }, [terminalError, id, queryClient, router]);
  return {
    sessionRecap,
    closeRecap: () => {
      queryClient.setQueryData(['seated', id], {seated: false, stack: 0});
      router.push('/lobby');
    }
  };
}
