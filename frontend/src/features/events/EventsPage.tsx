import { AlertTriangle, Bell, CheckCircle2, Info, RefreshCw, SearchX } from 'lucide-react'
import { type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { CardFooter } from '../../components/ui/Card'
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/DataTable'
import { EmptyState } from '../../components/ui/EmptyState'
import { ErrorState } from '../../components/ui/ErrorState'
import { EventIcon } from '../../components/ui/EventIcon'
import { LoadingState } from '../../components/ui/LoadingState'
import { PageHeader } from '../../components/ui/PageHeader'
import { SearchField } from '../../components/ui/SearchField'
import { SegmentedTabs } from '../../components/ui/SegmentedTabs'
import { Select } from '../../components/ui/Select'
import {
  eventPageSize,
  eventSeverityLabel,
  eventTarget,
  eventTone,
  eventTypeLabel,
} from '../../lib/events'
import { formatDateTime, formatRelativeTime } from '../../lib/format'
import type { MonitorEvent } from '../../lib/api/monitoring'
import { type RangeKey, ranges, useEventsPage } from './useEventsPage'

export function EventsPage() {
  const {
    range,
    setRange,
    severity,
    setSeverity,
    search,
    setSearch,
    events,
    hasMore,
    loadingMore,
    loadMore,
    all,
    counts,
    filtered,
  } = useEventsPage()

  return (
    <div className="space-y-4">
      <PageHeader
        title="Événements"
        breadcrumb={[{ label: 'Activité' }, { label: 'Événements' }]}
        badge={
          <Badge tone={counts.error > 0 ? 'danger' : counts.warning > 0 ? 'warning' : 'success'}>
            {all.length} événement(s)
          </Badge>
        }
        description="Changements d'état des conteneurs détectés par la réconciliation Docker, unités systemd entrées en échec ou rétablies. Les événements de collecteur ne sont pas persistés."
        actions={
          <Button
            size="sm"
            variant="secondary"
            icon={<RefreshCw className="h-3.5 w-3.5" />}
            onClick={() => {
              void events.refetch()
            }}
          >
            Actualiser
          </Button>
        }
      />

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <Summary
          icon={<Bell className="h-3.5 w-3.5" />}
          label="Total"
          value={all.length}
          tone="text-text-primary"
        />
        <Summary
          icon={<Info className="h-3.5 w-3.5" />}
          label="Informations"
          value={counts.info}
          tone="text-info"
        />
        <Summary
          icon={<AlertTriangle className="h-3.5 w-3.5" />}
          label="Avertissements"
          value={counts.warning}
          tone="text-warning"
        />
        <Summary
          icon={<AlertTriangle className="h-3.5 w-3.5" />}
          label="Erreurs"
          value={counts.error}
          tone="text-danger"
        />
      </div>

      <div className="flex flex-wrap items-center gap-2 rounded-lg border border-line bg-card p-2.5">
        <SearchField
          value={search}
          onChange={setSearch}
          placeholder="Rechercher dans les événements…"
          ariaLabel="Rechercher un événement"
          className="min-w-[220px] flex-1 border-0 bg-surface"
        />
        <SegmentedTabs
          ariaLabel="Filtrer par gravité"
          value={severity}
          onChange={setSeverity}
          options={[
            { value: 'all', label: 'Tous', count: all.length },
            { value: 'info', label: 'Info', count: counts.info },
            { value: 'warning', label: 'Avertissement', count: counts.warning },
            { value: 'error', label: 'Erreur', count: counts.error },
          ]}
        />
        <Select
          aria-label="Période des événements"
          value={range}
          onChange={(event) => setRange(event.target.value as RangeKey)}
          className="min-w-[130px]"
        >
          {ranges.map((item) => (
            <option key={item.value} value={item.value}>
              {item.label}
            </option>
          ))}
        </Select>
      </div>

      {events.isLoading && <LoadingState label="Chargement des événements…" rows={4} />}
      {events.isError && (
        <ErrorState
          message={events.error.message}
          onRetry={() => {
            void events.refetch()
          }}
        />
      )}

      {!events.isLoading && !events.isError && all.length === 0 && (
        <EmptyState
          icon={CheckCircle2}
          title="Aucun événement enregistré"
          description="Aucun changement d'état de conteneur ni d'unité systemd n'a été détecté sur la période. Les événements apparaîtront lors des démarrages, arrêts, redémarrages ou échecs."
        />
      )}

      {!events.isLoading && !events.isError && all.length > 0 && filtered.length === 0 && (
        <EmptyState
          icon={SearchX}
          title="Aucun événement ne correspond"
          description="Modifiez la recherche, la gravité ou la période pour retrouver un événement."
          action={
            <Button
              size="sm"
              variant="secondary"
              onClick={() => {
                setSearch('')
                setSeverity('all')
              }}
            >
              Réinitialiser les filtres
            </Button>
          }
        />
      )}

      {filtered.length > 0 && (
        <>
          <Table minWidthClassName="min-w-[900px]">
            <THead>
              <tr>
                <TH>Date</TH>
                <TH>Gravité</TH>
                <TH>Type</TH>
                <TH>Message</TH>
                <TH>Cible</TH>
                <TH>Ancienneté</TH>
              </tr>
            </THead>
            <TBody>
              {filtered.map((event) => (
                <TR key={event.id}>
                  <TD className="font-mono text-xs">{formatDateTime(event.occurredAt)}</TD>
                  <TD>
                    <Badge tone={eventTone(event.severity)} dot>
                      {eventSeverityLabel(event.severity)}
                    </Badge>
                  </TD>
                  <TD>
                    <span className="flex items-center gap-2 text-xs font-medium text-text-primary">
                      <EventIcon type={event.type} className="text-text-muted" />
                      {eventTypeLabel(event.type)}
                    </span>
                  </TD>
                  <TD>
                    <span className="block max-w-[420px] text-xs">{event.message || '—'}</span>
                  </TD>
                  <TD>
                    <EventTargetLink event={event} />
                  </TD>
                  <TD className="font-mono text-[10px]">{formatRelativeTime(event.occurredAt)}</TD>
                </TR>
              ))}
            </TBody>
          </Table>
          <CardFooter className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-line bg-card font-mono text-[10px] text-text-muted">
            <span>
              {filtered.length} sur {all.length} événement(s)
            </span>
            <span>Ordre antéchronologique · {eventPageSize} événements par page</span>
            {hasMore && (
              <Button variant="secondary" size="sm" disabled={loadingMore} onClick={loadMore}>
                {loadingMore ? 'Chargement…' : 'Charger plus'}
              </Button>
            )}
          </CardFooter>
        </>
      )}
    </div>
  )
}

function Summary({
  icon,
  label,
  value,
  tone,
}: {
  icon: ReactNode
  label: string
  value: number
  tone: string
}) {
  return (
    <div className="flex items-start justify-between gap-3 rounded-lg border border-line bg-card p-4">
      <div>
        <p className="font-mono text-[11px] uppercase tracking-wider text-text-muted">{label}</p>
        <p className={`mt-1 font-mono text-2xl font-semibold tracking-tight ${tone}`}>{value}</p>
      </div>
      <span className="flex h-7 w-7 items-center justify-center rounded-md border border-line bg-elevated text-text-muted">
        {icon}
      </span>
    </div>
  )
}

function EventTargetLink({ event }: { event: MonitorEvent }) {
  const target = eventTarget(event)
  if (!target) return <span className="font-mono text-[10px] text-text-muted">—</span>
  return (
    <Link
      to={target.to}
      className="font-mono text-[10px] text-accent transition-colors hover:underline"
    >
      {target.label}
    </Link>
  )
}
