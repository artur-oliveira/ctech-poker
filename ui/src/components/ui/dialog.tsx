'use client';
import {Dialog as Primitive} from '@base-ui/react/dialog';
import {X} from 'lucide-react';
import {cn} from '@/lib/utils';
import {Button} from './button'

const Dialog = Primitive.Root, DialogTrigger = Primitive.Trigger, DialogClose = Primitive.Close

function DialogContent({className, children, ...props}: Primitive.Popup.Props) {
  return <Primitive.Portal><Primitive.Backdrop
    className="fixed inset-0 z-50 bg-black/75 backdrop-blur-sm data-open:animate-in data-closed:animate-out"/><Primitive.Popup
    className={cn('fixed left-1/2 top-1/2 z-50 w-[calc(100%-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2 rounded-2xl border border-white/15 bg-[var(--surface-control)] p-6 text-[var(--on-brand)] shadow-2xl outline-none', className)} {...props}>{children}<Primitive.Close
    render={<Button variant="ghost" size="icon" className="absolute right-3 top-3"/>}><X/><span
    className="sr-only">Fechar</span></Primitive.Close></Primitive.Popup></Primitive.Portal>
}

function DialogHeader(p: React.ComponentProps<'div'>) {
  return <div {...p} className={cn('mb-5 space-y-2', p.className)}/>
}

function DialogTitle(p: Primitive.Title.Props) {
  return <Primitive.Title {...p} className={cn('text-2xl font-bold', p.className)}/>
}

function DialogDescription(p: Primitive.Description.Props) {
  return <Primitive.Description {...p} className={cn('text-sm text-[var(--muted-rose)]', p.className)}/>
}

function DialogFooter(p: React.ComponentProps<'div'>) {
  return <div {...p} className={cn('mt-6 flex justify-end gap-2', p.className)}/>
}

export {Dialog, DialogTrigger, DialogClose, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter}
