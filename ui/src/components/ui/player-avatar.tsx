'use client';
import type {CSSProperties} from 'react';
import {Medal} from 'lucide-react';
import {Avatar, AvatarFallback, AvatarImage} from './avatar';
import {avatarBadgeLabel} from '@/lib/avatarCosmetics';
import {BADGE_ART, BADGE_TONE} from '@/components/avatar/badges';
import {FRAME_ART} from '@/components/avatar/frames';
import {HAT_ART, HAT_RISE} from '@/components/avatar/hats';
import {cn, initials} from '@/lib/utils';

export function PlayerAvatar({name, avatarUrl, size, isViewer = false, className, decorative = false,
                               frameId, hatId, badgeIds}: {
  name?: string;
  avatarUrl?: string;
  size?: number;
  isViewer?: boolean;
  className?: string;
  decorative?: boolean;
  /** Equipped cosmetics (#292), composed client-side over the avatar as SVG
   * art in its 64x64 box (DESIGN.md "Avatar cosmetics art contract"). With
   * none set the markup is byte-identical to a plain avatar; an id the
   * registry doesn't know renders nothing. Pass primitives (and a memoised
   * `badgeIds`) so a memoised `Seat` keeps its render budget. */
  frameId?: string;
  hatId?: string;
  /** Reserved slot (the wire still carries it): no accessory art exists yet,
   * so it renders nothing and an accessory alone leaves the avatar plain. */
  accessoryId?: string;
  /** Up to three equipped badges, a small row under the avatar. */
  badgeIds?: string[];
}) {
  const label = isViewer ? 'Você' : name || 'Jogador';
  const style = size ? {width: size, height: size} as CSSProperties : undefined;
  const avatar = <Avatar className={cn(className)} style={style}
                         role={decorative ? undefined : 'img'}
                         aria-label={decorative ? undefined : `Avatar de ${label}`} aria-hidden={decorative || undefined}>
    {avatarUrl && <AvatarImage src={avatarUrl} alt=""/>}
    <AvatarFallback>{isViewer ? 'EU' : initials(name)}</AvatarFallback>
  </Avatar>;
  if (!frameId && !hatId && !badgeIds?.length) return avatar;
  // Bottom to top: frame (the ring over the photo edge), then the hat resting
  // on top of the ring and rising above the box.
  const Hat = hatId ? HAT_ART[hatId] : undefined;
  const Frame = frameId ? FRAME_ART[frameId] : undefined;
  // A sized avatar scales its badges like the seat does (40%, 12-20px), so a
  // 28px leaderboard row does not wear a 20px badge.
  // A hat rises above the box (HAT_RISE of it): a surface that stacks a label
  // right above the avatar reserves that much with `var(--avatar-hat-room)`.
  const sized = size ? {
    '--avatar-badge-size': `${Math.min(20, Math.max(12, Math.round(size * .4)))}px`,
    ...(Hat && {'--avatar-hat-room': `${Math.round(size * HAT_RISE)}px`}),
  } as CSSProperties : undefined;
  return <span className="player-avatar-decorated" style={sized}>
    {avatar}
    {(frameId || hatId) && <span className="player-avatar-art" aria-hidden="true">
      {Frame && <Frame/>}
      {Hat && <Hat/>}
    </span>}
    {!!badgeIds?.length && <span className="player-avatar-badges" aria-hidden="true">
      {badgeIds.map(id => {
        const Glyph = BADGE_ART[id] ?? Medal;
        const tone = BADGE_TONE[id];
        const style = tone && {
          '--badge-bg': `var(--cosmetic-${tone.bg})`,
          '--badge-fg': `var(--cosmetic-${tone.fg})`,
        } as CSSProperties;
        return <span key={id} className="player-avatar-badge" style={style} title={avatarBadgeLabel(id)}>
          <Glyph aria-hidden="true"/>
        </span>;
      })}
    </span>}
  </span>;
}
