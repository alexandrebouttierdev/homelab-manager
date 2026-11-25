import { collectorStatusLabel, collectorStatusTone } from '../../lib/collectors'
import { formatRelativeTime } from '../../lib/format'
import { StatusDot } from './StatusDot'

/** One collector with its translated state and last successful cycle. */
export function CollectorRow({
  name,
  status,
  lastSuccessAt,
}: {
  name: string
  status?: string
  lastSuccessAt?: string | null
}) {
  const tone = status ? collectorStatusTone(status) : 'neutral'
  return (
    <div className="flex items-center justify-between gap-3 rounded-md border border-line/70 bg-surface/50 px-3 py-2.5">
      <div className="flex items-center gap-2">
        <StatusDot tone={tone} pulse={status === 'ok'} />
        <span className="text-xs font-medium text-text-primary">{name}</span>
      </div>
      <div className="text-right text-xs text-text-muted">
        <span className="block text-text-secondary">
          {status ? collectorStatusLabel(status) : 'Inconnu'}
        </span>
        <span>
          {lastSuccessAt ? `Dernier succès ${formatRelativeTime(lastSuccessAt)}` : 'Aucun succès'}
        </span>
      </div>
    </div>
  )
}
