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

async function getJSON<T>(path: string): Promise<T> {
  const response = await fetch(path, { credentials: 'same-origin' })
  if (!response.ok) throw new Error('Impossible de charger les données de supervision.')
  return response.json() as Promise<T>
}

export function getDashboard() {
  return getJSON<DashboardData>('/api/v1/dashboard')
}

export function getHost() {
  return getJSON<HostInfo>('/api/v1/host')
}

export async function listContainers(includeRemoved = false) {
  const query = includeRemoved ? '?includeRemoved=true' : ''
  const body = await getJSON<{ containers: ContainerInfo[] }>(`/api/v1/containers${query}`)
  return body.containers
}

export function getContainer(containerID: string) {
  return getJSON<ContainerInfo>(`/api/v1/containers/${encodeURIComponent(containerID)}`)
}

export async function getContainerMetrics(containerID: string, metric: string, from: Date, to = new Date()) {
  const query = new URLSearchParams({ from: from.toISOString(), to: to.toISOString() })
  const body = await getJSON<{ samples: MetricSample[] }>(
    `/api/v1/containers/${encodeURIComponent(containerID)}/metrics/${encodeURIComponent(metric)}?${query}`,
  )
  return body.samples
}

export async function getHostMetrics(metric: string, from: Date, to = new Date()) {
  const query = new URLSearchParams({ from: from.toISOString(), to: to.toISOString() })
  const body = await getJSON<{ samples: MetricSample[] }>(
    `/api/v1/host/metrics/${encodeURIComponent(metric)}?${query}`,
  )
  return body.samples
}

export async function getHostTemperatures() {
  const body = await getJSON<{ samples: MetricSample[] }>('/api/v1/host/temperatures')
  return body.samples
}

export async function listEvents() {
  const to = new Date()
  const from = new Date(to.getTime() - 24 * 60 * 60 * 1000)
  const query = new URLSearchParams({ from: from.toISOString(), to: to.toISOString() })
  const body = await getJSON<{ events: MonitorEvent[] }>(`/api/v1/events?${query}`)
  return body.events
}

export type MonitoringSettings = {
  systemCollectIntervalSeconds: number
  dockerCollectIntervalSeconds: number
  dockerDiscoveryIntervalSeconds: number
  metricsRetentionDays: number
  collectors: { system: CollectorStatus; docker: CollectorStatus }
}

export function getSettings() {
  return getJSON<MonitoringSettings>('/api/v1/settings')
}
