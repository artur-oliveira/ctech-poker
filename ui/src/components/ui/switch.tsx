'use client';
import {Switch as Primitive} from '@base-ui/react/switch';
import {cn} from '@/lib/utils';

function Switch({className, ...props}: Primitive.Root.Props) {
  return <Primitive.Root
    className={cn('switch-hit relative inline-flex shrink-0 items-center justify-center rounded-full outline-none focus-visible:ring-2 focus-visible:ring-[var(--brand)]/40 disabled:cursor-not-allowed disabled:opacity-50', className)} {...props}>
    <span className="switch-track relative flex h-6 w-11 items-center overflow-hidden rounded-full bg-white/15 transition-colors">
      <Primitive.Thumb className="switch-thumb block size-4.5 rounded-full bg-white shadow"/>
    </span>
  </Primitive.Root>;
}

export {Switch};
