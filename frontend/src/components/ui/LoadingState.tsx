import { cn } from '../../lib/cn'

type LoadingStateProps = {
  label?: string
  rows?: number
  className?: string
}

export function LoadingState({ label = 'Chargement des données…', rows = 3, className }: LoadingStateProps) {
  return (
    <div className={cn('space-y-3', className)} role="status" aria-live="polite">
      <p className="font-mono text-[11px] uppercase tracking-wider text-text-muted">{label}</p>
      {Array.from({ length: rows }, (_, index) => (
        <div
          key={index}
          className="h-16 animate-pulse rounded-lg border border-line bg-card"
          style={{ animationDelay: `${index * 80}ms` }}
        />
      ))}
    </div>
  )
}

export function InlineLoader({ label = 'Chargement…' }: { label?: string }) {
  return (
    <span className="inline-flex items-center gap-2 font-mono text-[11px] text-text-muted" role="status">
      <span className="h-3 w-3 animate-spin rounded-full border border-line border-t-accent" aria-hidden="true" />
      {label}
    </span>
  )
}
