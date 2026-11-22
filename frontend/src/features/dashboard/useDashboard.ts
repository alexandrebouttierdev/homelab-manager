import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { EChartsOption } from 'echarts'
import { useMemo } from 'react'
import {
  getCurrentHostMetrics,
  getDashboard,
  getHostMetrics,
  listContainers,
  listEvents,
  stepForRange,
} from '../../lib/api/monitoring'
import { collectorStatusLabel } from '../../lib/collectors'
import { containerStateLabel, isErrorState, isRunning } from '../../lib/containers'
import { eventTypeLabel } from '../../lib/events'
import { formatRelativeTime, formatShortTime } from '../../lib/format'
import { filesystemSummary, interfaceSummary } from '../../lib/host'
import { latestOne, memoryPercentSeries, series } from '../../lib/metrics'
import { useRefreshInterval } from '../../lib/refresh'
import { useChartTheme } from '../../lib/theme'

const oneHourAgo = () => new Date(Date.now() - 60 * 60 * 1000)

// Both series must share the same range and step: memoryPercentSeries pairs
// them by timestamp.
async function getMemoryHistory() {
  const to = new Date()
  const from = new Date(to.getTime() - 60 * 60 * 1000)
  const step = stepForRange(to.getTime() - from.getTime())
  const [used, total] = await Promise.all([
    getHostMetrics('memory.used', from, { to, step }),
    getHostMetrics('memory.total', from, { to, step }),
  ])
  return memoryPercentSeries(used, total)
}

export type AttentionItem = {
  key: string
  title: string
  detail: string
  tone: 'warning' | 'danger' | 'info'
  to?: string
}

/** View model of the overview page: every value is ready to display. */
export function useDashboard() {
  const queryClient = useQueryClient()
  const refreshInterval = useRefreshInterval()
  const chartTheme = useChartTheme()

  const dashboard = useQuery({
    queryKey: ['dashboard'],
    queryFn: getDashboard,
    refetchInterval: refreshInterval || false,
  })
  const current = useQuery({
    queryKey: ['host', 'current-metrics'],
    queryFn: getCurrentHostMetrics,
    refetchInterval: refreshInterval || false,
  })
  const containers = useQuery({
    queryKey: ['containers', false],
    queryFn: () => listContainers(false),
    refetchInterval: refreshInterval || false,
  })
  const events = useQuery({
    queryKey: ['events', 'dashboard'],
    queryFn: () => listEvents({ limit: 6 }),
    refetchInterval: refreshInterval || false,
  })
  const cpuHistory = useQuery({
    queryKey: ['host', 'metrics', 'cpu.percent', '1h'],
    queryFn: () => getHostMetrics('cpu.percent', oneHourAgo(), { labels: { core: 'all' } }),
    refetchInterval: refreshInterval ? 60_000 : false,
  })
  const memoryHistory = useQuery({
    queryKey: ['host', 'metrics', 'memory.percent', '1h'],
    queryFn: getMemoryHistory,
    refetchInterval: refreshInterval ? 60_000 : false,
  })

  const samples = current.data?.samples ?? []
  const cpu = latestOne(samples, 'cpu.percent', { core: 'all' })
  const cores = latestOne(samples, 'cpu.cores')?.value
  const load1 = latestOne(samples, 'cpu.load1')?.value
  const load5 = latestOne(samples, 'cpu.load5')?.value
  const load15 = latestOne(samples, 'cpu.load15')?.value
  const memoryTotal = latestOne(samples, 'memory.total')?.value
  const memoryUsed = latestOne(samples, 'memory.used')?.value
  const memoryAvailable = latestOne(samples, 'memory.available')?.value
  const memoryCached = latestOne(samples, 'memory.cached')?.value
  const swapTotal = latestOne(samples, 'swap.total')?.value
  const swapUsed = latestOne(samples, 'swap.used')?.value
  const memoryPercent = memoryTotal && memoryUsed != null ? (memoryUsed / memoryTotal) * 100 : null

  const filesystems = filesystemSummary(samples)
  const interfaces = interfaceSummary(samples).filter((iface) => iface.name !== 'lo')
  const primaryInterface = interfaces[0]

  const cpuValues = series(cpuHistory.data ?? [], 'cpu.percent', { core: 'all' }).map(
    (sample) => sample.value,
  )
  const memoryValues = (memoryHistory.data ?? []).map((sample) => sample.value)

  const chartOption = useMemo<EChartsOption>(() => {
    const cpuSeries = series(cpuHistory.data ?? [], 'cpu.percent', { core: 'all' })
    const memorySeries = memoryHistory.data ?? []
    const timestamps = Array.from(
      new Set([
        ...cpuSeries.map((sample) => sample.recordedAt),
        ...memorySeries.map((sample) => sample.recordedAt),
      ]),
    ).sort()
    const cpuMap = new Map(cpuSeries.map((sample) => [sample.recordedAt, sample.value]))
    const memoryMap = new Map(memorySeries.map((sample) => [sample.recordedAt, sample.value]))

    return {
      backgroundColor: 'transparent',
      tooltip: {
        trigger: 'axis',
        backgroundColor: chartTheme.tooltipBackground,
        borderColor: chartTheme.tooltipBorder,
        textStyle: { color: chartTheme.tooltipText, fontSize: 11 },
      },
      legend: {
        data: ['CPU', 'RAM'],
        right: 0,
        top: 0,
        itemWidth: 8,
        itemHeight: 8,
        icon: 'roundRect',
        textStyle: { color: chartTheme.text, fontSize: 10, fontFamily: 'JetBrains Mono' },
      },
      grid: { left: 42, right: 8, top: 30, bottom: 24 },
      xAxis: {
        type: 'category',
        boundaryGap: false,
        data: timestamps,
        axisTick: { show: false },
        axisLine: { lineStyle: { color: chartTheme.axis } },
        axisLabel: {
          color: chartTheme.muted,
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
        axisLabel: { color: chartTheme.muted, fontSize: 10, formatter: '{value} %' },
        splitLine: { lineStyle: { color: chartTheme.grid } },
      },
      series: [
        {
          name: 'CPU',
          type: 'line',
          smooth: true,
          showSymbol: false,
          connectNulls: true,
          data: timestamps.map((timestamp) => cpuMap.get(timestamp) ?? null),
          lineStyle: { color: chartTheme.cpu, width: 1.8 },
          itemStyle: { color: chartTheme.cpu },
          areaStyle: { color: chartTheme.cpu, opacity: 0.08 },
        },
        {
          name: 'RAM',
          type: 'line',
          smooth: true,
          showSymbol: false,
          connectNulls: true,
          data: timestamps.map((timestamp) => memoryMap.get(timestamp) ?? null),
          lineStyle: { color: chartTheme.ram, width: 1.8 },
          itemStyle: { color: chartTheme.ram },
          areaStyle: { color: chartTheme.ram, opacity: 0.06 },
        },
      ],
    }
  }, [chartTheme, cpuHistory.data, memoryHistory.data])

  const systemStatus = dashboard.data?.collectors.system.status
  const dockerStatus = dashboard.data?.collectors.docker.status
  const systemOk = systemStatus === 'ok'
  const dockerOk = dockerStatus === 'ok'
  const starting =
    !systemOk || !dockerOk
      ? [systemStatus, dockerStatus].every((status) => status === 'ok' || status === 'starting')
      : false
  const overallTone: 'success' | 'info' | 'warning' | 'danger' =
    systemOk && dockerOk
      ? 'success'
      : starting
        ? 'info'
        : systemOk || dockerOk
          ? 'warning'
          : 'danger'
  const overallLabel =
    systemOk && dockerOk
      ? 'Statut général : optimal'
      : starting
        ? 'Statut général : démarrage'
        : systemOk || dockerOk
          ? 'Statut général : dégradé'
          : 'Statut général : indisponible'

  const containerList = containers.data ?? []
  const runningContainers = containerList.filter((container) => isRunning(container.state))
  const problemContainers = containerList.filter((container) => isErrorState(container.state))
  const stoppedContainers = containerList.filter(
    (container) => !isRunning(container.state) && !isErrorState(container.state),
  )
  const highlightedContainers = [
    ...problemContainers,
    ...runningContainers,
    ...stoppedContainers,
  ].slice(0, 6)

  const attentionItems: AttentionItem[] = []
  // A collector that is still starting is not an anomaly yet.
  for (const [key, name] of [
    ['system', 'Collecte système'],
    ['docker', 'Collecte Docker'],
  ] as const) {
    const collector = dashboard.data?.collectors[key]
    if (!collector || collector.status === 'ok' || collector.status === 'starting') continue
    attentionItems.push({
      key,
      title: `${name} : ${collectorStatusLabel(collector.status).toLowerCase()}`,
      detail: collector.lastSuccessAt
        ? `Dernier succès ${formatRelativeTime(collector.lastSuccessAt)}.`
        : key === 'docker'
          ? 'Le démon Docker local ne répond pas.'
          : 'Aucune collecte système réussie.',
      tone: collector.status === 'unavailable' ? 'danger' : 'warning',
    })
  }
  for (const container of problemContainers.slice(0, 2)) {
    attentionItems.push({
      key: `container-${container.id}`,
      title: container.name,
      detail: `${containerStateLabel(container.state)} · ${container.status || 'état instable'}`,
      tone: container.state === 'dead' ? 'danger' : 'warning',
      to: `/containers/${encodeURIComponent(container.id)}`,
    })
  }
  for (const event of (events.data ?? [])
    .filter((item) => item.severity === 'warning' || item.severity === 'error')
    .slice(0, 2)) {
    attentionItems.push({
      key: `event-${event.id}`,
      title: eventTypeLabel(event.type),
      detail: event.message,
      tone: event.severity === 'error' ? 'danger' : 'warning',
      to: event.containerId ? `/containers/${encodeURIComponent(event.containerId)}` : '/events',
    })
  }

  const isLoading = dashboard.isLoading || current.isLoading
  const hasError = dashboard.isError || current.isError
  const lastCollection = current.data?.lastSuccessAt ?? dashboard.data?.host.collector.lastSuccessAt

  return {
    refresh: () => void queryClient.invalidateQueries(),
    refreshInterval,
    dashboard: dashboard.data ?? null,
    isLoading,
    error: hasError ? (dashboard.error ?? current.error) : null,
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
    containersError: containers.error,
    attentionItems,
    events: events.data ?? [],
    eventsError: events.error,
  }
}
