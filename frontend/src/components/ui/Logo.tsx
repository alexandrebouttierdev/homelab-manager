import { Server } from 'lucide-react'
import { Link } from 'react-router-dom'
import { cn } from '../../lib/cn'

type LogoProps = {
  compact?: boolean
  to?: string
  className?: string
}

export function Logo({ compact = false, to = '/', className }: LogoProps) {
  return (
    <Link to={to} className={cn('group flex items-center gap-2.5', className)} aria-label="Homelab Manager — accueil">
      <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md border border-line bg-elevated text-accent transition-colors group-hover:border-accent/40">
        <Server className="h-4 w-4" aria-hidden="true" />
      </span>
      {!compact && (
        <span className="min-w-0 leading-tight">
          <span className="block truncate text-sm font-semibold tracking-tight text-text-primary">Homelab Manager</span>
          <span className="block font-mono text-[10px] uppercase tracking-wider text-text-muted">Supervision locale</span>
        </span>
      )}
    </Link>
  )
}
