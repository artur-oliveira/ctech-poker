import {Button as Primitive} from '@base-ui/react/button';
import {cva, type VariantProps} from 'class-variance-authority';
import {cn} from '@/lib/utils'

const variants = cva('inline-flex items-center justify-center gap-2 rounded-xl text-sm font-semibold transition-all outline-none focus-visible:ring-3 focus-visible:ring-[#af2a2f]/40 disabled:pointer-events-none disabled:opacity-50 [&_svg]:size-4', {
  variants: {
    variant: {
      default: 'bg-[#af2a2f] text-white hover:bg-[#c7353b] shadow-lg shadow-[#af2a2f]/20',
      outline: 'border border-white/20 bg-white/5 text-white hover:bg-white/10',
      ghost: 'text-white hover:bg-white/10',
      light: 'bg-[#f6f0e7] text-[#5b1218] hover:bg-white',
      destructive: 'bg-red-600 text-white hover:bg-red-500'
    }, size: {default: 'h-10 px-4', sm: 'h-8 px-3', lg: 'h-12 px-6', icon: 'size-10'}
  }, defaultVariants: {variant: 'default', size: 'default'}
})

function Button({className, variant, size, nativeButton, ...props}: Primitive.Props & VariantProps<typeof variants>) {
  return <Primitive nativeButton={nativeButton ?? !props.render}
                    className={cn(variants({variant, size}), className)} {...props}/>
};
export {Button, variants as buttonVariants}
