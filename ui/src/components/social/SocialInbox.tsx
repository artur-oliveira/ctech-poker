'use client';
import {useEffect, useRef} from 'react';
import {useRouter} from 'next/navigation';
import {useQueryClient} from '@tanstack/react-query';
import {RotateCw} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {SkeletonList} from '@/components/ui/skeleton';
import {markInboxRead, type SocialInboxEvent} from '@/lib/api/social';
import {inviteActionable, SOCIAL_KEYS, socialEventCopy} from '@/lib/social';
import type {SocialActionState} from '@/lib/hooks/useSocialActions';

/** Durable activity feed. Nothing here grants access on its own: accepting an
 * invite only authorises opening the room — capacity, terms, currency and
 * buy-in are revalidated by the normal join flow. */
export function SocialInbox({events, isLoading = false, isError = false, hasNext = false, loadingMore = false,
  onMoreAction, onRetryAction, onNavigateAction, actions, nameOf}: {
  events: SocialInboxEvent[];
  isLoading?: boolean;
  isError?: boolean;
  hasNext?: boolean;
  loadingMore?: boolean;
  onMoreAction?: () => void;
  onRetryAction?: () => void;
  /** Lets a host that is itself an overlay (the lobby drawer) close before the
   * accepted invite navigates away. */
  onNavigateAction?: () => void;
  actions: SocialActionState;
  nameOf: (playerId: string) => string;
}) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const markedRef = useRef<Set<string>>(new Set());

  // Opening the activity list is what marks it read; the badge is derived from
  // the same unread flag, so it clears in the same pass.
  useEffect(() => {
    const unread = events.filter(event => event.unread && !markedRef.current.has(event.event_id))
      .map(event => event.event_id);
    if (!unread.length) return;
    for (const id of unread) markedRef.current.add(id);
    markInboxRead(unread)
      .then(() => queryClient.invalidateQueries({queryKey: SOCIAL_KEYS.root}))
      .catch(() => {
        // Read receipts are cosmetic; a failure just leaves the badge up.
        for (const id of unread) markedRef.current.delete(id);
      });
  }, [events, queryClient]);

  if (isLoading) return <SkeletonList label="Carregando atividades" count={3} height={56} className="people-list"/>;
  if (isError) return <div className="people-empty" role="alert">
    <p>Não foi possível carregar suas atividades.</p>
    {onRetryAction && <Button type="button" variant="outline" onClick={onRetryAction}>
      <RotateCw aria-hidden="true"/> Tentar novamente
    </Button>}
  </div>;
  if (!events.length) return <div className="people-empty">
    <p>Nenhuma atividade por aqui.</p>
    <small>Solicitações de amizade e convites de mesa aparecem nesta lista.</small>
  </div>;

  return <div className="people-list-shell">
    <ul className="people-list">
      {events.map(event => {
        const busy = actions.pending?.id === event.event_id;
        return <li key={event.event_id}
                   className={`people-row people-row-activity ${event.unread ? 'is-unread' : ''}`}>
          <div className="people-row-identity">
            {/* actor_name is resolved server-side for every actor (#73); nameOf
                — sourced from the friends/requests lists already in memory —
                is only a fallback for an older cached page without it. */}
            <b>{socialEventCopy(event, event.actor_name || nameOf(event.actor_id))}</b>
            <small>{new Date(event.created_at).toLocaleString('pt-BR')}</small>
          </div>
          {inviteActionable(event) && event.room_id && <div className="people-row-actions">
            <Button type="button" disabled={busy} onClick={async () => {
              if (await actions.run('accept-invite', event.event_id)) {
                onNavigateAction?.();
                router.push(`/table?id=${event.room_id}`);
              }
            }}>Entrar</Button>
            <Button type="button" variant="ghost" disabled={busy}
                    onClick={() => void actions.run('decline-invite', event.event_id)}>Recusar</Button>
          </div>}
        </li>;
      })}
    </ul>
    {hasNext && onMoreAction && <Button type="button" variant="ghost" disabled={loadingMore} onClick={onMoreAction}>
      {loadingMore ? 'Carregando…' : 'Carregar mais'}
    </Button>}
  </div>;
}
