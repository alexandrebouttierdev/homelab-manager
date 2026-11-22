import { cn } from '../../lib/cn'

export type SegmentedOption<T extends string | number> = {
  value: T
  label: string
  count?: number
}

type SegmentedTabsProps<T extends string | number> = {
  options: ReadonlyArray<SegmentedOption<T>>
  value: T
  onChange: (value: T) => void
  ariaLabel: string
  className?: string
}

export function SegmentedTabs<T extends string | number>({
  options,
  value,
  onChange,
  ariaLabel,
  className,
}: SegmentedTabsProps<T>) {
  return (
    <div
      role="group"
      aria-label={ariaLabel}
      className={cn('inline-flex flex-wrap items-center gap-1 rounded-lg border border-line bg-card p-1', className)}
    >
      {options.map((option) => {
        const active = option.value === value
        return (
          <button
            key={option.value}
            type="button"
            aria-pressed={active}
            onClick={() => onChange(option.value)}
            className={cn(
              'inline-flex items-center gap-2 rounded-md px-3 py-1.5 text-xs font-medium transition-colors',
              active
                ? 'bg-elevated text-text-primary shadow-sm'
                : 'text-text-secondary hover:bg-elevated/60 hover:text-text-primary',
            )}
          >
            {option.label}
            {option.count != null && (
              <span className={cn(
                'rounded-sm px-1 font-mono text-[10px]',
                active ? 'bg-accent/15 text-accent' : 'bg-elevated text-text-muted',
              )}>
                {option.count}
              </span>
            )}
          </button>
        )
      })}
    </div>
  )
}
