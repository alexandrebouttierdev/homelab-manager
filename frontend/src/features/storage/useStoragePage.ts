import { loadState } from '../../hooks/loadState'
import { useQuery } from '@tanstack/react-query'
import type { EChartsOption } from 'echarts'
import { useState } from 'react'
import { getCurrentHostMetrics, getHostMetrics } from '../../lib/api/monitoring'
import { axisTooltip, timeAxis, timePoints } from '../../lib/charts'
import { formatPercent } from '../../lib/format'
import { diskSummary, filesystemSummary } from '../../lib/host'
import { series } from '../../lib/metrics'
import { useRefreshInterval } from '../../lib/refresh'
import { useChartTheme } from '../../lib/theme'

export type RangeKey = '6h' | '24h' | '7d'

export const ranges: Array<{ value: RangeKey; label: string; milliseconds: number }> = [
  { value: '6h', label: '6 h', milliseconds: 6 * 60 * 60 * 1000 },
  { value: '24h', label: '24 h', milliseconds: 24 * 60 * 60 * 1000 },
  { value: '7d', label: '7 j', milliseconds: 7 * 24 * 60 * 60 * 1000 },
]

export function rangeStart(range: RangeKey): Date {
  return new Date(
    Date.now() -
      (ranges.find((item) => item.value === range)?.milliseconds ?? ranges[1].milliseconds),
  )
}

/** Logic and view model of StoragePage. */
export function useStoragePage() {
  const refreshInterval = useRefreshInterval()
  const chartTheme = useChartTheme()
  const [range, setRange] = useState<RangeKey>('24h')
  const [selectedMountpoint, setSelectedMountpoint] = useState('')

  const current = useQuery({
    queryKey: ['host', 'current-metrics'],
    queryFn: getCurrentHostMetrics,
    refetchInterval: refreshInterval || false,
  })
  const samples = current.data?.samples ?? []
  const filesystems = filesystemSummary(samples)
  const disks = diskSummary(samples)
  const mountpoint = selectedMountpoint || filesystems.entries[0]?.mountpoint || ''

  const history = useQuery({
    queryKey: ['host', 'metrics', 'filesystem.percent', mountpoint, range],
    queryFn: () =>
      getHostMetrics('filesystem.percent', rangeStart(range), { labels: { mountpoint } }),
    enabled: Boolean(mountpoint),
  })
  const historySeries = series(history.data ?? [], 'filesystem.percent', { mountpoint })

  const chartOption: EChartsOption = {
    backgroundColor: 'transparent',
    tooltip: axisTooltip(chartTheme, (value) => formatPercent(value)),
    grid: { left: 44, right: 12, top: 18, bottom: 28 },
    xAxis: timeAxis(chartTheme),
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
        name: mountpoint || 'Occupation',
        type: 'line',
        smooth: true,
        showSymbol: false,
        data: timePoints(historySeries),
        lineStyle: { color: chartTheme.cpu, width: 1.8 },
        itemStyle: { color: chartTheme.cpu },
        areaStyle: { color: chartTheme.cpu, opacity: 0.08 },
      },
    ],
  }

  return {
    range,
    setRange,
    setSelectedMountpoint,
    current: loadState(current),
    filesystems,
    disks,
    mountpoint,
    history: loadState(history),
    historySeries,
    chartOption,
  }
}
