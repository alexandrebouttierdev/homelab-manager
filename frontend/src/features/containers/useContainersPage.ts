import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { listContainers } from '../../lib/api/monitoring'
import {
  formatPorts,
  imageTag,
  isErrorState,
  isRunning,
  isStopped,
  matchesContainerSearch,
  matchesStateFilter,
  sortContainers,
  type ContainerSortOrder,
  type ContainerStateFilter,
} from '../../lib/containers'
import { useRefreshInterval } from '../../lib/refresh'

/** Live usage is loaded for the first running rows only, to bound requests. */
export const maxUsageRows = 50

/** Container inventory with search, state filter and sort. */
export function useContainersPage() {
  const queryClient = useQueryClient()
  const refreshInterval = useRefreshInterval()
  const [searchParams] = useSearchParams()
  const [search, setSearch] = useState(searchParams.get('image') ?? '')
  const [stateFilter, setStateFilter] = useState<ContainerStateFilter>('all')
  const [sortOrder, setSortOrder] = useState<ContainerSortOrder>('name')
  const [includeRemoved, setIncludeRemoved] = useState(false)

  const containers = useQuery({
    queryKey: ['containers', includeRemoved],
    queryFn: () => listContainers(includeRemoved),
    refetchInterval: refreshInterval || false,
  })

  const all = useMemo(() => containers.data ?? [], [containers.data])
  const rows = useMemo(
    () =>
      sortContainers(
        all.filter(
          (container) =>
            matchesStateFilter(container.state, stateFilter) &&
            matchesContainerSearch(container, search),
        ),
        sortOrder,
      ).map((container, index) => ({
        container,
        ports: formatPorts(container.ports),
        tag: imageTag(container.image),
        showUsage: isRunning(container.state) && index < maxUsageRows,
      })),
    [all, search, sortOrder, stateFilter],
  )

  return {
    search,
    setSearch,
    stateFilter,
    setStateFilter,
    sortOrder,
    setSortOrder,
    includeRemoved,
    setIncludeRemoved,
    counts: {
      all: all.length,
      running: all.filter((container) => isRunning(container.state)).length,
      stopped: all.filter((container) => isStopped(container.state)).length,
      errors: all.filter((container) => isErrorState(container.state)).length,
    },
    rows,
    isLoading: containers.isLoading,
    error: containers.error,
    retry: () => void containers.refetch(),
    refresh: () => void queryClient.invalidateQueries({ queryKey: ['containers'] }),
    resetFilters: () => {
      setSearch('')
      setStateFilter('all')
    },
  }
}
