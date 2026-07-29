'use client';
import type {CSSProperties} from 'react';
import {Avatar, AvatarFallback, AvatarImage} from './avatar';
import {cn, initials} from '@/lib/utils';

export function PlayerAvatar({name, avatarUrl, size, isViewer = false, className, decorative = false}: {
  name?: string;
  avatarUrl?: string;
  size?: number;
  isViewer?: boolean;
  className?: string;
  decorative?: boolean;
}) {
  const label = isViewer ? 'Você' : name || 'Jogador';
  const style = size ? {width: size, height: size} as CSSProperties : undefined;
  return <Avatar className={cn(className)} style={style} role={decorative ? undefined : 'img'}
                 aria-label={decorative ? undefined : `Avatar de ${label}`} aria-hidden={decorative || undefined}>
    {avatarUrl && <AvatarImage src={avatarUrl} alt=""/>}
    <AvatarFallback>{isViewer ? 'EU' : initials(name)}</AvatarFallback>
  </Avatar>;
}
