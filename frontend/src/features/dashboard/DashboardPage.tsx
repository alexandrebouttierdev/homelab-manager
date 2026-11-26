import {
  Activity,
  AlertTriangle,
  ArrowDownRight,
  ArrowUpRight,
  CheckCircle2,
  ChevronRight,
  Container as ContainerIcon,
  Cpu,
  Database,
  HardDrive,
  MemoryStick,
  Network,
  RefreshCw,
  Server,
  ShieldCheck,
} from 'lucide-react'
import { Link } from 'react-router-dom'
import { Chart } from '../../components/charts/Chart'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardFooter, CardHeader } from '../../components/ui/Card'
import { EmptyState } from '../../components/ui/EmptyState'
import { ErrorState } from '../../components/ui/ErrorState'
import { EventIcon } from '../../components/ui/EventIcon'
import { LoadingState } from '../../components/ui/LoadingState'
import { MetricCard } from '../../components/ui/MetricCard'
import { PageHeader } from '../../components/ui/PageHeader'
import { VulnerabilitiesCard } from './VulnerabilitiesCard'
import { ProgressBar } from '../../components/ui/ProgressBar'
import { Sparkline } from '../../components/ui/Sparkline'
import { StatusDot } from '../../components/ui/StatusDot'
import { containerStateLabel, containerStateTone, imageTag, isRunning } from '../../lib/containers'
import {
  formatBytes,
  formatNumber,
  formatPercent,
  formatRate,
  formatRelativeTime,
  formatShortTime,
  formatUptime,
} from '../../lib/format'
import { eventSeverityLabel, eventTone, eventTypeLabel } from '../../lib/events'
import { CollectorRow } from '../../components/ui/CollectorRow'
import { usageTone } from '../../lib/host'
import { useDashboard } from './useDashboard'

export function DashboardPage() {
  const {
    refresh,
    refreshInterval,
    dashboard,
    isLoading,
    error,
    lastCollection,
    overallTone,
    overallLabel,
    cpu,
    cores,
    load1,
    load5,
    load15,
    cpuValues,
    memoryTotal,
    memoryUsed,
    memoryAvailable,
    memoryCached,
    memoryPercent,
    memoryValues,
    swapTotal,
    swapUsed,
    filesystems,
    interfaces,
    primaryInterface,
    chartOption,
    containerList,
    runningContainers,
    stoppedContainers,
    problemContainers,
    highlightedContainers,
    containersError,
    attentionItems,
    events,
    eventsError,
  } = useDashboard()

  return (
    <div className="space-y-6">
      <PageHeader
        title="Vue d'ensemble"
        breadcrumb={[{ label: 'MonoOps' }, { label: "Vue d'ensemble" }]}
        badge={
          <span className="inline-flex items-center gap-2 text-[13px] text-text-secondary">
            <StatusDot tone={overallTone} />
            {overallLabel}
          </span>
        }
        description={
          dashboard ? (
            <>
              <span className="font-mono text-[13px] text-text-primary">
                {dashboard.host.hostname || 'hôte local'}
              </span>
              {' · '}
              {`${dashboard.host.os.distribution || 'Linux'} ${dashboard.host.os.version || ''}`.trim()}
              {' · noyau '}
              <span className="font-mono text-[13px]">
                {dashboard.host.kernelVersion || 'indisponible'}
              </span>
              {' · '}
              {lastCollection
                ? `dernière collecte ${formatRelativeTime(lastCollection)}`
                : 'collecte en attente'}
            </>
          ) : (
            'Supervision locale de la machine qui héberge le Manager.'
          )
        }
        actions={
          <>
            <Button
              size="sm"
              variant="secondary"
              icon={<RefreshCw className="h-3.5 w-3.5" />}
              onClick={refresh}
            >
              Rafraîchir
            </Button>
            <Link
              to="/host"
              className="inline-flex h-8 items-center gap-1.5 rounded-md border border-line-strong bg-card px-3 text-[13px] font-medium text-text-primary transition-colors hover:bg-elevated"
            >
              <Server className="h-3.5 w-3.5" />
              Machine
            </Link>
          </>
        }
      />

      {error && (
        <ErrorState
          message={error?.message ?? 'Impossible de charger la supervision.'}
          onRetry={refresh}
        />
      )}

      {isLoading && !error && <LoadingState label="Chargement de la supervision…" rows={2} />}

      {!isLoading && !error && (
        <>
          <section
            aria-label="Mesures instantanées"
            className="grid gap-px overflow-hidden rounded-xl border border-line bg-line sm:grid-cols-2 xl:grid-cols-4"
          >
            <MetricCard
              variant="plain"
              label="Processeur (CPU)"
              title={cores ? `${Math.round(cores)} cœurs logiques` : 'Processeur'}
              value={formatPercent(cpu?.value)}
              badge={<Badge tone="info">{cores ? `${Math.round(cores)} cœurs` : 'CPU'}</Badge>}
              aside={
                <div className="text-xs text-text-muted">
                  <span className="block">Charge 1/5/15</span>
                  <span className="text-text-secondary">
                    {load1 != null ? load1.toFixed(2) : '—'} ·{' '}
                    {load5 != null ? load5.toFixed(2) : '—'} ·{' '}
                    {load15 != null ? load15.toFixed(2) : '—'}
                  </span>
                </div>
              }
              hint={cpu ? `Mesure du ${formatShortTime(cpu.recordedAt)}` : 'Mesure indisponible'}
            >
              {cpuValues.length > 1 ? (
                <Sparkline values={cpuValues} tone="cpu" />
              ) : (
                <p className="text-xs text-text-muted">Historique en cours de constitution.</p>
              )}
            </MetricCard>

            <MetricCard
              variant="plain"
              label="Mémoire vive (RAM)"
              title={memoryTotal ? `${formatBytes(memoryTotal)} installés` : 'Mémoire'}
              value={formatBytes(memoryUsed)}
              unit={memoryTotal ? `/ ${formatBytes(memoryTotal)}` : undefined}
              badge={
                <Badge tone={memoryPercent != null && memoryPercent > 90 ? 'warning' : 'accent'}>
                  {formatPercent(memoryPercent)}
                </Badge>
              }
              hint={`Cache ${formatBytes(memoryCached)} · Disponible ${formatBytes(memoryAvailable)}`}
            >
              <ProgressBar value={memoryPercent} tone="ram" label="Utilisation mémoire" />
              <p className="text-xs text-text-muted">
                Swap {formatBytes(swapUsed)} / {formatBytes(swapTotal)}
              </p>
            </MetricCard>

            <MetricCard
              variant="plain"
              label="Stockage système & média"
              title={
                filesystems.volumes > 0
                  ? `${filesystems.volumes} volume(s) · ${filesystems.entries.length} montage(s)`
                  : 'Stockage'
              }
              value={formatBytes(filesystems.used)}
              unit={filesystems.total > 0 ? `/ ${formatBytes(filesystems.total)}` : undefined}
              badge={
                <Badge tone={usageTone(filesystems.percent)}>
                  {formatPercent(filesystems.percent)}
                </Badge>
              }
              hint={
                filesystems.total > 0
                  ? `${formatBytes(filesystems.total - filesystems.used)} libres`
                  : 'Aucun système de fichiers mesuré'
              }
            >
              <div className="space-y-2">
                {filesystems.entries.slice(0, 2).map((entry) => (
                  <div key={entry.mountpoint} className="space-y-1">
                    <div className="flex items-center justify-between gap-2 text-xs text-text-muted">
                      <span className="truncate">{entry.mountpoint}</span>
                      <span>{formatPercent(entry.percent)}</span>
                    </div>
                    <ProgressBar value={entry.percent} tone={usageTone(entry.percent)} />
                  </div>
                ))}
                {filesystems.entries.length === 0 && (
                  <p className="text-xs text-text-muted">Donnée indisponible.</p>
                )}
              </div>
            </MetricCard>

            <MetricCard
              variant="plain"
              label="Trafic réseau"
              title={primaryInterface?.name ?? 'Interfaces'}
              value={
                primaryInterface?.rxRate != null
                  ? formatRate(primaryInterface.rxRate)
                  : formatBytes(primaryInterface?.rx)
              }
              unit="RX"
              badge={<Badge tone="neutral">{primaryInterface?.name ?? 'Réseau'}</Badge>}
              aside={
                primaryInterface ? (
                  <div className="text-xs text-text-muted">
                    <span className="block">TX</span>
                    <span className="text-text-secondary">
                      {primaryInterface.txRate != null
                        ? formatRate(primaryInterface.txRate)
                        : formatBytes(primaryInterface.tx)}
                    </span>
                  </div>
                ) : undefined
              }
              hint={
                primaryInterface
                  ? `Cumul RX ${formatBytes(primaryInterface.rx)} · TX ${formatBytes(primaryInterface.tx)}`
                  : 'Aucune interface mesurée'
              }
            >
              <div className="flex items-center justify-between text-xs text-text-muted">
                <span className="flex items-center gap-1">
                  <ArrowDownRight className="h-3 w-3 text-net" />{' '}
                  {formatRate(primaryInterface?.rxRate)}
                </span>
                <span className="flex items-center gap-1">
                  <ArrowUpRight className="h-3 w-3 text-accent" />{' '}
                  {formatRate(primaryInterface?.txRate)}
                </span>
              </div>
            </MetricCard>
          </section>

          <div className="grid items-start gap-5 xl:grid-cols-3">
            <Card className="xl:col-span-2">
              <CardHeader
                title="Charge CPU et mémoire"
                subtitle="60 dernières minutes · échantillons bruts"
                icon={<Activity className="h-3.5 w-3.5" />}
                actions={
                  <Badge tone="neutral">
                    Auto {refreshInterval > 0 ? `${refreshInterval / 1000} s` : 'pause'}
                  </Badge>
                }
              />
              <CardBody>
                <Chart option={chartOption} height={272} label="Historique CPU et mémoire" />
              </CardBody>
              <CardFooter className="flex flex-wrap items-center justify-between gap-2 text-xs text-text-muted">
                <span>
                  CPU {formatPercent(cpu?.value)} · RAM {formatPercent(memoryPercent)}
                </span>
                <span>{cpuValues.length + memoryValues.length} points sur la période</span>
              </CardFooter>
            </Card>

            <Card>
              <CardHeader
                title="Trafic par interface"
                subtitle="Compteurs et débits courants"
                icon={<Network className="h-3.5 w-3.5" />}
                actions={
                  <Link
                    to="/network"
                    className="text-xs text-text-secondary transition-colors hover:text-text-primary"
                  >
                    Détails
                  </Link>
                }
              />
              <CardBody className="space-y-3">
                {interfaces.length === 0 && (
                  <p className="text-sm text-text-muted">Aucune interface réseau mesurée.</p>
                )}
                {interfaces.slice(0, 4).map((iface) => (
                  <div
                    key={iface.name}
                    className="rounded-md border border-line/70 bg-surface/50 px-3 py-2.5"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="font-mono text-xs text-text-primary">{iface.name}</span>
                      <span
                        className={
                          iface.errors > 0 ? 'text-xs text-warning' : 'text-xs text-text-muted'
                        }
                        title="Compteurs cumulés depuis le démarrage de la machine"
                      >
                        {formatNumber(iface.errors)} erreur(s) · {formatNumber(iface.drops)}{' '}
                        perte(s)
                      </span>
                    </div>
                    <div className="mt-2 grid grid-cols-2 gap-2 text-xs">
                      <span className="flex items-center gap-1 text-text-secondary">
                        <ArrowDownRight className="h-3 w-3 text-net" /> {formatRate(iface.rxRate)}
                      </span>
                      <span className="flex items-center gap-1 text-text-secondary">
                        <ArrowUpRight className="h-3 w-3 text-accent" /> {formatRate(iface.txRate)}
                      </span>
                    </div>
                  </div>
                ))}
              </CardBody>
            </Card>
          </div>

          <div className="grid items-start gap-5 xl:grid-cols-3">
            <Card className="xl:col-span-2">
              <CardHeader
                title="Services Docker"
                subtitle={`${containerList.length} conteneur(s) observé(s)`}
                icon={<ContainerIcon className="h-3.5 w-3.5" />}
                actions={
                  <div className="flex flex-wrap items-center gap-1.5">
                    <Badge tone="success" dot>
                      {runningContainers.length} actifs
                    </Badge>
                    <Badge tone="neutral">{stoppedContainers.length} arrêtés</Badge>
                    <Badge
                      tone={problemContainers.length > 0 ? 'danger' : 'neutral'}
                      dot={problemContainers.length > 0}
                    >
                      {problemContainers.length} incident(s)
                    </Badge>
                    <Link
                      to="/containers"
                      className="ml-1 inline-flex items-center gap-1 text-xs text-text-secondary transition-colors hover:text-text-primary"
                    >
                      Voir tout <ChevronRight className="h-3 w-3" />
                    </Link>
                  </div>
                }
              />
              <CardBody>
                {containersError && <ErrorState message={containersError.message} />}
                {!containersError && highlightedContainers.length === 0 && (
                  <EmptyState
                    icon={ContainerIcon}
                    title="Aucun conteneur détecté"
                    description="Le collecteur Docker ne rapporte actuellement aucun conteneur local."
                    className="border-0 bg-transparent py-8"
                  />
                )}
                {highlightedContainers.length > 0 && (
                  <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
                    {highlightedContainers.map((container) => {
                      const tag = imageTag(container.image)
                      return (
                        <Link
                          key={container.id}
                          to={`/containers/${encodeURIComponent(container.id)}`}
                          className="group rounded-md border border-line/70 bg-surface/50 p-3 transition-colors hover:border-line-strong hover:bg-elevated/50"
                        >
                          <div className="flex items-center gap-2">
                            <StatusDot
                              tone={containerStateTone(container.state)}
                              pulse={isRunning(container.state)}
                              label={containerStateLabel(container.state)}
                            />
                            <span className="min-w-0 truncate text-xs font-medium text-text-primary group-hover:text-accent">
                              {container.name}
                            </span>
                            <Badge tone={containerStateTone(container.state)} className="ml-auto">
                              {containerStateLabel(container.state)}
                            </Badge>
                          </div>
                          <p className="mt-1.5 truncate text-xs text-text-muted">
                            {tag.repository}
                          </p>
                          <div className="mt-2 flex items-center justify-between text-xs text-text-muted">
                            <span>
                              {isRunning(container.state)
                                ? formatUptime(container.uptimeSeconds)
                                : containerStateLabel(container.state)}
                            </span>
                            <span>{container.restartCount} redémarrage(s)</span>
                          </div>
                        </Link>
                      )
                    })}
                  </div>
                )}
              </CardBody>
              <CardFooter className="flex flex-wrap items-center justify-between gap-2 text-xs text-text-muted">
                <span>Lecture seule · aucune action Docker n'est exposée.</span>
                <Link
                  to="/events"
                  className="inline-flex items-center gap-1 text-text-secondary hover:text-text-primary"
                >
                  Voir les événements <ChevronRight className="h-3 w-3" />
                </Link>
              </CardFooter>
            </Card>

            <Card>
              <CardHeader
                title="Attention requise"
                subtitle="Collecteurs, conteneurs et événements"
                icon={<AlertTriangle className="h-3.5 w-3.5" />}
                actions={
                  attentionItems.length > 0 ? (
                    <Badge tone="warning">{attentionItems.length} point(s)</Badge>
                  ) : (
                    <Badge tone="success">OK</Badge>
                  )
                }
              />
              <CardBody className="space-y-2">
                {attentionItems.length === 0 && (
                  <div className="flex items-start gap-2 rounded-md border border-success/20 bg-success/5 px-3 py-2.5">
                    <CheckCircle2
                      className="mt-0.5 h-4 w-4 shrink-0 text-success"
                      aria-hidden="true"
                    />
                    <div>
                      <p className="text-xs font-medium text-text-primary">
                        Aucune anomalie détectée
                      </p>
                      <p className="mt-0.5 text-xs text-text-muted">
                        Les collecteurs répondent et aucun conteneur n'est en erreur.
                      </p>
                    </div>
                  </div>
                )}
                {attentionItems.map((item) => (
                  <Link
                    key={item.key}
                    to={item.to ?? '/events'}
                    className="flex items-start gap-2 rounded-md border border-line/70 bg-surface/50 px-3 py-2.5 transition-colors hover:border-line-strong hover:bg-elevated/40"
                  >
                    <AlertTriangle
                      className={
                        item.tone === 'danger'
                          ? 'mt-0.5 h-3.5 w-3.5 shrink-0 text-danger'
                          : 'mt-0.5 h-3.5 w-3.5 shrink-0 text-warning'
                      }
                      aria-hidden="true"
                    />
                    <div className="min-w-0">
                      <p className="truncate text-xs font-medium text-text-primary">{item.title}</p>
                      <p className="mt-0.5 line-clamp-2 text-xs text-text-muted">{item.detail}</p>
                    </div>
                  </Link>
                ))}
              </CardBody>
              <CardFooter>
                <Link
                  to="/events"
                  className="inline-flex items-center gap-1 text-xs text-text-secondary transition-colors hover:text-text-primary"
                >
                  Consulter le journal <ChevronRight className="h-3 w-3" />
                </Link>
              </CardFooter>
            </Card>
          </div>

          <div className="grid items-start gap-5 xl:grid-cols-3">
            <Card className="xl:col-span-2">
              <CardHeader
                title="Journal d'activité récent"
                subtitle="Dernières 24 heures"
                icon={<Activity className="h-3.5 w-3.5" />}
                actions={
                  <Link
                    to="/events"
                    className="text-xs text-text-secondary transition-colors hover:text-text-primary"
                  >
                    Tout voir
                  </Link>
                }
              />
              <CardBody className="space-y-1">
                {eventsError && <ErrorState message={eventsError.message} />}
                {!eventsError && events.length === 0 && (
                  <p className="py-6 text-center text-sm text-text-muted">
                    Aucun événement enregistré sur les dernières 24 heures.
                  </p>
                )}
                {events.map((event) => (
                  <div
                    key={event.id}
                    className="flex items-start gap-3 rounded-md px-2 py-2.5 transition-colors hover:bg-elevated/30"
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
                      <p className="mt-0.5 line-clamp-2 text-xs text-text-muted">{event.message}</p>
                    </div>
                    <div className="shrink-0 text-right text-xs text-text-muted">
                      <span className="block">{formatRelativeTime(event.occurredAt)}</span>
                      {event.containerId && (
                        <Link
                          to={`/containers/${encodeURIComponent(event.containerId)}`}
                          className="text-accent hover:underline"
                        >
                          conteneur
                        </Link>
                      )}
                    </div>
                  </div>
                ))}
              </CardBody>
            </Card>

            <Card>
              <CardHeader
                title="État des collecteurs"
                subtitle="Supervision locale"
                icon={<ShieldCheck className="h-3.5 w-3.5" />}
                actions={
                  <Link
                    to="/settings"
                    className="text-xs text-text-secondary transition-colors hover:text-text-primary"
                  >
                    Configurer
                  </Link>
                }
              />
              <CardBody className="space-y-3">
                <CollectorRow
                  name="Système"
                  status={dashboard?.collectors.system.status}
                  lastSuccessAt={dashboard?.collectors.system.lastSuccessAt}
                />
                <CollectorRow
                  name="Docker"
                  status={dashboard?.collectors.docker.status}
                  lastSuccessAt={dashboard?.collectors.docker.lastSuccessAt}
                />
                <div className="rounded-md border border-line/70 bg-surface/50 px-3 py-2.5">
                  <p className="text-xs text-text-muted">Hôte</p>
                  <p className="mt-1 text-xs text-text-primary">
                    {dashboard?.host.hostname || '—'}
                  </p>
                  <p className="mt-0.5 text-xs text-text-muted">
                    Uptime {formatUptime(dashboard?.host.uptimeSeconds)} · noyau{' '}
                    {dashboard?.host.kernelVersion || '—'}
                  </p>
                </div>
              </CardBody>
              <CardFooter className="text-xs text-text-muted">
                {lastCollection
                  ? `Dernière collecte ${formatRelativeTime(lastCollection)}`
                  : 'Aucune collecte réussie'}
              </CardFooter>
            </Card>
          </div>

          <div className="grid items-start gap-5 xl:grid-cols-3">
            <VulnerabilitiesCard />
          </div>

          <p className="flex flex-wrap items-center gap-2 text-xs text-text-muted">
            <Database className="h-3 w-3" aria-hidden="true" />
            Données collectées localement et conservées dans SQLite.
            <span className="text-line-strong">•</span>
            <HardDrive className="h-3 w-3" aria-hidden="true" />
            {filesystems.volumes} volume(s) de stockage
            <span className="text-line-strong">•</span>
            <MemoryStick className="h-3 w-3" aria-hidden="true" />
            {formatPercent(memoryPercent)} de mémoire utilisée
            <span className="text-line-strong">•</span>
            <Cpu className="h-3 w-3" aria-hidden="true" />
            {formatPercent(cpu?.value)} CPU
          </p>
        </>
      )}
    </div>
  )
}
