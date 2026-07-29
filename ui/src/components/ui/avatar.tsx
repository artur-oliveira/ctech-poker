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
  return <Primitive.Image className={cn('size-full object-cover', className)} {...props}/>;
}

export {Avatar, AvatarFallback, AvatarImage};
