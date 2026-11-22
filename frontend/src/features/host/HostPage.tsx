import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Activity,
  Clock3,
  Cpu,
  HardDrive,
  MemoryStick,
  RefreshCw,
  Server,
  ShieldCheck,
  Thermometer,
} from 'lucide-react'
import { Link } from 'react-router-dom'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardFooter, CardHeader } from '../../components/ui/Card'
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/DataTable'
import { EmptyState } from '../../components/ui/EmptyState'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { MetricCard } from '../../components/ui/MetricCard'
import { PageHeader } from '../../components/ui/PageHeader'
import { ProgressBar } from '../../components/ui/ProgressBar'
import {
  getCurrentHostMetrics,
  getHost,
  getSettings,
  type MetricSample,
} from '../../lib/api/monitoring'
import {
  formatBytes,
  formatDateTime,
  formatMetricValue,
  formatPercent,
  formatRelativeTime,
  formatUptime,
} from '../../lib/format'
import { filesystemSummary } from '../../lib/host'
import { latestGrouped, latestOne, latestSamples } from '../../lib/metrics'
import { useRefreshInterval } from '../../lib/refresh'
import { CollectorRow } from '../../components/ui/CollectorRow'
import { collectorStatusLabel, collectorStatusTone } from '../../lib/collectors'

function metricLabel(name: string): string {
  return name.replaceAll('.', ' ').replace(/\b\w/g, (letter) => letter.toUpperCase())
}

function metricLabels(sample: MetricSample): string {
  const labels = Object.entries(sample.labels ?? {})
  if (labels.length === 0) return '—'
  return labels.map(([key, value]) => `${key}=${value}`).join(' · ')
}

export function HostPage() {
  const queryClient = useQueryClient()
  const refreshInterval = useRefreshInterval()
  const host = useQuery({
    queryKey: ['host'],
    queryFn: getHost,
    refetchInterval: refreshInterval || false,
  })
  const current = useQuery({
    queryKey: ['host', 'current-metrics'],
    queryFn: getCurrentHostMetrics,
    refetchInterval: refreshInterval || false,
  })
  const settings = useQuery({
    queryKey: ['settings'],
    queryFn: getSettings,
    staleTime: 60_000,
  })

  const samples = current.data?.samples ?? []
  const cpu = latestOne(samples, 'cpu.percent', { core: 'all' })?.value ?? null
  const cores = latestOne(samples, 'cpu.cores')?.value ?? null
  const load1 = latestOne(samples, 'cpu.load1')?.value ?? null
  const load5 = latestOne(samples, 'cpu.load5')?.value ?? null
  const load15 = latestOne(samples, 'cpu.load15')?.value ?? null
  const memoryTotal = latestOne(samples, 'memory.total')?.value ?? null
  const memoryUsed = latestOne(samples, 'memory.used')?.value ?? null
  const memoryAvailable = latestOne(samples, 'memory.available')?.value ?? null
  const memoryPercent = memoryTotal && memoryUsed != null ? (memoryUsed / memoryTotal) * 100 : null
  const filesystems = filesystemSummary(samples)
  const temperatures = Array.from(latestGrouped(samples, 'temperature.celsius', 'sensor').entries())
    .map(([sensor, sample]) => ({ sensor, value: sample.value }))
    .sort((left, right) => right.value - left.value)
  const metrics = latestSamples(samples).sort((left, right) => left.name.localeCompare(right.name))

  const systemStatus = settings.data?.collectors.system ?? host.data?.collector
  const dockerStatus = settings.data?.collectors.docker

  return (
    <div className="space-y-4">
      <PageHeader
        title="Machine"
        breadcrumb={[{ label: 'Système' }, { label: 'Machine' }]}
        badge={
          <Badge tone={collectorStatusTone(host.data?.collector.status ?? '')} dot>
            {host.data ? collectorStatusLabel(host.data.collector.status) : 'Collecte'}
          </Badge>
        }
        description={
          host.data ? (
            <>
              <span className="font-mono text-text-primary">
                {host.data.hostname || 'hôte local'}
              </span>
              {' · '}
              {`${host.data.os.distribution || 'Linux'} ${host.data.os.version || ''}`.trim()}
              {' · noyau '}
              <span className="font-mono">{host.data.kernelVersion || 'indisponible'}</span>
            </>
          ) : (
            'Identité et ressources de la machine qui héberge le Manager.'
          )
        }
        actions={
          <Button
            size="sm"
            variant="secondary"
            icon={<RefreshCw className="h-3.5 w-3.5" />}
            onClick={() => {
              void queryClient.invalidateQueries()
            }}
          >
            Rafraîchir
          </Button>
        }
      />

      {(host.isError || current.isError) && (
        <ErrorState
          message={(host.error ?? current.error)?.message ?? 'Informations système indisponibles.'}
          onRetry={() => {
            void queryClient.invalidateQueries()
          }}
        />
      )}

      {(host.isLoading || current.isLoading) && !host.isError && !current.isError && (
        <LoadingState label="Chargement des informations système…" rows={2} />
      )}

      {host.data && !host.isError && (
        <>
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <MetricCard
              label="Processeur"
              title={cores ? `${Math.round(cores)} cœurs logiques` : 'Processeur'}
              value={formatPercent(cpu)}
              badge={<Badge tone="info">CPU</Badge>}
              hint={`Charge 1/5/15 : ${load1?.toFixed(2) ?? '—'} · ${load5?.toFixed(2) ?? '—'} · ${load15?.toFixed(2) ?? '—'}`}
            >
              <ProgressBar value={cpu} tone="cpu" label="Utilisation CPU" />
            </MetricCard>

            <MetricCard
              label="Mémoire vive"
              title={memoryTotal ? `${formatBytes(memoryTotal)} installés` : 'Mémoire'}
              value={formatBytes(memoryUsed)}
              unit={memoryTotal ? `/ ${formatBytes(memoryTotal)}` : undefined}
              badge={<Badge tone="accent">{formatPercent(memoryPercent)}</Badge>}
              hint={`Disponible ${formatBytes(memoryAvailable)}`}
            >
              <ProgressBar value={memoryPercent} tone="ram" label="Utilisation mémoire" />
            </MetricCard>

            <MetricCard
              label="Stockage"
              title={`${filesystems.entries.length} système(s) de fichiers`}
              value={formatBytes(filesystems.used)}
              unit={filesystems.total > 0 ? `/ ${formatBytes(filesystems.total)}` : undefined}
              badge={<Badge tone="success">{formatPercent(filesystems.percent)}</Badge>}
              hint={
                filesystems.total > 0
                  ? `${formatBytes(filesystems.available)} disponibles`
                  : 'Aucune mesure'
              }
            >
              <ProgressBar
                value={filesystems.percent}
                tone="success"
                label="Utilisation du stockage"
              />
            </MetricCard>

            <MetricCard
              label="Températures"
              title={temperatures.length > 0 ? 'Capteurs locaux' : 'Aucun capteur'}
              value={temperatures[0] ? formatMetricValue(temperatures[0].value, 'celsius') : '—'}
              badge={
                <Badge
                  tone={temperatures.some((sensor) => sensor.value >= 80) ? 'warning' : 'neutral'}
                >
                  {temperatures.length} capteur(s)
                </Badge>
              }
              hint={
                temperatures[0]
                  ? `Maximum : ${temperatures[0].sensor}`
                  : 'Le matériel n’expose aucun capteur lisible.'
              }
            />
          </div>

          <div className="grid items-start gap-4 xl:grid-cols-3">
            <Card className="xl:col-span-2">
              <CardHeader
                title="Identité de l'hôte"
                subtitle="Informations lues localement"
                icon={<Server className="h-3.5 w-3.5" />}
              />
              <CardBody>
                <dl className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                  <Info label="Nom d'hôte" value={host.data.hostname || '—'} mono />
                  <Info
                    label="Distribution"
                    value={`${host.data.os.distribution || '—'} ${host.data.os.version || ''}`.trim()}
                  />
                  <Info label="Noyau" value={host.data.kernelVersion || '—'} mono />
                  <Info label="Démarrage" value={formatDateTime(host.data.bootTime)} />
                  <Info label="Uptime" value={formatUptime(host.data.uptimeSeconds)} />
                  <Info
                    label="Dernière collecte"
                    value={
                      host.data.collector.lastSuccessAt
                        ? formatRelativeTime(host.data.collector.lastSuccessAt)
                        : 'Aucune'
                    }
                  />
                </dl>
              </CardBody>
              <CardFooter className="flex flex-wrap items-center justify-between gap-2 font-mono text-[10px] text-text-muted">
                <span className="flex items-center gap-1.5">
                  <Clock3 className="h-3 w-3" /> Uptime calculé depuis l'heure de démarrage.
                </span>
                <Link
                  to="/metrics"
                  className="inline-flex items-center gap-1 text-text-secondary hover:text-text-primary"
                >
                  <Activity className="h-3 w-3" /> Historique des métriques
                </Link>
              </CardFooter>
            </Card>

            <Card>
              <CardHeader
                title="Collecteurs"
                subtitle="État de la supervision"
                icon={<ShieldCheck className="h-3.5 w-3.5" />}
              />
              <CardBody className="space-y-3">
                <CollectorRow
                  name="Système"
                  status={systemStatus?.status}
                  lastSuccessAt={systemStatus?.lastSuccessAt}
                />
                <CollectorRow
                  name="Docker"
                  status={dockerStatus?.status}
                  lastSuccessAt={dockerStatus?.lastSuccessAt}
                />
                <div className="rounded-md border border-line/70 bg-surface/50 px-3 py-2.5 font-mono text-[10px] text-text-muted">
                  <p>Intervalle système : {settings.data?.systemCollectIntervalSeconds ?? '—'} s</p>
                  <p className="mt-1">
                    Rétention : {settings.data?.metricsRetentionDays ?? '—'} jours
                  </p>
                </div>
              </CardBody>
              <CardFooter>
                <Link
                  to="/settings"
                  className="text-[11px] text-text-secondary transition-colors hover:text-text-primary"
                >
                  Voir la configuration effective
                </Link>
              </CardFooter>
            </Card>
          </div>

          <div className="grid items-start gap-4 xl:grid-cols-2">
            <Card>
              <CardHeader
                title="Systèmes de fichiers"
                subtitle="Capacité et occupation"
                icon={<HardDrive className="h-3.5 w-3.5" />}
                actions={
                  <Link
                    to="/storage"
                    className="text-[11px] text-text-secondary transition-colors hover:text-text-primary"
                  >
                    Détails
                  </Link>
                }
              />
              <CardBody className="space-y-3">
                {filesystems.entries.length === 0 && (
                  <p className="text-sm text-text-muted">Aucun système de fichiers mesuré.</p>
                )}
                {filesystems.entries.slice(0, 5).map((entry) => (
                  <div key={entry.mountpoint} className="space-y-1.5">
                    <div className="flex items-center justify-between gap-2 font-mono text-[11px]">
                      <span className="truncate text-text-primary">{entry.mountpoint}</span>
                      <span className="text-text-muted">
                        {formatBytes(entry.used)} / {formatBytes(entry.total)}
                      </span>
                    </div>
                    <ProgressBar
                      value={entry.percent}
                      tone={entry.percent != null && entry.percent > 90 ? 'warning' : 'success'}
                      label={`Occupation ${entry.mountpoint}`}
                    />
                  </div>
                ))}
              </CardBody>
            </Card>

            <Card>
              <CardHeader
                title="Capteurs de température"
                subtitle="Valeurs les plus récentes"
                icon={<Thermometer className="h-3.5 w-3.5" />}
              />
              <CardBody>
                {temperatures.length === 0 ? (
                  <EmptyState
                    icon={Thermometer}
                    title="Aucun capteur disponible"
                    description="Le matériel ou le noyau n'expose aucun capteur de température lisible."
                    className="border-0 bg-transparent py-6"
                  />
                ) : (
                  <div className="space-y-2">
                    {temperatures.map((sensor) => (
                      <div
                        key={sensor.sensor}
                        className="flex items-center justify-between gap-3 rounded-md border border-line/70 bg-surface/50 px-3 py-2"
                      >
                        <span className="truncate font-mono text-xs text-text-secondary">
                          {sensor.sensor}
                        </span>
                        <Badge
                          tone={
                            sensor.value >= 80
                              ? 'danger'
                              : sensor.value >= 65
                                ? 'warning'
                                : 'success'
                          }
                        >
                          {formatMetricValue(sensor.value, 'celsius')}
                        </Badge>
                      </div>
                    ))}
                  </div>
                )}
              </CardBody>
            </Card>
          </div>

          <Card>
            <CardHeader
              title="Dernières mesures de l'hôte"
              subtitle={`${metrics.length} série(s) · échantillons bruts`}
              icon={<MemoryStick className="h-3.5 w-3.5" />}
            />
            <CardBody className="p-0">
              {metrics.length === 0 ? (
                <p className="p-4 text-sm text-text-muted">Aucune mesure disponible.</p>
              ) : (
                <Table minWidthClassName="min-w-[760px]" className="rounded-none border-0">
                  <THead>
                    <tr>
                      <TH>Métrique</TH>
                      <TH>Étiquettes</TH>
                      <TH>Valeur</TH>
                      <TH>Horodatage</TH>
                    </tr>
                  </THead>
                  <TBody>
                    {metrics.slice(0, 80).map((sample) => (
                      <TR
                        key={`${sample.name}-${JSON.stringify(sample.labels)}-${sample.recordedAt}`}
                      >
                        <TD className="font-mono text-xs text-text-primary">
                          {metricLabel(sample.name)}
                        </TD>
                        <TD className="font-mono text-[10px] text-text-muted">
                          {metricLabels(sample)}
                        </TD>
                        <TD className="font-mono text-xs">
                          {formatMetricValue(sample.value, sample.unit)}
                        </TD>
                        <TD className="font-mono text-[10px]">
                          {formatDateTime(sample.recordedAt)}
                        </TD>
                      </TR>
                    ))}
                  </TBody>
                </Table>
              )}
            </CardBody>
            <CardFooter className="flex flex-wrap items-center justify-between gap-2 font-mono text-[10px] text-text-muted">
              <span className="flex items-center gap-1.5">
                <Cpu className="h-3 w-3" /> {metrics.length} séries distinctes
              </span>
              <span>Les valeurs indisponibles ne sont jamais remplacées par zéro.</span>
            </CardFooter>
          </Card>
        </>
      )}
    </div>
  )
}

function Info({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="min-w-0 rounded-md border border-line/70 bg-surface/50 px-3 py-2">
      <dt className="font-mono text-[10px] uppercase tracking-wider text-text-muted">{label}</dt>
      <dd
        className={`mt-1 truncate text-xs text-text-primary ${mono ? 'font-mono' : ''}`}
        title={value}
      >
        {value}
      </dd>
    </div>
  )
}
