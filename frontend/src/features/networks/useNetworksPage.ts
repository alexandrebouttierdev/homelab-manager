import { useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { getDockerInventory } from '../../lib/api/docker'
import { listContainers } from '../../lib/api/monitoring'
import { unreadableSection, unusedNetworks } from '../../lib/dockerInventory'
import { isRunning } from '../../lib/containers'
import { useRefreshInterval } from '../../lib/refresh'

export type NetworkGroup = {
  name: string
  containers: Array<{ id: string; name: string; state: string }>
  running: number
  /** Distinct states of the attached containers. */
  states: string[]
}

/** Logic and view model of NetworksPage. */
export function useNetworksPage() {
  const refreshInterval = useRefreshInterval()
  const [search, setSearch] = useState('')
  const containers = useQuery({
    queryKey: ['containers', false],
    queryFn: () => listContainers(false),
    refetchInterval: refreshInterval || false,
  })
  const inventory = useQuery({
    queryKey: ['docker', 'inventory'],
    queryFn: getDockerInventory,
    refetchInterval: refreshInterval || false,
  })
  const unused = inventory.isError
    ? unreadableSection(inventory.error)
    : unusedNetworks(inventory.data)

  const networks = useMemo(() => {
    const grouped = new Map<string, NetworkGroup>()
    for (const container of containers.data ?? []) {
      for (const network of container.networks) {
        const existing = grouped.get(network) ?? {
          name: network,
          containers: [],
          running: 0,
          states: [],
        }
        existing.containers.push({ id: container.id, name: container.name, state: container.state })
        if (isRunning(container.state)) existing.running += 1
        if (!existing.states.includes(container.state)) existing.states.push(container.state)
        grouped.set(network, existing)
      }
    }
    return Array.from(grouped.values()).sort((left, right) => left.name.localeCompare(right.name))
  }, [containers.data])

  const filtered = networks.filter(
    (network) =>
      !search ||
      `${network.name} ${network.containers.map((container) => container.name).join(' ')}`
        .toLowerCase()
        .includes(search.toLowerCase()),
  )

  return {
    search,
    setSearch,
    unused,
    isLoading: containers.isLoading,
    error: containers.error,
    retry: () => void containers.refetch(),
    attachedContainerCount: (containers.data ?? []).filter(
      (container) => container.networks.length > 0,
    ).length,
    activeNetworkCount: networks.filter((network) => network.running > 0).length,
    networks,
    filtered,
  }
}
