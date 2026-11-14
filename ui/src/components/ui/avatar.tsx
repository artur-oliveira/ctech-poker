'use client';
import {Avatar as Primitive} from '@base-ui/react/avatar';
import {cn} from '@/lib/utils';

function Avatar({className, ...props}: Primitive.Root.Props) {
  return <Primitive.Root
    className={cn('flex size-9 shrink-0 items-center justify-center overflow-hidden rounded-full bg-[var(--brand)] text-sm font-bold text-[var(--on-brand)]', className)} {...props}/>;
}

function AvatarFallback({className, ...props}: Primitive.Fallback.Props) {
  return <Primitive.Fallback
    className={cn('flex size-full items-center justify-center uppercase', className)} {...props}/>;
}

function AvatarImage({className, ...props}: Primitive.Image.Props) {
  // A photo is identity, not a download: no drag to another tab, no long-press
  // "save image", no context menu. pointer-events:none also hands every click
  // to the control wrapping the avatar.
  return <Primitive.Image draggable={false} onContextMenu={event => event.preventDefault()}
                          className={cn('pointer-events-none size-full select-none object-cover [-webkit-touch-callout:none] [-webkit-user-drag:none]', className)} {...props}/>;
}

export {Avatar, AvatarFallback, AvatarImage};
