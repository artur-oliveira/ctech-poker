'use client';
import type {CSSProperties} from 'react';
import {Medal} from 'lucide-react';
import {Avatar, AvatarFallback, AvatarImage} from './avatar';
import {avatarBadgeLabel} from '@/lib/avatarCosmetics';
import {cn, initials} from '@/lib/utils';

export function PlayerAvatar({name, avatarUrl, size, isViewer = false, className, decorative = false, frameId, badgeIds}: {
  name?: string;
  avatarUrl?: string;
  size?: number;
  isViewer?: boolean;
  className?: string;
  decorative?: boolean;
  /** Equipped seasonal avatar frame (#292) — `data-frame` selects the ring
   * style in CSS. `undefined` (every caller except the profile screens)
   * renders byte-identical to before this field existed. */
  frameId?: string;
  /** Up to three equipped seasonal badges (#292), rendered as a small row
   * under the avatar. Composition, not server rendering, per the issue's own
   * "client composes, server never renders the avatar" decision. */
  badgeIds?: string[];
}) {
  const label = isViewer ? 'Você' : name || 'Jogador';
  const style = size ? {width: size, height: size} as CSSProperties : undefined;
  const avatar = <Avatar className={cn(className)} style={style}
                         data-frame={frameId || undefined} role={decorative ? undefined : 'img'}
                         aria-label={decorative ? undefined : `Avatar de ${label}`} aria-hidden={decorative || undefined}>
    {avatarUrl && <AvatarImage src={avatarUrl} alt=""/>}
    <AvatarFallback>{isViewer ? 'EU' : initials(name)}</AvatarFallback>
  </Avatar>;
  if (!badgeIds?.length) return avatar;
  return <span className="player-avatar-decorated">
    {avatar}
    <span className="player-avatar-badges" aria-hidden="true">
      {badgeIds.map(id => <span key={id} className="player-avatar-badge" title={avatarBadgeLabel(id)}>
        <Medal aria-hidden="true"/>
      </span>)}
    </span>
  </span>;
}
