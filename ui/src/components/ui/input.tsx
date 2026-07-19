import {cn} from '@/lib/utils';

export function Input({className, ...props}: React.ComponentProps<'input'>) {
  return <input
    className={cn('h-10 w-full rounded-xl border border-white/15 bg-white/5 px-3 text-sm text-white outline-none placeholder:text-white/40 focus:border-[#d9464d] focus:ring-3 focus:ring-[#af2a2f]/20 aria-invalid:border-red-500', className)} {...props}/>
}
