import { requestJSON } from './client'

export type DockerImage = {
  id: string
  tags: string[]
  createdAt: string
  sizeBytes: number
  /** Containers, running or not, created from the image. */
  containers: number
}

export type DockerVolume = {
  name: string
  driver: string
  scope: string
  createdAt: string | null
  containers: number
}

export type DockerNetwork = {
  id: string
  name: string
  driver: string
  scope: string
  internal: boolean
  /** Created by Docker itself (bridge, host, none): never "unused". */
  predefined: boolean
  subnets: string[]
  containers: number
}

export type DockerInventory = {
  images: DockerImage[]
  volumes: DockerVolume[]
  networks: DockerNetwork[]
  truncated: boolean
  observedAt: string | null
  status: 'starting' | 'ok' | 'stale' | 'unavailable'
  lastSuccessAt: string | null
}

export function getDockerInventory() {
  return requestJSON<DockerInventory>('/api/v1/docker/inventory')
}
