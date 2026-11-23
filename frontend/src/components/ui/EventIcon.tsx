import {
  AlertTriangle,
  Activity,
  CheckCircle2,
  Play,
  Plus,
  RotateCw,
  Square,
  Trash2,
} from 'lucide-react'
import { cn } from '../../lib/cn'

type EventIconProps = {
  type: string
  className?: string
}

export function EventIcon({ type, className }: EventIconProps) {
  const classes = cn('h-3.5 w-3.5', className)

  switch (type) {
    case 'container.started':
      return <Play className={classes} aria-hidden="true" />
    case 'container.stopped':
      return <Square className={classes} aria-hidden="true" />
    case 'container.restarted':
      return <RotateCw className={classes} aria-hidden="true" />
    case 'container.created':
      return <Plus className={classes} aria-hidden="true" />
    case 'container.removed':
      return <Trash2 className={classes} aria-hidden="true" />
    case 'service.recovered':
      return <CheckCircle2 className={classes} aria-hidden="true" />
    case 'container.died':
    case 'service.failed':
    case 'collector.unavailable':
      return <AlertTriangle className={classes} aria-hidden="true" />
    default:
      return <Activity className={classes} aria-hidden="true" />
  }
}
