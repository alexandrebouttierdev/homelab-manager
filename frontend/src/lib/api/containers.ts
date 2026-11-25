import { requestJSON } from './client'

/** The three changes the Manager may ask Docker for (API_CONTRACTS.md § 6.4). */
export type ContainerVerb = 'start' | 'stop' | 'restart'

export type ContainerActionFailure =
  'not_found' | 'rejected' | 'not_allowed' | 'timeout' | 'unavailable' | 'interrupted'

/** The last action requested on a container since the Manager started. */
export type ContainerAction = {
  containerId: string
  name: string
  action: ContainerVerb
  state: 'running' | 'succeeded' | 'failed'
  failure: ContainerActionFailure | null
  requestedBy: string
  requestedAt: string
  finishedAt: string | null
}

export type ContainerLogLine = {
  stream: 'stdout' | 'stderr'
  /** Null when Docker gave no readable time. */
  time: string | null
  message: string
  /** The line was longer than 16 KiB and is cut. */
  cut: boolean
}

export type ContainerLogs = {
  container: { id: string; name: string }
  lines: ContainerLogLine[]
  /** Fewer lines than asked: the answer exceeded the bound of the proxy. */
  truncated: boolean
  tail: number
}

export function getContainerLogs(containerID: string, tail: number) {
  return requestJSON<ContainerLogs>(
    `/api/v1/containers/${encodeURIComponent(containerID)}/logs?tail=${tail}`,
  )
}

export function getContainerActionSettings() {
  return requestJSON<{ enabled: boolean; actions: ContainerVerb[] }>(
    '/api/v1/docker/container-actions',
  )
}

export async function requestContainerAction(containerID: string, action: ContainerVerb) {
  const response = await requestJSON<{ action: ContainerAction }>(
    `/api/v1/containers/${encodeURIComponent(containerID)}/actions`,
    { method: 'POST', body: JSON.stringify({ action }) },
  )
  return response.action
}

export async function getLastContainerAction(containerID: string) {
  const response = await requestJSON<{ action: ContainerAction }>(
    `/api/v1/containers/${encodeURIComponent(containerID)}/actions/last`,
  )
  return response.action
}
