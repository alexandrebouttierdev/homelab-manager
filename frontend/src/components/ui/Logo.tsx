import { Link } from 'react-router-dom'
import { cn } from '../../lib/cn'

type LogoProps = {
  compact?: boolean
  to?: string
  className?: string
}

export function Logo({ compact = false, to = '/', className }: LogoProps) {
  return (
    <Link
      to={to}
      className={cn('group flex items-center gap-2.5', className)}
      aria-label="MonoOps — accueil"
    >
      <svg
        viewBox="0 0 32 32"
        fill="none"
        className="h-[30px] w-[30px] shrink-0"
        aria-hidden="true"
      >
        <rect
          x="1"
          y="1"
          width="30"
          height="30"
          rx="8"
          className="fill-elevated stroke-line-strong transition-colors group-hover:stroke-text-muted"
        />
        {/* The mark: one ring (the single machine) with its status light. */}
        <circle cx="16" cy="16" r="7.7" strokeWidth="3.1" className="stroke-text-primary" />
        <circle cx="21.45" cy="10.55" r="3.8" className="fill-elevated" />
        <circle cx="21.45" cy="10.55" r="2.8" className="fill-success" />
      </svg>
      {!compact && (
        <span className="min-w-0 truncate text-sm font-semibold tracking-[-0.01em] text-text-primary">
          MonoOps
        </span>
      )}
    </Link>
  )
}
