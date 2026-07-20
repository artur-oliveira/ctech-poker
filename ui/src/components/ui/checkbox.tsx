'use client'
import {Checkbox as Primitive} from '@base-ui/react/checkbox';
import {Check} from 'lucide-react';
import {cn} from '@/lib/utils';

export function Checkbox({className, ...props}: Primitive.Root.Props) {
  return <Primitive.Root
    className={cn('grid size-5 shrink-0 place-items-center rounded border border-white/25 bg-white/5 data-checked:border-[var(--brand)] data-checked:bg-[var(--brand)]', className)} {...props}><Primitive.Indicator><Check
    className="size-3.5 text-white"/></Primitive.Indicator></Primitive.Root>
}
