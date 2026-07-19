'use client'

import {Select as SelectPrimitive} from '@base-ui/react/select'
import {Check, ChevronDown} from 'lucide-react'
import {cn} from '@/lib/utils'

const Select = SelectPrimitive.Root

function SelectValue({className, ...props}: SelectPrimitive.Value.Props) {
  return <SelectPrimitive.Value data-slot="select-value" className={cn('flex flex-1 text-left', className)} {...props}/>
}

function SelectTrigger({className, children, ...props}: SelectPrimitive.Trigger.Props) {
  return <SelectPrimitive.Trigger data-slot="select-trigger" className={cn('flex h-10 w-full items-center justify-between gap-2 rounded-xl border border-white/15 bg-[#211416] px-3 text-sm text-white outline-none transition-colors focus-visible:border-[#df5a61] focus-visible:ring-3 focus-visible:ring-[#af2a2f]/30 disabled:cursor-not-allowed disabled:opacity-50', className)} {...props}>
    {children}
    <SelectPrimitive.Icon render={<ChevronDown className="size-4 text-[#ad9fa0]"/>}/>
  </SelectPrimitive.Trigger>
}

function SelectContent({className, children, side = 'bottom', sideOffset = 6, align = 'start', ...props}: SelectPrimitive.Popup.Props & Pick<SelectPrimitive.Positioner.Props, 'align' | 'side' | 'sideOffset'>) {
  return <SelectPrimitive.Portal>
    <SelectPrimitive.Positioner side={side} sideOffset={sideOffset} align={align} className="isolate z-50">
      <SelectPrimitive.Popup data-slot="select-content" className={cn('relative z-50 max-h-(--available-height) w-(--anchor-width) min-w-40 overflow-y-auto rounded-xl bg-[#211416] p-1 text-white shadow-2xl ring-1 ring-white/15 duration-100 data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95', className)} {...props}>
        <SelectPrimitive.List>{children}</SelectPrimitive.List>
      </SelectPrimitive.Popup>
    </SelectPrimitive.Positioner>
  </SelectPrimitive.Portal>
}

function SelectItem({className, children, ...props}: SelectPrimitive.Item.Props) {
  return <SelectPrimitive.Item data-slot="select-item" className={cn('relative flex w-full cursor-default items-center rounded-lg py-2 pr-8 pl-2 text-sm outline-none select-none focus:bg-white/10 data-disabled:pointer-events-none data-disabled:opacity-50', className)} {...props}>
    <SelectPrimitive.ItemText className="flex-1">{children}</SelectPrimitive.ItemText>
    <SelectPrimitive.ItemIndicator render={<span className="absolute right-2 flex size-4 items-center justify-center"/>}><Check className="size-4 text-[#df5a61]"/></SelectPrimitive.ItemIndicator>
  </SelectPrimitive.Item>
}

export {Select, SelectContent, SelectItem, SelectTrigger, SelectValue}
