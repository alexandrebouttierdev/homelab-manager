import type { ReactNode } from 'react'
import { cn } from '../../lib/cn'

type MetricCardProps = {
  label: string
  title?: ReactNode
  value: ReactNode
  unit?: ReactNode
  hint?: ReactNode
  badge?: ReactNode
  aside?: ReactNode
  children?: ReactNode
  footer?: ReactNode
  className?: string
}

export function MetricCard({
  label,
  title,
  value,
  unit,
  hint,
  badge,
  aside,
  children,
  footer,
  className,
}: MetricCardProps) {
  return (
    <div
      className={cn(
        'flex min-w-0 flex-col gap-3 rounded-lg border border-line bg-card p-4',
        className,
      )}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="font-mono text-[11px] uppercase tracking-wider text-text-muted">{label}</p>
          {title && (
            <p className="mt-0.5 truncate text-xs font-medium text-text-primary">{title}</p>
          )}
        </div>
        {badge && <div className="shrink-0">{badge}</div>}
      </div>

      <div className="flex flex-wrap items-end justify-between gap-2">
        <div className="flex min-w-0 items-baseline gap-1.5">
          <span className="truncate font-mono text-2xl font-semibold tracking-tight text-text-primary">
            {value}
          </span>
          {unit && <span className="shrink-0 font-mono text-xs text-text-muted">{unit}</span>}
        </div>
        {aside && <div className="text-right">{aside}</div>}
      </div>

      {hint && <div className="font-mono text-[11px] text-text-muted">{hint}</div>}
      {children}
      {footer && <div className="mt-auto">{footer}</div>}
    </div>
  )
}
