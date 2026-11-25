import { Activity, HardDrive, RefreshCw } from 'lucide-react'
import { Chart } from '../../components/charts/Chart'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardFooter, CardHeader } from '../../components/ui/Card'
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/DataTable'
import { EmptyState } from '../../components/ui/EmptyState'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { PageHeader } from '../../components/ui/PageHeader'
import { ProgressBar } from '../../components/ui/ProgressBar'
import { SegmentedTabs } from '../../components/ui/SegmentedTabs'
import { Select } from '../../components/ui/Select'
import { formatBytes, formatMetricValue, formatPercent } from '../../lib/format'
import { ranges, useStoragePage } from './useStoragePage'

export function StoragePage() {
  const {
    range,
    setRange,
    setSelectedMountpoint,
    current,
    filesystems,
    disks,
    mountpoint,
    history,
    historySeries,
    chartOption,
  } = useStoragePage()

  return (
    <div className="space-y-4">
      <PageHeader
        title="Stockage"
        breadcrumb={[{ label: 'Système' }, { label: 'Stockage' }]}
        badge={
          <Badge
            tone={filesystems.percent != null && filesystems.percent > 90 ? 'warning' : 'success'}
          >
            {formatPercent(filesystems.percent)}
          </Badge>
        }
        description="Capacité, occupation et activité d'E/S des systèmes de fichiers locaux. Les valeurs proviennent directement des collecteurs système."
        actions={
          <Button
            size="sm"
            variant="secondary"
            icon={<RefreshCw className="h-3.5 w-3.5" />}
            onClick={() => {
              void current.refetch()
            }}
          >
            Actualiser
          </Button>
        }
      />

      {current.isError && (
        <ErrorState
          message={current.error.message}
          onRetry={() => {
            void current.refetch()
          }}
        />
      )}
      {current.isLoading && !current.isError && (
        <LoadingState label="Chargement du stockage…" rows={2} />
      )}

      {!current.isLoading && !current.isError && (
        <>
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <Summary
              label="Capacité totale"
              value={formatBytes(filesystems.total)}
              tone="text-text-primary"
            />
            <Summary
              label="Espace utilisé"
              value={formatBytes(filesystems.used)}
              tone="text-warning"
            />
            <Summary
              label="Espace disponible"
              value={formatBytes(filesystems.available)}
              tone="text-success"
            />
            <Summary
              label="Systèmes de fichiers"
              value={String(filesystems.entries.length)}
              tone="text-accent"
            />
          </div>

          {filesystems.entries.length === 0 ? (
            <EmptyState
              icon={HardDrive}
              title="Aucun système de fichiers mesuré"
              description="Les partitions locales ne sont pas accessibles ou le collecteur système n'a pas encore publié de mesure."
            />
          ) : (
            <Card>
              <CardHeader
                title="Systèmes de fichiers"
                subtitle={`${filesystems.entries.length} partition(s) · occupation courante`}
                icon={<HardDrive className="h-3.5 w-3.5" />}
              />
              <CardBody className="p-0">
                <Table minWidthClassName="min-w-[820px]" className="rounded-none border-0">
                  <THead>
                    <tr>
                      <TH>Point de montage</TH>
                      <TH>Capacité</TH>
                      <TH>Utilisé</TH>
                      <TH>Disponible</TH>
                      <TH>Occupation</TH>
                      <TH>Statut</TH>
                    </tr>
                  </THead>
                  <TBody>
                    {filesystems.entries.map((entry) => (
                      <TR key={entry.mountpoint}>
                        <TD className="font-mono text-xs text-text-primary">{entry.mountpoint}</TD>
                        <TD className="font-mono text-xs">{formatBytes(entry.total)}</TD>
                        <TD className="font-mono text-xs">{formatBytes(entry.used)}</TD>
                        <TD className="font-mono text-xs">{formatBytes(entry.available)}</TD>
                        <TD>
                          <div className="min-w-[160px] space-y-1">
                            <div className="flex items-center justify-between gap-2 text-xs">
                              <span className="text-text-secondary">
                                {formatPercent(entry.percent)}
                              </span>
                              <span className="text-text-muted">
                                {formatBytes(entry.available)} libres
                              </span>
                            </div>
                            <ProgressBar
                              value={entry.percent}
                              tone={
                                entry.percent != null && entry.percent > 90 ? 'warning' : 'success'
                              }
                              label={`Occupation ${entry.mountpoint}`}
                            />
                          </div>
                        </TD>
                        <TD>
                          <Badge
                            tone={
                              entry.percent != null && entry.percent > 90 ? 'warning' : 'success'
                            }
                            dot
                          >
                            {entry.percent != null && entry.percent > 90
                              ? 'À surveiller'
                              : 'Normal'}
                          </Badge>
                        </TD>
                      </TR>
                    ))}
                  </TBody>
                </Table>
              </CardBody>
              <CardFooter className="flex flex-wrap items-center justify-between gap-2 text-xs text-text-muted">
                <span>Espace disponible calculé à partir des blocs réellement disponibles.</span>
                <span>Les systèmes non lisibles sont omis.</span>
              </CardFooter>
            </Card>
          )}

          <div className="grid items-start gap-4 xl:grid-cols-3">
            <Card className="xl:col-span-2">
              <CardHeader
                title="Historique d'occupation"
                subtitle={mountpoint ? `Point de montage ${mountpoint}` : 'Aucun point de montage'}
                icon={<Activity className="h-3.5 w-3.5" />}
                actions={
                  <div className="flex flex-wrap items-center gap-2">
                    <Select
                      aria-label="Choisir un point de montage"
                      value={mountpoint}
                      onChange={(event) => setSelectedMountpoint(event.target.value)}
                      className="h-8 max-w-[190px] text-xs"
                    >
                      {filesystems.entries.map((entry) => (
                        <option key={entry.mountpoint} value={entry.mountpoint}>
                          {entry.mountpoint}
                        </option>
                      ))}
                    </Select>
                    <SegmentedTabs
                      ariaLabel="Période de l'historique de stockage"
                      value={range}
                      onChange={setRange}
                      options={ranges.map((item) => ({ value: item.value, label: item.label }))}
                    />
                  </div>
                }
              />
              <CardBody>
                {history.isError && <ErrorState message={history.error.message} />}
                {!history.isError && historySeries.length === 0 && (
                  <p className="py-8 text-center text-sm text-text-muted">
                    Aucun échantillon sur la période sélectionnée.
                  </p>
                )}
                {historySeries.length > 0 && (
                  <Chart
                    option={chartOption}
                    height={280}
                    label="Historique d'occupation du stockage"
                  />
                )}
              </CardBody>
              <CardFooter className="text-xs text-text-muted">
                {historySeries.length} échantillon(s) · résolution brute
              </CardFooter>
            </Card>

            <Card>
              <CardHeader
                title="Activité d'E/S"
                subtitle="Valeurs courantes par périphérique"
                icon={<Activity className="h-3.5 w-3.5" />}
              />
              <CardBody className="space-y-2">
                {disks.length === 0 && (
                  <p className="text-sm text-text-muted">Aucune activité disque mesurée.</p>
                )}
                {disks.map((disk) => (
                  <div
                    key={disk.device}
                    className="rounded-md border border-line/70 bg-surface/50 px-3 py-2.5"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="font-mono text-xs text-text-primary">{disk.device}</span>
                      <span className="text-xs text-text-muted">
                        {formatMetricValue(disk.readIops, 'iops')} R ·{' '}
                        {formatMetricValue(disk.writeIops, 'iops')} W
                      </span>
                    </div>
                    <div className="mt-1.5 grid grid-cols-2 gap-2 text-xs text-text-muted">
                      <span>Lu {formatBytes(disk.readBytes)}</span>
                      <span>Écrit {formatBytes(disk.writeBytes)}</span>
                      <span>Latence R {formatMetricValue(disk.readLatency, 'milliseconds')}</span>
                      <span>Latence W {formatMetricValue(disk.writeLatency, 'milliseconds')}</span>
                    </div>
                  </div>
                ))}
              </CardBody>
            </Card>
          </div>
        </>
      )}
    </div>
  )
}

function Summary({ label, value, tone }: { label: string; value: string; tone: string }) {
  return (
    <div className="rounded-lg border border-line bg-card p-4">
      <p className="text-xs text-text-muted">{label}</p>
      <p className={`mt-1 text-[28px] font-semibold tracking-[-0.03em] ${tone}`}>{value}</p>
    </div>
  )
}
