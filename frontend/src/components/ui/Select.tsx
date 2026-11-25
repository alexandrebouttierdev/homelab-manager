import { ChevronDown } from 'lucide-react'
import type { SelectHTMLAttributes } from 'react'
import { cn } from '../../lib/cn'

type SelectProps = SelectHTMLAttributes<HTMLSelectElement> & {
  containerClassName?: string
}

export function Select({ className, containerClassName, children, ...props }: SelectProps) {
  return (
    <div className={cn('relative inline-flex items-center', containerClassName)}>
      <select
        className={cn(
          'h-9 w-full appearance-none rounded-lg border border-line bg-card py-0 pl-3 pr-8 text-[13px] text-text-primary outline-none transition-colors hover:border-line-strong focus:border-text-muted',
          className,
        )}
        {...props}
      >
        {children}
      </select>
      <ChevronDown
        className="pointer-events-none absolute right-2.5 h-3.5 w-3.5 text-text-muted"
        aria-hidden="true"
      />
    </div>
  )
}
