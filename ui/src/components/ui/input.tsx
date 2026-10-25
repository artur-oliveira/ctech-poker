// standards:allow @aoctech/ui Input shrinks to 32px under compact density (the app runs compact), which table controls (chat, raise slider) must not do, and paints bg-background instead of the table surface. Gap in ctech-ui: density opt-out on Input.

import {cn} from '@/lib/utils';

export function Input({className, ...props}: React.ComponentProps<'input'>) {
  return <input
    className={cn('h-11 w-full rounded-xl border border-white/15 bg-white/5 px-3 text-sm text-white outline-none transition-[border-color,box-shadow,background-color] duration-200 placeholder:text-[var(--muted-rose)] focus:border-[var(--brand-bright)] focus:ring-3 focus:ring-[var(--focus-ring)]/45 aria-invalid:border-[var(--danger)] aria-invalid:ring-[var(--danger)]/25', className)} {...props}/>;
}
