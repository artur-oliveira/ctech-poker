'use client';
import {type ComponentProps, useRef} from 'react';
import {Camera, LoaderCircle, Trash2} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {PlayerAvatar} from '@/components/ui/player-avatar';
import {useAvatarRemove, useAvatarUpload} from '@/lib/hooks/useProfileEdits';

/**
 * The avatar, its camera button and the file input behind it. The header
 * popover and `/player-profile` both render this: the layout differs only by
 * `size` and the container class, and the upload itself is
 * `useAvatarUpload`'s, so the two surfaces cannot drift on what a photo costs
 * or what the player is told afterwards.
 */
export function ProfilePhotoEditor({name, avatarUrl, size, className, frameId, badgeIds}: {
  name?: string;
  avatarUrl?: string;
  size: number;
  className: string;
  /** Equipped seasonal frame/badges (#292) — only `/player-profile` passes
   * these today; the header popover renders the plain avatar it always has. */
  frameId?: string;
  badgeIds?: string[];
}) {
  const fileInput = useRef<HTMLInputElement>(null);
  const upload = useAvatarUpload();

  return <div className={className}>
    <PlayerAvatar name={name} avatarUrl={avatarUrl} size={size} frameId={frameId} badgeIds={badgeIds}/>
    <Button type="button" size="icon" className="profile-avatar-camera" disabled={upload.isPending}
            aria-label={avatarUrl ? 'Trocar foto de perfil' : 'Adicionar foto de perfil'}
            onClick={() => fileInput.current?.click()}>
      {upload.isPending ? <LoaderCircle className="spin" aria-hidden="true"/> : <Camera aria-hidden="true"/>}
    </Button>
    <input ref={fileInput} className="sr-only" type="file" accept="image/jpeg,image/png"
           aria-label="Selecionar foto de perfil" onChange={event => {
      const file = event.target.files?.[0];
      if (file) upload.mutate(file);
      event.target.value = '';
    }}/>
  </div>;
}

/**
 * Deleting the photo. Rendered only when there is one to delete, and laid out
 * by its caller: an icon in the popover's identity row, a labelled button
 * beside "Salvar nome" on the route.
 */
export function ProfilePhotoRemoveButton({children, ...props}: ComponentProps<typeof Button>) {
  const remove = useAvatarRemove();
  return <Button type="button" variant="ghost" disabled={remove.isPending}
                 onClick={() => remove.mutate()} {...props}>
    {remove.isPending ? <LoaderCircle className="spin" aria-hidden="true"/> : <Trash2 aria-hidden="true"/>}
    {children}
  </Button>;
}
