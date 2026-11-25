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
      className={cn(
        'inline-flex flex-wrap items-center gap-0.5 rounded-lg border border-line bg-surface p-0.5',
        className,
      )}
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
              'inline-flex h-7 items-center gap-2 rounded-md px-3 text-[13px] transition-colors',
              active
                ? 'bg-elevated font-medium text-text-primary'
                : 'text-text-muted hover:text-text-primary',
            )}
          >
            {option.label}
            {option.count != null && (
              <span className={cn('text-xs', active ? 'text-text-secondary' : 'text-text-muted')}>
                {option.count}
              </span>
            )}
          </button>
        )
      })}
    </div>
  )
}
