'use client';
import {Popover as Primitive} from '@base-ui/react/popover';
import {cn} from '@/lib/utils';

const Popover = Primitive.Root, PopoverTrigger = Primitive.Trigger;

function PopoverContent({
  className,
  children,
  side = 'bottom',
  sideOffset = 8,
  align = 'end',
  ...props
}: Primitive.Popup.Props & Pick<Primitive.Positioner.Props, 'align' | 'side' | 'sideOffset'>) {
  return <Primitive.Portal>
    <Primitive.Positioner side={side} sideOffset={sideOffset} align={align} className="isolate z-50">
      <Primitive.Popup
        className={cn('w-72 rounded-2xl border border-white/15 bg-[var(--surface-control)] p-4 text-[var(--on-brand)] shadow-2xl outline-none data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95', className)} {...props}>
        {children}
      </Primitive.Popup>
    </Primitive.Positioner>
  </Primitive.Portal>;
}

export {Popover, PopoverTrigger, PopoverContent};
