import { ChevronRight } from 'lucide-react'
import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { cn } from '../../lib/cn'

export type BreadcrumbItem = {
  label: string
  to?: string
}

type PageHeaderProps = {
  title: string
  description?: ReactNode
  breadcrumb?: BreadcrumbItem[]
  badge?: ReactNode
  actions?: ReactNode
  className?: string
}

export function PageHeader({
  title,
  description,
  breadcrumb,
  badge,
  actions,
  className,
}: PageHeaderProps) {
  return (
    <header className={cn('flex flex-wrap items-start justify-between gap-4', className)}>
      <div className="min-w-0">
        {breadcrumb && breadcrumb.length > 0 && (
          <nav
            aria-label="Fil d'Ariane"
            className="mb-2 flex flex-wrap items-center gap-1 text-[13px] text-text-muted"
          >
            {breadcrumb.map((item, index) => (
              <span key={`${item.label}-${index}`} className="inline-flex items-center gap-1">
                {index > 0 && (
                  <ChevronRight className="h-3 w-3 text-text-muted/60" aria-hidden="true" />
                )}
                {item.to ? (
                  <Link to={item.to} className="transition-colors hover:text-text-secondary">
                    {item.label}
                  </Link>
                ) : (
                  <span className="text-text-secondary">{item.label}</span>
                )}
              </span>
            ))}
          </nav>
        )}
        <div className="flex flex-wrap items-center gap-2.5">
          <h1 className="text-[28px] font-semibold leading-tight tracking-[-0.025em] text-text-primary">
            {title}
          </h1>
          {badge}
        </div>
        {description && (
          <p className="mt-1.5 max-w-3xl text-[13.5px] leading-relaxed text-text-muted">
            {description}
          </p>
        )}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </header>
  )
}
