import { ApiError, requestJSON } from './client'

export type CollectorStatus = {
  status: string
  lastSuccessAt: string | null
}

export type MetricSample = {
  name: string
  value: number
  unit: string
  labels: Record<string, string> | null
  recordedAt: string
  /** Present on aggregated samples: value is then the bucket mean. */
  min?: number
  max?: number
  count?: number
}

export type HostInfo = {
  hostname: string
  os: { distribution: string; version: string }
  kernelVersion: string
  bootTime: string | null
  uptimeSeconds: number | null
  collector: CollectorStatus
}

export type ContainerInfo = {
  id: string
  dockerId: string
  name: string
  image: string
  state: string
  status: string
  createdAt: string | null
  startedAt: string | null
  uptimeSeconds: number | null
  restartCount: number
  restartPolicy: string
  ports: Array<Record<string, unknown>>
  networks: string[]
  mounts: Array<Record<string, unknown>>
  lastObservedAt: string
  removedAt: string | null
}

export type MonitorEvent = {
  id: string
  containerId: string | null
  type: string
  severity: string
  message: string
  occurredAt: string
}

export type DashboardData = {
  host: HostInfo
  containers: { total: number; running: number; stopped: number; other: number }
  metrics: MetricSample[]
  collectors: { system: CollectorStatus; docker: CollectorStatus }
}

export type CurrentMetrics = {
  samples: MetricSample[]
  status: string
  lastSuccessAt: string | null
}

export type MonitoringSettings = {
  systemCollectIntervalSeconds: number
  dockerCollectIntervalSeconds: number
  dockerDiscoveryIntervalSeconds: number
  metricsRetentionDays: number
  metricsRetentionDefaultDays: number
  metricsRetentionSource: 'config' | 'user'
  collectors: { system: CollectorStatus; docker: CollectorStatus }
}

export type MetricStep = 'raw' | '1m' | '5m' | '1h'

const metricSteps: MetricStep[] = ['raw', '1m', '5m', '1h']
const historyLimit = 2000

/** Resolution recommended for a time range (docs/API_CONTRACTS.md § 5.3). */
export function stepForRange(milliseconds: number): MetricStep {
  const hour = 60 * 60 * 1000
  if (milliseconds <= hour) return 'raw'
  if (milliseconds <= 6 * hour) return '1m'
  if (milliseconds <= 24 * hour) return '5m'
  return '1h'
}

export type HistoryOptions = {
  to?: Date
  /** Keep only the series carrying these label values (server-side). */
  labels?: Record<string, string>
  /** Defaults to stepForRange(to - from). */
  step?: MetricStep
}

/**
 * Loads a metric history, moving to the next coarser step when the range
 * holds too many points: a raw answer filling the limit is truncated, an
 * aggregated one is refused with TOO_MANY_POINTS.
 */
async function metricHistory(
  path: string,
  from: Date,
  options: HistoryOptions,
): Promise<MetricSample[]> {
  const to = options.to ?? new Date()
  let index = metricSteps.indexOf(options.step ?? stepForRange(to.getTime() - from.getTime()))
  for (;;) {
    const step = metricSteps[index]
    const query = new URLSearchParams(timeRangeQuery(from, to, historyLimit))
    query.set('step', step)
    for (const [key, value] of Object.entries(options.labels ?? {}))
      query.append('label', `${key}:${value}`)
    const coarser = index < metricSteps.length - 1
    try {
      const body = await requestJSON<{ samples: MetricSample[] }>(`${path}?${query.toString()}`)
      if (step === 'raw' && body.samples.length >= historyLimit && coarser) {
        index += 1
        continue
      }
      return body.samples
    } catch (error) {
      if (error instanceof ApiError && error.code === 'TOO_MANY_POINTS' && coarser) {
        index += 1
        continue
      }
      throw error
    }
  }
}

function timeRangeQuery(from: Date, to: Date, limit?: number): string {
  const query = new URLSearchParams({
    from: from.toISOString(),
    to: to.toISOString(),
  })
  if (limit != null) query.set('limit', String(limit))
  return query.toString()
}

export function getDashboard() {
  return requestJSON<DashboardData>('/api/v1/dashboard')
}

export function getHost() {
  return requestJSON<HostInfo>('/api/v1/host')
}

export function getCurrentHostMetrics() {
  return requestJSON<CurrentMetrics>('/api/v1/host/metrics/current')
}

export function getHostMetrics(metric: string, from: Date, options: HistoryOptions = {}) {
  return metricHistory(`/api/v1/host/metrics/${encodeURIComponent(metric)}`, from, options)
}

export async function getHostTemperatures() {
  const body = await requestJSON<{ samples: MetricSample[] }>('/api/v1/host/temperatures')
  return body.samples
}

export async function listContainers(includeRemoved = false) {
  const query = includeRemoved ? '?includeRemoved=true' : ''
  const body = await requestJSON<{ containers: ContainerInfo[] }>(`/api/v1/containers${query}`)
  return body.containers
}

export function getContainer(containerID: string) {
  return requestJSON<ContainerInfo>(`/api/v1/containers/${encodeURIComponent(containerID)}`)
}

export function getCurrentContainerMetrics(containerID: string) {
  return requestJSON<CurrentMetrics>(
    `/api/v1/containers/${encodeURIComponent(containerID)}/metrics/current`,
  )
}

export function getContainerMetrics(
  containerID: string,
  metric: string,
  from: Date,
  options: HistoryOptions = {},
) {
  return metricHistory(
    `/api/v1/containers/${encodeURIComponent(containerID)}/metrics/${encodeURIComponent(metric)}`,
    from,
    options,
  )
}

export async function getContainerEvents(
  containerID: string,
  from: Date,
  to = new Date(),
  limit?: number,
) {
  const body = await requestJSON<{ events: MonitorEvent[] }>(
    `/api/v1/containers/${encodeURIComponent(containerID)}/events?${timeRangeQuery(from, to, limit)}`,
  )
  return body.events
}

export async function listEvents(options: { from?: Date; to?: Date; limit?: number } = {}) {
  const to = options.to ?? new Date()
  const from = options.from ?? new Date(to.getTime() - 24 * 60 * 60 * 1000)
  const body = await requestJSON<{ events: MonitorEvent[] }>(
    `/api/v1/events?${timeRangeQuery(from, to, options.limit ?? 500)}`,
  )
  return body.events
}

export function getSettings() {
  return requestJSON<MonitoringSettings>('/api/v1/settings')
}

export function updateRetention(metricsRetentionDays: number) {
  return requestJSON<MonitoringSettings>('/api/v1/settings/retention', {
    method: 'PUT',
    body: JSON.stringify({ metricsRetentionDays }),
  })
}
