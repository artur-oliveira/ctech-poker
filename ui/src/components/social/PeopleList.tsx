'use client';
import Link from 'next/link';
import {Check, RotateCw, UserPlus, X} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {PlayerAvatar} from '@/components/ui/player-avatar';
import {SkeletonList} from '@/components/ui/skeleton';
import type {ReportSurface, SocialPlayer} from '@/lib/api/social';
import type {SocialActionState} from '@/lib/hooks/useSocialActions';
import {PlayerActionsMenu} from '@/components/social/PlayerActionsMenu';
import {lastPlayedLabel, presenceLabel} from '@/lib/social';
import {playerName} from '@/lib/utils';

export type PeopleListVariant = 'friends' | 'incoming' | 'outgoing' | 'recent' | 'blocked';

const SURFACE: Record<PeopleListVariant, ReportSurface> = {
  friends: 'profile',
  incoming: 'profile',
  outgoing: 'profile',
  recent: 'recent_player',
  blocked: 'profile'
};

/** Dense list, never a card grid: these are people to act on, and the row is
 * the unit of action. Presence is the only status shown for a friend; a table
 * appears as a join link only when the server published a room_id, which it
 * does exclusively for a friend who opted in and is at a joinable public
 * table — never a private one, and never the room code or stakes. */
export function PeopleList({
  variant, items, isLoading = false, isError = false, isStale = false, onRetryAction,
  emptyTitle, emptyHint, hasNext = false, loadingMore = false, onMoreAction, actions, onInviteAction, invitedIds
}: {
  variant: PeopleListVariant;
  items: SocialPlayer[];
  isLoading?: boolean;
  isError?: boolean;
  // True while the cache is being served during a failed refetch: the rows are
  // still useful, they just may be behind.
  isStale?: boolean;
  onRetryAction?: () => void;
  emptyTitle: string;
  emptyHint?: string;
  hasNext?: boolean;
  loadingMore?: boolean;
  onMoreAction?: () => void;
  actions: SocialActionState;
  onInviteAction?: (player: SocialPlayer) => void;
  invitedIds?: string[];
}) {
  if (isLoading) return <SkeletonList label="Carregando pessoas" count={4} height={64} className="people-list"/>;
  if (isError) return <div className="people-empty" role="alert">
    <p>Não foi possível carregar esta lista.</p>
    {onRetryAction && <Button type="button" variant="outline" onClick={onRetryAction}>
      <RotateCw aria-hidden="true"/> Tentar novamente
    </Button>}
  </div>;
  if (!items.length) return <div className="people-empty">
    <p>{emptyTitle}</p>
    {emptyHint && <small>{emptyHint}</small>}
  </div>;

  return <div className="people-list-shell">
    {isStale && <p className="people-stale" role="status">Sem conexão agora — mostrando a última lista carregada.</p>}
    <ul className="people-list">
      {items.map(player => {
        const busy = actions.pending?.id === player.player_id;
        const name = playerName(player.player_id, undefined, player.name);
        return <li key={player.player_id} className="people-row">
          <PlayerAvatar name={player.name} avatarUrl={player.avatar_url} size={40}/>
          <div className="people-row-identity">
            <b>{name}</b>
            <small>
              {variant === 'recent' && lastPlayedLabel(player.last_played_at)}
              {variant === 'recent' && player.presence && ' · '}
              {player.presence && <span className={`presence-dot presence-${player.presence}`}>
                {presenceLabel(player.presence)}
              </span>}
              {variant === 'blocked' && 'Bloqueado'}
              {variant === 'outgoing' && 'Solicitação enviada'}
              {variant === 'incoming' && 'Quer ser seu amigo'}
            </small>
          </div>
          <div className="people-row-actions">
            {variant === 'incoming' && <>
              <Button type="button" size="icon" aria-label={`Aceitar ${name}`} disabled={busy}
                      onClick={() => void actions.run('accept', player.player_id)}><Check aria-hidden="true"/></Button>
              <Button type="button" size="icon" variant="ghost" aria-label={`Recusar ${name}`} disabled={busy}
                      onClick={() => void actions.run('decline', player.player_id)}><X aria-hidden="true"/></Button>
            </>}
            {variant === 'outgoing' &&
              <Button type="button" variant="ghost" disabled={busy}
                      onClick={() => void actions.run('cancel', player.player_id)}>Cancelar</Button>}
            {variant === 'blocked' &&
              <Button type="button" variant="outline" disabled={busy}
                      onClick={() => void actions.run('unblock', player.player_id)}>Desbloquear</Button>}
            {variant === 'recent' && player.relationship === 'none' && !player.blocked &&
              <Button type="button" variant="outline" disabled={busy} aria-label={`Adicionar ${name}`}
                      onClick={() => void actions.run('request', player.player_id)}>
                <UserPlus aria-hidden="true"/> Adicionar
              </Button>}
            {variant === 'friends' && player.room_id &&
              <Link href={`/table?id=${player.room_id}`} className="social-actions-item">Entrar na mesa</Link>}
            {variant === 'friends' && onInviteAction &&
              <Button type="button" variant="outline" disabled={busy || invitedIds?.includes(player.player_id)}
                      onClick={() => onInviteAction(player)}>
                {invitedIds?.includes(player.player_id) ? 'Convidado' : 'Convidar'}
              </Button>}
            <PlayerActionsMenu target={player} actions={actions} surface={SURFACE[variant]}/>
          </div>
        </li>;
      })}
    </ul>
    {hasNext && onMoreAction && <Button type="button" variant="ghost" disabled={loadingMore} onClick={onMoreAction}>
      {loadingMore ? 'Carregando…' : 'Carregar mais'}
    </Button>}
  </div>;
}
