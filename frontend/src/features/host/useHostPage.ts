import { loadState } from '../../hooks/loadState'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  getCurrentHostMetrics,
  getHost,
  getSettings,
  type MetricSample,
} from '../../lib/api/monitoring'
import { filesystemSummary } from '../../lib/host'
import { latestGrouped, latestOne, latestSamples } from '../../lib/metrics'
import { useRefreshInterval } from '../../lib/refresh'

export function metricLabel(name: string): string {
  return name.replaceAll('.', ' ').replace(/\b\w/g, (letter) => letter.toUpperCase())
}

export function metricLabels(sample: MetricSample): string {
  const labels = Object.entries(sample.labels ?? {})
  if (labels.length === 0) return '—'
  return labels.map(([key, value]) => `${key}=${value}`).join(' · ')
}

/** A sensor at or above this temperature is highlighted. */
const hotSensorCelsius = 80

/** Logic and view model of HostPage. */
export function useHostPage() {
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

  return {
    hostInfo: host.data ?? null,
    settingsInfo: settings.data ?? null,
    queryClient,
    host: loadState(host),
    current: loadState(current),
    settings: loadState(settings),
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
    temperatureAlert: temperatures.some((sensor) => sensor.value >= hotSensorCelsius),
    metrics,
    systemStatus,
    dockerStatus,
  }
}
