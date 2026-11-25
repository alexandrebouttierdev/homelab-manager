import { cn } from '../../lib/cn'

export type ProgressTone = 'accent' | 'success' | 'warning' | 'danger' | 'cpu' | 'ram' | 'net'

const tones: Record<ProgressTone, string> = {
  accent: 'bg-accent',
  success: 'bg-success',
  warning: 'bg-warning',
  danger: 'bg-danger',
  cpu: 'bg-cpu',
  ram: 'bg-ram',
  net: 'bg-net',
}

type ProgressBarProps = {
  value: number | null | undefined
  tone?: ProgressTone
  className?: string
  label?: string
}

export function ProgressBar({ value, tone = 'accent', className, label }: ProgressBarProps) {
  const clamped = value == null || !Number.isFinite(value) ? 0 : Math.min(Math.max(value, 0), 100)

  return (
    <div
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={value == null ? undefined : Math.round(clamped)}
      className={cn('h-1 w-full overflow-hidden rounded-full bg-line', className)}
    >
      <div
        className={cn('h-full rounded-full transition-[width] duration-500', tones[tone])}
        style={{ width: `${clamped}%` }}
      />
    </div>
  )
}
