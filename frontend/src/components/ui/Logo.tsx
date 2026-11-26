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
        <g
          className="stroke-text-primary"
          strokeWidth="1.8"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <rect x="8" y="8.5" width="16" height="6.5" rx="1.5" />
          <rect x="8" y="17" width="16" height="6.5" rx="1.5" />
        </g>
        <circle cx="11.5" cy="11.75" r="1.3" className="fill-success" />
        <circle cx="11.5" cy="20.25" r="1.3" className="fill-success" />
      </svg>
      {!compact && (
        <span className="min-w-0 truncate text-sm font-semibold tracking-[-0.01em] text-text-primary">
          MonoOps
        </span>
      )}
    </Link>
  )
}
