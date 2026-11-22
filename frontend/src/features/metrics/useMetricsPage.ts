import { loadState } from '../../hooks/loadState'
import { useQuery } from '@tanstack/react-query'
import type { EChartsOption } from 'echarts'
import { useState } from 'react'
import { getCurrentHostMetrics, getHostMetrics } from '../../lib/api/monitoring'
import { formatBytes, formatChartTime, formatRate } from '../../lib/format'
import { average, seriesByLabel } from '../../lib/metrics'
import { useChartTheme } from '../../lib/theme'

export type RangeKey = '1h' | '6h' | '24h' | '7d'

export type MetricDefinition = {
  name: string
  label: string
  group: string
  unit: string
  labelKey?: string
}

export const metrics: MetricDefinition[] = [
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

export const ranges: Array<{ value: RangeKey; label: string; milliseconds: number }> = [
  { value: '1h', label: '1 h', milliseconds: 60 * 60 * 1000 },
  { value: '6h', label: '6 h', milliseconds: 6 * 60 * 60 * 1000 },
  { value: '24h', label: '24 h', milliseconds: 24 * 60 * 60 * 1000 },
  { value: '7d', label: '7 j', milliseconds: 7 * 24 * 60 * 60 * 1000 },
]

export function rangeStart(range: RangeKey): Date {
  return new Date(
    Date.now() -
      (ranges.find((item) => item.value === range)?.milliseconds ?? ranges[2].milliseconds),
  )
}

export function axisFormatter(unit: string) {
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

/** Logic and view model of MetricsPage. */
export function useMetricsPage() {
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

  const groups = Array.from(new Set(metrics.map((metric) => metric.group))).map((group) => ({
    group,
    metrics: metrics.filter((metric) => metric.group === group),
  }))

  return {
    metricName,
    setMetricName,
    setLabelValue,
    range,
    setRange,
    definition,
    labels,
    effectiveLabel,
    history: loadState(history),
    samples,
    latest,
    minimum,
    maximum,
    mean,
    chartOption,
    groups,
  }
}
