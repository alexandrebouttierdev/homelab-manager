import { AlertTriangle, RefreshCw } from 'lucide-react'
import { Button } from './Button'
import { Card } from './Card'

type ErrorStateProps = {
  message: string
  title?: string
  onRetry?: () => void
}

export function ErrorState({ message, title = 'Données indisponibles', onRetry }: ErrorStateProps) {
  return (
    <Card className="flex flex-wrap items-center justify-between gap-4 border-danger/30 bg-danger/5 px-4 py-3">
      <div className="flex min-w-0 items-start gap-3">
        <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-danger" aria-hidden="true" />
        <div className="min-w-0">
          <p className="text-sm font-medium text-text-primary">{title}</p>
          <p className="mt-0.5 text-xs text-text-secondary" role="alert">
            {message}
          </p>
        </div>
      </div>
      {onRetry && (
        <Button
          size="sm"
          variant="secondary"
          icon={<RefreshCw className="h-3.5 w-3.5" />}
          onClick={onRetry}
        >
          Réessayer
        </Button>
      )}
    </Card>
  )
}
