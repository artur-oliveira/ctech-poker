'use client';
import {Switch as Primitive} from '@base-ui/react/switch';
import {cn} from '@/lib/utils';

function Switch({className, ...props}: Primitive.Root.Props) {
  return <Primitive.Root
    className={cn('relative inline-flex h-6 w-11 shrink-0 items-center overflow-hidden rounded-full bg-white/15 outline-none transition-colors focus-visible:ring-2 focus-visible:ring-[var(--brand)]/40 data-checked:bg-[var(--brand)] disabled:cursor-not-allowed disabled:opacity-50', className)} {...props}>
    <Primitive.Thumb
      className="switch-thumb block size-4.5 rounded-full bg-white shadow"/>
  </Primitive.Root>;
}

export {Switch};
