'use client';
import {Progress as Primitive} from '@base-ui/react/progress';
import {cn} from '@/lib/utils';

// @base-ui/react/progress renders unstyled Root/Track/Indicator parts and
// does not size the Indicator for you (unlike Radix) — the width below is
// this component's responsibility, not a default we're overriding.
function Progress({className, indicatorClassName, value, ...props}: Primitive.Root.Props & {
  indicatorClassName?: string;
}) {
  return <Primitive.Root value={value}
                         className={cn('relative h-1.5 w-full overflow-hidden rounded-full bg-white/15', className)} {...props}>
    <Primitive.Track className="relative h-full w-full">
      <Primitive.Indicator
        className={cn('block h-full rounded-full transition-[width] duration-300', indicatorClassName)}
        style={{width: `${value ?? 0}%`}}/>
    </Primitive.Track>
  </Primitive.Root>;
}

export {Progress};
