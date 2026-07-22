'use client';
import {Checkbox as Primitive} from '@base-ui/react/checkbox';
import {Check} from 'lucide-react';
import {cn} from '@/lib/utils';

export function Checkbox({className, ...props}: Primitive.Root.Props) {
  return <Primitive.Root
    className={cn('grid size-5 shrink-0 place-items-center rounded border border-white/25 bg-white/5 transition-colors duration-150 data-checked:border-[var(--brand)] data-checked:bg-[var(--brand)]', className)} {...props}><Primitive.Indicator><Check
      className="size-3.5 animate-in zoom-in-50 text-white duration-150"/></Primitive.Indicator></Primitive.Root>;
}
