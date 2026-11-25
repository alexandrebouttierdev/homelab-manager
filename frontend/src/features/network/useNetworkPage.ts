import { loadState } from '../../hooks/loadState'
import { useQuery } from '@tanstack/react-query'
import type { EChartsOption } from 'echarts'
import { useMemo, useState } from 'react'
import { getCurrentHostMetrics, getHostMetrics, type MetricSample } from '../../lib/api/monitoring'
import { axisTooltip, timeAxis, timePoints } from '../../lib/charts'
import { formatRate } from '../../lib/format'
import { interfaceSummary } from '../../lib/host'
import { series } from '../../lib/metrics'
import { useRefreshInterval } from '../../lib/refresh'
import { useChartTheme } from '../../lib/theme'

export type RangeKey = '1h' | '6h' | '24h'

export const ranges: Array<{ value: RangeKey; label: string; milliseconds: number }> = [
  { value: '1h', label: '1 h', milliseconds: 60 * 60 * 1000 },
  { value: '6h', label: '6 h', milliseconds: 6 * 60 * 60 * 1000 },
  { value: '24h', label: '24 h', milliseconds: 24 * 60 * 60 * 1000 },
]

export function rangeStart(range: RangeKey): Date {
  return new Date(
    Date.now() -
      (ranges.find((item) => item.value === range)?.milliseconds ?? ranges[0].milliseconds),
  )
}

export async function getRateHistory(
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

/** Logic and view model of NetworkPage. */
export function useNetworkPage() {
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
    return {
      backgroundColor: 'transparent',
      tooltip: axisTooltip(chartTheme, (value) => formatRate(value)),
      legend: {
        data: ['RX', 'TX'],
        right: 0,
        top: 0,
        itemWidth: 8,
        itemHeight: 8,
        icon: 'roundRect',
        textStyle: { color: chartTheme.text, fontSize: 10, fontFamily: 'Instrument Sans Variable' },
      },
      grid: { left: 54, right: 10, top: 30, bottom: 28 },
      xAxis: timeAxis(chartTheme),
      yAxis: {
        type: 'value',
        axisTick: { show: false },
        axisLine: { show: false },
        axisLabel: {
          color: chartTheme.muted,
          fontSize: 10,
          formatter: (value: number) => formatRate(value),
        },
        splitLine: { lineStyle: { color: chartTheme.grid } },
      },
      series: [
        {
          name: 'RX',
          type: 'line',
          smooth: true,
          showSymbol: false,
          data: timePoints(rxSeries),
          lineStyle: { color: chartTheme.net, width: 1.8 },
          itemStyle: { color: chartTheme.net },
          areaStyle: { color: chartTheme.net, opacity: 0.08 },
        },
        {
          name: 'TX',
          type: 'line',
          smooth: true,
          showSymbol: false,
          data: timePoints(txSeries),
          lineStyle: { color: chartTheme.accent, width: 1.8 },
          itemStyle: { color: chartTheme.accent },
          areaStyle: { color: chartTheme.accent, opacity: 0.05 },
        },
      ],
    }
  }, [chartTheme, rxSeries, txSeries])

  return {
    range,
    setRange,
    setSelectedInterface,
    current: loadState(current),
    interfaces,
    iface,
    history: loadState(history),
    totalRx,
    totalTx,
    currentRxRate,
    currentTxRate,
    totalErrors,
    rxSeries,
    txSeries,
    chartOption,
  }
}
