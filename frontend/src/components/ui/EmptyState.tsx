import { Inbox, type LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { cn } from '../../lib/cn'
import { Card } from './Card'

type EmptyStateProps = {
  icon?: LucideIcon
  title: string
  description?: ReactNode
  action?: ReactNode
  className?: string
}

export function EmptyState({
  icon: Icon = Inbox,
  title,
  description,
  action,
  className,
}: EmptyStateProps) {
  return (
    <Card className={cn('flex flex-col items-center justify-center px-6 py-12 text-center', className)}>
      <span className="mb-3 flex h-11 w-11 items-center justify-center rounded-lg border border-line bg-elevated text-text-muted">
        <Icon className="h-5 w-5" aria-hidden="true" />
      </span>
      <h2 className="text-sm font-semibold text-text-primary">{title}</h2>
      {description && <p className="mt-1 max-w-md text-sm text-text-secondary">{description}</p>}
      {action && <div className="mt-4">{action}</div>}
    </Card>
  )
}
