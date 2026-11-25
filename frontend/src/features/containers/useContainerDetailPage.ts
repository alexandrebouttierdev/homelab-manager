import { loadState } from '../../hooks/loadState'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { EChartsOption } from 'echarts'
import { useMemo, useState } from 'react'
import { useParams } from 'react-router-dom'
import {
  getContainer,
  getContainerEvents,
  getContainerMetrics,
  getCurrentContainerMetrics,
  type MetricSample,
} from '../../lib/api/monitoring'
import { formatPorts, imageTag } from '../../lib/containers'
import { formatShortTime } from '../../lib/format'
import { latestOne, series } from '../../lib/metrics'
import { useRefreshInterval } from '../../lib/refresh'
import { useChartTheme } from '../../lib/theme'
import { useContainerAction, useContainerLogs } from './useContainerControls'

export type RangeKey = '1h' | '6h' | '24h'

export const ranges: Array<{ value: RangeKey; label: string; milliseconds: number }> = [
  { value: '1h', label: '1 h', milliseconds: 60 * 60 * 1000 },
  { value: '6h', label: '6 h', milliseconds: 6 * 60 * 60 * 1000 },
  { value: '24h', label: '24 h', milliseconds: 24 * 60 * 60 * 1000 },
]

export function rangeStart(range: RangeKey): Date {
  const milliseconds =
    ranges.find((item) => item.value === range)?.milliseconds ?? ranges[0].milliseconds
  return new Date(Date.now() - milliseconds)
}

export function buildTimestamps(...seriesList: Array<Array<{ recordedAt: string }>>): string[] {
  return Array.from(
    new Set(seriesList.flatMap((values) => values.map((value) => value.recordedAt))),
  ).sort()
}

export function buildChartOption(
  cpuSeries: MetricSample[],
  memorySeries: Array<{ recordedAt: string; value: number }>,
  theme: ReturnType<typeof useChartTheme>,
): EChartsOption {
  const timestamps = buildTimestamps(cpuSeries, memorySeries)
  const cpuMap = new Map(cpuSeries.map((sample) => [sample.recordedAt, sample.value]))
  const memoryMap = new Map(memorySeries.map((sample) => [sample.recordedAt, sample.value]))

  return {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'axis',
      backgroundColor: theme.tooltipBackground,
      borderColor: theme.tooltipBorder,
      textStyle: { color: theme.tooltipText, fontSize: 11 },
    },
    legend: {
      data: ['CPU', 'RAM'],
      right: 0,
      top: 0,
      itemWidth: 8,
      itemHeight: 8,
      icon: 'roundRect',
      textStyle: { color: theme.text, fontSize: 10, fontFamily: 'JetBrains Mono' },
    },
    grid: { left: 42, right: 8, top: 30, bottom: 24 },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: timestamps,
      axisTick: { show: false },
      axisLine: { lineStyle: { color: theme.axis } },
      axisLabel: {
        color: theme.muted,
        fontSize: 10,
        formatter: (value: string) => formatShortTime(value),
      },
    },
    yAxis: {
      type: 'value',
      min: 0,
      max: 100,
      axisTick: { show: false },
      axisLine: { show: false },
      axisLabel: { color: theme.muted, fontSize: 10, formatter: '{value} %' },
      splitLine: { lineStyle: { color: theme.grid } },
    },
    series: [
      {
        name: 'CPU',
        type: 'line',
        smooth: true,
        showSymbol: false,
        connectNulls: true,
        data: timestamps.map((timestamp) => cpuMap.get(timestamp) ?? null),
        lineStyle: { color: theme.cpu, width: 1.8 },
        itemStyle: { color: theme.cpu },
        areaStyle: { color: theme.cpu, opacity: 0.08 },
      },
      {
        name: 'RAM',
        type: 'line',
        smooth: true,
        showSymbol: false,
        connectNulls: true,
        data: timestamps.map((timestamp) => memoryMap.get(timestamp) ?? null),
        lineStyle: { color: theme.ram, width: 1.8 },
        itemStyle: { color: theme.ram },
        areaStyle: { color: theme.ram, opacity: 0.06 },
      },
    ],
  }
}

/** Logic and view model of ContainerDetailPage. */
export function useContainerDetailPage() {
  const queryClient = useQueryClient()
  const refreshInterval = useRefreshInterval()
  const chartTheme = useChartTheme()
  const containerID = useParams().containerID ?? ''
  const [range, setRange] = useState<RangeKey>('1h')

  const container = useQuery({
    queryKey: ['container', containerID],
    queryFn: () => getContainer(containerID),
    enabled: Boolean(containerID),
    refetchInterval: refreshInterval || false,
  })
  const current = useQuery({
    queryKey: ['container', containerID, 'current-metrics'],
    queryFn: () => getCurrentContainerMetrics(containerID),
    enabled: Boolean(containerID),
    refetchInterval: refreshInterval || false,
  })
  const cpuHistory = useQuery({
    queryKey: ['container', containerID, 'metrics', 'cpu.percent', range],
    queryFn: () => getContainerMetrics(containerID, 'cpu.percent', rangeStart(range)),
    enabled: Boolean(containerID),
  })
  const memoryHistory = useQuery({
    queryKey: ['container', containerID, 'metrics', 'memory.percent', range],
    queryFn: () => getContainerMetrics(containerID, 'memory.percent', rangeStart(range)),
    enabled: Boolean(containerID),
  })
  const events = useQuery({
    queryKey: ['container', containerID, 'events'],
    queryFn: () =>
      getContainerEvents(containerID, new Date(Date.now() - 24 * 60 * 60 * 1000), new Date(), 100),
    enabled: Boolean(containerID),
    refetchInterval: refreshInterval || false,
  })

  const samples = current.data?.samples ?? []
  const cpu = latestOne(samples, 'cpu.percent')?.value ?? null
  const memoryUsage = latestOne(samples, 'memory.usage_bytes')?.value ?? null
  const memoryLimit = latestOne(samples, 'memory.limit_bytes')?.value ?? null
  const memoryPercent = latestOne(samples, 'memory.percent')?.value ?? null
  const processes = latestOne(samples, 'process.count')?.value ?? null
  const rxRate = latestOne(samples, 'network.rx_bytes_per_second')?.value ?? null
  const txRate = latestOne(samples, 'network.tx_bytes_per_second')?.value ?? null
  const rxBytes = latestOne(samples, 'network.rx_bytes')?.value ?? null
  const txBytes = latestOne(samples, 'network.tx_bytes')?.value ?? null

  const cpuSeries = useMemo(() => series(cpuHistory.data ?? [], 'cpu.percent'), [cpuHistory.data])
  const memorySeries = useMemo(
    () => series(memoryHistory.data ?? [], 'memory.percent'),
    [memoryHistory.data],
  )
  const chartOption = useMemo(
    () => buildChartOption(cpuSeries, memorySeries, chartTheme),
    [chartTheme, cpuSeries, memorySeries],
  )

  const data = container.data
  const tag = data ? imageTag(data.image) : null
  const ports = data ? formatPorts(data.ports) : []
  const removed = Boolean(data?.removedAt)
  const logs = useContainerLogs(data ? containerID : '', removed)
  const actions = useContainerAction(containerID, data?.name ?? '', data?.state ?? '', removed)

  return {
    eventList: events.data ?? [],
    queryClient,
    containerID,
    range,
    setRange,
    containerState: loadState(container),
    cpuHistory: loadState(cpuHistory),
    memoryHistory: loadState(memoryHistory),
    events: loadState(events),
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
    logs,
    actions,
  }
}
