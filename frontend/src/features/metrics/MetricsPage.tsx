import { useQuery } from '@tanstack/react-query'
import type { EChartsOption } from 'echarts'
import { Activity, BarChart3, RefreshCw } from 'lucide-react'
import { useState } from 'react'
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
import { getCurrentHostMetrics, getHostMetrics } from '../../lib/api/monitoring'
import {
  formatBytes,
  formatChartTime,
  formatDateTime,
  formatMetricValue,
  formatRate,
} from '../../lib/format'
import { average, seriesByLabel } from '../../lib/metrics'
import { useChartTheme } from '../../lib/theme'

type RangeKey = '1h' | '6h' | '24h' | '7d'

type MetricDefinition = {
  name: string
  label: string
  group: string
  unit: string
  labelKey?: string
}

const metrics: MetricDefinition[] = [
  {
    name: 'cpu.percent',
    label: 'Utilisation CPU',
    group: 'Processeur',
    unit: 'percent',
    labelKey: 'core',
  },
  { name: 'cpu.load1', label: 'Charge moyenne (1 min)', group: 'Processeur', unit: 'processes' },
  { name: 'cpu.load5', label: 'Charge moyenne (5 min)', group: 'Processeur', unit: 'processes' },
  { name: 'cpu.load15', label: 'Charge moyenne (15 min)', group: 'Processeur', unit: 'processes' },
  { name: 'memory.used', label: 'Mémoire utilisée', group: 'Mémoire', unit: 'bytes' },
  { name: 'memory.available', label: 'Mémoire disponible', group: 'Mémoire', unit: 'bytes' },
  { name: 'memory.cached', label: 'Cache mémoire', group: 'Mémoire', unit: 'bytes' },
  { name: 'swap.used', label: 'Swap utilisée', group: 'Mémoire', unit: 'bytes' },
  {
    name: 'filesystem.percent',
    label: 'Occupation des systèmes de fichiers',
    group: 'Stockage',
    unit: 'percent',
    labelKey: 'mountpoint',
  },
  {
    name: 'filesystem.used',
    label: 'Espace utilisé',
    group: 'Stockage',
    unit: 'bytes',
    labelKey: 'mountpoint',
  },
  {
    name: 'disk.read_bytes',
    label: 'Octets lus',
    group: 'Stockage',
    unit: 'bytes',
    labelKey: 'device',
  },
  {
    name: 'disk.write_bytes',
    label: 'Octets écrits',
    group: 'Stockage',
    unit: 'bytes',
    labelKey: 'device',
  },
  {
    name: 'disk.read_iops',
    label: 'IOPS lecture',
    group: 'Stockage',
    unit: 'iops',
    labelKey: 'device',
  },
  {
    name: 'disk.write_iops',
    label: 'IOPS écriture',
    group: 'Stockage',
    unit: 'iops',
    labelKey: 'device',
  },
  {
    name: 'disk.read_latency_ms',
    label: 'Latence lecture',
    group: 'Stockage',
    unit: 'milliseconds',
    labelKey: 'device',
  },
  {
    name: 'disk.write_latency_ms',
    label: 'Latence écriture',
    group: 'Stockage',
    unit: 'milliseconds',
    labelKey: 'device',
  },
  {
    name: 'network.rx_bytes_per_second',
    label: 'Débit reçu',
    group: 'Réseau',
    unit: 'bytes_per_second',
    labelKey: 'interface',
  },
  {
    name: 'network.tx_bytes_per_second',
    label: 'Débit envoyé',
    group: 'Réseau',
    unit: 'bytes_per_second',
    labelKey: 'interface',
  },
  {
    name: 'network.rx_bytes',
    label: 'Octets reçus (cumul)',
    group: 'Réseau',
    unit: 'bytes',
    labelKey: 'interface',
  },
  {
    name: 'network.tx_bytes',
    label: 'Octets envoyés (cumul)',
    group: 'Réseau',
    unit: 'bytes',
    labelKey: 'interface',
  },
  {
    name: 'network.errors',
    label: 'Erreurs réseau',
    group: 'Réseau',
    unit: 'count',
    labelKey: 'interface',
  },
  {
    name: 'network.drops',
    label: 'Paquets perdus',
    group: 'Réseau',
    unit: 'count',
    labelKey: 'interface',
  },
  {
    name: 'temperature.celsius',
    label: 'Température',
    group: 'Capteurs',
    unit: 'celsius',
    labelKey: 'sensor',
  },
]

const ranges: Array<{ value: RangeKey; label: string; milliseconds: number }> = [
  { value: '1h', label: '1 h', milliseconds: 60 * 60 * 1000 },
  { value: '6h', label: '6 h', milliseconds: 6 * 60 * 60 * 1000 },
  { value: '24h', label: '24 h', milliseconds: 24 * 60 * 60 * 1000 },
  { value: '7d', label: '7 j', milliseconds: 7 * 24 * 60 * 60 * 1000 },
]

function rangeStart(range: RangeKey): Date {
  return new Date(
    Date.now() -
      (ranges.find((item) => item.value === range)?.milliseconds ?? ranges[2].milliseconds),
  )
}

function axisFormatter(unit: string) {
  return (value: number) => {
    switch (unit) {
      case 'percent':
        return `${value} %`
      case 'bytes':
        return formatBytes(value)
      case 'bytes_per_second':
        return formatRate(value)
      case 'celsius':
        return `${value} °C`
      default:
        return String(value)
    }
  }
}

export function MetricsPage() {
  const chartTheme = useChartTheme()
  const [metricName, setMetricName] = useState('cpu.percent')
  const [labelValue, setLabelValue] = useState('')
  const [range, setRange] = useState<RangeKey>('24h')

  const definition = metrics.find((metric) => metric.name === metricName) ?? metrics[0]
  // The series list (cores, devices…) comes from the latest cycle, so the
  // history only loads the selected series at the finest possible step.
  const current = useQuery({
    queryKey: ['host', 'current-metrics'],
    queryFn: getCurrentHostMetrics,
  })
  const labelKey = definition.labelKey
  const labels = labelKey
    ? Array.from(
        new Set(
          (current.data?.samples ?? [])
            .filter((sample) => sample.name === metricName && sample.labels?.[labelKey])
            .map((sample) => sample.labels?.[labelKey] ?? ''),
        ),
      ).sort()
    : ['global']
  const effectiveLabel =
    labelValue && labels.includes(labelValue)
      ? labelValue
      : labels.includes('all')
        ? 'all'
        : (labels[0] ?? '')
  const history = useQuery({
    queryKey: ['host', 'metrics', metricName, labelKey ? effectiveLabel : 'global', range],
    queryFn: () =>
      getHostMetrics(
        metricName,
        rangeStart(range),
        labelKey ? { labels: { [labelKey]: effectiveLabel } } : {},
      ),
    enabled: !labelKey || effectiveLabel !== '',
  })

  const grouped = labelKey
    ? seriesByLabel(history.data ?? [], metricName, labelKey)
    : new Map([
        [
          'global',
          [...(history.data ?? [])].sort((left, right) =>
            left.recordedAt.localeCompare(right.recordedAt),
          ),
        ],
      ])
  const samples = grouped.get(labelKey ? effectiveLabel : 'global') ?? []
  const values = samples.map((sample) => sample.value)
  const latest = samples[samples.length - 1]
  const minimum = values.length > 0 ? Math.min(...values) : null
  const maximum = values.length > 0 ? Math.max(...values) : null
  const mean = average(values)

  const chartOption: EChartsOption = {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'axis',
      backgroundColor: chartTheme.tooltipBackground,
      borderColor: chartTheme.tooltipBorder,
      textStyle: { color: chartTheme.tooltipText, fontSize: 11 },
    },
    grid: { left: 58, right: 12, top: 18, bottom: 28 },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: samples.map((sample) => sample.recordedAt),
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
      min: definition.unit === 'percent' ? 0 : undefined,
      max: definition.unit === 'percent' ? 100 : undefined,
      axisTick: { show: false },
      axisLine: { show: false },
      axisLabel: {
        color: chartTheme.muted,
        fontSize: 10,
        formatter: axisFormatter(definition.unit),
      },
      splitLine: { lineStyle: { color: chartTheme.grid } },
    },
    series: [
      {
        name: definition.label,
        type: 'line',
        smooth: true,
        showSymbol: false,
        data: samples.map((sample) => sample.value),
        lineStyle: { color: chartTheme.cpu, width: 1.8 },
        itemStyle: { color: chartTheme.cpu },
        areaStyle: { color: chartTheme.cpu, opacity: 0.08 },
      },
    ],
  }

  const groups = Array.from(new Set(metrics.map((metric) => metric.group)))

  return (
    <div className="space-y-4">
      <PageHeader
        title="Métriques"
        breadcrumb={[{ label: 'Système' }, { label: 'Métriques' }]}
        badge={<Badge tone="neutral">{samples.length} échantillon(s)</Badge>}
        description="Historiques bruts des métriques de l'hôte. Les échantillons sont bornés par la période sélectionnée et la rétention configurée."
        actions={
          <Button
            size="sm"
            variant="secondary"
            icon={<RefreshCw className="h-3.5 w-3.5" />}
            onClick={() => {
              void history.refetch()
            }}
          >
            Actualiser
          </Button>
        }
      />

      <div className="flex flex-wrap items-center gap-2 rounded-lg border border-line bg-card p-2.5">
        <Select
          aria-label="Choisir une métrique"
          value={metricName}
          onChange={(event) => {
            setMetricName(event.target.value)
            setLabelValue('')
          }}
          className="min-w-[260px]"
        >
          {groups.map((group) => (
            <optgroup key={group} label={group}>
              {metrics
                .filter((metric) => metric.group === group)
                .map((metric) => (
                  <option key={metric.name} value={metric.name}>
                    {metric.label}
                  </option>
                ))}
            </optgroup>
          ))}
        </Select>

        {definition.labelKey && labels.length > 1 && (
          <Select
            aria-label={`Choisir ${definition.labelKey}`}
            value={effectiveLabel}
            onChange={(event) => setLabelValue(event.target.value)}
            className="min-w-[170px]"
          >
            {labels.map((label) => (
              <option key={label} value={label}>
                {label}
              </option>
            ))}
          </Select>
        )}

        <span className="font-mono text-[10px] text-text-muted">{definition.name}</span>

        <div className="ml-auto">
          <SegmentedTabs
            ariaLabel="Période de l'historique"
            value={range}
            onChange={setRange}
            options={ranges.map((item) => ({ value: item.value, label: item.label }))}
          />
        </div>
      </div>

      {history.isError && (
        <ErrorState
          message={history.error.message}
          onRetry={() => {
            void history.refetch()
          }}
        />
      )}
      {history.isLoading && !history.isError && (
        <LoadingState label="Chargement de l'historique…" rows={2} />
      )}

      {!history.isLoading && !history.isError && samples.length === 0 && (
        <EmptyState
          icon={BarChart3}
          title="Aucun échantillon"
          description="Cette métrique n'a pas encore été collectée sur la période sélectionnée, ou n'est pas disponible sur cet hôte."
        />
      )}

      {samples.length > 0 && (
        <>
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <Stat
              label="Dernière valeur"
              value={formatMetricValue(latest?.value, definition.unit)}
              hint={latest ? formatDateTime(latest.recordedAt) : '—'}
            />
            <Stat
              label="Moyenne"
              value={formatMetricValue(mean, definition.unit)}
              hint={`${samples.length} échantillon(s)`}
            />
            <Stat
              label="Minimum"
              value={formatMetricValue(minimum, definition.unit)}
              hint="Sur la période"
            />
            <Stat
              label="Maximum"
              value={formatMetricValue(maximum, definition.unit)}
              hint="Sur la période"
            />
          </div>

          <Card>
            <CardHeader
              title={definition.label}
              subtitle={`${range.replace('h', ' heure(s)').replace('7d', '7 jours')} · ${effectiveLabel === 'global' ? 'série globale' : `${definition.labelKey} ${effectiveLabel}`}`}
              icon={<Activity className="h-3.5 w-3.5" />}
              actions={<Badge tone="neutral">{definition.unit}</Badge>}
            />
            <CardBody>
              <Chart
                option={chartOption}
                height={320}
                label={`Historique de ${definition.label}`}
              />
            </CardBody>
            <CardFooter className="flex flex-wrap items-center justify-between gap-2 font-mono text-[10px] text-text-muted">
              <span>{samples.length} point(s) · résolution brute</span>
              <span>Les valeurs manquantes ne sont pas interpolées.</span>
            </CardFooter>
          </Card>

          <Card>
            <CardHeader
              title="Derniers échantillons"
              subtitle="Les 20 valeurs les plus récentes"
              icon={<BarChart3 className="h-3.5 w-3.5" />}
            />
            <CardBody className="p-0">
              <Table minWidthClassName="min-w-[520px]" className="rounded-none border-0">
                <THead>
                  <tr>
                    <TH>Horodatage</TH>
                    <TH>Valeur</TH>
                    <TH>Étiquettes</TH>
                  </tr>
                </THead>
                <TBody>
                  {[...samples]
                    .reverse()
                    .slice(0, 20)
                    .map((sample) => (
                      <TR key={`${sample.recordedAt}-${sample.value}`}>
                        <TD className="font-mono text-xs">{formatDateTime(sample.recordedAt)}</TD>
                        <TD className="font-mono text-xs text-text-primary">
                          {formatMetricValue(sample.value, sample.unit)}
                        </TD>
                        <TD className="font-mono text-[10px] text-text-muted">
                          {sample.labels
                            ? Object.entries(sample.labels)
                                .map(([key, value]) => `${key}=${value}`)
                                .join(' · ')
                            : '—'}
                        </TD>
                      </TR>
                    ))}
                </TBody>
              </Table>
            </CardBody>
          </Card>
        </>
      )}
    </div>
  )
}

function Stat({ label, value, hint }: { label: string; value: string; hint: string }) {
  return (
    <div className="rounded-lg border border-line bg-card p-4">
      <p className="font-mono text-[11px] uppercase tracking-wider text-text-muted">{label}</p>
      <p className="mt-1 font-mono text-xl font-semibold tracking-tight text-text-primary">
        {value}
      </p>
      <p className="mt-1 font-mono text-[10px] text-text-muted">{hint}</p>
    </div>
  )
}
