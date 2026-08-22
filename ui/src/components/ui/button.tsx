import {Button as Primitive} from '@base-ui/react/button';
import {cva, type VariantProps} from 'class-variance-authority';
import {cn} from '@/lib/utils';

const variants = cva('inline-flex touch-manipulation items-center justify-center gap-2 rounded-xl text-sm font-semibold outline-none transition-[color,background-color,border-color,box-shadow,transform] duration-200 ease-[var(--ease-out-quart)] focus-visible:ring-3 focus-visible:ring-[var(--focus-ring)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--ink)] active:translate-y-px disabled:pointer-events-none disabled:opacity-50 disabled:active:translate-y-0 [&_svg]:size-4', {
  variants: {
    variant: {
      default: 'bg-[var(--brand)] text-[var(--on-brand)] hover:bg-[var(--brand-bright)] shadow-lg shadow-[var(--brand)]/20',
      outline: 'border border-white/20 bg-white/5 text-[var(--on-brand)] hover:bg-white/10',
      ghost: 'text-[var(--on-brand)] hover:bg-white/10',
      light: 'bg-[var(--paper)] text-[var(--wine)] hover:bg-[var(--on-brand)]',
      destructive: 'bg-[var(--danger)] text-[var(--on-brand)] hover:bg-red-500'
    }, size: {default: 'h-11 px-4', sm: 'h-11 px-3', lg: 'h-12 px-6', icon: 'size-11'}
  }, defaultVariants: {variant: 'default', size: 'default'}
});

function Button({className, variant, size, nativeButton, ...props}: Primitive.Props & VariantProps<typeof variants>) {
  return <Primitive nativeButton={nativeButton ?? !props.render}
                    className={cn(variants({variant, size}), className)} {...props}/>;
}

export {Button, variants as buttonVariants};
