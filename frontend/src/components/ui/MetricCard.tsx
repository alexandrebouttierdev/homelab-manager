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
  /** `plain` drops the frame, for cells of a shared strip. */
  variant?: 'card' | 'plain'
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
  variant = 'card',
  className,
}: MetricCardProps) {
  return (
    <div
      className={cn(
        'flex min-w-0 flex-col gap-3 bg-card px-5 pb-4 pt-[18px]',
        variant === 'card' && 'rounded-xl border border-line',
        className,
      )}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-[13px] font-medium text-text-secondary">{label}</p>
          {title && <p className="mt-0.5 truncate text-xs text-text-muted">{title}</p>}
        </div>
        {badge && <div className="shrink-0">{badge}</div>}
      </div>

      <div className="flex flex-wrap items-end justify-between gap-2">
        <div className="flex min-w-0 items-baseline gap-1.5">
          <span className="truncate text-[32px] font-semibold leading-none tracking-[-0.03em] text-text-primary">
            {value}
          </span>
          {unit && <span className="shrink-0 text-[15px] text-text-muted">{unit}</span>}
        </div>
        {aside && <div className="text-right">{aside}</div>}
      </div>

      {hint && <div className="text-[13px] text-text-muted">{hint}</div>}
      {children}
      {footer && <div className="mt-auto">{footer}</div>}
    </div>
  )
}
