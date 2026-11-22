import {
  Activity,
  ArrowLeft,
  Container as ContainerIcon,
  ExternalLink,
  HardDrive,
  MemoryStick,
  Network,
  RefreshCw,
  RotateCw,
} from 'lucide-react'
import { type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { Chart } from '../../components/charts/Chart'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardFooter, CardHeader } from '../../components/ui/Card'
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/DataTable'
import { EmptyState } from '../../components/ui/EmptyState'
import { ErrorState } from '../../components/ui/ErrorState'
import { EventIcon } from '../../components/ui/EventIcon'
import { LoadingState } from '../../components/ui/LoadingState'
import { MetricCard } from '../../components/ui/MetricCard'
import { PageHeader } from '../../components/ui/PageHeader'
import { ProgressBar } from '../../components/ui/ProgressBar'
import { SegmentedTabs } from '../../components/ui/SegmentedTabs'
import { StatusDot } from '../../components/ui/StatusDot'
import {
  containerStateLabel,
  containerStateTone,
  isRunning,
  mountField,
  shortId,
} from '../../lib/containers'
import {
  formatBytes,
  formatDateTime,
  formatPercent,
  formatRate,
  formatRelativeTime,
  formatUptime,
} from '../../lib/format'
import { eventSeverityLabel, eventTone, eventTypeLabel } from '../../lib/events'
import { ranges, useContainerDetailPage } from './useContainerDetailPage'

export function ContainerDetailPage() {
  const {
    queryClient,
    containerID,
    range,
    setRange,
    containerState,
    eventList,
    cpuHistory,
    memoryHistory,
    events,
    cpu,
    memoryUsage,
    memoryLimit,
    memoryPercent,
    processes,
    rxRate,
    txRate,
    rxBytes,
    txBytes,
    cpuSeries,
    memorySeries,
    chartOption,
    data,
    tag,
    ports,
  } = useContainerDetailPage()

  return (
    <div className="space-y-4">
      <PageHeader
        title={data?.name ?? 'Détail du conteneur'}
        breadcrumb={[
          { label: 'Docker', to: '/containers' },
          { label: 'Conteneurs', to: '/containers' },
          { label: data?.name ?? 'Détail' },
        ]}
        badge={
          data ? (
            <>
              <Badge tone={containerStateTone(data.state)} dot>
                {containerStateLabel(data.state)}
              </Badge>
              {data.removedAt && <Badge tone="danger">Supprimé</Badge>}
            </>
          ) : undefined
        }
        description={
          data ? (
            <>
              <span className="font-mono">{data.image}</span>
              {' · '}
              <span className="font-mono">{shortId(data.dockerId, 24)}</span>
              {' · observé '}
              {formatRelativeTime(data.lastObservedAt)}
            </>
          ) : (
            'Informations détaillées du conteneur Docker local.'
          )
        }
        actions={
          <>
            <Link
              to="/containers"
              className="inline-flex h-8 items-center gap-1.5 rounded-md border border-line bg-card px-2.5 text-xs font-medium text-text-secondary transition-colors hover:bg-elevated hover:text-text-primary"
            >
              <ArrowLeft className="h-3.5 w-3.5" />
              Retour
            </Link>
            <Button
              size="sm"
              variant="secondary"
              icon={<RefreshCw className="h-3.5 w-3.5" />}
              onClick={() => {
                void queryClient.invalidateQueries({ queryKey: ['container', containerID] })
              }}
            >
              Rafraîchir
            </Button>
          </>
        }
      />

      {containerState.isLoading && <LoadingState label="Chargement du conteneur…" rows={2} />}
      {containerState.isError && (
        <ErrorState
          message={containerState.error.message}
          onRetry={() => {
            void containerState.refetch()
          }}
        />
      )}

      {data && (
        <>
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <MetricCard
              label="Processeur"
              title="Charge courante"
              value={formatPercent(cpu)}
              badge={<Badge tone="info">CPU</Badge>}
              hint={
                isRunning(data.state) ? 'Statistiques Docker en direct' : 'Conteneur non démarré'
              }
            >
              <ProgressBar value={cpu} tone="cpu" label="Utilisation CPU" />
            </MetricCard>

            <MetricCard
              label="Mémoire"
              title={memoryLimit ? `${formatBytes(memoryLimit)} de limite` : 'Mémoire du conteneur'}
              value={formatBytes(memoryUsage)}
              unit={memoryLimit ? `/ ${formatBytes(memoryLimit)}` : undefined}
              badge={<Badge tone="accent">{formatPercent(memoryPercent)}</Badge>}
              hint={`Processus : ${processes ?? '—'}`}
            >
              <ProgressBar value={memoryPercent} tone="ram" label="Utilisation mémoire" />
            </MetricCard>

            <MetricCard
              label="Réseau"
              title="Débit courant"
              value={formatRate(rxRate)}
              unit="RX"
              badge={<Badge tone="neutral">TX {formatRate(txRate)}</Badge>}
              hint={`Cumul RX ${formatBytes(rxBytes)} · TX ${formatBytes(txBytes)}`}
            />

            <MetricCard
              label="Disponibilité"
              title={data.restartPolicy ? `Politique ${data.restartPolicy}` : 'Politique inconnue'}
              value={isRunning(data.state) ? formatUptime(data.uptimeSeconds) : '—'}
              badge={
                <Badge tone={data.restartCount > 0 ? 'warning' : 'neutral'}>
                  {data.restartCount} redémarrage(s)
                </Badge>
              }
              hint={
                data.startedAt
                  ? `Démarré ${formatRelativeTime(data.startedAt)}`
                  : 'Date de démarrage indisponible'
              }
            />
          </div>

          <Card>
            <CardHeader
              title="Historique CPU & mémoire"
              subtitle={`${range.replace('h', ' heure(s)')} · échantillons bruts`}
              icon={<Activity className="h-3.5 w-3.5" />}
              actions={
                <SegmentedTabs
                  ariaLabel="Période de l'historique"
                  value={range}
                  onChange={setRange}
                  options={ranges.map((item) => ({ value: item.value, label: item.label }))}
                />
              }
            />
            <CardBody>
              {cpuHistory.isError || memoryHistory.isError ? (
                <ErrorState
                  message={
                    (cpuHistory.error ?? memoryHistory.error)?.message ?? 'Historique indisponible.'
                  }
                />
              ) : cpuSeries.length === 0 && memorySeries.length === 0 ? (
                <p className="py-8 text-center text-sm text-text-muted">
                  Aucun échantillon sur la période sélectionnée.
                </p>
              ) : (
                <Chart
                  option={chartOption}
                  height={300}
                  label="Historique CPU et mémoire du conteneur"
                />
              )}
            </CardBody>
            <CardFooter className="flex flex-wrap items-center justify-between gap-2 font-mono text-[10px] text-text-muted">
              <span>
                {cpuSeries.length} point(s) CPU · {memorySeries.length} point(s) mémoire
              </span>
              <span>Les valeurs sont des échantillons bruts, sans agrégation.</span>
            </CardFooter>
          </Card>

          <div className="grid items-start gap-4 xl:grid-cols-2">
            <Card>
              <CardHeader
                title="Configuration observée"
                icon={<ContainerIcon className="h-3.5 w-3.5" />}
              />
              <CardBody>
                <dl className="grid gap-3 sm:grid-cols-2">
                  <DetailItem
                    label="Image"
                    value={<span className="font-mono">{data.image}</span>}
                  />
                  <DetailItem label="Tag" value={<Badge tone="neutral">{tag?.tag ?? '—'}</Badge>} />
                  <DetailItem
                    label="ID Docker"
                    value={<span className="font-mono">{shortId(data.dockerId, 24)}</span>}
                  />
                  <DetailItem
                    label="État"
                    value={
                      <Badge tone={containerStateTone(data.state)}>
                        {containerStateLabel(data.state)}
                      </Badge>
                    }
                  />
                  <DetailItem label="Créé le" value={formatDateTime(data.createdAt)} />
                  <DetailItem label="Démarré le" value={formatDateTime(data.startedAt)} />
                  <DetailItem
                    label="Dernière observation"
                    value={formatDateTime(data.lastObservedAt)}
                  />
                  <DetailItem
                    label="Politique de redémarrage"
                    value={<span className="font-mono">{data.restartPolicy || '—'}</span>}
                  />
                </dl>
              </CardBody>
            </Card>

            <Card>
              <CardHeader title="Réseaux & ports" icon={<Network className="h-3.5 w-3.5" />} />
              <CardBody className="space-y-4">
                <div>
                  <p className="mb-2 font-mono text-[10px] uppercase tracking-wider text-text-muted">
                    Réseaux attachés
                  </p>
                  {data.networks.length === 0 ? (
                    <p className="text-sm text-text-muted">Aucun réseau rapporté.</p>
                  ) : (
                    <div className="flex flex-wrap gap-1.5">
                      {data.networks.map((network) => (
                        <Badge key={network} tone="neutral">
                          {network}
                        </Badge>
                      ))}
                    </div>
                  )}
                </div>
                <div>
                  <p className="mb-2 font-mono text-[10px] uppercase tracking-wider text-text-muted">
                    Ports publiés
                  </p>
                  {ports.length === 0 ? (
                    <p className="text-sm text-text-muted">Aucun port publié.</p>
                  ) : (
                    <ul className="space-y-1.5">
                      {ports.map((port) => (
                        <li
                          key={port}
                          className="flex items-center gap-2 font-mono text-xs text-text-secondary"
                        >
                          <ExternalLink className="h-3 w-3 text-text-muted" aria-hidden="true" />
                          {port}
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              </CardBody>
            </Card>
          </div>

          <Card>
            <CardHeader
              title="Volumes montés"
              subtitle={`${data.mounts.length} montage(s)`}
              icon={<HardDrive className="h-3.5 w-3.5" />}
            />
            <CardBody className="p-0">
              {data.mounts.length === 0 ? (
                <p className="p-4 text-sm text-text-muted">Aucun montage rapporté.</p>
              ) : (
                <Table minWidthClassName="min-w-[720px]" className="rounded-none border-0">
                  <THead>
                    <tr>
                      <TH>Source</TH>
                      <TH>Destination</TH>
                      <TH>Type</TH>
                      <TH>Accès</TH>
                    </tr>
                  </THead>
                  <TBody>
                    {data.mounts.map((mount, index) => (
                      <TR key={`${mountField(mount, 'source')}-${index}`}>
                        <TD className="font-mono text-xs">{mountField(mount, 'source') || '—'}</TD>
                        <TD className="font-mono text-xs">
                          {mountField(mount, 'destination') || '—'}
                        </TD>
                        <TD>
                          <Badge tone="neutral">{mountField(mount, 'type') || '—'}</Badge>
                        </TD>
                        <TD>{mountField(mount, 'rw') || '—'}</TD>
                      </TR>
                    ))}
                  </TBody>
                </Table>
              )}
            </CardBody>
          </Card>

          <Card>
            <CardHeader
              title="Événements du conteneur"
              subtitle="Dernières 24 heures"
              icon={<RotateCw className="h-3.5 w-3.5" />}
              actions={
                <Link
                  to="/events"
                  className="text-[11px] text-text-secondary transition-colors hover:text-text-primary"
                >
                  Journal global
                </Link>
              }
            />
            <CardBody className="space-y-1">
              {events.isError && <ErrorState message={events.error.message} />}
              {!events.isError && eventList.length === 0 && (
                <EmptyState
                  icon={MemoryStick}
                  title="Aucun événement"
                  description="Aucun changement d'état n'a été enregistré pour ce conteneur sur les dernières 24 heures."
                  className="border-0 bg-transparent py-8"
                />
              )}
              {eventList.map((event) => (
                <div
                  key={event.id}
                  className="flex items-start gap-3 rounded-md px-2 py-2.5 hover:bg-elevated/30"
                >
                  <span className="mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-md border border-line bg-surface text-text-muted">
                    <EventIcon type={event.type} />
                  </span>
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="text-xs font-medium text-text-primary">
                        {eventTypeLabel(event.type)}
                      </p>
                      <Badge tone={eventTone(event.severity)}>
                        {eventSeverityLabel(event.severity)}
                      </Badge>
                    </div>
                    <p className="mt-0.5 text-[11px] text-text-muted">{event.message}</p>
                  </div>
                  <span
                    className="shrink-0 font-mono text-[10px] text-text-muted"
                    title={formatDateTime(event.occurredAt)}
                  >
                    {formatRelativeTime(event.occurredAt)}
                  </span>
                </div>
              ))}
            </CardBody>
          </Card>

          <p className="flex items-center gap-2 font-mono text-[10px] text-text-muted">
            <StatusDot tone={containerStateTone(data.state)} />
            Conteneur observé par le Manager · aucune administration Docker n'est disponible.
          </p>
        </>
      )}
    </div>
  )
}

function DetailItem({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="min-w-0 rounded-md border border-line/70 bg-surface/50 px-3 py-2">
      <dt className="font-mono text-[10px] uppercase tracking-wider text-text-muted">{label}</dt>
      <dd className="mt-1 truncate text-xs text-text-primary">{value}</dd>
    </div>
  )
}
