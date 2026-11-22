import { cn } from '../../lib/cn'

export type StatusTone = 'success' | 'warning' | 'danger' | 'neutral' | 'accent' | 'info'

const tones: Record<StatusTone, string> = {
  success: 'bg-success',
  warning: 'bg-warning',
  danger: 'bg-danger',
  neutral: 'bg-text-muted',
  accent: 'bg-accent',
  info: 'bg-info',
}

type StatusDotProps = {
  tone?: StatusTone
  pulse?: boolean
  className?: string
  label?: string
}

export function StatusDot({ tone = 'success', pulse = false, className, label }: StatusDotProps) {
  return (
    <span
      className={cn('relative inline-flex h-2 w-2 shrink-0', className)}
      title={label}
      aria-label={label}
    >
      {pulse && (
        <span
          className={cn(
            'absolute inline-flex h-full w-full animate-ping rounded-full opacity-60',
            tones[tone],
          )}
        />
      )}
      <span className={cn('relative inline-flex h-2 w-2 rounded-full', tones[tone])} />
    </span>
  )
}
