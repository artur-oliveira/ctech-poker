import {cn} from '@/lib/utils';

export function Input({className, ...props}: React.ComponentProps<'input'>) {
  return <input
    className={cn('h-10 w-full rounded-xl border border-white/15 bg-white/5 px-3 text-sm text-white outline-none placeholder:text-white/40 focus:border-[var(--brand-bright)] focus:ring-2 focus:ring-[var(--brand)]/20 aria-invalid:border-red-500', className)} {...props}/>
}
