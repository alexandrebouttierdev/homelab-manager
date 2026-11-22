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
  formatBytes,
  formatDateTime,
  formatMetricValue,
  formatPercent,
  formatRelativeTime,
  formatUptime,
} from '../../lib/format'
import { CollectorRow } from '../../components/ui/CollectorRow'
import { collectorStatusLabel, collectorStatusTone } from '../../lib/collectors'
import { metricLabel, metricLabels, useHostPage } from './useHostPage'

export function HostPage() {
  const {
    queryClient,
    host,
    current,
    cpu,
    cores,
    load1,
    load5,
    load15,
    memoryTotal,
    memoryUsed,
    memoryAvailable,
    memoryPercent,
    filesystems,
    temperatures,
    metrics,
    systemStatus,
    dockerStatus,
    hostInfo,
    settingsInfo,
    temperatureAlert,
  } = useHostPage()

  return (
    <div className="space-y-4">
      <PageHeader
        title="Machine"
        breadcrumb={[{ label: 'Système' }, { label: 'Machine' }]}
        badge={
          <Badge tone={collectorStatusTone(hostInfo?.collector.status ?? '')} dot>
            {hostInfo ? collectorStatusLabel(hostInfo.collector.status) : 'Collecte'}
          </Badge>
        }
        description={
          hostInfo ? (
            <>
              <span className="font-mono text-text-primary">
                {hostInfo.hostname || 'hôte local'}
              </span>
              {' · '}
              {`${hostInfo.os.distribution || 'Linux'} ${hostInfo.os.version || ''}`.trim()}
              {' · noyau '}
              <span className="font-mono">{hostInfo.kernelVersion || 'indisponible'}</span>
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

      {hostInfo && !host.isError && (
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
                <Badge tone={temperatureAlert ? 'warning' : 'neutral'}>
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
                  <Info label="Nom d'hôte" value={hostInfo.hostname || '—'} mono />
                  <Info
                    label="Distribution"
                    value={`${hostInfo.os.distribution || '—'} ${hostInfo.os.version || ''}`.trim()}
                  />
                  <Info label="Noyau" value={hostInfo.kernelVersion || '—'} mono />
                  <Info label="Démarrage" value={formatDateTime(hostInfo.bootTime)} />
                  <Info label="Uptime" value={formatUptime(hostInfo.uptimeSeconds)} />
                  <Info
                    label="Dernière collecte"
                    value={
                      hostInfo.collector.lastSuccessAt
                        ? formatRelativeTime(hostInfo.collector.lastSuccessAt)
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
                  <p>Intervalle système : {settingsInfo?.systemCollectIntervalSeconds ?? '—'} s</p>
                  <p className="mt-1">
                    Rétention : {settingsInfo?.metricsRetentionDays ?? '—'} jours
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
