import { useQuery } from '@tanstack/react-query'
import type { EChartsOption } from 'echarts'
import { ArrowDownRight, ArrowUpRight, Cable, RefreshCw } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Chart } from '../../components/charts/Chart'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardFooter, CardHeader } from '../../components/ui/Card'
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/DataTable'
import { EmptyState } from '../../components/ui/EmptyState'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { PageHeader } from '../../components/ui/PageHeader'
import { SegmentedTabs } from '../../components/ui/SegmentedTabs'
import { Select } from '../../components/ui/Select'
import { getCurrentHostMetrics, getHostMetrics, type MetricSample } from '../../lib/api/monitoring'
import { formatBytes, formatChartTime, formatNumber, formatRate } from '../../lib/format'
import { interfaceSummary } from '../../lib/host'
import { series } from '../../lib/metrics'
import { useRefreshInterval } from '../../lib/refresh'
import { useChartTheme } from '../../lib/theme'

type RangeKey = '1h' | '6h' | '24h'

const ranges: Array<{ value: RangeKey; label: string; milliseconds: number }> = [
  { value: '1h', label: '1 h', milliseconds: 60 * 60 * 1000 },
  { value: '6h', label: '6 h', milliseconds: 6 * 60 * 60 * 1000 },
  { value: '24h', label: '24 h', milliseconds: 24 * 60 * 60 * 1000 },
]

function rangeStart(range: RangeKey): Date {
  return new Date(
    Date.now() -
      (ranges.find((item) => item.value === range)?.milliseconds ?? ranges[0].milliseconds),
  )
}

async function getRateHistory(
  iface: string,
  range: RangeKey,
): Promise<{ rx: MetricSample[]; tx: MetricSample[] }> {
  const from = rangeStart(range)
  const [rx, tx] = await Promise.all([
    getHostMetrics('network.rx_bytes_per_second', from, { labels: { interface: iface } }),
    getHostMetrics('network.tx_bytes_per_second', from, { labels: { interface: iface } }),
  ])
  return {
    rx: series(rx, 'network.rx_bytes_per_second', { interface: iface }),
    tx: series(tx, 'network.tx_bytes_per_second', { interface: iface }),
  }
}

export function NetworkPage() {
  const refreshInterval = useRefreshInterval()
  const chartTheme = useChartTheme()
  const [range, setRange] = useState<RangeKey>('1h')
  const [selectedInterface, setSelectedInterface] = useState('')

  const current = useQuery({
    queryKey: ['host', 'current-metrics'],
    queryFn: getCurrentHostMetrics,
    refetchInterval: refreshInterval || false,
  })
  const samples = current.data?.samples ?? []
  const interfaces = interfaceSummary(samples).filter((iface) => iface.name !== 'lo')
  const iface = selectedInterface || interfaces[0]?.name || ''

  const history = useQuery({
    queryKey: ['network', 'rates', iface, range],
    queryFn: () => getRateHistory(iface, range),
    enabled: Boolean(iface),
  })

  const totalRx = interfaces.reduce((sum, entry) => sum + (entry.rx ?? 0), 0)
  const totalTx = interfaces.reduce((sum, entry) => sum + (entry.tx ?? 0), 0)
  const currentRxRate = interfaces.reduce((sum, entry) => sum + (entry.rxRate ?? 0), 0)
  const currentTxRate = interfaces.reduce((sum, entry) => sum + (entry.txRate ?? 0), 0)
  const totalErrors = interfaces.reduce((sum, entry) => sum + entry.errors + entry.drops, 0)

  const rxSeries = useMemo(() => history.data?.rx ?? [], [history.data])
  const txSeries = useMemo(() => history.data?.tx ?? [], [history.data])

  const chartOption = useMemo<EChartsOption>(() => {
    const timestamps = Array.from(
      new Set([
        ...rxSeries.map((sample) => sample.recordedAt),
        ...txSeries.map((sample) => sample.recordedAt),
      ]),
    ).sort()
    const rxMap = new Map(rxSeries.map((sample) => [sample.recordedAt, sample.value]))
    const txMap = new Map(txSeries.map((sample) => [sample.recordedAt, sample.value]))

    return {
      backgroundColor: 'transparent',
      tooltip: {
        trigger: 'axis',
        backgroundColor: chartTheme.tooltipBackground,
        borderColor: chartTheme.tooltipBorder,
        textStyle: { color: chartTheme.tooltipText, fontSize: 11 },
      },
      legend: {
        data: ['RX', 'TX'],
        right: 0,
        top: 0,
        itemWidth: 8,
        itemHeight: 8,
        icon: 'roundRect',
        textStyle: { color: chartTheme.text, fontSize: 10, fontFamily: 'JetBrains Mono' },
      },
      grid: { left: 54, right: 10, top: 30, bottom: 28 },
      xAxis: {
        type: 'category',
        boundaryGap: false,
        data: timestamps,
        axisTick: { show: false },
        axisLine: { lineStyle: { color: chartTheme.axis } },
        axisLabel: {
          color: chartTheme.muted,
          fontSize: 10,
          formatter: (value: string) => formatChartTime(value),
        },
      },
      yAxis: {
        type: 'value',
        axisTick: { show: false },
        axisLine: { show: false },
        axisLabel: {
          color: chartTheme.muted,
          fontSize: 10,
          formatter: (value: number) => formatBytes(value),
        },
        splitLine: { lineStyle: { color: chartTheme.grid } },
      },
      series: [
        {
          name: 'RX',
          type: 'line',
          smooth: true,
          showSymbol: false,
          connectNulls: true,
          data: timestamps.map((timestamp) => rxMap.get(timestamp) ?? null),
          lineStyle: { color: chartTheme.net, width: 1.8 },
          itemStyle: { color: chartTheme.net },
          areaStyle: { color: chartTheme.net, opacity: 0.08 },
        },
        {
          name: 'TX',
          type: 'line',
          smooth: true,
          showSymbol: false,
          connectNulls: true,
          data: timestamps.map((timestamp) => txMap.get(timestamp) ?? null),
          lineStyle: { color: chartTheme.accent, width: 1.8 },
          itemStyle: { color: chartTheme.accent },
          areaStyle: { color: chartTheme.accent, opacity: 0.05 },
        },
      ],
    }
  }, [chartTheme, rxSeries, txSeries])

  return (
    <div className="space-y-4">
      <PageHeader
        title="Réseau"
        breadcrumb={[{ label: 'Système' }, { label: 'Réseau' }]}
        badge={
          <Badge tone={totalErrors > 0 ? 'warning' : 'success'}>
            {totalErrors > 0 ? `${formatNumber(totalErrors)} erreur(s)` : 'Aucune erreur'}
          </Badge>
        }
        description="Interfaces locales, compteurs cumulés et débits mesurés par le collecteur système. Les adresses et compteurs proviennent du noyau Linux."
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
        <LoadingState label="Chargement des interfaces réseau…" rows={2} />
      )}

      {!current.isLoading && !current.isError && (
        <>
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <Summary
              label="Trafic reçu"
              value={formatBytes(totalRx)}
              hint={`${interfaces.length} interface(s)`}
              tone="text-net"
            />
            <Summary
              label="Trafic envoyé"
              value={formatBytes(totalTx)}
              hint="Compteurs cumulés"
              tone="text-accent"
            />
            <Summary
              label="Débit courant"
              value={formatRate(currentRxRate)}
              hint={`TX ${formatRate(currentTxRate)}`}
              tone="text-text-primary"
            />
            <Summary
              label="Erreurs & pertes"
              value={formatNumber(totalErrors)}
              hint="Cumul sur les interfaces"
              tone={totalErrors > 0 ? 'text-warning' : 'text-success'}
            />
          </div>

          {interfaces.length === 0 ? (
            <EmptyState
              icon={Cable}
              title="Aucune interface mesurée"
              description="Le collecteur système n'a pas publié de compteurs réseau pour cette machine."
            />
          ) : (
            <Card>
              <CardHeader
                title="Interfaces réseau"
                subtitle={`${interfaces.length} interface(s) active(s) ou observée(s)`}
                icon={<Cable className="h-3.5 w-3.5" />}
              />
              <CardBody className="p-0">
                <Table minWidthClassName="min-w-[980px]" className="rounded-none border-0">
                  <THead>
                    <tr>
                      <TH>Interface</TH>
                      <TH>Adresses</TH>
                      <TH>MAC</TH>
                      <TH>RX</TH>
                      <TH>TX</TH>
                      <TH>Débits</TH>
                      <TH>Erreurs</TH>
                      <TH>État</TH>
                    </tr>
                  </THead>
                  <TBody>
                    {interfaces.map((entry) => (
                      <TR key={entry.name}>
                        <TD className="font-mono text-xs text-text-primary">{entry.name}</TD>
                        <TD>
                          <div className="space-y-0.5 font-mono text-[10px] text-text-secondary">
                            {entry.addresses.length === 0 && (
                              <span className="text-text-muted">—</span>
                            )}
                            {entry.addresses.map((address) => (
                              <span key={address} className="block">
                                {address}
                              </span>
                            ))}
                          </div>
                        </TD>
                        <TD className="font-mono text-[10px]">{entry.mac || '—'}</TD>
                        <TD className="font-mono text-xs">{formatBytes(entry.rx)}</TD>
                        <TD className="font-mono text-xs">{formatBytes(entry.tx)}</TD>
                        <TD>
                          <div className="space-y-1 font-mono text-[10px]">
                            <span className="flex items-center gap-1 text-text-secondary">
                              <ArrowDownRight className="h-3 w-3 text-net" />{' '}
                              {formatRate(entry.rxRate)}
                            </span>
                            <span className="flex items-center gap-1 text-text-secondary">
                              <ArrowUpRight className="h-3 w-3 text-accent" />{' '}
                              {formatRate(entry.txRate)}
                            </span>
                          </div>
                        </TD>
                        <TD>
                          <Badge tone={entry.errors + entry.drops > 0 ? 'warning' : 'success'}>
                            {formatNumber(entry.errors + entry.drops)}
                          </Badge>
                        </TD>
                        <TD>
                          <Badge
                            tone={entry.rx != null || entry.tx != null ? 'success' : 'neutral'}
                            dot={entry.rx != null || entry.tx != null}
                          >
                            {entry.rx != null || entry.tx != null ? 'Active' : 'Inconnue'}
                          </Badge>
                        </TD>
                      </TR>
                    ))}
                  </TBody>
                </Table>
              </CardBody>
              <CardFooter className="flex flex-wrap items-center justify-between gap-2 font-mono text-[10px] text-text-muted">
                <span>Compteurs cumulés depuis le démarrage de la machine.</span>
                <span>Loopback exclu de la vue synthétique.</span>
              </CardFooter>
            </Card>
          )}

          <Card>
            <CardHeader
              title="Débit par interface"
              subtitle={iface ? `Interface ${iface}` : 'Aucune interface sélectionnée'}
              icon={<ArrowDownRight className="h-3.5 w-3.5" />}
              actions={
                <div className="flex flex-wrap items-center gap-2">
                  <Select
                    aria-label="Choisir une interface réseau"
                    value={iface}
                    onChange={(event) => setSelectedInterface(event.target.value)}
                    className="h-8 max-w-[170px] font-mono text-[11px]"
                  >
                    {interfaces.map((entry) => (
                      <option key={entry.name} value={entry.name}>
                        {entry.name}
                      </option>
                    ))}
                  </Select>
                  <SegmentedTabs
                    ariaLabel="Période de l'historique réseau"
                    value={range}
                    onChange={setRange}
                    options={ranges.map((item) => ({ value: item.value, label: item.label }))}
                  />
                </div>
              }
            />
            <CardBody>
              {history.isError && <ErrorState message={history.error.message} />}
              {!history.isError && rxSeries.length === 0 && txSeries.length === 0 && (
                <p className="py-8 text-center text-sm text-text-muted">
                  Aucun débit historique sur la période. Les débits nécessitent au moins deux
                  collectes consécutives.
                </p>
              )}
              {(rxSeries.length > 0 || txSeries.length > 0) && (
                <Chart option={chartOption} height={300} label="Historique des débits réseau" />
              )}
            </CardBody>
            <CardFooter className="flex flex-wrap items-center justify-between gap-2 font-mono text-[10px] text-text-muted">
              <span>
                {rxSeries.length} point(s) RX · {txSeries.length} point(s) TX
              </span>
              <span>Les débits sont calculés entre deux collectes successives.</span>
            </CardFooter>
          </Card>
        </>
      )}
    </div>
  )
}

function Summary({
  label,
  value,
  hint,
  tone,
}: {
  label: string
  value: string
  hint: string
  tone: string
}) {
  return (
    <div className="rounded-lg border border-line bg-card p-4">
      <p className="font-mono text-[11px] uppercase tracking-wider text-text-muted">{label}</p>
      <p className={`mt-1 font-mono text-2xl font-semibold tracking-tight ${tone}`}>{value}</p>
      <p className="mt-1 font-mono text-[10px] text-text-muted">{hint}</p>
    </div>
  )
}
