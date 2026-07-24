import {cn} from '@/lib/utils';

export function Label({className, ...props}: React.ComponentProps<'label'>) {
  return <label className={cn('text-sm font-medium text-[var(--paper)]', className)} {...props}/>;
}
