import { ChevronRight, ShieldAlert } from 'lucide-react'
import { Link } from 'react-router-dom'
import { Badge } from '../../components/ui/Badge'
import { Card, CardBody, CardFooter, CardHeader } from '../../components/ui/Card'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { formatCount } from '../../lib/format'
import { useVulnerabilitiesCard } from './useVulnerabilitiesCard'

export function VulnerabilitiesCard() {
  const card = useVulnerabilitiesCard()
  return (
    <Card>
      <CardHeader
        title="Vulnérabilités"
        subtitle="Paquets système de la machine"
        icon={<ShieldAlert className="h-3.5 w-3.5" />}
        actions={
          card.status && (
            <Badge tone={card.status.tone} dot>
              {card.status.label}
            </Badge>
          )
        }
      />
      <CardBody className="space-y-3">
        {card.state.isLoading && <LoadingState label="Chargement…" rows={1} />}
        {card.state.isError && (
          <ErrorState message={card.state.error.message} onRetry={card.state.refetch} />
        )}
        {card.empty === 'never' && (
          <p className="text-xs text-text-secondary">Aucune analyse encore.</p>
        )}
        {card.empty === 'clean' && (
          <p className="text-xs text-success">Aucune vulnérabilité connue dans les paquets.</p>
        )}
        {card.status?.failure && (
          <p className="text-xs text-danger" role="alert">
            Dernière analyse en échec ({card.status.failure}).
          </p>
        )}
        {card.counts.length > 0 && card.empty !== 'never' && (
          <div className="grid grid-cols-4 gap-2">
            {card.counts.map((count) => (
              <div key={count.severity} className="rounded-md border border-line/70 px-2 py-1.5">
                <p className="text-xs text-text-muted">{count.label}</p>
                <p className="text-lg font-semibold tracking-[-0.02em] text-text-primary">
                  {formatCount(count.value)}
                </p>
              </div>
            ))}
          </div>
        )}
        {card.recent.length > 0 && (
          <ul className="space-y-1.5">
            {card.recent.map((finding) => (
              <li key={finding.id} className="flex items-center justify-between gap-2 text-xs">
                <span className="min-w-0 truncate">
                  <span className="font-mono text-text-primary">{finding.vulnerabilityId}</span>
                  <span className="text-text-muted"> · {finding.package}</span>
                </span>
                <span className="flex shrink-0 items-center gap-2">
                  <Badge tone={finding.severity.tone}>{finding.severity.label}</Badge>
                  <span className="text-xs text-text-muted">{finding.found}</span>
                </span>
              </li>
            ))}
          </ul>
        )}
      </CardBody>
      <CardFooter className="flex justify-end text-xs text-text-muted">
        <Link
          to={card.link}
          className="inline-flex items-center gap-1 text-text-secondary hover:text-text-primary"
        >
          {card.total > 0 ? `Voir les ${formatCount(card.total)} vulnérabilités` : 'Voir l’analyse'}
          <ChevronRight className="h-3 w-3" />
        </Link>
      </CardFooter>
    </Card>
  )
}
