import { loadState } from '../../hooks/loadState'
import { useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { listContainers } from '../../lib/api/monitoring'
import { mountField } from '../../lib/containers'
import { useRefreshInterval } from '../../lib/refresh'

export type MountGroup = {
  key: string
  source: string
  destination: string
  type: string
  access: string
  containers: Array<{ id: string; name: string }>
}

/** Logic and view model of VolumesPage. */
export function useVolumesPage() {
  const refreshInterval = useRefreshInterval()
  const [search, setSearch] = useState('')
  const containers = useQuery({
    queryKey: ['containers', false],
    queryFn: () => listContainers(false),
    refetchInterval: refreshInterval || false,
  })

  const mounts = useMemo(() => {
    const grouped = new Map<string, MountGroup>()
    for (const container of containers.data ?? []) {
      for (const mount of container.mounts) {
        const source = mountField(mount, 'source')
        const destination = mountField(mount, 'destination')
        const type = mountField(mount, 'type')
        const access = mountField(mount, 'rw')
        const key = `${type}|${source}|${destination}|${access}`
        const existing = grouped.get(key) ?? {
          key,
          source,
          destination,
          type,
          access,
          containers: [],
        }
        existing.containers.push({ id: container.id, name: container.name })
        grouped.set(key, existing)
      }
    }
    return Array.from(grouped.values()).sort(
      (left, right) =>
        left.destination.localeCompare(right.destination) ||
        left.source.localeCompare(right.source),
    )
  }, [containers.data])

  const filtered = mounts.filter(
    (mount) =>
      !search ||
      `${mount.source} ${mount.destination} ${mount.type} ${mount.containers.map((container) => container.name).join(' ')}`
        .toLowerCase()
        .includes(search.toLowerCase()),
  )

  const bindMounts = mounts.filter((mount) => mount.type === 'bind').length
  const namedVolumes = mounts.filter((mount) => mount.type === 'volume').length

  return {
    search,
    setSearch,
    containers: loadState(containers),
    mounts,
    filtered,
    bindMounts,
    namedVolumes,
  }
}
